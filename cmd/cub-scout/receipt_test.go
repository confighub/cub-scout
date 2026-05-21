// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// makeReceiptArgoLive returns an Argo-owned Deployment with the annotations
// the auto-detect path uses. The receipt CLI is exercised end-to-end against
// this fake object via the loadReceiptLiveFn seam.
func makeReceiptArgoLive() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
				"annotations": map[string]interface{}{
					"argocd.argoproj.io/tracking-id": "payments-api:apps/Deployment:prod/api",
				},
				"labels": map[string]interface{}{
					"argocd.argoproj.io/instance": "payments-api",
				},
				"managedFields": []interface{}{
					map[string]interface{}{
						"manager":   "argocd-controller",
						"operation": "Apply",
					},
				},
			},
			"spec": map[string]interface{}{"replicas": int64(3)},
		},
	}
}

// withFakeReceiptLoader installs a fake loadReceiptLiveFn that returns obj
// and restores the production loader on cleanup. The seam at
// loadReceiptLiveFn lets the CLI integration tests run without a cluster.
func withFakeReceiptLoader(t *testing.T, obj *unstructured.Unstructured) {
	t.Helper()
	prev := loadReceiptLiveFn
	loadReceiptLiveFn = func(_ context.Context, _, _, _ string) (*unstructured.Unstructured, error) {
		return obj, nil
	}
	t.Cleanup(func() { loadReceiptLiveFn = prev })
}

// resetReceiptFlags zeros out the package-scoped flags so consecutive
// tests don't bleed state into each other.
func resetReceiptFlags(t *testing.T) {
	t.Helper()
	prev := struct {
		ns, pred, at, fmt, out string
	}{receiptNamespace, receiptPredicate, receiptAtCommit, receiptFormat, receiptOut}
	receiptNamespace = ""
	receiptPredicate = ""
	receiptAtCommit = ""
	receiptFormat = "ascii"
	receiptOut = ""
	t.Cleanup(func() {
		receiptNamespace = prev.ns
		receiptPredicate = prev.pred
		receiptAtCommit = prev.at
		receiptFormat = prev.fmt
		receiptOut = prev.out
	})
}

func TestReceiptVerify_ASCII_AutoDetectInconclusive(t *testing.T) {
	// Argo-owned deployment in standalone test environment: no Argo CLI
	// installed so CollectGitSourceAnchorForOwner returns nil. After the
	// Codex #446 round-4 fix, AutoDetectPredicate declines (no resolved
	// anchor → no default predicate, OmissionAutoDetectedPredicate). The
	// receipt is INCONCLUSIVE with an empty predicateName.
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	// ASCII rendering structural checks (one-screen review format).
	for _, want := range []string{
		"Verdict: INCONCLUSIVE",
		"Scope:   Deployment/api in prod",
		"By:      cub-scout",
		"auto-detected-predicate",
		"Fingerprint: ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in ASCII output, got:\n%s", want, out)
		}
	}
}

func TestReceiptVerify_JSON_RoundTripsToStatement(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("output is not valid in-toto Statement v1 JSON: %v\nraw:\n%s", err, out)
	}

	if stmt.Type != agent.StatementType {
		t.Errorf("Statement type: got %q, want %q", stmt.Type, agent.StatementType)
	}
	if stmt.PredicateType != agent.PredicateTypeReceiptV1 {
		t.Errorf("Statement predicateType: got %q, want %q", stmt.PredicateType, agent.PredicateTypeReceiptV1)
	}
	if len(stmt.Subject) == 0 {
		t.Error("Statement.Subject is empty; expected at least the k8s-live subject")
	}
	if stmt.Predicate.Fingerprint == "" {
		t.Error("Predicate.Fingerprint not stamped on JSON output")
	}
	if err := agent.VerifyStatementFingerprint(stmt); err != nil {
		t.Errorf("fingerprint integrity check failed: %v", err)
	}
}

func TestReceiptVerify_OutFile_WritesJSONRegardlessOfConsoleFormat(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "api.receipt.json")

	// Console format is ASCII, but the on-disk artifact must be JSON.
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--format", "ascii",
			"--out", outPath,
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --out returned error: %v", err)
		}
	})

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read --out file: %v", err)
	}
	var stmt agent.Statement
	if err := json.Unmarshal(data, &stmt); err != nil {
		t.Fatalf("on-disk artifact must be valid in-toto Statement JSON, even when console format is ascii: %v\nraw:\n%s", err, data)
	}
	if stmt.Type != agent.StatementType {
		t.Errorf("on-disk Statement type: got %q, want %q", stmt.Type, agent.StatementType)
	}
	if stmt.Predicate.Fingerprint == "" {
		t.Error("on-disk artifact missing fingerprint")
	}
}

func TestReceiptVerify_InvalidFormat_Errors(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--format", "xml",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for --format xml")
	}
	if !strings.Contains(err.Error(), "invalid --format") {
		t.Errorf("error should explain invalid format; got %v", err)
	}
}

func TestReceiptVerify_BadSubject_Errors(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{"receipt", "verify", "not-a-valid-subject"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for malformed subject")
	}
	if !strings.Contains(err.Error(), "invalid subject") {
		t.Errorf("error should explain the subject parse failure; got %v", err)
	}
}

func TestReceiptVerify_StandaloneMode_OmitsConfigHubUnitSubject(t *testing.T) {
	// Without ConfigHub auth, the receipt must still build but record the
	// missing unit subject as an omission. This proves the standalone-mode
	// path is real and the connected-mode-only data is not silently treated
	// as available.
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	found := false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == agent.OmissionConfigHubUnitSubject {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected OmissionConfigHubUnitSubject in standalone-mode output; omissions=%+v", stmt.Predicate.Omissions)
	}
}

// TestReceiptHasNoMutatingNextSteps is the acceptance-criteria guardrail
// from #446: receipts emitted by any predicate evaluator and the build
// orchestration MUST NOT contain mutating actionType or mutating
// nextCommand patterns. The filter is defense in depth — if a future
// evaluator slips a mutating step in, FilterNextSteps will drop it before
// the receipt is stamped.
func TestReceiptHasNoMutatingNextSteps(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, step := range stmt.Predicate.NextSteps {
		// Allowed actionTypes (mirrors FilterNextSteps in
		// pkg/agent/receipt_predicates.go).
		if step.ActionType != "read-only" && step.ActionType != "waiting" && step.ActionType != "human-decision" {
			t.Errorf("receipt emitted next-step with non-read-only actionType %q: %+v", step.ActionType, step)
		}

		lower := strings.ToLower(step.NextCommand)
		for _, forbidden := range []string{
			"apply",
			"edit",
			"patch",
			"delete",
			"create",
			"update",
			"replace",
			"scale",
			"rollout",
			"reconcile",
		} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("receipt emitted next-step with mutating nextCommand %q (forbidden fragment: %q)", step.NextCommand, forbidden)
			}
		}
	}
}
