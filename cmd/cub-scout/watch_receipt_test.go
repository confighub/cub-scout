// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// --- parseWatchEmitReceiptOn ----------------------------------------

func TestParseWatchEmitReceiptOn_Empty(t *testing.T) {
	set, err := parseWatchEmitReceiptOn("")
	if err != nil {
		t.Fatalf("empty input should not error: %v", err)
	}
	if set != nil {
		t.Errorf("empty input should disable feature (nil set); got %v", set)
	}
}

func TestParseWatchEmitReceiptOn_All(t *testing.T) {
	set, err := parseWatchEmitReceiptOn("all")
	if err != nil {
		t.Fatalf("'all' should parse: %v", err)
	}
	for _, want := range watchKnownEventTypes {
		if !set[want] {
			t.Errorf("'all' should include %q", want)
		}
	}
}

func TestParseWatchEmitReceiptOn_CommaList(t *testing.T) {
	set, err := parseWatchEmitReceiptOn("drift.detected,ownership.changed")
	if err != nil {
		t.Fatalf("comma list should parse: %v", err)
	}
	if !set["drift.detected"] || !set["ownership.changed"] {
		t.Error("expected both event types in set")
	}
	if set["resource.discovered"] {
		t.Error("resource.discovered must NOT be in set")
	}
}

func TestParseWatchEmitReceiptOn_RejectsUnknown(t *testing.T) {
	_, err := parseWatchEmitReceiptOn("drift.detected,not-a-real-type")
	if err == nil {
		t.Fatal("unknown event type must be rejected")
	}
	if !strings.Contains(err.Error(), "not-a-real-type") {
		t.Errorf("error must echo the bad value; got %v", err)
	}
}

// --- unsupportedEmitReceiptTypes startup-warning helper -------------

func TestUnsupportedEmitReceiptTypes_AllSupported(t *testing.T) {
	emitOn := map[string]bool{
		"drift.detected":    true,
		"ownership.changed": true,
	}
	got := unsupportedEmitReceiptTypes(emitOn)
	if len(got) != 0 {
		t.Errorf("all-supported emit set must return empty unsupported list; got %v", got)
	}
}

func TestUnsupportedEmitReceiptTypes_SomeUnsupported(t *testing.T) {
	// `all` includes resource.discovered + scan.finding which v1 does
	// not build receipts for. Codex round-6 P2: surface this gap up
	// front so the user isn't surprised later.
	emitOn := map[string]bool{
		"drift.detected":      true,
		"ownership.changed":   true,
		"resource.discovered": true,
		"scan.finding":        true,
	}
	got := unsupportedEmitReceiptTypes(emitOn)
	wantSet := map[string]bool{
		"resource.discovered": true,
		"scan.finding":        true,
	}
	if len(got) != len(wantSet) {
		t.Fatalf("expected %d unsupported types; got %d (%v)", len(wantSet), len(got), got)
	}
	for _, t2 := range got {
		if !wantSet[t2] {
			t.Errorf("unexpected unsupported type %q", t2)
		}
	}
	// Output must be sorted for stable warning text across runs.
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("unsupportedEmitReceiptTypes must return sorted output; got %v", got)
		}
	}
}

func TestSupportedEmitReceiptTypesSorted_StableOrder(t *testing.T) {
	got := supportedEmitReceiptTypesSorted()
	if len(got) != len(watchEventTypesWithReceiptSupport) {
		t.Errorf("must return all supported types; got %d want %d", len(got), len(watchEventTypesWithReceiptSupport))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("output must be sorted; got %v", got)
		}
	}
}

// --- makeReceiptWarnFn rate-limit -----------------------------------

