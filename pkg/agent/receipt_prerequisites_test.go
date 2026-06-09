// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"
)

func buildPrereqReceipt(t *testing.T, facts []PrerequisiteFactResult) Statement {
	t.Helper()
	ev, err := BuildPrerequisitesEvidence(
		ObjectSetSource{Type: "file", Ref: "prereqs.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "helm-expt-demo"},
		facts,
	)
	if err != nil {
		t.Fatalf("BuildPrerequisitesEvidence: %v", err)
	}
	stmt, err := BuildPrerequisitesReceipt(BuildPrerequisitesReceiptInput{
		Evidence:   ev,
		Verifier:   Verifier{Tool: "cub-scout", Version: "test"},
		VerifiedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildPrerequisitesReceipt: %v", err)
	}
	if stmt.Predicate.PredicateName != string(PredicatePrerequisitesMet) {
		t.Fatalf("predicate = %s", stmt.Predicate.PredicateName)
	}
	if err := VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return stmt
}

func TestPrerequisites_PASSWhenAllPresent(t *testing.T) {
	stmt := buildPrereqReceipt(t, []PrerequisiteFactResult{
		{Kind: PrerequisiteKindNamespace, Name: "helm-expt-demo", Status: PrerequisitePresent},
		{Kind: PrerequisiteKindSecret, Name: "app-db-secret", Namespace: "helm-expt-demo", Detail: "keys: password", Status: PrerequisitePresent},
	})
	if stmt.Predicate.Verdict != VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.Prerequisites.Summary.Present != 2 {
		t.Fatalf("present = %d, want 2", stmt.Predicate.Evidence.Prerequisites.Summary.Present)
	}
}

// The F3 root cause as a pre-flight check: the required Secret is absent.
func TestPrerequisites_BLOCKOnMissingSecret(t *testing.T) {
	stmt := buildPrereqReceipt(t, []PrerequisiteFactResult{
		{Kind: PrerequisiteKindNamespace, Name: "helm-expt-demo", Status: PrerequisitePresent},
		{Kind: PrerequisiteKindSecret, Name: "app-db-secret", Namespace: "helm-expt-demo", Status: PrerequisiteMissing, Error: "secrets \"app-db-secret\" not found"},
	})
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.Prerequisites.Summary.Missing != 1 {
		t.Fatalf("missing = %d, want 1", stmt.Predicate.Evidence.Prerequisites.Summary.Missing)
	}
}

func TestPrerequisites_INCONCLUSIVEOnCheckGap(t *testing.T) {
	stmt := buildPrereqReceipt(t, []PrerequisiteFactResult{
		{Kind: PrerequisiteKindCRD, Name: "servicemonitors.monitoring.coreos.com", Status: PrerequisiteInconclusive, Error: "the server could not find the requested resource"},
	})
	if stmt.Predicate.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict = %s, want INCONCLUSIVE", stmt.Predicate.Verdict)
	}
	found := false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionPrerequisitesCoverage {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected %s omission", OmissionPrerequisitesCoverage)
	}
}
