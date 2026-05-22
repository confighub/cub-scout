// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// buildSampleReceipt produces a real, fingerprint-stamped Statement for
// the input-attestation tests. The Statement is what would come out of
// `cub-scout receipt verify` for an Argo-owned Deployment with a
// resolved git anchor.
func buildSampleReceipt(t *testing.T) Statement {
	t.Helper()
	live := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "api",
				"namespace": "prod",
			},
			"spec": map[string]interface{}{"replicas": int64(3)},
		},
	}
	stmt, err := BuildReceipt(BuildReceiptInput{
		Live: live,
		Scope: Scope{
			Kind:      "Deployment",
			Name:      "api",
			Namespace: "prod",
		},
		Owner:         Ownership{Type: OwnerArgo, SubType: "application"},
		PredicateName: PredicateAppliedMatchesSpec,
		Spec: &SpecAnchor{
			Anchor: SpecAnchorBody{
				Type:     "git",
				RepoURL:  "https://github.com/org/repo",
				Revision: "abc123def456",
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
				Revision: "abc123def456",
				Path:     "apps/prod/api",
			},
		},
		Verifier:   Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt: time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("buildSampleReceipt: %v", err)
	}
	return stmt
}

// --- BuildAttestationRef --------------------------------------------

func TestBuildAttestationRef_HappyPath(t *testing.T) {
	stmt := buildSampleReceipt(t)
	ref, err := BuildAttestationRef(stmt)
	if err != nil {
		t.Fatalf("BuildAttestationRef: %v", err)
	}

	if !strings.HasPrefix(ref.URI, AttestationURIScheme) {
		t.Errorf("URI must start with %q; got %q", AttestationURIScheme, ref.URI)
	}
	hex := strings.TrimPrefix(stmt.Predicate.Fingerprint, "sha256:")
	if !strings.HasSuffix(ref.URI, hex[:12]) {
		t.Errorf("URI must end with the 12-char short fingerprint %q; got %q", hex[:12], ref.URI)
	}
	if ref.Digest["sha256"] != hex {
		t.Errorf("Digest sha256 must equal full hex %q; got %q", hex, ref.Digest["sha256"])
	}
}

func TestBuildAttestationRef_RejectsEmptyFingerprint(t *testing.T) {
	stmt := buildSampleReceipt(t)
	stmt.Predicate.Fingerprint = ""
	_, err := BuildAttestationRef(stmt)
	if err == nil {
		t.Fatal("empty fingerprint must be rejected")
	}
	if !strings.Contains(err.Error(), "empty fingerprint") {
		t.Errorf("error must name the problem; got %v", err)
	}
}

func TestBuildAttestationRef_RejectsMalformedFingerprint(t *testing.T) {
	stmt := buildSampleReceipt(t)
	stmt.Predicate.Fingerprint = "md5:abc123" // wrong algorithm prefix
	_, err := BuildAttestationRef(stmt)
	if err == nil {
		t.Fatal("malformed fingerprint must be rejected")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Errorf("error must explain malformed; got %v", err)
	}
}

func TestBuildAttestationRef_RejectsTamperedStatement(t *testing.T) {
	// Build a valid receipt, then tamper with the verdict. The
	// fingerprint no longer matches the body. BuildAttestationRef
	// MUST refuse to chain — silently chaining a tampered receipt
	// would break the trust property of inputAttestations[].
	stmt := buildSampleReceipt(t)
	stmt.Predicate.Verdict = VerdictBLOCK // was PASS

	_, err := BuildAttestationRef(stmt)
	if err == nil {
		t.Fatal("tampered statement must be rejected")
	}
	if !strings.Contains(err.Error(), "fingerprint") {
		t.Errorf("error must name the fingerprint mismatch; got %v", err)
	}
}

// --- BuildAttestationRefsFromPaths ----------------------------------

func TestBuildAttestationRefsFromPaths_HappyPath(t *testing.T) {
	stmt1 := buildSampleReceipt(t)
	stmt2 := buildSampleReceipt(t)
	stmt2.Predicate.Scope.Name = "worker" // different fingerprint
	if err := StampFingerprint(&stmt2); err != nil {
		t.Fatalf("StampFingerprint: %v", err)
	}

	fakeLoad := func(path string) (Statement, error) {
		switch path {
		case "/fake/api.receipt.json":
			return stmt1, nil
		case "/fake/worker.receipt.json":
			return stmt2, nil
		}
		t.Fatalf("unexpected path %q", path)
		return Statement{}, nil
	}

	refs, err := BuildAttestationRefsFromPaths(
		[]string{"/fake/api.receipt.json", "/fake/worker.receipt.json"},
		fakeLoad,
	)
	if err != nil {
		t.Fatalf("BuildAttestationRefsFromPaths: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs; got %d", len(refs))
	}
	if refs[0].Digest["sha256"] == refs[1].Digest["sha256"] {
		t.Errorf("two different statements must produce different digests; both = %q", refs[0].Digest["sha256"])
	}
}

