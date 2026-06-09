// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// BuildReceiptInput is the orchestration input for BuildReceipt. The CLI
// command (cub-scout receipt verify) constructs this from cluster reads,
// optional ConfigHub reads, and the existing compare three-way helpers,
// then calls BuildReceipt to produce the final Statement.
type BuildReceiptInput struct {
	// Live is the K8s object cub-scout observed. Required.
	Live *unstructured.Unstructured

	// Scope is the resource identity (kind/name/namespace/cluster).
	// Required. The CLI fills this from the positional arg.
	Scope Scope

	// Owner is the detected ownership (from DetectOwnership). Used by
	// AutoDetectPredicate when PredicateName is empty.
	Owner Ownership

	// PredicateName names the predicate to evaluate. Empty triggers
	// AutoDetectPredicate; empty-after-auto-detect produces an
	// INCONCLUSIVE receipt with an omission.
	PredicateName PredicateName

	// Spec is the desired-state anchor. Required for applied-matches-
	// spec when an explicit anchor is needed; nil otherwise (the
	// evaluator falls back to the controller-resolved anchor in
	// Evidence.GitSource).
	Spec *SpecAnchor

	// Evidence is the structured evidence body. CompareThreeWay is
	// opaque; Attribution, SourceTruth, and GitSource are typed.
	Evidence Evidence

	// Claim is the human-readable assertion. If empty, BuildReceipt
	// derives a sensible default from PredicateName + Scope.
	Claim string

	// Connected indicates whether the build is running with ConfigHub
	// auth. Used to distinguish "connected-mode evidence is missing
	// because we don't have it" from "we never tried because we're
	// standalone".
	Connected bool

	// ConfigHubUnitSlug / UnitRevision / UnitCanonicalJSON populate
	// the optional confighub-unit:// subject. Required when Connected
	// is true and the resource is ConfigHub-linked; absence in that
	// case produces an OmissionConfigHubUnitSubject entry.
	ConfigHubUnitSlug      string
	ConfigHubUnitRev       int
	ConfigHubUnitCanonical []byte

	// Verifier identifies the tool building the receipt. The CLI
	// fills this with "cub-scout" + the current build version.
	Verifier Verifier

	// VerifiedAt is the timestamp. If zero, BuildReceipt uses
	// time.Now().UTC().
	VerifiedAt time.Time

	// Strategy is the source-truth strategy the caller declared (via
	// --strategy). Required for source-truth-pass; signals auto-detect
	// to pick source-truth-pass when no Argo/Flux/ConfigHub anchor
	// resolved. cub-scout never infers a strategy.
	Strategy string

	// Since is the cutoff timestamp for no-manual-edits-since (via
	// --since). Zero means "not provided"; the predicate then declines
	// with OmissionSinceMissing.
	Since time.Time

	// InputAttestations references prior receipts by digest, forming a
	// chain or DAG of evidence artifacts (#448). v1 always emitted [];
	// v2 lets callers attach explicit references via --input-attestation
	// (the CLI path) or programmatically (the agent-API path).
	//
	// The field type is `[]VerifiedAttestationRef` rather than the raw
	// `[]AttestationRef` wire shape: external callers cannot construct a
	// VerifiedAttestationRef directly (the type's only field is
	// unexported), so they MUST go through BuildAttestationRef or
	// BuildAttestationRefsFromPaths — both of which verify the source
	// Statement's fingerprint before returning. This enforces the
	// chained-receipt trust property at the API boundary, not just on
	// the CLI path (#463 / Codex round-6 P1 #463-B).
	//
	// Construction primitives live in receipt_inputattestations.go:
	// BuildAttestationRef (single Statement → VerifiedAttestationRef) and
	// BuildAttestationRefsFromPaths (file paths → loaded statements →
	// verified refs).
	//
	// BuildReceipt always emits an explicit JSON array — empty when no
	// refs are attached, non-empty when they are. The full Statement's
	// canonical JSON (and therefore its fingerprint) covers this field
	// by construction.
	InputAttestations []VerifiedAttestationRef
}

