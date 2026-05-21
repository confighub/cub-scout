// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// --- EvaluateSourceTruthPass -----------------------------------------

func TestEvaluateSourceTruthPass_NoStrategy_INCONCLUSIVE(t *testing.T) {
	res := EvaluateSourceTruthPass(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
	})
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict: got %q, want INCONCLUSIVE", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionStrategyMissing) {
		t.Errorf("missing-strategy must produce OmissionStrategyMissing; got %v", res.Omissions)
	}
}

func TestEvaluateSourceTruthPass_NoEvidence_INCONCLUSIVE(t *testing.T) {
	res := EvaluateSourceTruthPass(PredicateInput{
		Scope:    Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Strategy: "git-argo",
	})
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict: got %q, want INCONCLUSIVE", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionSourceTruthEvidence) {
		t.Errorf("missing-evidence must produce OmissionSourceTruthEvidence; got %v", res.Omissions)
	}
}

func TestEvaluateSourceTruthPass_StrategyMismatch_BLOCK(t *testing.T) {
	res := EvaluateSourceTruthPass(PredicateInput{
		Scope:    Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Strategy: "git-argo",
		Evidence: Evidence{
			SourceTruth: &SourceTruthEvidence{
				DeclaredStrategy: StrategyConfigHubOCIArgo.Human(),
				Status:           StatusPASS,
			},
		},
	})
	if res.Verdict != VerdictBLOCK {
		t.Fatalf("strategy mismatch must produce BLOCK; got %q", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionStrategyMismatch) {
		t.Errorf("strategy mismatch must produce OmissionStrategyMismatch; got %v", res.Omissions)
	}
	if len(res.NextSteps) == 0 || res.NextSteps[0].ActionType != "read-only" {
		t.Errorf("strategy mismatch next step must be read-only; got %+v", res.NextSteps)
	}
}

func TestEvaluateSourceTruthPass_StrategyAgreesHumanForm_PASS(t *testing.T) {
	res := EvaluateSourceTruthPass(PredicateInput{
		Scope:    Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Strategy: "git-argo",
		Evidence: Evidence{
			SourceTruth: &SourceTruthEvidence{
				// Evidence stores the Human form; declared can be machine form.
				DeclaredStrategy: StrategyGitArgo.Human(),
				Status:           StatusPASS,
				SourceTruth:      VerdictAGREED,
			},
		},
	})
	if res.Verdict != VerdictPASS {
		t.Fatalf("PASS status must map to VerdictPASS; got %q", res.Verdict)
	}
}

func TestEvaluateSourceTruthPass_StatusWATCH_VerdictWATCH(t *testing.T) {
	res := EvaluateSourceTruthPass(PredicateInput{
		Scope:    Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Strategy: "git-argo",
		Evidence: Evidence{
			SourceTruth: &SourceTruthEvidence{
				DeclaredStrategy: StrategyGitArgo.Human(),
				Status:           StatusWATCH,
				ProofGaps:        []string{"controller.revision_or_digest"},
			},
		},
	})
	if res.Verdict != VerdictWATCH {
		t.Errorf("WATCH status must map to VerdictWATCH; got %q", res.Verdict)
	}
	// Proof gap should be mirrored as an omission.
	if !hasOmission(res.Omissions, OmissionSourceTruthComplete) {
		t.Errorf("WATCH must mirror proof_gaps into OmissionSourceTruthComplete; got %v", res.Omissions)
	}
}

func TestEvaluateSourceTruthPass_StatusBLOCK_VerdictBLOCK(t *testing.T) {
	res := EvaluateSourceTruthPass(PredicateInput{
		Scope:    Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Strategy: "git-argo",
		Evidence: Evidence{
			SourceTruth: &SourceTruthEvidence{
				DeclaredStrategy: StrategyGitArgo.Human(),
				Status:           StatusBLOCK,
				SourceTruth:      VerdictMISMATCH,
			},
		},
	})
	if res.Verdict != VerdictBLOCK {
		t.Errorf("BLOCK status must map to VerdictBLOCK; got %q", res.Verdict)
	}
}

func TestEvaluateSourceTruthPass_StatusASK_VerdictINCONCLUSIVE(t *testing.T) {
	res := EvaluateSourceTruthPass(PredicateInput{
		Scope:    Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Strategy: "git-argo",
		Evidence: Evidence{
			SourceTruth: &SourceTruthEvidence{
				DeclaredStrategy: StrategyGitArgo.Human(),
				Status:           StatusASK,
				SourceTruth:      VerdictUNKNOWN,
			},
		},
	})
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Errorf("ASK status must map to VerdictINCONCLUSIVE; got %q", res.Verdict)
	}
}