func TestBuildAttestationRefsFromPaths_LoaderErrorShortCircuits(t *testing.T) {
	stmt := buildSampleReceipt(t)
	loaded := false
	fakeLoad := func(path string) (Statement, error) {
		if path == "/fake/api.receipt.json" {
			loaded = true
			return stmt, nil
		}
		// Second call returns an I/O error
		return Statement{}, &testIOErr{path: path}
	}

	_, err := BuildAttestationRefsFromPaths(
		[]string{"/fake/api.receipt.json", "/fake/missing.receipt.json"},
		fakeLoad,
	)
	if err == nil {
		t.Fatal("loader error on second path must short-circuit")
	}
	if !loaded {
		t.Error("first path should have been loaded before the second's error")
	}
}

type testIOErr struct{ path string }

func (e *testIOErr) Error() string { return "i/o error on " + e.path }

// --- BuildReceipt with InputAttestations -----------------------------

func TestBuildReceipt_InputAttestationsAttached(t *testing.T) {
	// The chain target: build a downstream receipt that references the
	// upstream one. Verify the downstream's predicate.inputAttestations[]
	// contains the right ref AND the downstream fingerprint covers
	// that field (changing the upstream changes the downstream
	// fingerprint).
	upstream := buildSampleReceipt(t)
	upstreamRef, err := BuildAttestationRef(upstream)
	if err != nil {
		t.Fatalf("BuildAttestationRef upstream: %v", err)
	}

	live := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "worker", "namespace": "prod"},
			"spec":       map[string]interface{}{"replicas": int64(1)},
		},
	}
	downstream, err := BuildReceipt(BuildReceiptInput{
		Live:              live,
		Scope:             Scope{Kind: "Deployment", Name: "worker", Namespace: "prod"},
		Owner:             Ownership{Type: OwnerUnknown},
		PredicateName:     "",
		InputAttestations: []AttestationRef{upstreamRef},
		Verifier:          Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt:        time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildReceipt downstream: %v", err)
	}
	if len(downstream.Predicate.InputAttestations) != 1 {
		t.Fatalf("downstream should reference 1 attestation; got %d", len(downstream.Predicate.InputAttestations))
	}
	if downstream.Predicate.InputAttestations[0].Digest["sha256"] != upstreamRef.Digest["sha256"] {
		t.Errorf("downstream attestation digest must match upstream; got %q vs %q",
			downstream.Predicate.InputAttestations[0].Digest["sha256"], upstreamRef.Digest["sha256"])
	}

	// The downstream's fingerprint MUST cover the inputAttestations[]
	// field. Tampering the field on the loaded copy and recomputing
	// should produce a different digest.
	tampered := downstream
	tampered.Predicate.InputAttestations = []AttestationRef{} // strip
	recomputed, err := ComputeStatementFingerprint(tampered)
	if err != nil {
		t.Fatalf("ComputeStatementFingerprint: %v", err)
	}
	if recomputed == downstream.Predicate.Fingerprint {
		t.Error("stripping inputAttestations[] must change the recomputed fingerprint; the field is supposed to be covered")
	}
}

func TestBuildReceipt_NilInputAttestations_EmptyArray(t *testing.T) {
	// When the caller passes nil, the receipt must still serialize as
	// an empty JSON array — never null. Consumers iterate the field
	// unconditionally; null would break them.
	live := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "api", "namespace": "prod"},
		},
	}
	stmt, err := BuildReceipt(BuildReceiptInput{
		Live:              live,
		Scope:             Scope{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Owner:             Ownership{Type: OwnerUnknown},
		InputAttestations: nil,
		Verifier:          Verifier{Tool: "cub-scout", Version: "v0.0.0-test"},
		VerifiedAt:        time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildReceipt: %v", err)
	}
	if stmt.Predicate.InputAttestations == nil {
		t.Fatal("nil input must serialize as empty array, not nil")
	}
	if len(stmt.Predicate.InputAttestations) != 0 {
		t.Errorf("nil input must produce empty inputAttestations[]; got %d entries", len(stmt.Predicate.InputAttestations))
	}
}