// BuildReceipt orchestrates: build subjects → resolve predicate →
// evaluate predicate → assemble Statement → stamp fingerprint. Returns
// the finished Statement.
//
// BuildReceipt is pure (no cluster reads, no network) — the caller is
// responsible for fetching cluster state, ConfigHub state, and the
// evidence body before invoking. This keeps the function testable
// without a real cluster and locks the read-only invariant: BuildReceipt
// cannot read or write anything.
func BuildReceipt(in BuildReceiptInput) (Statement, error) {
	if in.Live == nil {
		return Statement{}, fmt.Errorf("build-receipt: nil Live object")
	}

	// 1. Build subjects.
	subjects := []Subject{}
	omissions := []Omission{}

	liveSubject, err := BuildK8sLiveSubject(in.Live)
	if err != nil {
		return Statement{}, fmt.Errorf("build-receipt: k8s-live subject: %w", err)
	}
	subjects = append(subjects, liveSubject)

	// Connected-mode: include the confighub-unit:// subject when we
	// have the unit data. Standalone-mode omits with an explicit entry.
	if in.Connected && in.ConfigHubUnitSlug != "" && in.ConfigHubUnitRev > 0 && len(in.ConfigHubUnitCanonical) > 0 {
		unitSubject, err := BuildConfigHubUnitSubject(in.ConfigHubUnitSlug, in.ConfigHubUnitRev, in.ConfigHubUnitCanonical)
		if err != nil {
			return Statement{}, fmt.Errorf("build-receipt: confighub-unit subject: %w", err)
		}
		subjects = append(subjects, unitSubject)
	} else if !in.Connected {
		omissions = append(omissions, Omission{
			Missing:  OmissionConfigHubUnitSubject,
			Reason:   "standalone-mode build; no ConfigHub auth available to fetch unit subject",
			Severity: "info",
		})
	} else {
		// Connected but unit data missing. Could be a non-ConfigHub-
		// managed resource, or a connected-mode lookup that failed.
		omissions = append(omissions, Omission{
			Missing:  OmissionConfigHubUnitSubject,
			Reason:   "connected mode but no ConfigHub unit linkage found for this resource",
			Severity: "info",
		})
	}

	// 2. Resolve the predicate. Caller-provided wins; otherwise auto-
	// detect from owner + signals (--strategy, --since). Empty after
	// auto-detect → INCONCLUSIVE.
	predicateInput := PredicateInput{
		Scope:     in.Scope,
		Evidence:  in.Evidence,
		Spec:      in.Spec,
		Connected: in.Connected,
		Strategy:  in.Strategy,
		Since:     in.Since,
		Live:      in.Live,
	}
	predName := in.PredicateName
	if predName == "" {
		detected, detectedOmission := AutoDetectPredicate(predicateInput, in.Owner)
		predName = detected
		if detectedOmission != nil {
			omissions = append(omissions, *detectedOmission)
		}
	}

	// 3. Evaluate the predicate.
	var result PredicateResult
	switch predName {
	case PredicateAppliedMatchesSpec:
		result = EvaluateAppliedMatchesSpec(predicateInput)
	case PredicateSourceTruthPass:
		result = EvaluateSourceTruthPass(predicateInput)
	case PredicateNoManualEditsSince:
		result = EvaluateNoManualEditsSince(predicateInput)
	case PredicateObjectSetMatches:
		return Statement{}, fmt.Errorf("build-receipt: predicate %q requires object-set evidence; use BuildObjectSetReceipt or `cub-scout receipt verify --file <manifests>`", predName)
	case PredicateWorkloadsConverged:
		return Statement{}, fmt.Errorf("build-receipt: predicate %q requires workload evidence; use BuildWorkloadsConvergedReceipt or `cub-scout receipt verify --file <manifests> --predicate workloads-converged`", predName)
	case PredicatePrerequisitesMet:
		return Statement{}, fmt.Errorf("build-receipt: predicate %q requires declared prerequisites; use BuildPrerequisitesReceipt or `cub-scout receipt verify --prerequisites <file>`", predName)
	case "":
		// Auto-detect failed. The omission entry was already appended
		// above; the predicate evaluation produces INCONCLUSIVE.
		result = PredicateResult{
			Verdict:   VerdictINCONCLUSIVE,
			Omissions: []Omission{},
			NextSteps: []ReceiptNextStep{},
		}
	default:
		return Statement{}, fmt.Errorf("build-receipt: unknown predicate %q (cub-scout v1 implements %v)", predName, AllPredicates())
	}

	// 4. Filter nextSteps (defense in depth — predicate evaluators must
	// not produce mutating actionType, but we filter anyway).
	filteredSteps, filterOmissions := FilterNextSteps(result.NextSteps)
	omissions = append(omissions, result.Omissions...)
	omissions = append(omissions, filterOmissions...)

	// 5. Derive a default claim if the caller didn't provide one.
	claim := in.Claim
	if claim == "" {
		claim = deriveDefaultClaim(predName, in.Scope, in.Spec, in.Strategy, in.Since)
	}

	// 6. Set timestamp.
	verifiedAt := in.VerifiedAt
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}

	// 7. Build the predicate body.
	//
	// InputAttestations: unwrap each VerifiedAttestationRef into its
	// underlying AttestationRef for the wire shape. Reject zero-valued
	// wrappers at the API boundary — the only way to obtain a non-zero
	// VerifiedAttestationRef is via BuildAttestationRef (which verifies
	// the source Statement's fingerprint), so a zero value is a forgery
	// attempt and must not enter the chain. See #463 / Codex round-6
	// P1 #463-B.
	//
	// Always serialize as a JSON array, never null — the receipt
	// contract guarantees consumers can iterate the field without a
	// nil-check. v2 #448 wires non-empty values through this path.
	inputAttestations := make([]AttestationRef, 0, len(in.InputAttestations))
	for i, v := range in.InputAttestations {
		if v.IsZero() {
			return Statement{}, fmt.Errorf("build-receipt: inputAttestations[%d] is a zero-value VerifiedAttestationRef; construct via BuildAttestationRef or BuildAttestationRefsFromPaths so the source receipt's fingerprint is verified before chaining", i)
		}
		inputAttestations = append(inputAttestations, v.Ref())
	}
	pred := Predicate{
		Version:           PredicateVersion,
		Claim:             claim,
		Scope:             in.Scope,
		Verifier:          in.Verifier,
		VerifiedAt:        verifiedAt.UTC().Format(time.RFC3339),
		PredicateName:     string(predName),
		Spec:              in.Spec,
		Verdict:           result.Verdict,
		Evidence:          in.Evidence,
		Omissions:         omissions,
		InputAttestations: inputAttestations,
		NextSteps:         filteredSteps,
	}

	// 8. Build the Statement.
	stmt := Statement{
		Type:          StatementType,
		Subject:       subjects,
		PredicateType: PredicateTypeReceiptV1,
		Predicate:     pred,
	}

	// 9. Stamp the fingerprint.
	if err := StampFingerprint(&stmt); err != nil {
		return Statement{}, fmt.Errorf("build-receipt: stamp fingerprint: %w", err)
	}

	return stmt, nil
}

