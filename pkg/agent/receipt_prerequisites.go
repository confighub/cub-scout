// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"sort"
	"time"
)

// Prerequisite fact kinds.
const (
	PrerequisiteKindCRD          = "CRD"
	PrerequisiteKindSecret       = "Secret"
	PrerequisiteKindNamespace    = "Namespace"
	PrerequisiteKindStorageClass = "StorageClass"
	PrerequisiteKindIngressClass = "IngressClass"
)

// Prerequisite fact statuses.
const (
	PrerequisitePresent      = "present"
	PrerequisiteMissing      = "missing"
	PrerequisiteInconclusive = "inconclusive"
)

// PrerequisitesEvidence is attached under predicate.evidence.prerequisites
// for prerequisites-met receipts. It records, for each declared cluster-state
// dependency (a "target fact" in helm-expt terms — required CRDs, Secrets,
// StorageClasses, namespaces, IngressClasses), whether it is present live.
// This is the pre-flight complement to workloads-converged: it catches an
// unmet prerequisite (e.g. an absent Secret) before the workload tries to
// start (helm-expt finding F3, confighub/cub-scout#477).
type PrerequisitesEvidence struct {
	Source         ObjectSetSource          `json:"source"`
	Scope          ObjectSetScope           `json:"scope"`
	DeclaredDigest string                   `json:"declaredDigest"`
	LiveDigest     string                   `json:"liveDigest"`
	Summary        PrerequisitesSummary     `json:"summary"`
	Facts          []PrerequisiteFactResult `json:"facts"`
}

type PrerequisitesSummary struct {
	Required     int `json:"required"`
	Present      int `json:"present"`
	Missing      int `json:"missing"`
	Inconclusive int `json:"inconclusive"`
}

// PrerequisiteFactResult is the observed live state of one declared fact.
type PrerequisiteFactResult struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

func prerequisiteFactKey(f PrerequisiteFactResult) string {
	return f.Kind + "|" + f.Namespace + "|" + f.Name
}

// BuildPrerequisitesEvidence summarizes per-fact observations into the
// evidence body and computes the declared/live digests. It is pure: callers
// run the live checks before invoking.
func BuildPrerequisitesEvidence(source ObjectSetSource, scope ObjectSetScope, facts []PrerequisiteFactResult) (PrerequisitesEvidence, error) {
	if len(facts) == 0 {
		return PrerequisitesEvidence{}, fmt.Errorf("prerequisites receipt: no declared prerequisites")
	}

	sorted := append([]PrerequisiteFactResult(nil), facts...)
	sort.Slice(sorted, func(i, j int) bool { return prerequisiteFactKey(sorted[i]) < prerequisiteFactKey(sorted[j]) })

	declaredSet := make([]interface{}, 0, len(sorted))
	liveSet := make([]interface{}, 0, len(sorted))
	summary := PrerequisitesSummary{Required: len(sorted)}
	for _, f := range sorted {
		declaredSet = append(declaredSet, map[string]interface{}{
			"key": prerequisiteFactKey(f), "kind": f.Kind, "name": f.Name, "namespace": f.Namespace, "detail": f.Detail,
		})
		liveSet = append(liveSet, map[string]interface{}{
			"key": prerequisiteFactKey(f), "status": f.Status,
		})
		switch f.Status {
		case PrerequisitePresent:
			summary.Present++
		case PrerequisiteMissing:
			summary.Missing++
		default:
			summary.Inconclusive++
		}
	}

	declaredDigest, err := digestJSON(declaredSet)
	if err != nil {
		return PrerequisitesEvidence{}, fmt.Errorf("digest declared prerequisites: %w", err)
	}
	liveDigest, err := digestJSON(liveSet)
	if err != nil {
		return PrerequisitesEvidence{}, fmt.Errorf("digest live prerequisites: %w", err)
	}
	source.ObjectCount = summary.Required

	return PrerequisitesEvidence{
		Source:         source,
		Scope:          scope,
		DeclaredDigest: declaredDigest,
		LiveDigest:     liveDigest,
		Summary:        summary,
		Facts:          sorted,
	}, nil
}

// BuildPrerequisitesReceiptInput is the input to BuildPrerequisitesReceipt.
type BuildPrerequisitesReceiptInput struct {
	Evidence          PrerequisitesEvidence
	Verifier          Verifier
	VerifiedAt        time.Time
	InputAttestations []VerifiedAttestationRef
}

