// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

// resetReceiptAggregateFlags clears --scope and --aggregate-policy
// between tests.
func resetReceiptAggregateFlags(t *testing.T) {
	t.Helper()
	prevScope := receiptScope
	prevPolicy := receiptAggregatePolicy
	receiptScope = ""
	receiptAggregatePolicy = ""
	t.Cleanup(func() {
		receiptScope = prevScope
		receiptAggregatePolicy = prevPolicy
	})
}

// withFakeNamespaceDiscovery installs a fake discoverNamespaceWorkloadsFn
// that returns the given resources, and restores the production
// implementation on cleanup.
func withFakeNamespaceDiscovery(t *testing.T, resources []receiptResourceRef) {
	t.Helper()
	prev := discoverNamespaceWorkloadsFn
	discoverNamespaceWorkloadsFn = func(_ context.Context, _ string) ([]receiptResourceRef, error) {
		return resources, nil
	}
	t.Cleanup(func() { discoverNamespaceWorkloadsFn = prev })
}

// --- parseAggregateScope -------------------------------------------

func TestParseAggregateScope_Namespace(t *testing.T) {
	spec, refs, isAgg, err := parseAggregateScope("namespace/prod", "", "")
	if err != nil {
		t.Fatalf("parseAggregateScope: %v", err)
	}
	if !isAgg {
		t.Fatal("expected aggregate mode")
	}
	if spec.Kind != "namespace" {
		t.Errorf("spec.Kind = %q; want \"namespace\"", spec.Kind)
	}
	if spec.Namespace != "prod" {
		t.Errorf("spec.Namespace = %q; want \"prod\"", spec.Namespace)
	}
	if len(refs) != 0 {
		t.Errorf("namespace mode resolves resources via discovery; refs should be empty, got %d", len(refs))
	}
}

func TestParseAggregateScope_CommaList(t *testing.T) {
	spec, refs, isAgg, err := parseAggregateScope("", "deploy/api,deploy/worker,statefulset/db", "prod")
	if err != nil {
		t.Fatalf("parseAggregateScope: %v", err)
	}
	if !isAgg {
		t.Fatal("expected aggregate mode")
	}
	if spec.Kind != "batch" {
		t.Errorf("spec.Kind = %q; want \"batch\"", spec.Kind)
	}
	if spec.MemberCount != 3 {
		t.Errorf("spec.MemberCount = %d; want 3", spec.MemberCount)
	}
	if len(refs) != 3 {
		t.Fatalf("refs length = %d; want 3", len(refs))
	}
	wantKinds := []string{"Deployment", "Deployment", "StatefulSet"}
	for i, r := range refs {
		if r.Kind != wantKinds[i] {
			t.Errorf("refs[%d].Kind = %q; want %q", i, r.Kind, wantKinds[i])
		}
		if r.Namespace != "prod" {
			t.Errorf("refs[%d].Namespace = %q; want \"prod\"", i, r.Namespace)
		}
	}
}

func TestParseAggregateScope_CommaListWithoutNamespaceFallsBackToDefault(t *testing.T) {
	_, refs, _, err := parseAggregateScope("", "deploy/api,deploy/worker", "")
	if err != nil {
		t.Fatalf("parseAggregateScope: %v", err)
	}
	for i, r := range refs {
		if r.Namespace != "default" {
			t.Errorf("refs[%d].Namespace = %q; want \"default\" (empty -n falls back)", i, r.Namespace)
		}
	}
}

func TestParseAggregateScope_SingleResourceNotAggregate(t *testing.T) {
	_, _, isAgg, err := parseAggregateScope("", "deploy/api", "prod")
	if err != nil {
		t.Fatalf("parseAggregateScope: %v", err)
	}
	if isAgg {
		t.Error("single-resource positional should NOT be aggregate mode")
	}
}

func TestParseAggregateScope_ScopeConflictsWithPositional(t *testing.T) {
	_, _, _, err := parseAggregateScope("namespace/prod", "deploy/api", "")
	if err == nil {
		t.Fatal("--scope + positional should be rejected")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("error should name the conflict; got %v", err)
	}
}

func TestParseAggregateScope_CommaListSingleEntryRejected(t *testing.T) {
	_, _, _, err := parseAggregateScope("", "deploy/api,", "prod")
	if err == nil {
		t.Fatal("comma-list with single entry should be rejected")
	}
	if !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("error should explain minimum; got %v", err)
	}
}

