// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// receipt_aggregate.go — the CLI half of #448's aggregate-with-discovery
// surface. Wires `cub-scout receipt verify --scope <spec>` into the
// aggregate-receipt construction in pkg/agent/receipt_aggregate.go.
//
// CLI shapes:
//
//	cub-scout receipt verify --scope namespace/<ns> --strategy <s> --save
//	cub-scout receipt verify <kind>/<name>,<kind>/<name>,... -n <ns> --strategy <s> --save
//
// In both cases the output is N per-resource receipts (emitted to stdout
// in JSONL form, optionally saved via --save) plus 1 aggregate receipt
// (emitted last, with --fail-on applied to its verdict). The aggregate
// receipt's inputAttestations[] references every per-resource receipt.
//
// Read-only by construction: the discovery walks the dynamic client with
// List operations only; the aggregate construction is pure (no cluster
// reads beyond what the per-resource verifies already do).

// receiptScope is the CLI surface for the --scope flag. Set to e.g.
// "namespace/prod" for namespace discovery; empty means single-resource
// or comma-list mode.
var receiptScope string

// receiptAggregatePolicy is the --aggregate-policy flag value. v1
// supports only "max-severity" (the default); the flag is wired now so
// future policies don't break the surface.
var receiptAggregatePolicy string

// receiptResourceRef is the lightweight resource identity used by the
// aggregate discovery path. Mirrors agent.Scope but stays in cmd/
// (the discovery is a CLI concern; pkg/agent doesn't do cluster reads).
type receiptResourceRef struct {
	Kind      string
	Name      string
	Namespace string
}

// String returns the canonical "<kind>/<name>" form.
func (r receiptResourceRef) String() string {
	return r.Kind + "/" + r.Name
}

// discoverNamespaceWorkloadsFn is the function-variable seam tests
// swap to inject a prefab resource list. Production walks the dynamic
// client.
var discoverNamespaceWorkloadsFn = discoverNamespaceWorkloads

// scopedAggregateWorkloadKinds is the closed list of kinds the namespace-
// scope discovery enumerates. Picked deliberately: these are the kinds
// `applied-matches-spec` and `source-truth-pass` know how to evaluate.
// Adding new kinds requires plumbing in kindToGVR (cmd/cub-scout/scope.go)
// AND the per-predicate evaluators in pkg/agent.
//
// v1 keeps this list narrow — Pods, Services, ConfigMaps, etc. would
// flood the aggregate without adding evidence value (no controller
// anchor to verify against). Future v2 work can broaden.
var scopedAggregateWorkloadKinds = []string{
	"Deployment",
	"StatefulSet",
	"DaemonSet",
	"CronJob",
	"Job",
}

