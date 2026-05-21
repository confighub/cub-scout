// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
)

// PredicateName is the canonical name of a receipt predicate.
type PredicateName string

const (
	// PredicateAppliedMatchesSpec verifies that the live resource matches
	// the desired-state anchor (git path:line / OCI digest / ConfigHub
	// unit revision). v1 predicate.
	PredicateAppliedMatchesSpec PredicateName = "applied-matches-spec"

	// PredicateSourceTruthPass verifies that `compare source-truth` with
	// an explicitly-specified strategy returned PASS. v1 predicate
	// (lands in #446 batch 2).
	PredicateSourceTruthPass PredicateName = "source-truth-pass"

	// PredicateNoManualEditsSince verifies that no `cause: manual-edit`
	// entries exist in managedFields after the given timestamp. v1
	// predicate (lands in #446 batch 2).
	PredicateNoManualEditsSince PredicateName = "no-manual-edits-since"
)

// AllPredicates returns the list of v1 predicate names cub-scout knows
// about. Adding a new v1 predicate extends this list AND requires an
// evaluator registration in receipt_predicates.go.
func AllPredicates() []PredicateName {
	return []PredicateName{
		PredicateAppliedMatchesSpec,
		PredicateSourceTruthPass,
		PredicateNoManualEditsSince,
	}
}

// PredicateResult is the structured output of a predicate evaluator.
// Receipts pass this through into the Predicate envelope; Omissions and
// NextSteps appended.
type PredicateResult struct {
	Verdict   ReceiptVerdict
	Omissions []Omission
	NextSteps []ReceiptNextStep
}

// PredicateInput is what an evaluator receives. Holds the evidence body
// plus the resource scope and optional spec anchor. Predicate evaluators
// are pure functions of this input — they do not read the cluster, fetch
// from ConfigHub, or have side effects.
type PredicateInput struct {
	Scope    Scope
	Evidence Evidence
	Spec     *SpecAnchor
	// Connected indicates whether the receipt is being built with
	// ConfigHub auth. Predicates use this to distinguish "connected-mode
	// evidence is genuinely missing" from "we never had it because
	// standalone mode".
	Connected bool
}

// EvaluateAppliedMatchesSpec runs the `applied-matches-spec` predicate.
//
// Semantics:
//   - If the spec anchor is missing → INCONCLUSIVE + omission
//     `git-source-anchor`. cub-scout cannot compare LIVE to a spec that
//     isn't declared.
//   - If attribution.GitSource is missing (no controller anchor
//     resolvable) → INCONCLUSIVE + omission `git-source-anchor`. The
//     standalone case where no Argo/Flux tracer can resolve a git path.
//   - If attribution.GitSource doesn't match the spec anchor (repoUrl
//     and revision must match exactly; path checked when both are
//     populated) → BLOCK.
//   - If attribution.Cause is `manual-edit` → BLOCK. Someone bypassed
//     the GitOps loop.
//   - If attribution.Cause is `controller-drift` → PASS. Controller
//     reconciling matches spec.
//   - If attribution.Cause is `unknown` → INCONCLUSIVE + omission
//     `managedFields`. `parse, don't guess`.
//   - Otherwise (no Attribution evidence available) → INCONCLUSIVE +
//     omission `managedFields`.
//
// Standalone-mode (Connected=false) inherits the same logic: a
// resolvable controller anchor is sufficient regardless of mode. The
// connected-mode-only check (DRY/WET/LIVE three-way) lives in the
// `no-drift` predicate (v2).
func EvaluateAppliedMatchesSpec(in PredicateInput) PredicateResult {
	res := PredicateResult{
		Omissions: []Omission{},
		NextSteps: []ReceiptNextStep{},
	}

	if in.Spec == nil {
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionGitSourceAnchor,
			Reason:   "no spec anchor provided to applied-matches-spec; pass --at-commit or rely on controller-resolved anchor",
			Severity: "info",
		})
		return res
	}

	if in.Evidence.Attribution == nil || in.Evidence.GitSource == nil {
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionGitSourceAnchor,
			Reason:   "no controller-resolved git anchor on the resource; applied-matches-spec PASS requires a resolvable spec anchor",
			Severity: "warning",
		})
		return res
	}

	gs := in.Evidence.GitSource
	spec := in.Spec.Anchor

	// Anchor comparison. RepoURL + Revision must match; Path is compared
	// only when both sides have it set (some Flux paths are empty by
	// design).
	if !equalIgnoringEmpty(gs.RepoURL, spec.RepoURL) ||
		!equalIgnoringEmpty(gs.Revision, spec.Revision) ||
		(spec.Path != "" && gs.Path != "" && !pathsEquivalent(gs.Path, spec.Path)) {
		res.Verdict = VerdictBLOCK
		res.NextSteps = append(res.NextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      fmt.Sprintf("controller is tracking %s@%s; spec anchor is %s@%s — investigate the divergence before any apply", gs.RepoURL, gs.Revision, spec.RepoURL, spec.Revision),
			NextCommand: "cub-scout trace",
			NextSurface: "cub-scout",
		})
		return res
	}

	// Attribution cause check. If attribution exists and is decisive,
	// honor it.
	cause := in.Evidence.Attribution.Cause
	switch cause {
	case CauseManualEdit:
		res.Verdict = VerdictBLOCK
		res.NextSteps = append(res.NextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "live state was last written by a kubectl-* tool; the GitOps loop was bypassed since the controller's last apply",
			NextCommand: "kubectl get --show-managed-fields",
			NextSurface: "kubectl",
		})
		return res
	case CauseControllerDrift:
		res.Verdict = VerdictPASS
		res.NextSteps = append(res.NextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "controller is reconciling; spec anchor matches the resolved git source",
			NextCommand: "cub-scout explain",
			NextSurface: "cub-scout",
		})
		return res
	case CauseUnknown, "":
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionManagedFields,
			Reason:   "attribution layer returned cause:unknown; managedFields may be empty, stripped, or contain only unrecognized managers",
			Severity: "warning",
		})
		return res
	default:
		// Defensive: unknown cause string. Treat as INCONCLUSIVE.
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionManagedFields,
			Reason:   fmt.Sprintf("unrecognized attribution cause %q; cub-scout enumerates controller-drift, manual-edit, unknown", string(cause)),
			Severity: "warning",
		})
		return res
	}
}