func TestEvaluateSourceTruthPass_ProofGapsMirroredOnPASS(t *testing.T) {
	res := EvaluateSourceTruthPass(PredicateInput{
		Scope:    Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Strategy: "git-argo",
		Evidence: Evidence{
			SourceTruth: &SourceTruthEvidence{
				DeclaredStrategy: StrategyGitArgo.Human(),
				Status:           StatusPASS,
				ProofGaps:        []string{"runtime.helm_chart_anchor"},
			},
		},
	})
	if res.Verdict != VerdictPASS {
		t.Fatalf("PASS status must map to VerdictPASS; got %q", res.Verdict)
	}
	// Even on PASS, proof_gaps must be visible — that's the omissions[]
	// contract.
	found := false
	for _, o := range res.Omissions {
		if o.Missing == OmissionSourceTruthComplete && strings.Contains(o.Reason, "runtime.helm_chart_anchor") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("PASS with proof_gaps must mirror them into omissions; got %v", res.Omissions)
	}
}

// --- EvaluateNoManualEditsSince --------------------------------------

func makeLiveWithManagedFields(entries []metav1.ManagedFieldsEntry) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
			},
		},
	}
	obj.SetManagedFields(entries)
	return obj
}

func TestEvaluateNoManualEditsSince_NoSince_INCONCLUSIVE(t *testing.T) {
	res := EvaluateNoManualEditsSince(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
	})
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict: got %q, want INCONCLUSIVE", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionSinceMissing) {
		t.Errorf("missing-since must produce OmissionSinceMissing; got %v", res.Omissions)
	}
}

func TestEvaluateNoManualEditsSince_NoLive_INCONCLUSIVE(t *testing.T) {
	cutoff := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	res := EvaluateNoManualEditsSince(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Since: cutoff,
		Live:  nil,
	})
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict: got %q, want INCONCLUSIVE", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionManagedFields) {
		t.Errorf("nil-live must produce OmissionManagedFields; got %v", res.Omissions)
	}
}

func TestEvaluateNoManualEditsSince_NoManagedFields_INCONCLUSIVE(t *testing.T) {
	cutoff := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	res := EvaluateNoManualEditsSince(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Since: cutoff,
		Live:  makeLiveWithManagedFields(nil),
	})
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict: got %q, want INCONCLUSIVE", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionManagedFields) {
		t.Errorf("empty-managedFields must produce OmissionManagedFields; got %v", res.Omissions)
	}
}

func TestEvaluateNoManualEditsSince_OnlyController_PASS(t *testing.T) {
	cutoff := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	after := metav1.NewTime(time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC))
	res := EvaluateNoManualEditsSince(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Since: cutoff,
		Live: makeLiveWithManagedFields([]metav1.ManagedFieldsEntry{
			{Manager: ManagerArgoCD, Operation: metav1.ManagedFieldsOperationApply, Time: &after},
		}),
	})
	if res.Verdict != VerdictPASS {
		t.Errorf("only-controller writers after cutoff must produce PASS; got %q", res.Verdict)
	}
}

func TestEvaluateNoManualEditsSince_KubectlEditAfterCutoff_BLOCK(t *testing.T) {
	cutoff := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	after := metav1.NewTime(time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC))
	res := EvaluateNoManualEditsSince(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Since: cutoff,
		Live: makeLiveWithManagedFields([]metav1.ManagedFieldsEntry{
			{Manager: ManagerArgoCD, Operation: metav1.ManagedFieldsOperationApply, Time: &after},
			{Manager: ManagerKubectlEdit, Operation: metav1.ManagedFieldsOperationUpdate, Time: &after},
		}),
	})
	if res.Verdict != VerdictBLOCK {
		t.Errorf("kubectl-edit after cutoff must produce BLOCK; got %q", res.Verdict)
	}
	// The BLOCK message must include the violating manager name on the
	// nextStep reason so an operator can pinpoint the writer.
	if len(res.NextSteps) == 0 || !strings.Contains(res.NextSteps[0].Reason, ManagerKubectlEdit) {
		t.Errorf("BLOCK next step must name the violating manager; got %+v", res.NextSteps)
	}
}