// parseAggregateScope translates the CLI inputs (the --scope flag, the
// positional argument, the --namespace flag) into a structured spec and
// a list of resources to process. Caller is responsible for calling
// the discovery seam when Spec.Kind == "namespace".
//
// Returns the scope spec, the resolved resource list (empty if
// namespace-mode — discovery happens at the caller), and whether the
// inputs were aggregate-shaped (true when the caller should branch
// into the aggregate flow).
//
// Errors:
//   - --scope value malformed
//   - comma-list contains invalid kind/name pairs
//   - --scope conflicts with the positional argument
func parseAggregateScope(scopeFlag, positional, namespace string) (agent.AggregateScopeSpec, []receiptResourceRef, bool, error) {
	scopeFlag = strings.TrimSpace(scopeFlag)
	positional = strings.TrimSpace(positional)

	// Case 1: --scope namespace/<ns>
	if strings.HasPrefix(scopeFlag, "namespace/") {
		if positional != "" {
			return agent.AggregateScopeSpec{}, nil, false, fmt.Errorf(
				"--scope %q conflicts with positional argument %q; pass either --scope or a positional, not both",
				scopeFlag, positional,
			)
		}
		ns := strings.TrimPrefix(scopeFlag, "namespace/")
		ns = strings.TrimSpace(ns)
		if ns == "" {
			return agent.AggregateScopeSpec{}, nil, false, fmt.Errorf(
				"--scope %q: empty namespace; expected namespace/<name>",
				scopeFlag,
			)
		}
		return agent.AggregateScopeSpec{
				Kind:      "namespace",
				Namespace: ns,
				// MemberCount filled in after discovery
			},
			nil, // discovery happens at the caller
			true,
			nil
	}

	// Case 2: comma-list in the positional argument
	if strings.Contains(positional, ",") {
		if scopeFlag != "" {
			return agent.AggregateScopeSpec{}, nil, false, fmt.Errorf(
				"--scope %q conflicts with comma-list positional %q; pick one",
				scopeFlag, positional,
			)
		}
		refs := []receiptResourceRef{}
		for _, part := range strings.Split(positional, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			kindRaw, name, ok := parseKindName(part)
			if !ok {
				return agent.AggregateScopeSpec{}, nil, false, fmt.Errorf(
					"invalid comma-list entry %q (expected <kind>/<name>)",
					part,
				)
			}
			kind := normalizeKind(kindRaw)
			if kind == "" {
				return agent.AggregateScopeSpec{}, nil, false, fmt.Errorf(
					"invalid comma-list entry %q (unsupported kind)",
					part,
				)
			}
			ns := namespace
			if ns == "" {
				ns = "default"
			}
			refs = append(refs, receiptResourceRef{Kind: kind, Name: name, Namespace: ns})
		}
		if len(refs) < 2 {
			return agent.AggregateScopeSpec{}, nil, false, fmt.Errorf(
				"comma-list positional %q resolved to %d resource(s); aggregate mode requires at least 2",
				positional, len(refs),
			)
		}
		return agent.AggregateScopeSpec{
				Kind:        "batch",
				MemberCount: len(refs),
			},
			refs,
			true,
			nil
	}

	// Case 3: --scope set but doesn't match a known form
	if scopeFlag != "" {
		return agent.AggregateScopeSpec{}, nil, false, fmt.Errorf(
			"--scope %q: unrecognized scope spec (expected 'namespace/<ns>')",
			scopeFlag,
		)
	}

	// Not aggregate mode; caller falls back to single-resource flow.
	return agent.AggregateScopeSpec{}, nil, false, nil
}

// discoverNamespaceWorkloads walks the dynamic client for each kind in
// scopedAggregateWorkloadKinds in the given namespace and returns the
// resource list. List-only operations; no resource is fetched in full
// (the per-resource verify path handles individual Gets).
//
// Returns an empty list (not an error) when the namespace has no
// workloads of the supported kinds — the caller decides whether to
// error out or emit an empty aggregate.
func discoverNamespaceWorkloads(ctx context.Context, ns string) ([]receiptResourceRef, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}

	out := []receiptResourceRef{}
	for _, kind := range scopedAggregateWorkloadKinds {
		gvr := kindToGVR(kind)
		if gvr.Resource == "" {
			continue // unsupported kind
		}
		list, err := dynClient.Resource(gvr).Namespace(ns).List(ctx, v1.ListOptions{})
		if err != nil {
			// Per-kind errors are non-fatal — a missing CRD (e.g., no
			// CronJobs registered) shouldn't break discovery for the
			// other kinds. The caller's per-resource verify will catch
			// any genuine cluster-reachability problem.
			continue
		}
		for _, item := range list.Items {
			out = append(out, receiptResourceRef{
				Kind:      kind,
				Name:      item.GetName(),
				Namespace: ns,
			})
		}
	}
	return out, nil
}

// gvrCronJobV1 is a small helper exporting the GVR for CronJob.v1 —
// kindToGVR may resolve to the deprecated v1beta1 in older code paths;
// the aggregate discovery only wants the v1 form.
//
//nolint:unused // kept for future kind-targeted discovery
var gvrCronJobV1 = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}