// TestMakeReceiptWarnFn_FirstNPassThrough is the Codex round-6 P2
// (#463) regression: the first `firstN` warnings emit verbatim, after
// which the wrapper rate-limits with periodic summaries. Verifies the
// counts at each boundary.
func TestMakeReceiptWarnFn_FirstNPassThrough(t *testing.T) {
	var captured []string
	base := func(format string, args ...interface{}) {
		captured = append(captured, fmt.Sprintf(format, args...))
	}
	// firstN=3, summaryEvery=5 → first 3 pass; then suppressed until
	// the 5th post-burst call emits a summary.
	warn := makeReceiptWarnFn(base, 3, 5)

	// First 3 — pass through.
	for i := 0; i < 3; i++ {
		warn("burst warn %d", i)
	}
	if len(captured) != 3 {
		t.Fatalf("first 3 calls must pass through; got %d captured (%v)", len(captured), captured)
	}
	for i, msg := range captured {
		want := fmt.Sprintf("burst warn %d", i)
		if msg != want {
			t.Errorf("captured[%d] = %q; want %q", i, msg, want)
		}
	}

	// Next 4 — suppressed, no new output.
	for i := 0; i < 4; i++ {
		warn("suppressed warn %d", i)
	}
	if len(captured) != 3 {
		t.Errorf("4 suppressed warnings must not add output; got %d (%v)", len(captured), captured)
	}

	// 5th suppressed warn triggers the summary (sinceLastSummary
	// hits 5).
	warn("summary trigger")
	if len(captured) != 4 {
		t.Fatalf("5th suppressed warn must emit a summary; got %d captured (%v)", len(captured), captured)
	}
	summary := captured[3]
	if !strings.Contains(summary, "suppressed 5") {
		t.Errorf("summary must report 5 suppressed; got %q", summary)
	}

	// Another 4 suppressed — no new output.
	for i := 0; i < 4; i++ {
		warn("post-summary suppressed %d", i)
	}
	if len(captured) != 4 {
		t.Errorf("post-summary suppressed warnings must not add output; got %d", len(captured))
	}

	// 5th after summary triggers the next summary.
	warn("next summary trigger")
	if len(captured) != 5 {
		t.Fatalf("next summary must fire after another `summaryEvery` suppressions; got %d", len(captured))
	}
	if !strings.Contains(captured[4], "suppressed 5") {
		t.Errorf("second summary must also report 5 suppressed since last; got %q", captured[4])
	}
}

// TestMakeReceiptWarnFn_NilBase guards against a programmer-error nil
// base. The wrapper must no-op rather than panic.
func TestMakeReceiptWarnFn_NilBase(t *testing.T) {
	warn := makeReceiptWarnFn(nil, 10, 100)
	// Should be a no-op, not a panic.
	warn("anything")
}

// TestMakeReceiptWarnFn_DisabledRateLimit verifies that <=0 knobs
// disable the rate limit (every call passes through).
func TestMakeReceiptWarnFn_DisabledRateLimit(t *testing.T) {
	var captured int
	base := func(string, ...interface{}) { captured++ }
	warn := makeReceiptWarnFn(base, 0, 100)
	for i := 0; i < 50; i++ {
		warn("warn %d", i)
	}
	if captured != 50 {
		t.Errorf("firstN<=0 must disable rate limit; got %d, want 50", captured)
	}
}

// --- attachReceiptsIfRequested --------------------------------------

func TestAttachReceiptsIfRequested_DisabledNoChange(t *testing.T) {
	events := []watchEvent{
		{Type: "drift.detected", Resource: watchEventResource{Kind: "Deployment", Name: "api", Namespace: "prod"}},
	}
	out := attachReceiptsIfRequested(context.Background(), events, nil, nil, false, nil)
	if len(out) != 1 || out[0].Receipt != nil {
		t.Errorf("disabled feature should leave events untouched; got %+v", out)
	}
}

func TestAttachReceiptsIfRequested_DriftDetectedGetsReceipt(t *testing.T) {
	// Use the seam to inject a prefab Statement; no real BuildReceipt
	// call needed.
	stmt := &agent.Statement{
		Type:          agent.StatementType,
		PredicateType: agent.PredicateTypeReceiptV1,
		Predicate: agent.Predicate{
			Version: agent.PredicateVersion,
			Verdict: agent.VerdictPASS,
		},
	}
	prev := watchBuildReceiptForEventFn
	watchBuildReceiptForEventFn = func(_ context.Context, _ watchEvent, _ dynamic.Interface, _ bool) (*agent.Statement, error) {
		return stmt, nil
	}
	defer func() { watchBuildReceiptForEventFn = prev }()

	events := []watchEvent{
		{Type: "drift.detected", Resource: watchEventResource{Kind: "Deployment", Name: "api", Namespace: "prod"}},
	}
	emitOn := map[string]bool{"drift.detected": true}
	out := attachReceiptsIfRequested(context.Background(), events, emitOn, nil, false, nil)
	if len(out) != 1 || out[0].Receipt == nil {
		t.Fatalf("drift.detected event should get a receipt attached; got %+v", out)
	}
	if out[0].Receipt.Predicate.Verdict != agent.VerdictPASS {
		t.Errorf("attached receipt verdict mismatch")
	}
}

func TestAttachReceiptsIfRequested_UnsupportedEventTypeSkipsSilently(t *testing.T) {
	// scan.finding is in the emitOn set but NOT in
	// watchEventTypesWithReceiptSupport — should pass through silently
	// (no warning, no receipt).
	var warnings []string
	warnFn := func(format string, args ...interface{}) {
		warnings = append(warnings, format)
	}
	prev := watchBuildReceiptForEventFn
	watchBuildReceiptForEventFn = func(_ context.Context, _ watchEvent, _ dynamic.Interface, _ bool) (*agent.Statement, error) {
		t.Fatal("watchBuildReceiptForEventFn must NOT be called for unsupported event types")
		return nil, nil
	}
	defer func() { watchBuildReceiptForEventFn = prev }()

	events := []watchEvent{
		{Type: "scan.finding", Resource: watchEventResource{Kind: "Deployment", Name: "api", Namespace: "prod"}},
	}
	emitOn := map[string]bool{"scan.finding": true}
	out := attachReceiptsIfRequested(context.Background(), events, emitOn, nil, false, warnFn)
	if out[0].Receipt != nil {
		t.Error("scan.finding should not get a receipt in #449 v1")
	}
	if len(warnings) != 0 {
		t.Errorf("unsupported event type should pass through silently; got warnings: %v", warnings)
	}
}

