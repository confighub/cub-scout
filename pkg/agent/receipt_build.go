// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
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
	ConfigHubUnitSlug    string
	ConfigHubUnitRev     int
	ConfigHubUnitCanonical []byte

	// Verifier identifies the tool building the receipt. The CLI
	// fills this with "cub-scout" + the current build version.
	Verifier Verifier

	// VerifiedAt is the timestamp. If zero, BuildReceipt uses
	// time.Now().UTC().
	VerifiedAt time.Time
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
	// detect from owner. Empty after auto-detect → INCONCLUSIVE.
	predName := in.PredicateName
	if predName == "" {
		detected, detectedOmission := AutoDetectPredicate(
			PredicateInput{Scope: in.Scope, Evidence: in.Evidence, Spec: in.Spec, Connected: in.Connected},
			in.Owner,
		)
		predName = detected
		if detectedOmission != nil {
			omissions = append(omissions, *detectedOmission)
		}
	}

	// 3. Evaluate the predicate.
	var result PredicateResult
	switch predName {
	case PredicateAppliedMatchesSpec:
		result = EvaluateAppliedMatchesSpec(PredicateInput{
			Scope:     in.Scope,
			Evidence:  in.Evidence,
			Spec:      in.Spec,
			Connected: in.Connected,
		})
	case "":
		// Auto-detect failed. The omission entry was already appended
		// above; the predicate evaluation produces INCONCLUSIVE.
		result = PredicateResult{
			Verdict:   VerdictINCONCLUSIVE,
			Omissions: []Omission{},
			NextSteps: []ReceiptNextStep{},
		}
	default:
		return Statement{}, fmt.Errorf("build-receipt: predicate %q not implemented in v1 batch 1 (planned in batch 2)", predName)
	}

	// 4. Filter nextSteps (defense in depth — predicate evaluators must
	// not produce mutating actionType, but we filter anyway).
	filteredSteps, filterOmissions := FilterNextSteps(result.NextSteps)
	omissions = append(omissions, result.Omissions...)
	omissions = append(omissions, filterOmissions...)

	// 5. Derive a default claim if the caller didn't provide one.
	claim := in.Claim
	if claim == "" {
		claim = deriveDefaultClaim(predName, in.Scope, in.Spec)
	}

	// 6. Set timestamp.
	verifiedAt := in.VerifiedAt
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}

	// 7. Build the predicate body.
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
		InputAttestations: []AttestationRef{}, // v1 emits []
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
func deriveDefaultClaim(name PredicateName, scope Scope, spec *SpecAnchor) string {
	switch name {
	case PredicateAppliedMatchesSpec:
		if spec != nil && spec.Anchor.Path != "" {
			return fmt.Sprintf("applied matches spec at %s", spec.Anchor.Path)
		}
		return fmt.Sprintf("applied matches spec for %s/%s in %s", scope.Kind, scope.Name, scope.Namespace)
	case "":
		return fmt.Sprintf("no predicate could be auto-detected for %s/%s in %s", scope.Kind, scope.Name, scope.Namespace)
	default:
		return fmt.Sprintf("%s for %s/%s in %s", name, scope.Kind, scope.Name, scope.Namespace)
	}
}
