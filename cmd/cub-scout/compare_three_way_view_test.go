// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// fakeCubOutput builds a fake viewCubRunner that maps a command pattern to a
// JSON response. Keys are matched by joining the args with a space and doing a
// strings.Contains check — enough discrimination for these unit tests.
func fakeCubOutput(responses map[string]string) cubRunner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		for pattern, resp := range responses {
			if strings.Contains(joined, pattern) {
				return []byte(resp), nil
			}
		}
		return nil, fmt.Errorf("fakeCubRunner: no response for %q", joined)
	}
}

// fakeViewJSON builds the minimal `cub view get` JSON for a View with the
// given Where clause.
func fakeViewJSON(whereClause string) string {
	data := map[string]interface{}{
		"UUID": "806aac53-236c-446d-8ad6-91d6daf6810e",
		"Filter": map[string]interface{}{
			"Where": whereClause,
		},
	}
	b, _ := json.Marshal(data)
	return string(b)
}

// fakeUnitListJSON returns a `cub unit list -o json` JSON blob with the given slugs.
func fakeUnitListJSON(slugs []string) string {
	type unitRow struct {
		Slug string `json:"slug"`
	}
	rows := make([]unitRow, len(slugs))
	for i, s := range slugs {
		rows[i] = unitRow{Slug: s}
	}
	b, _ := json.Marshal(rows)
	return string(b)
}

// installFakeRunner swaps viewCubRunner for the test duration and restores it.
func installFakeRunner(t *testing.T, runner cubRunner) {
	t.Helper()
	orig := viewCubRunner
	viewCubRunner = runner
	t.Cleanup(func() { viewCubRunner = orig })
}

// installFakeDiscovery replaces the namespace/workload discovery fns for the
// test duration and restores them.
func installFakeDiscovery(t *testing.T, namespaceFn func() ([]string, error), workloadFn func(ns string) ([]WorkloadInfo, error)) {
	t.Helper()
	origNS := discoverThreeWayNamespacesFn
	origWL := discoverThreeWayWorkloadsFn
	discoverThreeWayNamespacesFn = namespaceFn
	discoverThreeWayWorkloadsFn = workloadFn
	t.Cleanup(func() {
		discoverThreeWayNamespacesFn = origNS
		discoverThreeWayWorkloadsFn = origWL
	})
}

