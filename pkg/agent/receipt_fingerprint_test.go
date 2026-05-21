// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
)

func makeTestStatement() Statement {
	return Statement{
		Type:          StatementType,
		Subject:       []Subject{{Name: "k8s-live://apps/v1/Deployment/prod/api", Digest: map[string]string{"sha256": "abc"}}},
		PredicateType: PredicateTypeReceiptV1,
		Predicate: Predicate{
			Version:           PredicateVersion,
			Claim:             "applied matches spec",
			Scope:             Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
			Verifier:          Verifier{Tool: "cub-scout", Version: "v2.3.0"},
			VerifiedAt:        "2026-05-21T10:30:00Z",
			PredicateName:     "applied-matches-spec",
			Verdict:           VerdictPASS,
			Omissions:         []Omission{},
			InputAttestations: []AttestationRef{},
		},
	}
}

func TestComputeStatementFingerprint_Deterministic(t *testing.T) {
	stmt := makeTestStatement()
	first, err := ComputeStatementFingerprint(stmt)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !strings.HasPrefix(first, "sha256:") {
		t.Errorf("fingerprint missing sha256 prefix: %s", first)
	}
	if len(first) != len("sha256:")+64 {
		t.Errorf("fingerprint length unexpected: %s (len %d)", first, len(first))
	}

	// Run many times — must produce the same value.
	for i := 0; i < 50; i++ {
		next, err := ComputeStatementFingerprint(stmt)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if next != first {
			t.Errorf("non-deterministic fingerprint: iter %d differs", i)
		}
	}
}

func TestComputeStatementFingerprint_IgnoresExistingFingerprintValue(t *testing.T) {
	// Setting Predicate.Fingerprint before computing must not change the
	// computed value (the function zeroes it before hashing).
	stmt := makeTestStatement()
	stmt.Predicate.Fingerprint = ""
	zeroed, err := ComputeStatementFingerprint(stmt)
	if err != nil {
		t.Fatalf("zeroed: %v", err)
	}

	stmt.Predicate.Fingerprint = "sha256:bogus"
	bogus, err := ComputeStatementFingerprint(stmt)
	if err != nil {
		t.Fatalf("bogus: %v", err)
	}

	stmt.Predicate.Fingerprint = "sha256:also-bogus"
	alsoBogus, err := ComputeStatementFingerprint(stmt)
	if err != nil {
		t.Fatalf("also-bogus: %v", err)
	}

	if zeroed != bogus || bogus != alsoBogus {
		t.Errorf("fingerprint should ignore predicate.fingerprint input; zeroed=%s bogus=%s alsoBogus=%s", zeroed, bogus, alsoBogus)
	}
}

func TestComputeStatementFingerprint_TamperDetection_Subject(t *testing.T) {
	a := makeTestStatement()
	b := makeTestStatement()
	b.Subject[0].Name = "k8s-live://apps/v1/Deployment/prod/different-name"

	fpA, _ := ComputeStatementFingerprint(a)
	fpB, _ := ComputeStatementFingerprint(b)
	if fpA == fpB {
		t.Errorf("tampering with subject must change the fingerprint; got identical %s", fpA)
	}
}

func TestComputeStatementFingerprint_TamperDetection_SubjectDigest(t *testing.T) {
	a := makeTestStatement()
	b := makeTestStatement()
	b.Subject[0].Digest["sha256"] = "def"

	fpA, _ := ComputeStatementFingerprint(a)
	fpB, _ := ComputeStatementFingerprint(b)
	if fpA == fpB {
		t.Errorf("tampering with subject digest must change the fingerprint; got identical %s", fpA)
	}
}

func TestComputeStatementFingerprint_TamperDetection_PredicateType(t *testing.T) {
	a := makeTestStatement()
	b := makeTestStatement()
	b.PredicateType = "https://attacker.example/receipt/v1"

	fpA, _ := ComputeStatementFingerprint(a)
	fpB, _ := ComputeStatementFingerprint(b)
	if fpA == fpB {
		t.Errorf("tampering with predicateType must change the fingerprint; got identical %s", fpA)
	}
}

func TestComputeStatementFingerprint_TamperDetection_Verdict(t *testing.T) {
	a := makeTestStatement()
	b := makeTestStatement()
	b.Predicate.Verdict = VerdictBLOCK

	fpA, _ := ComputeStatementFingerprint(a)
	fpB, _ := ComputeStatementFingerprint(b)
	if fpA == fpB {
		t.Errorf("tampering with verdict must change the fingerprint; got identical %s", fpA)
	}
}

func TestComputeStatementFingerprint_TamperDetection_Omissions(t *testing.T) {
	a := makeTestStatement()
	b := makeTestStatement()
	b.Predicate.Omissions = []Omission{{Missing: "test", Reason: "unit test"}}

	fpA, _ := ComputeStatementFingerprint(a)
	fpB, _ := ComputeStatementFingerprint(b)
	if fpA == fpB {
		t.Errorf("tampering with omissions must change the fingerprint; got identical %s", fpA)
	}
}

func TestComputeStatementFingerprint_TamperDetection_Type(t *testing.T) {
	a := makeTestStatement()
	b := makeTestStatement()
	b.Type = "https://attacker.example/Statement/v1"

	fpA, _ := ComputeStatementFingerprint(a)
	fpB, _ := ComputeStatementFingerprint(b)
	if fpA == fpB {
		t.Errorf("tampering with _type must change the fingerprint; got identical %s", fpA)
	}
}

func TestStampFingerprint_RoundTrip(t *testing.T) {
	stmt := makeTestStatement()
	if err := StampFingerprint(&stmt); err != nil {
		t.Fatalf("StampFingerprint: %v", err)
	}
	if stmt.Predicate.Fingerprint == "" {
		t.Fatal("StampFingerprint did not set Fingerprint")
	}
	if err := VerifyStatementFingerprint(stmt); err != nil {
		t.Errorf("Verify after Stamp must succeed; got: %v", err)
	}
}

func TestVerifyStatementFingerprint_DetectsTampering(t *testing.T) {
	stmt := makeTestStatement()
	if err := StampFingerprint(&stmt); err != nil {
		t.Fatalf("StampFingerprint: %v", err)
	}

	// Mutate the verdict after stamping. Verify must detect it.
	tampered := stmt
	tampered.Predicate.Verdict = VerdictBLOCK
	err := VerifyStatementFingerprint(tampered)
	if err == nil {
		t.Error("Verify must detect verdict tampering; got nil error")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("error message should mention mismatch; got: %v", err)
	}
}

func TestStampFingerprint_NilStatement(t *testing.T) {
	err := StampFingerprint(nil)
	if err == nil {
		t.Error("StampFingerprint(nil) must error")
	}
}