// runReceiptVerifyScoped handles the aggregate-receipt flow when
// `--scope namespace/<ns>` OR a comma-list positional triggers it.
//
// Steps:
//   1. Parse the scope spec (already done by the caller, here for
//      explicitness via re-parse).
//   2. Resolve the resource list (discovery for namespace mode; the
//      comma-list is already a resolved set).
//   3. For each resource, run the same verify logic the single-resource
//      flow uses (build live, build evidence, build per-resource
//      receipt). The receipts go through the existing BuildReceipt
//      machinery — same predicate auto-detection, same flag semantics.
//      Each per-resource receipt is optionally persisted to the store
//      if --save is set.
//   4. Build the aggregate receipt over the per-resource receipts via
//      pkg/agent.BuildAggregateReceipt with the configured policy
//      (default: max-severity).
//   5. Emit the per-resource receipts (JSONL stream) followed by the
//      aggregate receipt; --fail-on applies to the aggregate verdict.
//
// The function is intentionally separate from runReceiptVerify (the
// single-resource flow) — both paths share helpers but the aggregate
// flow has its own loop, its own error model (per-resource failures
// are non-fatal), and its own output shape.
func runReceiptVerifyScoped(
	cmd *cobra.Command,
	scopeSpec agent.AggregateScopeSpec,
	resources []receiptResourceRef,
) error {
	ctx := cmd.Context()

	format := strings.ToLower(strings.TrimSpace(receiptFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", receiptFormat)
	}

	// Resolve the policy (only max-severity supported in v1).
	policy, err := resolveAggregatePolicy(receiptAggregatePolicy)
	if err != nil {
		return err
	}

	// Parse --fail-on upfront so a bad value can't leak per-resource
	// receipts before erroring. Same property as the single-resource
	// flow per Codex round-6 P2.
	var failOnSet map[agent.ReceiptVerdict]bool
	if raw := strings.TrimSpace(receiptFailOn); raw != "" {
		parsed, parseErr := parseReceiptFailOn(raw)
		if parseErr != nil {
			return parseErr
		}
		failOnSet = parsed
	}

	// Parse --since once (same shared validation as single-resource).
	var since time.Time
	if raw := strings.TrimSpace(receiptSince); raw != "" {
		parsed, perr := time.Parse(time.RFC3339, raw)
		if perr != nil {
			return fmt.Errorf("invalid --since %q (expected RFC 3339): %w", raw, perr)
		}
		since = parsed
	}

	predicateExplicit := agent.PredicateName(strings.TrimSpace(receiptPredicate))
	if predicateExplicit == agent.PredicateSourceTruthPass && strings.TrimSpace(receiptStrategy) == "" {
		return fmt.Errorf("--predicate source-truth-pass requires --strategy")
	}
	if predicateExplicit == agent.PredicateNoManualEditsSince && since.IsZero() {
		return fmt.Errorf("--predicate no-manual-edits-since requires --since <RFC3339 timestamp>")
	}

	// Namespace-mode: run discovery to fill the resource list.
	if scopeSpec.Kind == "namespace" {
		discovered, derr := discoverNamespaceWorkloadsFn(ctx, scopeSpec.Namespace)
		if derr != nil {
			return fmt.Errorf("discover workloads in namespace %s: %w", scopeSpec.Namespace, derr)
		}
		resources = discovered
		scopeSpec.MemberCount = len(resources)
	}

	if len(resources) == 0 {
		return fmt.Errorf("aggregate scope resolved to 0 resources; nothing to verify")
	}

	connected := false
	// connected detection is the same as the single-resource flow;
	// kept inline to avoid a circular import.
	connected = scopedConnectedMode()

	// Per-resource loop. Each iteration emits one receipt; the
	// aggregate is built at the end from the verified refs.
	perResourceRefs := make([]agent.VerifiedAttestationRef, 0, len(resources))
	perResourceVerdicts := make([]agent.ReceiptVerdict, 0, len(resources))
	perResourceFailures := []string{}

	for _, r := range resources {
		stmt, buildErr := buildOnePerResourceReceipt(ctx, r, predicateExplicit, since, connected)
		if buildErr != nil {
			perResourceFailures = append(perResourceFailures, fmt.Sprintf("%s/%s in %s: %v", r.Kind, r.Name, r.Namespace, buildErr))
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: per-resource verify failed for %s/%s in %s: %v\n", r.Kind, r.Name, r.Namespace, buildErr)
			continue
		}

		// Emit the per-resource receipt to stdout (JSONL line for
		// machine consumption; aggregate flow always emits JSON
		// regardless of --format for the per-resource set).
		buf, mErr := json.Marshal(stmt)
		if mErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: marshal per-resource receipt for %s/%s failed: %v\n", r.Kind, r.Name, mErr)
			continue
		}
		fmt.Println(string(buf))

		// Persist if --save is set.
		if receiptSave {
			if err := saveOneReceipt(cmd, stmt); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: --save for %s/%s failed: %v\n", r.Kind, r.Name, err)
			}
		}

		// Build the VerifiedAttestationRef for the aggregate.
		ref, refErr := agent.BuildAttestationRef(stmt)
		if refErr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: build attestation ref for %s/%s failed: %v\n", r.Kind, r.Name, refErr)
			continue
		}
		perResourceRefs = append(perResourceRefs, ref)
		perResourceVerdicts = append(perResourceVerdicts, stmt.Predicate.Verdict)
	}

	if len(perResourceRefs) == 0 {
		return fmt.Errorf("aggregate scope: 0 of %d resources successfully verified; cannot build aggregate (%d failure(s))", len(resources), len(perResourceFailures))
	}

	// Build the aggregate receipt.
	omissions := []agent.Omission{}
	if len(perResourceFailures) > 0 {
		omissions = append(omissions, agent.Omission{
			Missing: agent.OmissionAggregatePartialCoverage,
			Reason: fmt.Sprintf(
				"%d of %d resources in scope failed per-resource verify; aggregate composed from %d successful verifies",
				len(perResourceFailures), len(resources), len(perResourceRefs),
			),
			Severity: "warning",
		})
	}

	aggregateStmt, aggErr := agent.BuildAggregateReceipt(agent.BuildAggregateReceiptInput{
		Inputs:        perResourceRefs,
		InputVerdicts: perResourceVerdicts,
		Scope:         scopeSpec,
		Policy:        policy,
		Verifier: agent.Verifier{
			Tool:    "cub-scout",
			Version: BuildTag,
		},
		VerifiedAt: time.Now().UTC(),
		Connected:  connected,
		Omissions:  omissions,
	})
	if aggErr != nil {
		return fmt.Errorf("build aggregate receipt: %w", aggErr)
	}

	// Emit the aggregate receipt (always JSON; ASCII rendering for
	// aggregates is a follow-up).
	aggregateBuf, err := json.MarshalIndent(aggregateStmt, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal aggregate receipt: %w", err)
	}
	fmt.Println(string(aggregateBuf))

	// --out: write the aggregate to a file path.
	if outPath := strings.TrimSpace(receiptOut); outPath != "" {
		if writeErr := os.WriteFile(outPath, append(aggregateBuf, '\n'), 0o644); writeErr != nil {
			return fmt.Errorf("write aggregate to %s: %w", outPath, writeErr)
		}
	}

	// --save the aggregate (the per-resource saves already happened).
	if receiptSave {
		if err := saveOneReceipt(cmd, aggregateStmt); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: --save for aggregate receipt failed: %v\n", err)
		}
	}

	// --fail-on: apply to the aggregate verdict.
	if failOnSet != nil && failOnSet[aggregateStmt.Predicate.Verdict] {
		return newExitCodeError(
			fmt.Errorf(
				"aggregate verdict %q matches --fail-on %q",
				aggregateStmt.Predicate.Verdict, receiptFailOn,
			),
			2,
		)
	}
	return nil
}

