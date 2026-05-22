// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// watch_receipt.go — receipt emission on watch events (#449).
//
// `--emit-receipt-on <event-types>` accepts a comma-separated list of
// watch event type names. When buildWatchEvents produces an event of a
// matching type, attachReceiptsIfRequested loads the live resource and
// runs an in-process BuildReceipt to attach the in-toto Statement to
// the event payload. Failures are logged and non-fatal: the underlying
// watch event still emits, with the `receipt` key omitted from the
// JSON (omitempty; see watchEvent.Receipt in watch.go for the wire
// shape rationale).
//
// Predicate mapping (#449 v1, locked):
//
//	drift.detected     → applied-matches-spec
//	ownership.changed  → applied-matches-spec
//	scan.finding       → skipped (would flood on initial finding burst)
//	resource.discovered → skipped (every first-poll discovery would emit)
//
// The skipped event types still accept the flag (so `--emit-receipt-on all`
// is a valid sugar) but produce nil receipts with no error — the watch
// stream is unchanged.

// watchEventTypeAll is the sugar value for `--emit-receipt-on` that
// covers every known watch event type. New event types added to
// buildWatchEvents automatically participate.
const watchEventTypeAll = "all"

// watchKnownEventTypes is the closed list of event types
// buildWatchEvents emits. Used to validate the --emit-receipt-on flag
// upfront so CI gates don't silently accept typos.
var watchKnownEventTypes = []string{
	"resource.discovered",
	"ownership.changed",
	"drift.detected",
	"scan.finding",
}

// watchEventTypesWithReceiptSupport names the subset of event types for
// which `--emit-receipt-on` actually builds a receipt. v1 of the flag
// (#463) shipped with `drift.detected` and `ownership.changed`; the
// #449 follow-up adds `resource.discovered` and `scan.finding` once
// the per-poll backpressure cap (watchReceiptBatchCap below) makes
// them safe on first-poll bursts.
//
// Per-event-type predicate mapping:
//
//	drift.detected      → applied-matches-spec (auto-detected from owner)
//	ownership.changed   → applied-matches-spec
//	resource.discovered → applied-matches-spec (the discovery moment captures the live state at first observation)
//	scan.finding        → applied-matches-spec (the receipt records the resource state at finding time; the finding itself lives on the event's details)
//
// All four use auto-detect → applied-matches-spec via BuildReceipt's
// existing AutoDetectPredicate flow. Source-truth-pass requires a
// caller-declared --strategy that the watch loop doesn't take; no-
// manual-edits-since requires a --since cutoff that's likewise out
// of scope for event-driven emission.
var watchEventTypesWithReceiptSupport = map[string]bool{
	"drift.detected":      true,
	"ownership.changed":   true,
	"resource.discovered": true,
	"scan.finding":        true,
}

// parseWatchEmitReceiptOn parses the --emit-receipt-on flag value into
// the set of event types that should trigger receipt emission. Empty
// input returns nil (feature disabled).
//
// Accepts:
//   - "" — feature disabled, returns nil set
//   - "all" — every entry in watchKnownEventTypes
//   - comma-separated subset of watchKnownEventTypes
//
// Unknown event types are rejected upfront. Same rationale as
// parseReceiptFailOn: CI gates shouldn't silently accept typos.
func parseWatchEmitReceiptOn(raw string) (map[string]bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	if strings.EqualFold(trimmed, watchEventTypeAll) {
		out := make(map[string]bool, len(watchKnownEventTypes))
		for _, t := range watchKnownEventTypes {
			out[t] = true
		}
		return out, nil
	}

	known := map[string]bool{}
	for _, t := range watchKnownEventTypes {
		known[t] = true
	}

	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		if !known[t] {
			return nil, fmt.Errorf(
				"--emit-receipt-on %q: unknown event type %q (valid: %s, or the sugar 'all')",
				raw, t, strings.Join(watchKnownEventTypes, ", "),
			)
		}
		out[t] = true
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--emit-receipt-on %q resolved to empty event-type set", raw)
	}
	return out, nil
}

// Rate-limit knobs for makeReceiptWarnFn. Exported as package-level
// vars so tests can shrink them; production keeps the defaults.
//
// Defaults (Codex round-6 P2, #463):
//   - first 10 warnings emit verbatim — operator sees the failure mode
//   - after that, suppress; every 100 suppressed warnings emit a
//     single summary line ("suppressed N since last summary")
//
// The intent is: the operator sees enough of the first burst to debug,
// but a long-running watch hitting a transient cluster-read issue on
// every poll doesn't fill stderr indefinitely.
var (
	watchReceiptWarnFirstN       = 10
	watchReceiptWarnSummaryEvery = 100
)

