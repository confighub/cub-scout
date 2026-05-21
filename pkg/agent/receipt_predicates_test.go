// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
)

// --- EvaluateAppliedMatchesSpec ---------------------------------------

func makeMatchedInput() PredicateInput {
	return PredicateInput{
		Scope: Scope{
			Kind:      "Deployment",
			Name:      "api",
			Namespace: "prod",
		},
		Spec: &SpecAnchor{
			Anchor: SpecAnchorBody{
				Type:     "git",
				RepoURL:  "https://github.com/org/repo",
				Revision: "abc123",
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
				Revision: "abc123",
				Path:     "apps/prod/api",
			},
		},
	}
}

func TestEvaluateAppliedMatchesSpec_NoSpec_INCONCLUSIVE(t *testing.T) {
	in := makeMatchedInput()
	in.Spec = nil
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict: got %q, want INCONCLUSIVE", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionGitSourceAnchor) {
		t.Errorf("missing-spec must produce OmissionGitSourceAnchor; got %v", res.Omissions)
	}
}

func TestEvaluateAppliedMatchesSpec_NoGitSource_INCONCLUSIVE(t *testing.T) {
	in := makeMatchedInput()
	in.Evidence.GitSource = nil
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("verdict: got %q, want INCONCLUSIVE", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionGitSourceAnchor) {
		t.Errorf("missing-git-source must produce OmissionGitSourceAnchor; got %v", res.Omissions)
	}
}

func TestEvaluateAppliedMatchesSpec_AnchorMismatch_BLOCK(t *testing.T) {
	in := makeMatchedInput()
	in.Spec.Anchor.Revision = "different-sha"
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictBLOCK {
		t.Fatalf("verdict: got %q, want BLOCK (anchor mismatch)", res.Verdict)
	}
	// Next step must explain divergence and not be mutating.
	if len(res.NextSteps) == 0 {
		t.Fatal("anchor mismatch must produce at least one next step explaining the divergence")
	}
	for _, s := range res.NextSteps {
		if s.ActionType != "read-only" {
			t.Errorf("anchor-mismatch next steps must be read-only; got %q", s.ActionType)
		}
	}
}

func TestEvaluateAppliedMatchesSpec_PathMismatch_BLOCK(t *testing.T) {
	in := makeMatchedInput()
	in.Spec.Anchor.Path = "apps/staging/api"
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictBLOCK {
		t.Fatalf("path mismatch must produce BLOCK; got %q", res.Verdict)
	}
}

func TestEvaluateAppliedMatchesSpec_PathLeadingSlash_NoFalseMismatch(t *testing.T) {
	in := makeMatchedInput()
	// Argo strips leading slash, Flux preserves; pathsEquivalent must
	// treat both as equal.
	in.Spec.Anchor.Path = "/apps/prod/api"
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictPASS {
		t.Errorf("leading-slash tolerance: got %q, want PASS", res.Verdict)
	}
}

func TestEvaluateAppliedMatchesSpec_ManualEdit_BLOCK(t *testing.T) {
	in := makeMatchedInput()
	in.Evidence.Attribution.Cause = CauseManualEdit
	in.Evidence.Attribution.ManagerHint = "kubectl-edit"
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictBLOCK {
		t.Fatalf("verdict: got %q, want BLOCK (manual-edit)", res.Verdict)
	}
	// Manual-edit next step must guide toward read-only investigation.
	if len(res.NextSteps) == 0 || res.NextSteps[0].ActionType != "read-only" {
		t.Errorf("manual-edit next step must be read-only; got %+v", res.NextSteps)
	}
}

func TestEvaluateAppliedMatchesSpec_ControllerDrift_PASS(t *testing.T) {
	in := makeMatchedInput()
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictPASS {
		t.Fatalf("controller-drift + matched anchor must produce PASS; got %q", res.Verdict)
	}
}

func TestEvaluateAppliedMatchesSpec_UnknownCause_INCONCLUSIVE(t *testing.T) {
	in := makeMatchedInput()
	in.Evidence.Attribution.Cause = CauseUnknown
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("unknown cause must produce INCONCLUSIVE; got %q", res.Verdict)
	}
	if !hasOmission(res.Omissions, OmissionManagedFields) {
		t.Errorf("unknown cause must record OmissionManagedFields; got %v", res.Omissions)
	}
}

