// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	// PredicateObjectSetMatches verifies that the live cluster contains
	// the rendered manifest object set and that every authored field in
	// that set still matches live. Kubernetes server defaults and
	// controller-populated status are deliberately outside this claim.
	PredicateObjectSetMatches PredicateName = "object-set-matches"

	// PredicateWorkloadsConverged verifies that every desired workload in
	// the rendered manifest set reached a ready/converged runtime state
	// (Deployments/StatefulSets/DaemonSets rolled out, Jobs Succeeded,
	// PVCs Bound, pods not wedged in CreateContainerConfigError /
	// CrashLoopBackOff / ImagePullBackOff). Unlike object-set-matches,
	// this predicate reads status. It is --file-routed and evaluated by
	// BuildWorkloadsConvergedReceipt, not the BuildReceipt switch.
	PredicateWorkloadsConverged PredicateName = "workloads-converged"

	// PredicatePrerequisitesMet verifies that a declared set of required
	// cluster-state facts (CRDs established, Secrets present with the named
	// keys, StorageClasses / IngressClasses / namespaces existing) are
	// present live before an install is applied. cub-scout consumes the
	// declared list (the "target facts"); it does not infer prerequisites
	// from a chart. --prerequisites-routed; evaluated by
	// BuildPrerequisitesReceipt, not the BuildReceipt switch.
	PredicatePrerequisitesMet PredicateName = "prerequisites-met"
)