// makeReceiptWarnFn wraps `base` with rate-limit behavior for
// receipt-build failures. The first `firstN` warnings pass through
// verbatim; after that the wrapper counts suppressed warnings and
// emits a single summary line every `summaryEvery` calls.
//
// The counters live on the closure, so the wrapper is stateful and
// persists across poll cycles. The watch loop constructs ONE wrapper
// at startup and reuses it for the duration.
//
// A `firstN` or `summaryEvery` of <= 0 disables the rate limit
// entirely (every call passes through).
func makeReceiptWarnFn(base func(format string, args ...interface{}), firstN, summaryEvery int) func(format string, args ...interface{}) {
	if base == nil {
		return func(string, ...interface{}) {}
	}
	if firstN <= 0 || summaryEvery <= 0 {
		return base
	}
	var count, sinceLastSummary int
	return func(format string, args ...interface{}) {
		count++
		if count <= firstN {
			base(format, args...)
			return
		}
		sinceLastSummary++
		if sinceLastSummary >= summaryEvery {
			base(
				"receipt-build warnings: suppressed %d additional warnings since the last summary (rate-limited to first %d + summary every %d; raise --emit-receipt-on supported set or check cluster connectivity if this is unexpected)",
				sinceLastSummary, firstN, summaryEvery,
			)
			sinceLastSummary = 0
		}
	}
}

