// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
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