// AllPredicates returns the list of v1 predicate names cub-scout knows
// about. Adding a new v1 predicate extends this list AND requires an
// evaluator registration in receipt_predicates.go.
func AllPredicates() []PredicateName {
	return []PredicateName{
		PredicateAppliedMatchesSpec,
		PredicateSourceTruthPass,
		PredicateNoManualEditsSince,
		PredicateObjectSetMatches,
		PredicateWorkloadsConverged,
		PredicatePrerequisitesMet,
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

	// Strategy is the explicit source-truth strategy the caller passed
	// (via --strategy). Required for source-truth-pass; cub-scout never
	// infers a strategy. Empty means "not provided".
	Strategy string

	// Since is the cutoff timestamp for the no-manual-edits-since
	// predicate. A zero value means "not provided" — the predicate then
	// emits INCONCLUSIVE + OmissionSinceMissing.
	Since time.Time

	// Live carries the resource the no-manual-edits-since predicate
	// walks for managedFields entries. Other predicates ignore this
	// field. Receipts are built from the same Live the orchestrator
	// already fetched; passing it down avoids a re-read.
	Live *unstructured.Unstructured
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
//  1. Argo/Flux/ConfigHub owner WITH a resolvable git anchor →
//     applied-matches-spec. The "resolvable" condition checks that
//     in.Evidence.GitSource is non-nil so we don't speculate a default
//     predicate we know can't run.
//  2. (Connected + --strategy provided → source-truth-pass) — v1 batch 2
//  3. (--since provided → no-manual-edits-since) — v1 batch 2
//  4. Otherwise → "" with an OmissionAutoDetectedPredicate entry.
//
// Codex #446 round-4 fix: the previous version returned applied-matches-
// spec for any Argo/Flux/ConfigHub owner without checking whether the
// anchor was actually resolvable. That made the predicate evaluator
// emit OmissionGitSourceAnchor rather than the documented
// OmissionAutoDetectedPredicate when the auto-detect should have
// declined to pick a default at all. The conditioned version below
// matches the locked priority order's omission semantics.
//
// Returns the chosen predicate name (or "" for no auto-detect) plus an
// omission entry to record when the auto-detect failed.
func AutoDetectPredicate(in PredicateInput, owner Ownership) (PredicateName, *Omission) {
	// Priority 1: Argo/Flux/ConfigHub + resolvable anchor → applied-matches-spec.
	switch owner.Type {
	case OwnerArgo, OwnerFlux, OwnerConfigHub:
		if in.Evidence.GitSource != nil {
			return PredicateAppliedMatchesSpec, nil
		}
	}

	// Priority 2: --strategy provided → source-truth-pass. The predicate
	// itself enforces the connected-mode gate; auto-detect only needs to
	// see the strategy signal.
	if strings.TrimSpace(in.Strategy) != "" {
		return PredicateSourceTruthPass, nil
	}

	// Priority 3: --since provided → no-manual-edits-since.
	if !in.Since.IsZero() {
		return PredicateNoManualEditsSince, nil
	}

	// Owner-specific decline reason takes precedence when the owner is one
	// of the controller types but no anchor resolved — that's a more
	// actionable hint than the generic "no signal" message.
	switch owner.Type {
	case OwnerArgo, OwnerFlux, OwnerConfigHub:
		return "", &Omission{
			Missing: OmissionAutoDetectedPredicate,
			Reason: fmt.Sprintf(
				"owner %q would map to applied-matches-spec but no git anchor was resolved for this resource; pass --predicate or wire up the tracer (argocd / flux CLI) so cub-scout can find the spec",
				owner.Type),
			Severity: "info",
		}
	default:
		return "", &Omission{
			Missing:  OmissionAutoDetectedPredicate,
			Reason:   fmt.Sprintf("no signal for default predicate (detected owner %q has no v1 default predicate); pass --predicate, --strategy, or --since", owner.Type),
			Severity: "info",
		}
	}
}

// EvaluateSourceTruthPass runs the `source-truth-pass` predicate.
//
// Semantics:
//   - No --strategy passed → INCONCLUSIVE + OmissionStrategyMissing.
//     cub-scout never infers a delivery strategy; that's the operator's
//     declared input (`compare source-truth` enforces the same rule).
//   - No source-truth evidence in the body → INCONCLUSIVE +
//     OmissionSourceTruthEvidence. The orchestrator should have run
//     `Derive` and attached the SourceTruthEvidence; if it didn't, the
//     receipt cannot claim anything about source truth.
//   - Strategy mismatch between caller and evidence → BLOCK +
//     OmissionStrategyMismatch. Don't silently honor the caller's strategy
//     over what the evidence claims to be under.
//   - Map Status to ReceiptVerdict (per the locked synthesis in
//     docs/proposals/receipts-way-forward.md):
//     PASS  → VerdictPASS
//     WATCH → VerdictWATCH
//     BLOCK → VerdictBLOCK
//     ASK   → VerdictWATCH (source-truth ASK means cub-scout couldn't
//     classify; the receipt records WATCH with the evidence's
//     proof_gaps mirrored into omissions[] so the consumer
//     knows what's missing. INCONCLUSIVE is reserved for
//     cases where the receipt itself can't be evaluated, not
//     for cases where the underlying source-truth surface
//     just couldn't classify.)
//
// Proof gaps in the source-truth evidence are mirrored into the receipt's
// omissions[] under OmissionSourceTruthComplete so a consumer reading
// only the receipt knows what the underlying source-truth derivation
// flagged.
func EvaluateSourceTruthPass(in PredicateInput) PredicateResult {
	res := PredicateResult{
		Omissions: []Omission{},
		NextSteps: []ReceiptNextStep{},
	}

	declared := strings.TrimSpace(in.Strategy)
	if declared == "" {
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionStrategyMissing,
			Reason:   "source-truth-pass requires --strategy; cub-scout does not infer the delivery strategy",
			Severity: "warning",
		})
		return res
	}

	if in.Evidence.SourceTruth == nil {
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionSourceTruthEvidence,
			Reason:   fmt.Sprintf("no source-truth evidence body attached; run `cub-scout compare source-truth --strategy %s` and feed the result back into the receipt orchestrator", declared),
			Severity: "warning",
		})
		return res
	}

	// Compare declared strategy with the strategy already captured in the
	// evidence body. The evidence stores the Human-rendered form
	// ("Git -> Argo -> Kubernetes"), so we normalize through ParseStrategy
	// when comparing — accept either the machine name ("git-argo") or the
	// Human form.
	evStrategy := in.Evidence.SourceTruth.DeclaredStrategy
	if !strategyAgrees(declared, evStrategy) {
		res.Verdict = VerdictBLOCK
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionStrategyMismatch,
			Reason:   fmt.Sprintf("receipt was asked to verify under strategy %q but source-truth evidence carries %q", declared, evStrategy),
			Severity: "warning",
		})
		res.NextSteps = append(res.NextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "rerun the source-truth derivation with the same strategy you want the receipt to claim; do not paper over the disagreement",
			NextCommand: "cub-scout compare source-truth --strategy " + declared,
			NextSurface: "cub-scout",
		})
		return res
	}

	// Mirror proof gaps into omissions[] regardless of status.
	for _, gap := range in.Evidence.SourceTruth.ProofGaps {
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionSourceTruthComplete,
			Reason:   "source-truth proof gap: " + gap,
			Severity: "info",
		})
	}

	switch in.Evidence.SourceTruth.Status {
	case StatusPASS:
		res.Verdict = VerdictPASS
		res.NextSteps = append(res.NextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "source-truth PASS under the declared strategy; all three surfaces agree under the strategy-implied authority",
			NextCommand: "cub-scout explain " + in.Scope.Kind + "/" + in.Scope.Name + " -n " + in.Scope.Namespace,
			NextSurface: "cub-scout",
		})
	case StatusWATCH:
		res.Verdict = VerdictWATCH
		res.NextSteps = append(res.NextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "source-truth WATCH; soft proof gap or signal warrants confirmation before acceptance",
			NextCommand: "cub-scout compare source-truth --strategy " + declared,
			NextSurface: "cub-scout",
		})
	case StatusBLOCK:
		res.Verdict = VerdictBLOCK
		res.NextSteps = append(res.NextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "source-truth BLOCK; a hard rule failed (strategy mismatch, controller-source resolution failure, etc.) — investigate before acceptance",
			NextCommand: "cub-scout compare source-truth --strategy " + declared,
			NextSurface: "cub-scout",
		})
	case StatusASK:
		// Per docs/proposals/receipts-way-forward.md: source-truth ASK
		// maps to WATCH (not INCONCLUSIVE). The receipt CAN be evaluated
		// — the underlying source-truth surface just couldn't classify
		// deterministically. WATCH says "soft signal; the consumer must
		// look at the proof_gaps[] to decide". INCONCLUSIVE is reserved
		// for receipts that themselves can't be built.
		res.Verdict = VerdictWATCH
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionSourceTruthComplete,
			Reason:   "source-truth ASK; cub-scout cannot classify deterministically — the evidence's proof_gaps explain what's missing",
			Severity: "warning",
		})
	default:
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionSourceTruthComplete,
			Reason:   fmt.Sprintf("unrecognized source-truth status %q; cub-scout enumerates PASS / WATCH / BLOCK / ASK", string(in.Evidence.SourceTruth.Status)),
			Severity: "warning",
		})
	}
	return res
}