func TestEvaluateAppliedMatchesSpec_DefensiveUnknownCauseString_INCONCLUSIVE(t *testing.T) {
	// If a new cause string lands without an evaluator update, the
	// predicate must remain conservative: INCONCLUSIVE + omission.
	in := makeMatchedInput()
	in.Evidence.Attribution.Cause = "future-cause-not-yet-known"
	res := EvaluateAppliedMatchesSpec(in)
	if res.Verdict != VerdictINCONCLUSIVE {
		t.Fatalf("unrecognized cause must produce INCONCLUSIVE; got %q", res.Verdict)
	}
	// Reason must include the surface name so a future maintainer
	// debugging it knows what to look for.
	found := false
	for _, o := range res.Omissions {
		if o.Missing == OmissionManagedFields && strings.Contains(o.Reason, "future-cause-not-yet-known") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an omission that names the unrecognized cause; got %v", res.Omissions)
	}
}

// --- FilterNextSteps --------------------------------------------------

func TestFilterNextSteps_DropsMutatingActionType(t *testing.T) {
	in := []ReceiptNextStep{
		{ActionType: "read-only", Reason: "ok", NextCommand: "cub-scout explain"},
		{ActionType: "mutating", Reason: "bad", NextCommand: "kubectl get"},
	}
	out, om := FilterNextSteps(in)
	if len(out) != 1 {
		t.Fatalf("expected one surviving step; got %d", len(out))
	}
	if out[0].ActionType != "read-only" {
		t.Errorf("survivor must be the read-only step; got %+v", out[0])
	}
	if len(om) != 1 {
		t.Errorf("expected exactly one omission for the dropped step; got %d", len(om))
	}
}

func TestFilterNextSteps_DropsMutatingNextCommand(t *testing.T) {
	in := []ReceiptNextStep{
		{ActionType: "read-only", Reason: "ok", NextCommand: "kubectl get deploy api"},
		{ActionType: "read-only", Reason: "sneaky", NextCommand: "kubectl apply -f x.yaml"},
		{ActionType: "read-only", Reason: "delete", NextCommand: "kubectl delete deploy x"},
		{ActionType: "read-only", Reason: "argo sync", NextCommand: "argocd app sync foo"},
		{ActionType: "read-only", Reason: "edit", NextCommand: "kubectl edit deploy api"},
		{ActionType: "read-only", Reason: "patch", NextCommand: "kubectl patch deploy api ..."},
		{ActionType: "read-only", Reason: "create", NextCommand: "kubectl create deploy x"},
		{ActionType: "read-only", Reason: "update", NextCommand: "kubectl update deploy x"},
		{ActionType: "read-only", Reason: "replace", NextCommand: "kubectl replace deploy x"},
		{ActionType: "read-only", Reason: "scale", NextCommand: "kubectl scale deploy api"},
		{ActionType: "read-only", Reason: "rollout", NextCommand: "kubectl rollout restart deploy"},
		{ActionType: "read-only", Reason: "reconcile", NextCommand: "flux reconcile ks app"},
	}
	out, om := FilterNextSteps(in)
	if len(out) != 1 {
		t.Errorf("only the get-deploy step is non-mutating; got %d survivors:\n%+v", len(out), out)
	}
	if len(om) != 11 {
		t.Errorf("expected 11 omissions for dropped mutating commands; got %d", len(om))
	}
}

func TestFilterNextSteps_AllowsWaitingAndHumanDecision(t *testing.T) {
	in := []ReceiptNextStep{
		{ActionType: "waiting", Reason: "controller still reconciling", NextCommand: ""},
		{ActionType: "human-decision", Reason: "operator must choose to revert or accept", NextCommand: ""},
	}
	out, om := FilterNextSteps(in)
	if len(out) != 2 {
		t.Errorf("waiting + human-decision must survive; got %d", len(out))
	}
	if len(om) != 0 {
		t.Errorf("waiting + human-decision must not produce omissions; got %d", len(om))
	}
}