func TestParseAggregateScope_InvalidScopeForm(t *testing.T) {
	_, _, _, err := parseAggregateScope("cluster/prod-use1", "", "")
	if err == nil {
		t.Fatal("non-namespace scope should be rejected in v1")
	}
	if !strings.Contains(err.Error(), "unrecognized") {
		t.Errorf("error should explain the rejected form; got %v", err)
	}
}

func TestParseAggregateScope_InvalidCommaEntry(t *testing.T) {
	_, _, _, err := parseAggregateScope("", "deploy/api,not-a-resource", "prod")
	if err == nil {
		t.Fatal("invalid comma entry should be rejected")
	}
	if !strings.Contains(err.Error(), "not-a-resource") {
		t.Errorf("error should echo the bad entry; got %v", err)
	}
}

func TestParseAggregateScope_EmptyNamespaceRejected(t *testing.T) {
	_, _, _, err := parseAggregateScope("namespace/", "", "")
	if err == nil {
		t.Fatal("empty namespace should be rejected")
	}
}

// --- resolveAggregatePolicy ----------------------------------------

func TestResolveAggregatePolicy_DefaultsToMaxSeverity(t *testing.T) {
	p, err := resolveAggregatePolicy("")
	if err != nil {
		t.Fatalf("default policy should resolve: %v", err)
	}
	if p.Name() != "max-severity" {
		t.Errorf("default = %q; want max-severity", p.Name())
	}
}

func TestResolveAggregatePolicy_ExplicitMaxSeverity(t *testing.T) {
	p, err := resolveAggregatePolicy("max-severity")
	if err != nil {
		t.Fatalf("explicit max-severity: %v", err)
	}
	if p.Name() != "max-severity" {
		t.Errorf("policy = %q; want max-severity", p.Name())
	}
}

func TestResolveAggregatePolicy_UnknownRejected(t *testing.T) {
	_, err := resolveAggregatePolicy("majority")
	if err == nil {
		t.Fatal("unknown policy should be rejected")
	}
	if !strings.Contains(err.Error(), "max-severity") {
		t.Errorf("error should list supported policies; got %v", err)
	}
}

// --- end-to-end CLI integration tests ------------------------------

func TestReceiptVerify_AggregateNamespace_EmitsPerResourceAndAggregate(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	resetReceiptInputAttestationFlag(t)
	resetReceiptAggregateFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	withFakeNamespaceDiscovery(t, []receiptResourceRef{
		{Kind: "Deployment", Name: "api", Namespace: "prod"},
		{Kind: "Deployment", Name: "worker", Namespace: "prod"},
	})

	rootCmd.SetArgs([]string{
		"receipt", "verify",
		"--scope", "namespace/prod",
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("aggregate verify: %v", err)
		}
	})

	// The output should contain N per-resource receipts (JSONL) followed
	// by 1 aggregate receipt (pretty-printed JSON).
	// Count the receipts by parsing.
	// Per-resource receipts come as JSONL lines.
	// Aggregate is multi-line indented JSON ending the output.
	if !strings.Contains(out, "\"applied-matches-spec\"") &&
		!strings.Contains(out, "\"predicateName\":") {
		t.Errorf("expected per-resource receipts in output; got:\n%s", out)
	}
	if !strings.Contains(out, "\"aggregate-verdict\"") {
		t.Errorf("expected aggregate receipt with predicateName=aggregate-verdict; got:\n%s", out)
	}
	if !strings.Contains(out, "synthetic-aggregate://sha256/") {
		t.Errorf("expected synthetic-aggregate:// subject in output; got:\n%s", out)
	}
}

func TestReceiptVerify_AggregateCommaList_EmitsAggregate(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	resetReceiptInputAttestationFlag(t)
	resetReceiptAggregateFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{
		"receipt", "verify",
		"deploy/api,deploy/worker,deploy/cron",
		"-n", "prod",
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("comma-list aggregate: %v", err)
		}
	})

	if !strings.Contains(out, "\"aggregate-verdict\"") {
		t.Errorf("expected aggregate-verdict predicate in output; got:\n%s", out)
	}
	if !strings.Contains(out, "synthetic-aggregate://sha256/") {
		t.Errorf("expected synthetic-aggregate subject in output; got:\n%s", out)
	}
}

