// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
	"time"
)

// --- MaxSeverityPolicy ---------------------------------------------

func TestMaxSeverityPolicy_Name(t *testing.T) {
	if got := (MaxSeverityPolicy{}).Name(); got != "max-severity" {
		t.Errorf("MaxSeverityPolicy.Name() = %q; want %q", got, "max-severity")
	}
}

func TestMaxSeverityPolicy_EmptyInput(t *testing.T) {
	v := MaxSeverityPolicy{}.Synthesize(nil)
	if v != VerdictINCONCLUSIVE {
		t.Errorf("empty input should synthesize to INCONCLUSIVE; got %q", v)
	}
}

func TestMaxSeverityPolicy_RankingOrder(t *testing.T) {
	// Severity: BLOCK > INCONCLUSIVE > WATCH > PASS
	cases := []struct {
		in   []ReceiptVerdict
		want ReceiptVerdict
		why  string
	}{
		{[]ReceiptVerdict{VerdictPASS}, VerdictPASS, "single PASS"},
		{[]ReceiptVerdict{VerdictPASS, VerdictPASS, VerdictPASS}, VerdictPASS, "all PASS"},
		{[]ReceiptVerdict{VerdictPASS, VerdictWATCH}, VerdictWATCH, "PASS + WATCH → WATCH"},
		{[]ReceiptVerdict{VerdictPASS, VerdictWATCH, VerdictINCONCLUSIVE}, VerdictINCONCLUSIVE, "PASS + WATCH + INCONCLUSIVE → INCONCLUSIVE"},
		{[]ReceiptVerdict{VerdictPASS, VerdictBLOCK}, VerdictBLOCK, "any BLOCK → BLOCK"},
		{[]ReceiptVerdict{VerdictBLOCK, VerdictINCONCLUSIVE}, VerdictBLOCK, "BLOCK outranks INCONCLUSIVE"},
		{[]ReceiptVerdict{VerdictINCONCLUSIVE, VerdictWATCH}, VerdictINCONCLUSIVE, "INCONCLUSIVE outranks WATCH"},
		{[]ReceiptVerdict{VerdictWATCH, VerdictPASS}, VerdictWATCH, "WATCH outranks PASS"},
	}
	for _, tc := range cases {
		got := MaxSeverityPolicy{}.Synthesize(tc.in)
		if got != tc.want {
			t.Errorf("%s: Synthesize(%v) = %q; want %q", tc.why, tc.in, got, tc.want)
		}
	}
}

// --- BuildSyntheticAggregateSubject --------------------------------

// makeAttestationRef builds a VerifiedAttestationRef for testing by
// going through BuildAttestationRef (the only legitimate construction
// path). We start from buildSampleReceipt + modify so each call
// produces a distinct fingerprint.
func makeAttestationRef(t *testing.T, distinguisher string) VerifiedAttestationRef {
	t.Helper()
	stmt := buildSampleReceipt(t)
	// Tweak something covered by the fingerprint so each helper call
	// produces a distinct receipt.
	stmt.Predicate.Claim = stmt.Predicate.Claim + " // " + distinguisher
	if err := StampFingerprint(&stmt); err != nil {
		t.Fatalf("re-stamp: %v", err)
	}
	ref, err := BuildAttestationRef(stmt)
	if err != nil {
		t.Fatalf("BuildAttestationRef: %v", err)
	}
	return ref
}

func TestBuildSyntheticAggregateSubject_HappyPath(t *testing.T) {
	refs := []VerifiedAttestationRef{
		makeAttestationRef(t, "a"),
		makeAttestationRef(t, "b"),
		makeAttestationRef(t, "c"),
	}

	subject, err := BuildSyntheticAggregateSubject(refs)
	if err != nil {
		t.Fatalf("BuildSyntheticAggregateSubject: %v", err)
	}
	if !strings.HasPrefix(subject.Name, "synthetic-aggregate://sha256/") {
		t.Errorf("subject.Name should start with synthetic-aggregate://sha256/; got %q", subject.Name)
	}
	if dig := subject.Digest["sha256"]; len(dig) != 64 {
		t.Errorf("subject.Digest[sha256] must be 64 hex chars; got %d", len(dig))
	}
	// aggregate-id is first 16 hex chars of the digest
	aggregateID := strings.TrimPrefix(subject.Name, "synthetic-aggregate://sha256/")
	if !strings.HasPrefix(subject.Digest["sha256"], aggregateID) {
		t.Errorf("aggregate-id %q must be the first 16 hex chars of digest %q", aggregateID, subject.Digest["sha256"])
	}
}