// deriveDefaultClaim builds a human-readable claim from the predicate
// and scope when the caller doesn't provide one.
func deriveDefaultClaim(name PredicateName, scope Scope, spec *SpecAnchor, strategy string, since time.Time) string {
	switch name {
	case PredicateAppliedMatchesSpec:
		if spec != nil && spec.Anchor.Path != "" {
			return fmt.Sprintf("applied matches spec at %s", spec.Anchor.Path)
		}
		return fmt.Sprintf("applied matches spec for %s/%s in %s", scope.Kind, scope.Name, scope.Namespace)
	case PredicateSourceTruthPass:
		if strings.TrimSpace(strategy) != "" {
			return fmt.Sprintf("source-truth PASS under strategy %q for %s/%s in %s", strategy, scope.Kind, scope.Name, scope.Namespace)
		}
		return fmt.Sprintf("source-truth PASS for %s/%s in %s", scope.Kind, scope.Name, scope.Namespace)
	case PredicateNoManualEditsSince:
		if !since.IsZero() {
			return fmt.Sprintf("no manual edits to %s/%s in %s since %s", scope.Kind, scope.Name, scope.Namespace, since.UTC().Format(time.RFC3339))
		}
		return fmt.Sprintf("no manual edits to %s/%s in %s", scope.Kind, scope.Name, scope.Namespace)
	case PredicateObjectSetMatches:
		if scope.Namespace != "" {
			return fmt.Sprintf("live object set matches rendered manifests in %s", scope.Namespace)
		}
		return "live object set matches rendered manifests"
	case "":
		return fmt.Sprintf("no predicate could be auto-detected for %s/%s in %s", scope.Kind, scope.Name, scope.Namespace)
	default:
		return fmt.Sprintf("%s for %s/%s in %s", name, scope.Kind, scope.Name, scope.Namespace)
	}
}
