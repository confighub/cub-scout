// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// withFakeSourceTruth installs a fake source-truth collector and restores
// the production one on cleanup. Lets the source-truth-pass CLI path run
// without a real ConfigHub or controller CLI.
func withFakeSourceTruth(t *testing.T, ev *agent.SourceTruthEvidence) {
	t.Helper()
	prev := collectSourceTruthForReceiptFn
	collectSourceTruthForReceiptFn = func(_ context.Context, _, _, _, _ string) (*agent.SourceTruthEvidence, error) {
		return ev, nil
	}
	t.Cleanup(func() { collectSourceTruthForReceiptFn = prev })
}

// resetReceiptBatch2Flags zeros out the batch-2 receipt flags (strategy,
// since) so consecutive tests don't bleed state.
func resetReceiptBatch2Flags(t *testing.T) {
	t.Helper()
	resetReceiptFlags(t)
	prevStrategy, prevSince := receiptStrategy, receiptSince
	receiptStrategy = ""
	receiptSince = ""
	t.Cleanup(func() {
		receiptStrategy = prevStrategy
		receiptSince = prevSince
	})
}

func makeLiveWithManagedFieldsForCLI(entries []metav1.ManagedFieldsEntry) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{"replicas": int64(3)},
		},
	}
	obj.SetManagedFields(entries)
	return obj
}

// --- source-truth-pass path ------------------------------------------

func TestReceiptVerify_SourceTruthPass_HappyPath(t *testing.T) {
	resetReceiptBatch2Flags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	withFakeSourceTruth(t, &agent.SourceTruthEvidence{
		DeclaredStrategy: agent.StrategyGitArgo.Human(),
		Status:           agent.StatusPASS,
		SourceTruth:      agent.VerdictAGREED,
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--strategy", "git-argo",
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --strategy returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicateSourceTruthPass) {
		t.Errorf("predicateName: got %q, want source-truth-pass", stmt.Predicate.PredicateName)
	}
	if stmt.Predicate.Verdict != agent.VerdictPASS {
		t.Errorf("verdict: got %q, want PASS", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.SourceTruth == nil {
		t.Fatal("source-truth-pass receipt must carry SourceTruth evidence")
	}
	if stmt.Predicate.Evidence.SourceTruth.Status != agent.StatusPASS {
		t.Errorf("SourceTruth.Status: got %q, want PASS", stmt.Predicate.Evidence.SourceTruth.Status)
	}
	if err := agent.VerifyStatementFingerprint(stmt); err != nil {
		t.Errorf("fingerprint integrity check failed: %v", err)
	}
}

func TestReceiptVerify_SourceTruthPass_ExplicitPredicateNoStrategy_Errors(t *testing.T) {
	resetReceiptBatch2Flags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--predicate", "source-truth-pass",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for --predicate source-truth-pass without --strategy")
	}
	if !strings.Contains(err.Error(), "--strategy") {
		t.Errorf("error must point at the missing --strategy flag; got %v", err)
	}
}

func TestReceiptVerify_SourceTruthPass_StrategyMismatch_BLOCK(t *testing.T) {
	resetReceiptBatch2Flags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	// Receipt is invoked with --strategy git-argo, but the evidence
	// derivation came back with confighub-oci-argo. The predicate must
	// emit BLOCK + OmissionStrategyMismatch.
	withFakeSourceTruth(t, &agent.SourceTruthEvidence{
		DeclaredStrategy: agent.StrategyConfigHubOCIArgo.Human(),
		Status:           agent.StatusPASS,
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--strategy", "git-argo",
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stmt.Predicate.Verdict != agent.VerdictBLOCK {
		t.Errorf("strategy mismatch must produce BLOCK; got %q", stmt.Predicate.Verdict)
	}
	found := false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == agent.OmissionStrategyMismatch {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected OmissionStrategyMismatch; got %v", stmt.Predicate.Omissions)
	}
}

// --- no-manual-edits-since path --------------------------------------

func TestReceiptVerify_NoManualEditsSince_HappyPath_PASS(t *testing.T) {
	resetReceiptBatch2Flags(t)
	cutoff := "2026-05-22T00:00:00Z"
	before := metav1.NewTime(time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC))
	withFakeReceiptLoader(t, makeLiveWithManagedFieldsForCLI([]metav1.ManagedFieldsEntry{
		{Manager: "argocd-controller", Operation: metav1.ManagedFieldsOperationApply, Time: &before},
	}))

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--since", cutoff,
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --since returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicateNoManualEditsSince) {
		t.Errorf("predicateName: got %q, want no-manual-edits-since", stmt.Predicate.PredicateName)
	}
	if stmt.Predicate.Verdict != agent.VerdictPASS {
		t.Errorf("controller-only managedFields must produce PASS; got %q", stmt.Predicate.Verdict)
	}
}

func TestReceiptVerify_NoManualEditsSince_KubectlEdit_BLOCK(t *testing.T) {
	resetReceiptBatch2Flags(t)
	after := metav1.NewTime(time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC))
	withFakeReceiptLoader(t, makeLiveWithManagedFieldsForCLI([]metav1.ManagedFieldsEntry{
		{Manager: "argocd-controller", Operation: metav1.ManagedFieldsOperationApply, Time: &after},
		{Manager: "kubectl-edit", Operation: metav1.ManagedFieldsOperationUpdate, Time: &after},
	}))

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--since", "2026-05-22T00:00:00Z",
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --since returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stmt.Predicate.Verdict != agent.VerdictBLOCK {
		t.Errorf("kubectl-edit after cutoff must produce BLOCK; got %q", stmt.Predicate.Verdict)
	}
}

func TestReceiptVerify_NoManualEditsSince_BadCutoff_Errors(t *testing.T) {
	resetReceiptBatch2Flags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--since", "yesterday-ish",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for malformed --since")
	}
	if !strings.Contains(err.Error(), "invalid --since") {
		t.Errorf("error must explain the parse failure; got %v", err)
	}
}

func TestReceiptVerify_NoManualEditsSince_ExplicitPredicateNoSince_Errors(t *testing.T) {
	resetReceiptBatch2Flags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--predicate", "no-manual-edits-since",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for --predicate no-manual-edits-since without --since")
	}
	if !strings.Contains(err.Error(), "--since") {
		t.Errorf("error must point at the missing --since flag; got %v", err)
	}
}

// --- auto-detect (CLI surface) ----------------------------------------

func TestReceiptVerify_AutoDetect_StrategyPicksSourceTruthPass(t *testing.T) {
	// No --predicate. --strategy alone must auto-detect source-truth-pass.
	resetReceiptBatch2Flags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	withFakeSourceTruth(t, &agent.SourceTruthEvidence{
		DeclaredStrategy: agent.StrategyGitArgo.Human(),
		Status:           agent.StatusPASS,
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--strategy", "git-argo",
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicateSourceTruthPass) {
		t.Errorf("predicateName: got %q, want source-truth-pass (auto-detect via --strategy)", stmt.Predicate.PredicateName)
	}
}

func TestReceiptVerify_AutoDetect_SincePicksNoManualEditsSince(t *testing.T) {
	resetReceiptBatch2Flags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--since", "2026-05-22T00:00:00Z",
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicateNoManualEditsSince) {
		t.Errorf("predicateName: got %q, want no-manual-edits-since (auto-detect via --since)", stmt.Predicate.PredicateName)
	}
}