// TestCollectThreeWayTargetsForView_HappyPath verifies the full multi-hop
// resolution: view get → extract filter → unit list → namespace/workload
// discovery → slug-match → threeWayTarget slice.
func TestCollectThreeWayTargetsForView_HappyPath(t *testing.T) {
	const viewUUID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	const whereClause = "metadata.labels.env = 'prod'"

	installFakeRunner(t, fakeCubOutput(map[string]string{
		"view get " + viewUUID: fakeViewJSON(whereClause),
		"unit list":            fakeUnitListJSON([]string{"payment-api", "checkout-svc"}),
	}))

	installFakeDiscovery(t,
		func() ([]string, error) { return []string{"prod"}, nil },
		func(ns string) ([]WorkloadInfo, error) {
			return []WorkloadInfo{
				{Kind: "Deployment", Name: "payment-api", Namespace: "prod", UnitSlug: "payment-api"},
				{Kind: "Deployment", Name: "checkout-svc", Namespace: "prod", UnitSlug: "checkout-svc"},
				{Kind: "Deployment", Name: "internal-tool", Namespace: "prod", UnitSlug: "internal-tool"}, // not in view
			}, nil
		},
	)

	targets, err := collectThreeWayTargetsForView(context.Background(), viewUUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d: %v", len(targets), targets)
	}

	// Order from sort.Strings on namespaces, then iteration order within ns.
	got := make([]string, len(targets))
	for i, tgt := range targets {
		got[i] = tgt.Namespace + "/" + tgt.ResourceArg
	}
	sort.Strings(got)
	want := []string{"prod/Deployment/checkout-svc", "prod/Deployment/payment-api"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("target[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// TestCollectThreeWayTargetsForView_URLInput verifies that a View Explorer URL
// is accepted and resolved to the same UUID as passing the UUID directly.
func TestCollectThreeWayTargetsForView_URLInput(t *testing.T) {
	const viewUUID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	const viewURL = "https://hub.confighub.com/x/view-explorer?view=" + viewUUID

	installFakeRunner(t, fakeCubOutput(map[string]string{
		"view get " + viewUUID: fakeViewJSON("metadata.labels.team = 'platform'"),
		"unit list":            fakeUnitListJSON([]string{"platform-api"}),
	}))

	installFakeDiscovery(t,
		func() ([]string, error) { return []string{"platform"}, nil },
		func(_ string) ([]WorkloadInfo, error) {
			return []WorkloadInfo{
				{Kind: "Deployment", Name: "platform-api", Namespace: "platform", UnitSlug: "platform-api"},
			}, nil
		},
	)

	targets, err := collectThreeWayTargetsForView(context.Background(), viewURL)
	if err != nil {
		t.Fatalf("unexpected error from URL input: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].ResourceArg != "Deployment/platform-api" {
		t.Errorf("ResourceArg = %q, want %q", targets[0].ResourceArg, "Deployment/platform-api")
	}
}

// TestCollectThreeWayTargetsForView_NoWhereClause verifies that a view with no
// Filter.Where returns an actionable error rather than an empty result.
func TestCollectThreeWayTargetsForView_NoWhereClause(t *testing.T) {
	const viewUUID = "806aac53-236c-446d-8ad6-91d6daf6810e"

	noFilter := `{"UUID":"` + viewUUID + `"}`
	installFakeRunner(t, fakeCubOutput(map[string]string{
		"view get " + viewUUID: noFilter,
	}))

	_, err := collectThreeWayTargetsForView(context.Background(), viewUUID)
	if err == nil {
		t.Fatal("expected error for view with no Where clause, got nil")
	}
	if !strings.Contains(err.Error(), "no Where filter") {
		t.Errorf("error %q should mention 'no Where filter'", err.Error())
	}
}

// TestCollectThreeWayTargetsForView_NoMatchingUnits verifies that an empty unit
// list produces an actionable error, not silent empty results.
func TestCollectThreeWayTargetsForView_NoMatchingUnits(t *testing.T) {
	const viewUUID = "806aac53-236c-446d-8ad6-91d6daf6810e"

	installFakeRunner(t, fakeCubOutput(map[string]string{
		"view get " + viewUUID: fakeViewJSON("metadata.labels.env = 'staging'"),
		"unit list":            `[]`,
	}))

	_, err := collectThreeWayTargetsForView(context.Background(), viewUUID)
	if err == nil {
		t.Fatal("expected error for view that matches no units, got nil")
	}
	if !strings.Contains(err.Error(), "matched no units") {
		t.Errorf("error %q should mention 'matched no units'", err.Error())
	}
}

// TestCollectThreeWayTargetsForView_SlugFilterIsStrict verifies that only
// workloads whose UnitSlug appears in the view's unit list are included; all
// others are silently excluded (not errors).
func TestCollectThreeWayTargetsForView_SlugFilterIsStrict(t *testing.T) {
	const viewUUID = "806aac53-236c-446d-8ad6-91d6daf6810e"

	installFakeRunner(t, fakeCubOutput(map[string]string{
		"view get " + viewUUID: fakeViewJSON("metadata.labels.team = 'payments'"),
		"unit list":            fakeUnitListJSON([]string{"payments-api"}),
	}))

	installFakeDiscovery(t,
		func() ([]string, error) { return []string{"prod", "staging"}, nil },
		func(ns string) ([]WorkloadInfo, error) {
			// Both namespaces have the same workload names but only "payments-api"
			// is in the view. "unrelated" must be excluded.
			return []WorkloadInfo{
				{Kind: "Deployment", Name: "payments-api", Namespace: ns, UnitSlug: "payments-api"},
				{Kind: "Deployment", Name: "unrelated", Namespace: ns, UnitSlug: "other-unit"},
			}, nil
		},
	)

	targets, err := collectThreeWayTargetsForView(context.Background(), viewUUID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Two namespaces × one matching workload = 2 targets.
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets (one per namespace), got %d: %v", len(targets), targets)
	}
	for _, tgt := range targets {
		if tgt.ResourceArg != "Deployment/payments-api" {
			t.Errorf("unexpected target %v — only payments-api should be included", tgt)
		}
	}
}

// TestThreeWayScopeView_String verifies that the Scope field in the report
// is "view/<uuid>" for both UUID and URL inputs.
func TestThreeWayScopeView_String(t *testing.T) {
	const uuid = "806aac53-236c-446d-8ad6-91d6daf6810e"
	cases := []struct {
		input string
		want  string
	}{
		{uuid, "view/" + uuid},
		{"https://hub.confighub.com/x/view-explorer?view=" + uuid, "view/" + uuid},
	}
	for _, tc := range cases {
		scope := threeWayScope{ScopeType: threeWayScopeView, ScopeValue: tc.input}
		if got := scope.String(); got != tc.want {
			t.Errorf("String() for %q = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestRunCompareThreeWay_MutualExclusion verifies that passing both --scope
// and --view returns a clear error.
func TestRunCompareThreeWay_MutualExclusion(t *testing.T) {
	orig := compareThreeWayScopeRaw
	origView := compareThreeWayView
	compareThreeWayScopeRaw = "cluster"
	compareThreeWayView = "806aac53-236c-446d-8ad6-91d6daf6810e"
	t.Cleanup(func() {
		compareThreeWayScopeRaw = orig
		compareThreeWayView = origView
	})

	err := runCompareThreeWay(nil, nil)
	if err == nil {
		t.Fatal("expected error for --scope + --view, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error %q should mention 'mutually exclusive'", err.Error())
	}
}