// equalIgnoringEmpty returns true if a and b are equal OR either is
// empty (meaning that side's evidence didn't carry the field; we
// can't compare).
func equalIgnoringEmpty(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	return a == b
}

// pathsEquivalent compares two git paths with leading-slash tolerance.
// Argo strips leading slashes; Flux preserves them; the canonical
// receipt path is whatever the controller actually reports.
func pathsEquivalent(a, b string) bool {
	a = strings.TrimPrefix(a, "/")
	b = strings.TrimPrefix(b, "/")
	a = strings.TrimSuffix(a, "/")
	b = strings.TrimSuffix(b, "/")
	return a == b
}

// AutoDetectPredicate picks the predicate to evaluate when the user
// didn't pass --predicate. The priority order is locked in the design
// (#446 round 2) so multiple-owner resources resolve deterministically.
//
// Priority:
//   1. Argo or Flux owner with a resolvable git anchor → applied-matches-spec
//   2. (Connected + --strategy provided → source-truth-pass) — v1 batch 2
//   3. (--since provided → no-manual-edits-since) — v1 batch 2
//   4. Otherwise → "" with an omission entry; the caller treats the
//      result as INCONCLUSIVE.
//
// Returns the chosen predicate name (or "" for no auto-detect) plus an
// omission entry to record when the auto-detect failed.
func AutoDetectPredicate(in PredicateInput, owner Ownership) (PredicateName, *Omission) {
	switch owner.Type {
	case OwnerArgo, OwnerFlux, OwnerConfigHub:
		// Controller-owned resources are the applied-matches-spec
		// target. The evaluator itself handles the case where the
		// anchor isn't resolvable (emits INCONCLUSIVE + omission).
		return PredicateAppliedMatchesSpec, nil
	default:
		return "", &Omission{
			Missing:  OmissionAutoDetectedPredicate,
			Reason:   fmt.Sprintf("no signal for default predicate (detected owner %q has no v1 default predicate); pass --predicate", owner.Type),
			Severity: "info",
		}
	}
}

// FilterNextSteps drops any nextSteps with mutating actionType or
// otherwise-disallowed nextCommand patterns. Returns the filtered slice
// plus a slice of omissions describing what was dropped.
//
// The receipt invariant (#410/#428): receipts emit artifacts, never
// mutate. nextSteps are advisory hints; they must not direct the
// consumer toward a mutating call. Defense in depth: predicate
// evaluators are not supposed to produce mutating steps in the first
// place, but this filter catches the case where one slips through.
func FilterNextSteps(steps []ReceiptNextStep) ([]ReceiptNextStep, []Omission) {
	allowedActionTypes := map[string]bool{
		"read-only":      true,
		"waiting":        true,
		"human-decision": true,
	}
	out := make([]ReceiptNextStep, 0, len(steps))
	omissions := []Omission{}
	for _, step := range steps {
		if !allowedActionTypes[step.ActionType] {
			omissions = append(omissions, Omission{
				Missing:  "next-step-allowed-action",
				Reason:   fmt.Sprintf("dropped nextStep with actionType %q (only read-only, waiting, human-decision allowed; mutating rejected at receipt-emit)", step.ActionType),
				Severity: "warning",
			})
			continue
		}
		if isMutatingCommand(step.NextCommand) {
			omissions = append(omissions, Omission{
				Missing:  "next-step-allowed-command",
				Reason:   fmt.Sprintf("dropped nextStep with mutating nextCommand %q (forbidden: apply / edit / patch / delete / sync / create / update)", step.NextCommand),
				Severity: "warning",
			})
			continue
		}
		out = append(out, step)
	}
	return out, omissions
}

// isMutatingCommand returns true if cmd contains any of the well-known
// mutating verbs. Pattern-matched on substrings because nextCommand may
// be a partial command, a flag-only string, or a full shell line.
func isMutatingCommand(cmd string) bool {
	if cmd == "" {
		return false
	}
	lower := strings.ToLower(cmd)
	mutatingFragments := []string{
		"apply",
		"edit",
		"patch",
		"delete",
		"sync ", // "argocd app sync" — guard the trailing space so "synced" is safe
		"create",
		"update",
		"replace",
		"scale",
		"rollout",
		"reconcile",
	}
	for _, frag := range mutatingFragments {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
}