// strategyAgrees returns true if declared (caller's CLI form like
// "git-argo") names the same strategy as evidence (the Human form like
// "Git -> Argo -> Kubernetes" recorded by Derive). Either side may be
// empty after trim.
func strategyAgrees(declared, evStrategy string) bool {
	declared = strings.TrimSpace(declared)
	evStrategy = strings.TrimSpace(evStrategy)
	if declared == "" || evStrategy == "" {
		return false
	}
	// Same machine form.
	if declared == evStrategy {
		return true
	}
	// Caller passed the machine form; evidence has the Human form.
	if parsed, ok := ParseStrategy(declared); ok {
		if parsed.Human() == evStrategy {
			return true
		}
	}
	return false
}

// EvaluateNoManualEditsSince runs the `no-manual-edits-since` predicate.
//
// Semantics:
//   - No --since cutoff → INCONCLUSIVE + OmissionSinceMissing.
//   - No live object on the input → INCONCLUSIVE + OmissionManagedFields
//     (defensive — orchestrator should always pass Live).
//   - No managedFields entries on the object → INCONCLUSIVE +
//     OmissionManagedFields. Old K8s, stripped admission webhook, etc.
//   - Any managedFields entry from an interactive manager (kubectl-*)
//     with Time > since → BLOCK.
//   - Any managedFields entry from an interactive manager with no Time
//     → INCONCLUSIVE + OmissionManagedFieldsTime. We cannot place it on
//     the timeline so we cannot honestly claim "no manual edits since T".
//   - Otherwise → PASS.
//
// Important caveat (from the receipts-way-forward.md design): K8s
// managedFields evidence is best-effort. If an admission webhook strips
// the entries, or the resource is migrated across K8s versions, the
// predicate degrades to INCONCLUSIVE rather than producing false PASS.
// `parse, don't guess`.
func EvaluateNoManualEditsSince(in PredicateInput) PredicateResult {
	res := PredicateResult{
		Omissions: []Omission{},
		NextSteps: []ReceiptNextStep{},
	}

	if in.Since.IsZero() {
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionSinceMissing,
			Reason:   "no-manual-edits-since requires --since <RFC3339 timestamp>; cub-scout does not invent a cutoff",
			Severity: "warning",
		})
		return res
	}

	if in.Live == nil {
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionManagedFields,
			Reason:   "no live object passed to no-manual-edits-since; cub-scout cannot read managedFields without the resource",
			Severity: "warning",
		})
		return res
	}

	entries := in.Live.GetManagedFields()
	if len(entries) == 0 {
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionManagedFields,
			Reason:   "live resource has no managedFields entries; no-manual-edits-since cannot establish a baseline without them",
			Severity: "warning",
		})
		return res
	}

	// Walk entries. Treat any interactive manager with Time > since as a
	// late manual edit (BLOCK). Treat any interactive manager with nil
	// Time as a timestamp gap (INCONCLUSIVE).
	var (
		violatingManagers []string
		nilTimeManagers   []string
	)
	for _, e := range entries {
		if !IsInteractiveManager(e.Manager) {
			continue
		}
		if e.Time == nil {
			nilTimeManagers = append(nilTimeManagers, e.Manager)
			continue
		}
		if e.Time.Time.After(in.Since) {
			violatingManagers = append(violatingManagers, fmt.Sprintf("%s@%s", e.Manager, e.Time.Time.UTC().Format(time.RFC3339)))
		}
	}

	if len(violatingManagers) > 0 {
		res.Verdict = VerdictBLOCK
		res.NextSteps = append(res.NextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      fmt.Sprintf("interactive writer wrote managedFields after the cutoff (%s): %s — investigate the unplanned edit before any apply", in.Since.UTC().Format(time.RFC3339), strings.Join(violatingManagers, ", ")),
			NextCommand: "kubectl get --show-managed-fields",
			NextSurface: "kubectl",
		})
		return res
	}

	if len(nilTimeManagers) > 0 {
		res.Verdict = VerdictINCONCLUSIVE
		res.Omissions = append(res.Omissions, Omission{
			Missing:  OmissionManagedFieldsTime,
			Reason:   fmt.Sprintf("interactive managedFields entries have nil Time; cannot place them on the timeline: %s", strings.Join(nilTimeManagers, ", ")),
			Severity: "warning",
		})
		return res
	}

	res.Verdict = VerdictPASS
	res.NextSteps = append(res.NextSteps, ReceiptNextStep{
		ActionType:  "read-only",
		Reason:      fmt.Sprintf("no interactive writer touched managedFields after %s", in.Since.UTC().Format(time.RFC3339)),
		NextCommand: "cub-scout explain " + in.Scope.Kind + "/" + in.Scope.Name + " -n " + in.Scope.Namespace,
		NextSurface: "cub-scout",
	})
	return res
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
//
// The list is expanded over the original (Codex #446 round-4 fix) to
// cover every common cluster-mutating verb a future evaluator could
// suggest. Read-only verbs (get, list, describe, watch, top, logs,
// events, version, config view, port-forward) are not on the list.
//
// Notes on tricky entries:
//   - "sync " has a trailing space so "synced" (a status word) doesn't
//     match.
//   - "set " has a trailing space so "settings" / "set-context" /
//     "setattr" don't false-positive. `kubectl set image / env / resources`
//     all match the leading "set ".
//   - "helm install" / "helm upgrade" are listed as fragments rather
//     than bare "install" / "upgrade" because the bare verbs collide
//     with informational phrases ("upgrade path"); the helm-prefixed
//     forms are the mutating ones.
//   - "argocd app sync" is caught by "sync ".
//   - "argocd cluster add" is caught by "cluster add".
//   - "flux create" / "flux delete" / "flux suspend" / "flux resume"
//     are caught by "create" / "delete" / "suspend " / "resume ".
//   - "exec" and "debug" can mutate via in-container side effects; the
//     receipt invariant rejects them defensively.
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
		"sync ",
		"create",
		"update",
		"replace",
		"scale",
		"rollout",
		"reconcile",
		"annotate",
		"label ",
		"set ",
		"exec",
		"debug",
		"helm install",
		"helm upgrade",
		"helm uninstall",
		"helm rollback",
		"cluster add",
		"suspend ",
		"resume ",
		"taint",
		"drain",
		"cordon",
		"uncordon",
		"evict",
		"port-forward", // strictly read-write — opens a writable channel
		"cp ",          // kubectl cp writes to / from the pod
	}
	for _, frag := range mutatingFragments {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
}