// unsupportedEmitReceiptTypes returns the (sorted) subset of `emitOn`
// event types that are NOT in watchEventTypesWithReceiptSupport. The
// watch loop calls this once at startup to emit a one-time warning so
// users who ask for, say, `scan.finding` don't silently get no
// receipts. Empty result = every requested type is supported.
func unsupportedEmitReceiptTypes(emitOn map[string]bool) []string {
	var out []string
	for t := range emitOn {
		if !watchEventTypesWithReceiptSupport[t] {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// supportedEmitReceiptTypesSorted returns the supported event types in
// stable alphabetical order. Used by the startup-warning message so
// the listed set is deterministic across runs.
func supportedEmitReceiptTypesSorted() []string {
	out := make([]string, 0, len(watchEventTypesWithReceiptSupport))
	for t := range watchEventTypesWithReceiptSupport {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// watchReceiptBatchCap is the per-poll cap on receipt-build attempts
// (#449 backpressure). When a single poll produces more than this many
// receipt-eligible events (the intersection of emitOn and
// watchEventTypesWithReceiptSupport), the cap kicks in:
//
//   - The first `cap` events get their receipt built and attached.
//   - The remaining events still emit (the watch stream is preserved
//     — backpressure is about receipt-build cost, not event drop) but
//     their `receipt` key is omitted (omitempty).
//   - A single stderr warning per poll summarizes the suppression
//     ("backpressure: suppressed receipt-build for X events of N
//     eligible; raise --emit-receipt-batch-cap if expected").
//
// This pattern parallels the rate-limited warnings in makeReceiptWarnFn:
// the first N pass through; the rest is summarized. It's per-poll
// rather than across-poll because event-discovery bursts are typically
// transient (first-poll resource.discovered flood, initial scan.finding
// burst) and once the catch-up window passes, subsequent polls see
// normal volumes.
//
// Default cap = 10. Set to 0 via --emit-receipt-batch-cap to disable
// receipt-build entirely (matches --emit-receipt-on being unset for
// suppression behaviour while keeping the flag explicit). Set to a
// large value (e.g., 1000) to effectively disable the cap.
var watchReceiptBatchCap = 10

// attachReceiptsIfRequested walks events and, for each one whose type
// is in `emitOn` AND whose type has receipt-build support
// (watchEventTypesWithReceiptSupport), builds a receipt and attaches
// it to event.Receipt. The receipt-build call goes through the same
// loadReceiptLiveFn seam tests already swap; production hits the
// dynamic client.
//
// Per-poll backpressure cap (#449): when the receipt-eligible event
// count exceeds watchReceiptBatchCap, the first `cap` get their
// receipts built; the rest are skipped (receipt key omitted per
// omitempty) and a single stderr summary fires. The cap is per-call
// (one watch poll = one call), so a long-running watch with quiet
// polls between bursts doesn't accumulate suppression state.
//
// Failures are logged to stderr (`warnFn`) and non-fatal — the
// underlying event still emits with the receipt key omitted.
func attachReceiptsIfRequested(
	ctx context.Context,
	events []watchEvent,
	emitOn map[string]bool,
	dynClient dynamic.Interface,
	connected bool,
	warnFn func(format string, args ...interface{}),
) []watchEvent {
	if len(emitOn) == 0 || len(events) == 0 {
		return events
	}

	// First pass: count the receipt-eligible events so the cap warning
	// can report the suppression accurately.
	eligibleCount := 0
	for _, ev := range events {
		if emitOn[ev.Type] && watchEventTypesWithReceiptSupport[ev.Type] {
			eligibleCount++
		}
	}

	cap := watchReceiptBatchCap
	if cap < 0 {
		cap = 0
	}

	built := 0
	suppressed := 0
	for i := range events {
		if !emitOn[events[i].Type] {
			continue
		}
		if !watchEventTypesWithReceiptSupport[events[i].Type] {
			// Caller asked us to consider this event type, but it's
			// not in the supported set (closed enumeration). Skip
			// with no warning; the startup-time warning already lists
			// unsupported types.
			continue
		}
		if built >= cap {
			suppressed++
			continue
		}
		stmt, err := watchBuildReceiptForEventFn(ctx, events[i], dynClient, connected)
		if err != nil {
			if warnFn != nil {
				warnFn("receipt build for %s/%s in %s (%s) failed: %v",
					events[i].Resource.Kind, events[i].Resource.Name, events[i].Resource.Namespace,
					events[i].Type, err)
			}
			continue
		}
		events[i].Receipt = stmt
		built++
	}

	if suppressed > 0 && warnFn != nil {
		warnFn(
			"backpressure: suppressed receipt-build for %d event(s) of %d eligible (cap=%d); raise --emit-receipt-batch-cap to enable, or set 0 to disable entirely",
			suppressed, eligibleCount, cap,
		)
	}

	return events
}

// watchBuildReceiptForEventFn is the function-variable seam tests swap
// to inject prefab receipts. Production wires it to
// watchBuildReceiptForEvent below.
var watchBuildReceiptForEventFn = watchBuildReceiptForEvent

// watchBuildReceiptForEvent loads the live resource named by the event
// and runs an in-process BuildReceipt with the applied-matches-spec
// predicate auto-detected from owner. Reuses the same evidence-
// collection helpers `cub-scout receipt verify` uses
// (DetectOwnership, AttributeFieldMutation, CollectGitSourceAnchorForOwner).
//
// Returns a Statement pointer; combined with `omitempty` on
// watchEvent.Receipt, the wire shape is either `receipt: { ... }` (key
// present, full Statement) or the receipt key omitted entirely (never
// `receipt: null` and never an empty object).
func watchBuildReceiptForEvent(
	ctx context.Context,
	event watchEvent,
	dynClient dynamic.Interface,
	connected bool,
) (*agent.Statement, error) {
	kind := strings.TrimSpace(event.Resource.Kind)
	name := strings.TrimSpace(event.Resource.Name)
	namespace := strings.TrimSpace(event.Resource.Namespace)
	if kind == "" || name == "" {
		return nil, fmt.Errorf("event missing kind/name: %+v", event.Resource)
	}
	if namespace == "" {
		namespace = "default"
	}

	live, err := loadLiveForWatchReceiptFn(ctx, dynClient, kind, name, namespace)
	if err != nil {
		return nil, fmt.Errorf("load live %s/%s in %s: %w", kind, name, namespace, err)
	}

	owner := agent.DetectOwnership(live)
	attribution := agent.AttributeFieldMutation(live, owner)
	gitSource := agent.CollectGitSourceAnchorForOwner(ctx, live, owner)

	evidence := agent.Evidence{
		Attribution: &attribution,
		GitSource:   gitSource,
	}

	var spec *agent.SpecAnchor
	if gitSource != nil {
		spec = &agent.SpecAnchor{
			Anchor: agent.SpecAnchorBody{
				Type:     "git",
				RepoURL:  gitSource.RepoURL,
				Revision: gitSource.Revision,
				Path:     gitSource.Path,
			},
		}
	}

	stmt, err := agent.BuildReceipt(agent.BuildReceiptInput{
		Live: live,
		Scope: agent.Scope{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		Owner:     owner,
		Spec:      spec,
		Evidence:  evidence,
		Connected: connected,
		Verifier: agent.Verifier{
			Tool:    "cub-scout",
			Version: BuildTag,
		},
		VerifiedAt: time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("build receipt: %w", err)
	}
	return &stmt, nil
}

// loadLiveForWatchReceipt fetches the live K8s object via the dynamic
// client. Tests can override via the loadLiveForWatchReceiptFn seam.
var loadLiveForWatchReceiptFn = loadLiveForWatchReceipt

func loadLiveForWatchReceipt(
	ctx context.Context,
	dynClient dynamic.Interface,
	kind, name, namespace string,
) (*unstructured.Unstructured, error) {
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return nil, fmt.Errorf("unsupported kind %q", kind)
	}
	return dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
}