func TestBuildSyntheticAggregateSubject_OrderIndependent(t *testing.T) {
	// Same set of inputs in different order should produce the same
	// subject — the aggregate is a set, not a list.
	a := makeAttestationRef(t, "a")
	b := makeAttestationRef(t, "b")
	c := makeAttestationRef(t, "c")

	subj1, err := BuildSyntheticAggregateSubject([]VerifiedAttestationRef{a, b, c})
	if err != nil {
		t.Fatalf("subj1: %v", err)
	}
	subj2, err := BuildSyntheticAggregateSubject([]VerifiedAttestationRef{c, a, b})
	if err != nil {
		t.Fatalf("subj2: %v", err)
	}
	if subj1.Name != subj2.Name {
		t.Errorf("aggregate subject must be order-independent; got %q vs %q", subj1.Name, subj2.Name)
	}
	if subj1.Digest["sha256"] != subj2.Digest["sha256"] {
		t.Errorf("aggregate digest must be order-independent; got %q vs %q", subj1.Digest["sha256"], subj2.Digest["sha256"])
	}
}

func TestBuildSyntheticAggregateSubject_EmptyInputRejected(t *testing.T) {
	_, err := BuildSyntheticAggregateSubject(nil)
	if err == nil {
		t.Fatal("empty refs must be rejected")
	}
	if !strings.Contains(err.Error(), "empty refs") {
		t.Errorf("error must name the problem; got %v", err)
	}
}

// --- BuildAggregateReceipt -----------------------------------------

func TestBuildAggregateReceipt_HappyPath(t *testing.T) {
	a := makeAttestationRef(t, "a")
	b := makeAttestationRef(t, "b")
	c := makeAttestationRef(t, "c")

	stmt, err := BuildAggregateReceipt(BuildAggregateReceiptInput{
		Inputs:        []VerifiedAttestationRef{a, b, c},
		InputVerdicts: []ReceiptVerdict{VerdictPASS, VerdictPASS, VerdictPASS},
		Scope: AggregateScopeSpec{
			Kind:        "namespace",
			Namespace:   "prod",
			MemberCount: 3,
		},
		Verifier:   Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		Connected:  false,
	})
	if err != nil {
		t.Fatalf("BuildAggregateReceipt: %v", err)
	}

	if stmt.Type != StatementType {
		t.Errorf("Statement.Type = %q; want %q", stmt.Type, StatementType)
	}
	if stmt.PredicateType != PredicateTypeReceiptV1 {
		t.Errorf("Statement.PredicateType = %q; want %q", stmt.PredicateType, PredicateTypeReceiptV1)
	}
	if len(stmt.Subject) != 1 {
		t.Fatalf("aggregate should carry exactly one subject (the synthetic one); got %d", len(stmt.Subject))
	}
	if !strings.HasPrefix(stmt.Subject[0].Name, SubjectSchemeSyntheticAggregate) {
		t.Errorf("subject scheme = %q; want prefix %q", stmt.Subject[0].Name, SubjectSchemeSyntheticAggregate)
	}
	if got := stmt.Predicate.PredicateName; got != string(PredicateAggregateVerdict) {
		t.Errorf("predicateName = %q; want %q", got, PredicateAggregateVerdict)
	}
	if stmt.Predicate.Verdict != VerdictPASS {
		t.Errorf("all-PASS inputs should synthesize to PASS; got %q", stmt.Predicate.Verdict)
	}
	if len(stmt.Predicate.InputAttestations) != 3 {
		t.Errorf("inputAttestations[] should have 3 entries; got %d", len(stmt.Predicate.InputAttestations))
	}
	if !strings.HasPrefix(stmt.Predicate.Fingerprint, "sha256:") {
		t.Errorf("fingerprint should be sha256:<hex>; got %q", stmt.Predicate.Fingerprint)
	}
	// Verify fingerprint round-trips
	if err := VerifyStatementFingerprint(stmt); err != nil {
		t.Errorf("aggregate fingerprint should verify: %v", err)
	}
}