// buildOnePerResourceReceipt runs the same per-resource verify the
// single-resource flow uses, but inline (without re-parsing flags).
// Returns the stamped Statement on success.
func buildOnePerResourceReceipt(
	ctx context.Context,
	r receiptResourceRef,
	predicateExplicit agent.PredicateName,
	since time.Time,
	connected bool,
) (agent.Statement, error) {
	live, err := loadReceiptLiveFn(ctx, r.Kind, r.Name, r.Namespace)
	if err != nil {
		return agent.Statement{}, fmt.Errorf("load live: %w", err)
	}

	owner := agent.DetectOwnership(live)
	attribution := agent.AttributeFieldMutation(live, owner)
	gitSource := agent.CollectGitSourceAnchorForOwner(ctx, live, owner)

	evidence := agent.Evidence{
		Attribution: &attribution,
		GitSource:   gitSource,
	}
	if strategy := strings.TrimSpace(receiptStrategy); strategy != "" {
		stEvidence, stErr := collectSourceTruthForReceiptFn(ctx, r.Kind, r.Name, r.Namespace, strategy)
		if stErr != nil {
			return agent.Statement{}, fmt.Errorf("collect source-truth evidence: %w", stErr)
		}
		evidence.SourceTruth = stEvidence
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
		if strings.TrimSpace(receiptAtCommit) != "" {
			spec.Anchor.Revision = strings.TrimSpace(receiptAtCommit)
		}
	}

	stmt, err := agent.BuildReceipt(agent.BuildReceiptInput{
		Live:          live,
		Scope:         agent.Scope{Kind: r.Kind, Name: r.Name, Namespace: r.Namespace},
		Owner:         owner,
		PredicateName: predicateExplicit,
		Spec:          spec,
		Evidence:      evidence,
		Connected:     connected,
		Strategy:      strings.TrimSpace(receiptStrategy),
		Since:         since,
		Verifier: agent.Verifier{
			Tool:    "cub-scout",
			Version: BuildTag,
		},
		VerifiedAt: time.Now().UTC(),
	})
	if err != nil {
		return agent.Statement{}, fmt.Errorf("build receipt: %w", err)
	}
	return stmt, nil
}

