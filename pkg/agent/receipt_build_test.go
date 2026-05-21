// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeArgoLive() *unstructured.Unstructured {
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
			},
			"spec": map[string]interface{}{"replicas": int64(3)},
		},
	}
}

func makeReceiptInput(modifiers ...func(*BuildReceiptInput)) BuildReceiptInput {
	in := BuildReceiptInput{
		Live: makeArgoLive(),
		Scope: Scope{
			Kind:      "Deployment",
			Name:      "api",
			Namespace: "prod",
			Cluster:   "prod-use2",
		},
		Owner: Ownership{
			Type:    OwnerArgo,
			SubType: "application",
		},
		PredicateName: PredicateAppliedMatchesSpec,
		Spec: &SpecAnchor{
			Anchor: SpecAnchorBody{
				Type:     "git",
				RepoURL:  "https://github.com/org/repo",
				Revision: "abc123def456",
				Path:     "apps/prod/api",
			},
		},
		Evidence: Evidence{
			Attribution: &FieldMutationAttribution{
				Cause:       CauseControllerDrift,
				ManagerHint: "argocd-controller",
			},
			GitSource: &GitSourceAnchor{
				RepoURL:  "https://github.com/org/repo",
				Revision: "abc123def456",
				Path:     "apps/prod/api",
			},
		},
		Connected:  true,
		Verifier:   Verifier{Tool: "cub-scout", Version: "v2.3.0"},
		VerifiedAt: time.Date(2026, 5, 21, 10, 30, 0, 0, time.UTC),
	}
	for _, m := range modifiers {
		m(&in)
	}
	return in
}

func TestBuildReceipt_HappyPath_ControllerDrift(t *testing.T) {
	in := makeReceiptInput()
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}

	if stmt.Type != StatementType {
		t.Errorf("statement type: got %q, want %q", stmt.Type, StatementType)
	}
	if stmt.PredicateType != PredicateTypeReceiptV1 {
		t.Errorf("predicate type: got %q, want %q", stmt.PredicateType, PredicateTypeReceiptV1)
	}
	if stmt.Predicate.Verdict != VerdictPASS {
		t.Errorf("verdict: got %q, want PASS", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Fingerprint == "" {
		t.Error("fingerprint not stamped")
	}

	// Verify fingerprint integrity.
	if err := VerifyStatementFingerprint(stmt); err != nil {
		t.Errorf("VerifyStatementFingerprint failed: %v", err)
	}

	// Connected with no ConfigHub linkage → exactly one omission about
	// missing unit subject.
	missings := []string{}
	for _, o := range stmt.Predicate.Omissions {
		missings = append(missings, o.Missing)
	}
	if !receiptContainsString(missings, OmissionConfigHubUnitSubject) {
		t.Errorf("expected omission %q; got %v", OmissionConfigHubUnitSubject, missings)
	}

	// Subjects: one k8s-live, no confighub-unit (since we didn't pass
	// unit data despite Connected=true).
	if len(stmt.Subject) != 1 {
		t.Fatalf("expected 1 subject (only k8s-live in this connected-but-no-unit-data case); got %d", len(stmt.Subject))
	}
	if !strings.HasPrefix(stmt.Subject[0].Name, SubjectSchemeK8sLive) {
		t.Errorf("first subject not k8s-live: %s", stmt.Subject[0].Name)
	}
}

func TestBuildReceipt_HappyPath_ConnectedWithUnit(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.ConfigHubUnitSlug = "payments-api"
		in.ConfigHubUnitRev = 42
		in.ConfigHubUnitCanonical = []byte(`{"kind":"Unit","spec":{"data":"..."}}`)
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}

	// Should have BOTH subjects now.
	if len(stmt.Subject) != 2 {
		t.Fatalf("expected 2 subjects; got %d (%v)", len(stmt.Subject), stmt.Subject)
	}
	if !strings.HasPrefix(stmt.Subject[1].Name, SubjectSchemeConfigHubUnit) {
		t.Errorf("second subject should be confighub-unit; got %s", stmt.Subject[1].Name)
	}

	// No OmissionConfigHubUnitSubject now.
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionConfigHubUnitSubject {
			t.Errorf("unexpected OmissionConfigHubUnitSubject when unit data was provided: %v", o)
		}
	}
}

func TestBuildReceipt_StandaloneMode_OmitsConfigHubSubject(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.Connected = false
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}

	if len(stmt.Subject) != 1 {
		t.Errorf("standalone mode should emit only the k8s-live subject; got %d subjects", len(stmt.Subject))
	}

	// Must have the OmissionConfigHubUnitSubject entry.
	found := false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionConfigHubUnitSubject {
			if !strings.Contains(o.Reason, "standalone") {
				t.Errorf("omission reason should mention standalone; got %q", o.Reason)
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("standalone mode must include OmissionConfigHubUnitSubject in omissions")
	}
}