func TestBuildAggregateReceipt_MaxSeverityBLOCK(t *testing.T) {
	a := makeAttestationRef(t, "a")
	b := makeAttestationRef(t, "b")
	c := makeAttestationRef(t, "c")

	stmt, err := BuildAggregateReceipt(BuildAggregateReceiptInput{
		Inputs:        []VerifiedAttestationRef{a, b, c},
		InputVerdicts: []ReceiptVerdict{VerdictPASS, VerdictBLOCK, VerdictWATCH},
		Scope: AggregateScopeSpec{
			Kind:        "namespace",
			Namespace:   "prod",
			MemberCount: 3,
		},
		Verifier:   Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildAggregateReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictBLOCK {
		t.Errorf("any BLOCK in inputs should synthesize to BLOCK; got %q", stmt.Predicate.Verdict)
	}
}

func TestBuildAggregateReceipt_INCONCLUSIVERecordsOmission(t *testing.T) {
	a := makeAttestationRef(t, "a")
	b := makeAttestationRef(t, "b")

	stmt, err := BuildAggregateReceipt(BuildAggregateReceiptInput{
		Inputs:        []VerifiedAttestationRef{a, b},
		InputVerdicts: []ReceiptVerdict{VerdictPASS, VerdictINCONCLUSIVE},
		Scope: AggregateScopeSpec{
			Kind:        "namespace",
			Namespace:   "prod",
			MemberCount: 2,
		},
		Verifier:   Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildAggregateReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictINCONCLUSIVE {
		t.Errorf("INCONCLUSIVE input → aggregate verdict INCONCLUSIVE; got %q", stmt.Predicate.Verdict)
	}
	found := false
	for _, om := range stmt.Predicate.Omissions {
		if om.Missing == OmissionAggregatePartialCoverage {
			found = true
			if !strings.Contains(om.Reason, "1 of 2") {
				t.Errorf("omission reason should describe the partial coverage; got %q", om.Reason)
			}
		}
	}
	if !found {
		t.Errorf("expected OmissionAggregatePartialCoverage entry; omissions = %+v", stmt.Predicate.Omissions)
	}
}

func TestBuildAggregateReceipt_EmptyInputsRejected(t *testing.T) {
	_, err := BuildAggregateReceipt(BuildAggregateReceiptInput{
		Inputs:        nil,
		InputVerdicts: nil,
		Scope:         AggregateScopeSpec{Kind: "namespace", Namespace: "prod"},
		Verifier:      Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
	})
	if err == nil {
		t.Fatal("empty Inputs must be rejected")
	}
	if !strings.Contains(err.Error(), "empty Inputs") {
		t.Errorf("error must name the problem; got %v", err)
	}
}

func TestBuildAggregateReceipt_LengthMismatchRejected(t *testing.T) {
	a := makeAttestationRef(t, "a")
	_, err := BuildAggregateReceipt(BuildAggregateReceiptInput{
		Inputs:        []VerifiedAttestationRef{a},
		InputVerdicts: []ReceiptVerdict{VerdictPASS, VerdictWATCH}, // length 2, Inputs length 1
		Scope:         AggregateScopeSpec{Kind: "namespace", Namespace: "prod"},
		Verifier:      Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
	})
	if err == nil {
		t.Fatal("length mismatch must be rejected")
	}
	if !strings.Contains(err.Error(), "length") {
		t.Errorf("error must mention length; got %v", err)
	}
}

func TestBuildAggregateReceipt_ZeroValueInputRejected(t *testing.T) {
	// Same API-boundary check as BuildReceipt: zero-value
	// VerifiedAttestationRef is the only forgeable surface from
	// outside the package, and it must be rejected.
	a := makeAttestationRef(t, "a")
	_, err := BuildAggregateReceipt(BuildAggregateReceiptInput{
		Inputs:        []VerifiedAttestationRef{a, {}}, // second is zero-value forgery
		InputVerdicts: []ReceiptVerdict{VerdictPASS, VerdictPASS},
		Scope:         AggregateScopeSpec{Kind: "namespace", Namespace: "prod"},
		Verifier:      Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
	})
	if err == nil {
		t.Fatal("zero-value VerifiedAttestationRef must be rejected")
	}
	if !strings.Contains(err.Error(), "zero-value VerifiedAttestationRef") {
		t.Errorf("error must name the API-boundary check; got %v", err)
	}
}

func TestBuildAggregateReceipt_ClaimMentionsScopeAndPolicy(t *testing.T) {
	a := makeAttestationRef(t, "a")
	stmt, err := BuildAggregateReceipt(BuildAggregateReceiptInput{
		Inputs:        []VerifiedAttestationRef{a},
		InputVerdicts: []ReceiptVerdict{VerdictPASS},
		Scope: AggregateScopeSpec{
			Kind:        "namespace",
			Namespace:   "prod",
			MemberCount: 1,
		},
		Verifier:   Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildAggregateReceipt: %v", err)
	}
	claim := stmt.Predicate.Claim
	if !strings.Contains(claim, "PASS") {
		t.Errorf("claim should mention verdict PASS; got %q", claim)
	}
	if !strings.Contains(claim, "namespace prod") {
		t.Errorf("claim should mention namespace prod; got %q", claim)
	}
	if !strings.Contains(claim, "max-severity") {
		t.Errorf("claim should mention policy max-severity; got %q", claim)
	}
}

func TestBuildAggregateReceipt_FingerprintCoversInputs(t *testing.T) {
	// Tampering with inputAttestations[] after build should invalidate
	// the fingerprint — the aggregate's integrity depends on the input
	// set being immutable.
	a := makeAttestationRef(t, "a")
	b := makeAttestationRef(t, "b")
	c := makeAttestationRef(t, "c")
	stmt, err := BuildAggregateReceipt(BuildAggregateReceiptInput{
		Inputs:        []VerifiedAttestationRef{a, b, c},
		InputVerdicts: []ReceiptVerdict{VerdictPASS, VerdictPASS, VerdictPASS},
		Scope: AggregateScopeSpec{
			Kind:        "namespace",
			Namespace:   "prod",
			MemberCount: 3,
		},
		Verifier:   Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildAggregateReceipt: %v", err)
	}

	// Strip an inputAttestation and recompute; should differ.
	tampered := stmt
	tampered.Predicate.InputAttestations = stmt.Predicate.InputAttestations[:2]
	recomputed, err := ComputeStatementFingerprint(tampered)
	if err != nil {
		t.Fatalf("ComputeStatementFingerprint: %v", err)
	}
	if recomputed == stmt.Predicate.Fingerprint {
		t.Error("stripping an inputAttestation should change the recomputed fingerprint")
	}
}