// saveOneReceipt persists a receipt via agent.SaveStatement, honoring
// --save-dir and the env-var fallback. Mirrors the single-resource
// save logic; refactored here so the aggregate flow can call it for
// each per-resource receipt + the aggregate itself.
func saveOneReceipt(cmd *cobra.Command, stmt agent.Statement) error {
	dir := strings.TrimSpace(receiptSaveDir)
	if dir == "" {
		resolved, dErr := agent.DefaultStoreDir()
		if dErr != nil {
			return fmt.Errorf("resolve receipt store: %w", dErr)
		}
		dir = resolved
	}
	savedPath, sErr := agent.SaveStatement(stmt, dir)
	if sErr != nil {
		if errors.Is(sErr, os.ErrExist) {
			fmt.Fprintf(cmd.ErrOrStderr(), "already saved: %s\n", savedPath)
			return nil
		}
		return fmt.Errorf("save: %w", sErr)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "saved: %s\n", savedPath)
	return nil
}

// resolveAggregatePolicy maps the --aggregate-policy flag value to an
// agent.AggregateVerdictPolicy. v1 supports only "max-severity" (or
// empty for the default).
func resolveAggregatePolicy(name string) (agent.AggregateVerdictPolicy, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "", "max-severity":
		return agent.MaxSeverityPolicy{}, nil
	default:
		return nil, fmt.Errorf(
			"--aggregate-policy %q: unknown policy (v1 supports: max-severity)",
			name,
		)
	}
}

// scopedConnectedMode mirrors the inline connected-mode check the
// single-resource flow uses, kept here so the aggregate flow doesn't
// import the hub package directly.
var scopedConnectedMode = defaultScopedConnectedMode

// defaultScopedConnectedMode is the production implementation. Tests
// can replace `scopedConnectedMode` to force standalone or connected
// without needing real ConfigHub auth.
func defaultScopedConnectedMode() bool {
	// Delegate to the same package-internal function the single-resource
	// flow uses inline. Keeping this as an indirection avoids a cycle
	// during test injection.
	return detectConnectedForReceipt()
}

// detectConnectedForReceipt is a test-injectable detection helper.
// Production wires it to the hub.NewClient().RequireConnected() check
// (defined in cmd/cub-scout/receipt.go's init via hubReceiptConnected
// to avoid an import cycle in this file).
var detectConnectedForReceipt = func() bool {
	// Defaults to false so standalone-mode tests don't accidentally
	// receive a connected receipt. Production code in receipt.go init
	// replaces this to use hub.NewClient().RequireConnected().
	return false
}

// scopeFlagWired is true once the --scope flag has been added to
// receiptVerifyCmd. Used by tests + the file-internal init.
//
//nolint:unused // referenced by init in receipt.go after refactor
var scopeFlagWired bool

// filepathAbsForScope is a small alias to allow tests to stub
// filepath.Abs if needed. Not currently swapped; reserved.
//
//nolint:unused // reserved
var filepathAbsForScope = filepath.Abs