func TestBuildReceipt_ManualEdit_VerdictBLOCK(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.Evidence.Attribution.Cause = CauseManualEdit
		in.Evidence.Attribution.ManagerHint = "kubectl-edit"
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Errorf("verdict: got %q, want BLOCK (manual-edit)", stmt.Predicate.Verdict)
	}
}

func TestBuildReceipt_UnknownCause_VerdictINCONCLUSIVE(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.Evidence.Attribution.Cause = CauseUnknown
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictINCONCLUSIVE {
		t.Errorf("verdict: got %q, want INCONCLUSIVE", stmt.Predicate.Verdict)
	}
	// Should carry a managedFields omission.
	found := false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionManagedFields {
			found = true
			break
		}
	}
	if !found {
		t.Error("unknown cause must produce a managedFields omission")
	}
}

func TestBuildReceipt_MissingGitSource_VerdictINCONCLUSIVE(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.Evidence.GitSource = nil
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictINCONCLUSIVE {
		t.Errorf("verdict: got %q, want INCONCLUSIVE (no git source)", stmt.Predicate.Verdict)
	}
}

func TestBuildReceipt_AnchorMismatch_VerdictBLOCK(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.Spec.Anchor.Revision = "different-sha"
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Errorf("verdict: got %q, want BLOCK (anchor mismatch)", stmt.Predicate.Verdict)
	}
}

func TestBuildReceipt_AutoDetectPredicate_OnArgoOwner(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.PredicateName = "" // trigger auto-detect
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	if stmt.Predicate.PredicateName != string(PredicateAppliedMatchesSpec) {
		t.Errorf("auto-detect should choose applied-matches-spec for Argo owner; got %q", stmt.Predicate.PredicateName)
	}
}

func TestBuildReceipt_AutoDetectPredicate_UnknownOwner(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.PredicateName = ""
		in.Owner = Ownership{Type: OwnerUnknown}
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictINCONCLUSIVE {
		t.Errorf("verdict: got %q, want INCONCLUSIVE (no auto-detected predicate)", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.PredicateName != "" {
		t.Errorf("predicateName should be empty when auto-detect failed; got %q", stmt.Predicate.PredicateName)
	}
	// Must carry the OmissionAutoDetectedPredicate.
	found := false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionAutoDetectedPredicate {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected OmissionAutoDetectedPredicate")
	}
}

func TestBuildReceipt_DefaultClaim_DerivedFromSpec(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.Claim = ""
	})
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	if !strings.Contains(stmt.Predicate.Claim, "applied matches spec at apps/prod/api") {
		t.Errorf("default claim should reference the spec path; got %q", stmt.Predicate.Claim)
	}
}

func TestBuildReceipt_NilLive_Errors(t *testing.T) {
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.Live = nil
	})
	_, err := BuildReceipt(in)
	if err == nil {
		t.Error("BuildReceipt with nil Live must error")
	}
}

func TestBuildReceipt_UnknownPredicate_Errors(t *testing.T) {
	// After #446 batch 2 implemented source-truth-pass and
	// no-manual-edits-since, the unknown-predicate path is reached only
	// for genuinely-unrecognized names. The error message must enumerate
	// the implemented predicates so the operator knows what's valid.
	in := makeReceiptInput(func(in *BuildReceiptInput) {
		in.PredicateName = PredicateName("not-a-real-predicate")
	})
	_, err := BuildReceipt(in)
	if err == nil {
		t.Fatal("BuildReceipt with unknown predicate must error")
	}
	if !strings.Contains(err.Error(), "unknown predicate") {
		t.Errorf("error must explain unknown-predicate; got %v", err)
	}
}

func TestBuildReceipt_FiltersMutatingNextSteps(t *testing.T) {
	// This indirect test: install a fake evaluator that produces a
	// mutating step, then verify it's filtered. Since we don't have a
	// public evaluator-registration API at v1, we test by examining the
	// receipt's NextSteps after build — they must NOT contain mutating
	// actionType. The hardcoded evaluator never produces mutating steps,
	// but the filter is defense-in-depth.
	in := makeReceiptInput()
	stmt, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	for _, s := range stmt.Predicate.NextSteps {
		if s.ActionType == "mutating" {
			t.Errorf("filter must drop mutating actionType; found %v", s)
		}
	}
}

func TestBuildReceipt_FingerprintReproducible(t *testing.T) {
	// Same input must produce same fingerprint.
	in := makeReceiptInput()
	first, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := BuildReceipt(in)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.Predicate.Fingerprint != second.Predicate.Fingerprint {
		t.Errorf("fingerprint reproducibility broken: %s vs %s",
			first.Predicate.Fingerprint, second.Predicate.Fingerprint)
	}
}

func receiptContainsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