func TestFilterNextSteps_PreservesOrder(t *testing.T) {
	in := []ReceiptNextStep{
		{ActionType: "read-only", Reason: "a"},
		{ActionType: "waiting", Reason: "b"},
		{ActionType: "human-decision", Reason: "c"},
	}
	out, _ := FilterNextSteps(in)
	if len(out) != 3 || out[0].Reason != "a" || out[1].Reason != "b" || out[2].Reason != "c" {
		t.Errorf("FilterNextSteps must preserve input order; got %+v", out)
	}
}

func TestFilterNextSteps_EmptyInput_EmptyOutput(t *testing.T) {
	out, om := FilterNextSteps(nil)
	if len(out) != 0 || len(om) != 0 {
		t.Errorf("nil input must produce empty output / no omissions; got %d steps, %d omissions", len(out), len(om))
	}
}

func TestFilterNextSteps_SyncedNotFalseFlagged(t *testing.T) {
	// "synced" must not be misread as "sync " (with trailing space).
	in := []ReceiptNextStep{
		{ActionType: "read-only", Reason: "argo synced status", NextCommand: "argocd app get foo (synced)"},
	}
	out, om := FilterNextSteps(in)
	if len(out) != 1 {
		t.Errorf("'synced' in nextCommand must not be flagged as mutating; got %d survivors", len(out))
	}
	if len(om) != 0 {
		t.Errorf("'synced' must not produce omissions; got %d", len(om))
	}
}

// --- AutoDetectPredicate ----------------------------------------------

func TestAutoDetectPredicate_Argo_AppliedMatchesSpec(t *testing.T) {
	name, om := AutoDetectPredicate(PredicateInput{}, Ownership{Type: OwnerArgo})
	if name != PredicateAppliedMatchesSpec {
		t.Errorf("Argo owner must auto-detect applied-matches-spec; got %q", name)
	}
	if om != nil {
		t.Errorf("Argo owner must not produce an omission; got %+v", om)
	}
}

func TestAutoDetectPredicate_Flux_AppliedMatchesSpec(t *testing.T) {
	name, om := AutoDetectPredicate(PredicateInput{}, Ownership{Type: OwnerFlux})
	if name != PredicateAppliedMatchesSpec {
		t.Errorf("Flux owner must auto-detect applied-matches-spec; got %q", name)
	}
	if om != nil {
		t.Errorf("Flux owner must not produce an omission; got %+v", om)
	}
}

func TestAutoDetectPredicate_ConfigHub_AppliedMatchesSpec(t *testing.T) {
	name, om := AutoDetectPredicate(PredicateInput{}, Ownership{Type: OwnerConfigHub})
	if name != PredicateAppliedMatchesSpec {
		t.Errorf("ConfigHub owner must auto-detect applied-matches-spec; got %q", name)
	}
	if om != nil {
		t.Errorf("ConfigHub owner must not produce an omission; got %+v", om)
	}
}

func TestAutoDetectPredicate_Helm_NoDefault(t *testing.T) {
	// Helm is owned but cub-scout doesn't auto-detect a predicate for
	// Helm at v1 — Helm receipts require an explicit --predicate.
	name, om := AutoDetectPredicate(PredicateInput{}, Ownership{Type: OwnerHelm})
	if name != "" {
		t.Errorf("Helm has no v1 default predicate; got %q", name)
	}
	if om == nil || om.Missing != OmissionAutoDetectedPredicate {
		t.Errorf("Helm must produce OmissionAutoDetectedPredicate; got %+v", om)
	}
}

func TestAutoDetectPredicate_Unknown_NoDefault(t *testing.T) {
	name, om := AutoDetectPredicate(PredicateInput{}, Ownership{Type: OwnerUnknown})
	if name != "" {
		t.Errorf("unknown owner must not auto-detect a predicate; got %q", name)
	}
	if om == nil || om.Missing != OmissionAutoDetectedPredicate {
		t.Errorf("unknown owner must produce OmissionAutoDetectedPredicate; got %+v", om)
	}
}

// --- helpers ----------------------------------------------------------

func hasOmission(omissions []Omission, missing string) bool {
	for _, o := range omissions {
		if o.Missing == missing {
			return true
		}
	}
	return false
}