func TestAttachReceiptsIfRequested_BuildFailureNonFatal(t *testing.T) {
	// Receipt-build returns an error; the watch event must still emit
	// with Receipt=nil and a warning must fire.
	prev := watchBuildReceiptForEventFn
	watchBuildReceiptForEventFn = func(_ context.Context, _ watchEvent, _ dynamic.Interface, _ bool) (*agent.Statement, error) {
		return nil, &simulatedErr{msg: "transient cluster read failure"}
	}
	defer func() { watchBuildReceiptForEventFn = prev }()

	var warnings []string
	warnFn := func(format string, args ...interface{}) {
		warnings = append(warnings, format)
	}

	events := []watchEvent{
		{Type: "drift.detected", Resource: watchEventResource{Kind: "Deployment", Name: "api", Namespace: "prod"}},
	}
	emitOn := map[string]bool{"drift.detected": true}
	out := attachReceiptsIfRequested(context.Background(), events, emitOn, nil, false, warnFn)
	if len(out) != 1 {
		t.Fatalf("event count must be preserved on build failure; got %d", len(out))
	}
	if out[0].Receipt != nil {
		t.Errorf("build failure must leave Receipt=nil; got %+v", out[0].Receipt)
	}
	if len(warnings) != 1 {
		t.Errorf("build failure must emit exactly one warning; got %d", len(warnings))
	}
}

type simulatedErr struct{ msg string }

func (e *simulatedErr) Error() string { return e.msg }

// --- watchBuildReceiptForEvent end-to-end ----------------------------

func TestWatchBuildReceiptForEvent_HappyPath(t *testing.T) {
	// Use the loadLiveForWatchReceiptFn seam to inject a fake live
	// resource (an Argo-owned Deployment). BuildReceipt then auto-
	// detects applied-matches-spec; without a real Argo tracer, the
	// receipt verdict is INCONCLUSIVE (no git anchor) — that's
	// fine; we're testing the build path, not the verdict.
	live := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
				"labels": map[string]interface{}{
					"argocd.argoproj.io/instance": "payments-api",
				},
			},
		},
	}
	prev := loadLiveForWatchReceiptFn
	loadLiveForWatchReceiptFn = func(_ context.Context, _ dynamic.Interface, kind, name, namespace string) (*unstructured.Unstructured, error) {
		if kind != "Deployment" || name != "api" || namespace != "prod" {
			t.Fatalf("unexpected lookup: %s/%s in %s", kind, name, namespace)
		}
		return live, nil
	}
	defer func() { loadLiveForWatchReceiptFn = prev }()

	// Patch the *production* function (not the seam) so the test
	// exercises the real watchBuildReceiptForEvent path. We do this by
	// resetting watchBuildReceiptForEventFn to the production value if
	// a prior test swapped it.
	watchBuildReceiptForEventFn = watchBuildReceiptForEvent

	event := watchEvent{
		Type:      "drift.detected",
		Timestamp: time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC),
		Resource:  watchEventResource{Kind: "Deployment", Name: "api", Namespace: "prod"},
	}
	stmt, err := watchBuildReceiptForEvent(context.Background(), event, nil, false)
	if err != nil {
		t.Fatalf("watchBuildReceiptForEvent: %v", err)
	}
	if stmt == nil {
		t.Fatal("expected non-nil receipt")
	}
	if stmt.Type != agent.StatementType {
		t.Errorf("wrong statement type: %q", stmt.Type)
	}
	if stmt.Predicate.Scope.Kind != "Deployment" {
		t.Errorf("scope kind: %q", stmt.Predicate.Scope.Kind)
	}
	// Fingerprint must be stamped and verify.
	if err := agent.VerifyStatementFingerprint(*stmt); err != nil {
		t.Errorf("emitted receipt fingerprint must verify: %v", err)
	}
}

func TestWatchBuildReceiptForEvent_MissingKindOrName_Errors(t *testing.T) {
	event := watchEvent{Type: "drift.detected", Resource: watchEventResource{Kind: "", Name: "", Namespace: "prod"}}
	_, err := watchBuildReceiptForEvent(context.Background(), event, nil, false)
	if err == nil {
		t.Fatal("missing kind/name must error")
	}
}