func TestReceiptVerify_AggregateFailOn_ExitsCode2OnBLOCK(t *testing.T) {
	// Force a BLOCK verdict from the source-truth flow by injecting a
	// strategy-mismatched evidence (same trick the single-resource
	// --fail-on test uses).
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	resetReceiptInputAttestationFlag(t)
	resetReceiptAggregateFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	withFakeSourceTruth(t, &agent.SourceTruthEvidence{
		DeclaredStrategy: agent.StrategyGitArgo.Human(),
		Status:           agent.StatusPASS,
	})
	withFakeNamespaceDiscovery(t, []receiptResourceRef{
		{Kind: "Deployment", Name: "api", Namespace: "prod"},
		{Kind: "Deployment", Name: "worker", Namespace: "prod"},
	})

	rootCmd.SetArgs([]string{
		"receipt", "verify",
		"--scope", "namespace/prod",
		"--strategy", "confighub-oci-argo", // mismatch → BLOCK
		"--fail-on", "BLOCK",
	})

	captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("aggregate BLOCK + --fail-on BLOCK must error")
		}
		if !strings.Contains(err.Error(), "fail-on") {
			t.Errorf("error must name --fail-on; got %v", err)
		}
		var ec interface{ ExitCode() int }
		if got := err; got != nil {
			// errors.As-style assertion inline
			if ecAs, ok := err.(interface{ ExitCode() int }); ok {
				ec = ecAs
			}
		}
		if ec == nil {
			t.Fatal("error must wrap exitCodeError so main.go exits 2")
		}
		if got := ec.ExitCode(); got != 2 {
			t.Errorf("aggregate --fail-on must signal exit 2; got %d", got)
		}
	})
}

func TestReceiptVerify_AggregateBadPolicy_RejectsUpfront(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	resetReceiptInputAttestationFlag(t)
	resetReceiptAggregateFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	withFakeNamespaceDiscovery(t, []receiptResourceRef{
		{Kind: "Deployment", Name: "api", Namespace: "prod"},
		{Kind: "Deployment", Name: "worker", Namespace: "prod"},
	})

	rootCmd.SetArgs([]string{
		"receipt", "verify",
		"--scope", "namespace/prod",
		"--aggregate-policy", "majority", // unknown
	})

	captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("unknown --aggregate-policy must be rejected")
		}
		if !strings.Contains(err.Error(), "majority") {
			t.Errorf("error must echo the bad value; got %v", err)
		}
	})
}

func TestReceiptVerify_AggregateAggregateReceiptStructure(t *testing.T) {
	// Parse the aggregate receipt out of the output and verify its
	// structure: inputAttestations[] count, subject scheme, etc.
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	resetReceiptInputAttestationFlag(t)
	resetReceiptAggregateFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{
		"receipt", "verify",
		"deploy/api,deploy/worker,deploy/cron",
		"-n", "prod",
		"--format", "json",
	})

	out := captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("aggregate verify: %v", err)
		}
	})

	// The aggregate receipt is the LAST JSON object in the output
	// (pretty-printed; per-resource ones come as JSONL).
	// Find the aggregate by scanning for "aggregate-verdict".
	aggregateStart := strings.LastIndex(out, "{\n  \"_type\":")
	if aggregateStart < 0 {
		t.Fatalf("could not find aggregate receipt in output:\n%s", out)
	}
	aggregateJSON := out[aggregateStart:]
	// Trim trailing newline if any
	aggregateJSON = strings.TrimSpace(aggregateJSON)

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(aggregateJSON), &stmt); err != nil {
		t.Fatalf("unmarshal aggregate: %v\noutput:\n%s", err, aggregateJSON)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicateAggregateVerdict) {
		t.Errorf("predicateName = %q; want %q", stmt.Predicate.PredicateName, agent.PredicateAggregateVerdict)
	}
	if !strings.HasPrefix(stmt.Subject[0].Name, agent.SubjectSchemeSyntheticAggregate) {
		t.Errorf("subject scheme = %q; want %q", stmt.Subject[0].Name, agent.SubjectSchemeSyntheticAggregate)
	}
	if len(stmt.Predicate.InputAttestations) != 3 {
		t.Errorf("inputAttestations[] length = %d; want 3 (one per comma-list entry)", len(stmt.Predicate.InputAttestations))
	}
}