// BuildPrerequisitesReceipt builds a single receipt asserting that the
// declared cluster-state prerequisites are present live. It is pure.
//
// Verdict: BLOCK if any required fact is missing; else INCONCLUSIVE if any
// fact could not be checked; else PASS.
func BuildPrerequisitesReceipt(in BuildPrerequisitesReceiptInput) (Statement, error) {
	if in.Evidence.Summary.Required == 0 {
		return Statement{}, fmt.Errorf("prerequisites receipt: no declared prerequisites")
	}
	if in.Evidence.DeclaredDigest == "" {
		return Statement{}, fmt.Errorf("prerequisites receipt: missing declared digest")
	}
	if in.Evidence.LiveDigest == "" {
		return Statement{}, fmt.Errorf("prerequisites receipt: missing live digest")
	}

	verifiedAt := in.VerifiedAt
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}

	s := in.Evidence.Summary
	verdict := VerdictPASS
	switch {
	case s.Missing > 0:
		verdict = VerdictBLOCK
	case s.Inconclusive > 0:
		verdict = VerdictINCONCLUSIVE
	}

	omissions := []Omission{
		{
			Missing:  OmissionPrerequisitesSnapshot,
			Reason:   "prerequisites-met reflects the cluster facts observed at verifiedAt; it does not prove they remain present afterward",
			Severity: "info",
		},
	}
	if s.Inconclusive > 0 {
		omissions = append(omissions, Omission{
			Missing:  OmissionPrerequisitesCoverage,
			Reason:   fmt.Sprintf("%d declared prerequisite(s) could not be checked because the live read failed", s.Inconclusive),
			Severity: "warning",
		})
	}

	nextSteps := []ReceiptNextStep{}
	switch verdict {
	case VerdictPASS:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "every declared prerequisite is present in the live cluster",
			NextCommand: "cub-scout doctor",
			NextSurface: "cub-scout",
		})
	case VerdictBLOCK:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "human-decision",
			Reason:      "one or more required cluster facts (CRD / Secret / StorageClass / namespace) are missing; provide them before applying the install",
			NextCommand: "cub-scout map list",
			NextSurface: "cub-scout",
		})
	case VerdictINCONCLUSIVE:
		nextSteps = append(nextSteps, ReceiptNextStep{
			ActionType:  "read-only",
			Reason:      "one or more prerequisites could not be checked; verify API availability and permissions",
			NextCommand: "cub-scout doctor",
			NextSurface: "cub-scout",
		})
	}
	filteredSteps, stepOmissions := FilterNextSteps(nextSteps)
	omissions = append(omissions, stepOmissions...)

	inputAttestations := make([]AttestationRef, 0, len(in.InputAttestations))
	for i, v := range in.InputAttestations {
		if v.IsZero() {
			return Statement{}, fmt.Errorf("prerequisites receipt: inputAttestations[%d] is a zero-value VerifiedAttestationRef; construct via BuildAttestationRef or BuildAttestationRefsFromPaths", i)
		}
		inputAttestations = append(inputAttestations, v.Ref())
	}

	claim := "declared prerequisites are present in the live cluster"
	if in.Evidence.Source.Ref != "" {
		claim = fmt.Sprintf("declared prerequisites from %s are present in the live cluster", in.Evidence.Source.Ref)
	}
	scope := Scope{
		Kind:      "Prerequisites",
		Name:      "install-prerequisites",
		Namespace: in.Evidence.Scope.Namespace,
	}

	evidence := in.Evidence
	stmt := Statement{
		Type: StatementType,
		Subject: []Subject{
			BuildDeclaredPrerequisitesSubject(in.Evidence.DeclaredDigest),
			BuildLivePrerequisitesSubject(in.Evidence.Scope, in.Evidence.LiveDigest),
		},
		PredicateType: PredicateTypeReceiptV1,
		Predicate: Predicate{
			Version:           PredicateVersion,
			Claim:             claim,
			Scope:             scope,
			Verifier:          in.Verifier,
			VerifiedAt:        verifiedAt.UTC().Format(time.RFC3339),
			PredicateName:     string(PredicatePrerequisitesMet),
			Verdict:           verdict,
			Evidence:          Evidence{Prerequisites: &evidence},
			Omissions:         omissions,
			InputAttestations: inputAttestations,
			NextSteps:         filteredSteps,
		},
	}

	if err := StampFingerprint(&stmt); err != nil {
		return Statement{}, fmt.Errorf("prerequisites receipt: stamp fingerprint: %w", err)
	}
	return stmt, nil
}

// BuildDeclaredPrerequisitesSubject builds the declared-prerequisites subject.
func BuildDeclaredPrerequisitesSubject(digest string) Subject {
	short := digest
	if len(short) > 16 {
		short = short[:16]
	}
	return Subject{
		Name:   SubjectSchemeDeclaredPrerequisites + "sha256/" + short,
		Digest: map[string]string{"sha256": digest},
	}
}

// BuildLivePrerequisitesSubject builds the live-prerequisites subject.
func BuildLivePrerequisitesSubject(scope ObjectSetScope, digest string) Subject {
	name := SubjectSchemeK8sLivePrerequisites + "cluster"
	if scope.Namespace != "" {
		name = SubjectSchemeK8sLivePrerequisites + "namespace/" + scope.Namespace
	}
	return Subject{
		Name:   name,
		Digest: map[string]string{"sha256": digest},
	}
}
