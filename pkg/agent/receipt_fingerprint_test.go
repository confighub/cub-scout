// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// TestComputeStatementFingerprint_DeletesFingerprintKey_NotZeroes proves
// the Codex round-4 fix: the canonical input to SHA-256 has the
// `predicate.fingerprint` key REMOVED, not just set to "". An external
// verifier following the documented contract would otherwise compute a
// different digest than the emitter.
//
// The check is direct: independently compute the expected digest by
// marshaling, parsing to a map, deleting the key, canonicalizing with
// the same CanonicalJSON, and SHA-256-ing — then compare to the
// production path's output. Any divergence (e.g., emitter writes
// "fingerprint":"" into the bytes it hashes) breaks the test.
func TestComputeStatementFingerprint_DeletesFingerprintKey_NotZeroes(t *testing.T) {
	stmt := makeTestStatement()

	got, err := ComputeStatementFingerprint(stmt)
	if err != nil {
		t.Fatalf("ComputeStatementFingerprint: %v", err)
	}

	// Re-derive the expected digest by hand: marshal, parse, delete the
	// key from the predicate object, canonicalize, hash. This must match
	// what ComputeStatementFingerprint produced.
	raw, err := json.Marshal(stmt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	pred := generic["predicate"].(map[string]interface{})
	// The struct has no omitempty, so Go emits "fingerprint":"" here.
	// Deleting the key is what production code does; we mirror that step
	// to derive the expected digest.
	delete(pred, "fingerprint")
	canon, err := CanonicalJSON(generic)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	sum := sha256.Sum256(canon)
	want := "sha256:" + hex.EncodeToString(sum[:])

	if got != want {
		t.Errorf("emitter hashed something other than canonical-without-fingerprint-key:\n  got:  %s\n  want: %s", got, want)
	}

	// Sanity check: also verify that hashing with the key PRESENT (but
	// empty) would produce a DIFFERENT digest. If this side also matched,
	// the test would be vacuous — proving the delete vs zero distinction
	// actually changes the bytes.
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("unmarshal v2: %v", err)
	}
	pred = generic["predicate"].(map[string]interface{})
	pred["fingerprint"] = "" // present but empty — the OLD broken shape
	canonWithEmptyKey, err := CanonicalJSON(generic)
	if err != nil {
		t.Fatalf("CanonicalJSON v2: %v", err)
	}
	sumWithEmptyKey := sha256.Sum256(canonWithEmptyKey)
	oldShapeHash := "sha256:" + hex.EncodeToString(sumWithEmptyKey[:])
	if oldShapeHash == got {
		t.Error("expected delete-key and empty-key digests to differ; if they were equal the test would not be meaningfully proving the fix")
	}
}