func TestEvaluateNoManualEditsSince_KubectlEditBeforeCutoff_PASS(t *testing.T) {
	cutoff := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	before := metav1.NewTime(time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC))
	res := EvaluateNoManualEditsSince(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Since: cutoff,
		Live: makeLiveWithManagedFields([]metav1.ManagedFieldsEntry{
			{Manager: ManagerKubectlEdit, Operation: metav1.ManagedFieldsOperationUpdate, Time: &before},
		}),
	})
	if res.Verdict != VerdictPASS {
		t.Errorf("kubectl-edit before cutoff must produce PASS; got %q", res.Verdict)
	}
}

func TestEvaluateNoManualEditsSince_KubectlNilTime_INCONCLUSIVE(t *testing.T) {
	cutoff := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	res := EvaluateNoManualEditsSince(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Since: cutoff,
		Live: makeLiveWithManagedFields([]metav1.ManagedFieldsEntry{
			{Manager: ManagerKubectlEdit, Operation: metav1.ManagedFieldsOperationUpdate, Time: nil},
		}),
	})
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Errorf("nil Time on interactive writer must produce INCONCLUSIVE; got %q", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionManagedFieldsTime) {
		t.Errorf("nil Time must produce OmissionManagedFieldsTime; got %v", res.Omissions)
	}
}

func TestEvaluateNoManualEditsSince_ControllerNilTime_StillPASS(t *testing.T) {
	// A nil-Time controller entry is fine — we don't claim anything about
	// controller writers, only about interactive ones. PASS as long as
	// no interactive writer is present.
	cutoff := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)
	res := EvaluateNoManualEditsSince(PredicateInput{
		Scope: Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Since: cutoff,
		Live: makeLiveWithManagedFields([]metav1.ManagedFieldsEntry{
			{Manager: ManagerArgoCD, Operation: metav1.ManagedFieldsOperationApply, Time: nil},
		}),
	})
	if res.Verdict != VerdictPASS {
		t.Errorf("controller-only with nil Time must still PASS; got %q", res.Verdict)
	}
}

// --- AutoDetectPredicate (extended for batch 2 signals) --------------

func TestAutoDetectPredicate_StrategyOnly_PicksSourceTruthPass(t *testing.T) {
	name, om := AutoDetectPredicate(PredicateInput{
		Strategy: "git-argo",
	}, Ownership{Type: OwnerUnknown})
	if name != PredicateSourceTruthPass {
		t.Errorf("--strategy must auto-detect source-truth-pass; got %q", name)
	}
	if om != nil {
		t.Errorf("--strategy must not produce an omission; got %+v", om)
	}
}

func TestAutoDetectPredicate_SinceOnly_PicksNoManualEditsSince(t *testing.T) {
	name, om := AutoDetectPredicate(PredicateInput{
		Since: time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
	}, Ownership{Type: OwnerUnknown})
	if name != PredicateNoManualEditsSince {
		t.Errorf("--since must auto-detect no-manual-edits-since; got %q", name)
	}
	if om != nil {
		t.Errorf("--since must not produce an omission; got %+v", om)
	}
}

func TestAutoDetectPredicate_ArgoAnchorBeatsStrategy(t *testing.T) {
	// Priority order is locked: Argo + resolvable anchor wins over
	// --strategy because applied-matches-spec is the more specific claim.
	name, _ := AutoDetectPredicate(PredicateInput{
		Strategy: "git-argo",
		Evidence: Evidence{
			GitSource: &GitSourceAnchor{
				RepoURL:  "https://github.com/org/repo",
				Revision: "abc",
				Path:     "apps/api",
			},
		},
	}, Ownership{Type: OwnerArgo})
	if name != PredicateAppliedMatchesSpec {
		t.Errorf("Argo + anchor must beat --strategy in auto-detect; got %q", name)
	}
}

func TestAutoDetectPredicate_StrategyBeatsSince(t *testing.T) {
	// When both signals are present, --strategy takes precedence per the
	// locked priority order.
	name, _ := AutoDetectPredicate(PredicateInput{
		Strategy: "git-argo",
		Since:    time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC),
	}, Ownership{Type: OwnerUnknown})
	if name != PredicateSourceTruthPass {
		t.Errorf("--strategy must beat --since in auto-detect; got %q", name)
	}
}
