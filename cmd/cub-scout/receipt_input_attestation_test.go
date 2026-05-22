// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

// resetReceiptInputAttestationFlag clears --input-attestation between tests.
func resetReceiptInputAttestationFlag(t *testing.T) {
	t.Helper()
	prev := receiptInputAttestations
	receiptInputAttestations = nil
	t.Cleanup(func() { receiptInputAttestations = prev })
}

// seedTwoReceipts produces two distinct receipts on disk (different
// scopes → different fingerprints) and returns their paths. Used by
// the multi-input-attestation chain test.
func seedTwoReceipts(t *testing.T) (string, string) {
	t.Helper()
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.receipt.json")
	b := filepath.Join(tmp, "b.receipt.json")

	for _, spec := range []struct {
		path, ns string
	}{
		{a, "prod-a"}, {b, "prod-b"},
	} {
		resetReceiptFlags(t)
		withFakeReceiptLoader(t, makeReceiptArgoLive())
		captureStdout(t, func() {
			rootCmd.SetArgs([]string{
				"receipt", "verify", "deploy/api",
				"-n", spec.ns,
				"--format", "json",
				"--out", spec.path,
			})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("seed %s: %v", spec.path, err)
			}
		})
	}
	return a, b
}

func TestReceiptVerify_InputAttestation_AttachesRefs(t *testing.T) {
	pathA, pathB := seedTwoReceipts(t)

	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptInputAttestationFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	receiptInputAttestations = []string{pathA, pathB}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--format", "json",
			"--input-attestation", pathA,
			"--input-attestation", pathB,
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("verify with --input-attestation: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(stmt.Predicate.InputAttestations) != 2 {
		t.Fatalf("expected 2 inputAttestations; got %d", len(stmt.Predicate.InputAttestations))
	}
	// Verify each ref points at one of the seeded receipts (fingerprint
	// equality, not URI equality — URI is just the readable label).
	a, err := agent.LoadStatement(pathA)
	if err != nil {
		t.Fatalf("load A: %v", err)
	}
	b, err := agent.LoadStatement(pathB)
	if err != nil {
		t.Fatalf("load B: %v", err)
	}
	expected := map[string]bool{
		strings.TrimPrefix(a.Predicate.Fingerprint, "sha256:"): true,
		strings.TrimPrefix(b.Predicate.Fingerprint, "sha256:"): true,
	}
	for _, ref := range stmt.Predicate.InputAttestations {
		if !expected[ref.Digest["sha256"]] {
			t.Errorf("inputAttestation digest %q not among seeded receipts %v", ref.Digest["sha256"], expected)
		}
		if !strings.HasPrefix(ref.URI, agent.AttestationURIScheme) {
			t.Errorf("URI must use the cub-scout-receipt scheme; got %q", ref.URI)
		}
	}
	// The new receipt's own fingerprint integrity holds with the
	// inputAttestations[] populated.
	if err := agent.VerifyStatementFingerprint(stmt); err != nil {
		t.Errorf("downstream fingerprint must verify: %v", err)
	}
}

func TestReceiptVerify_InputAttestation_TamperedRefuses(t *testing.T) {
	pathA, _ := seedTwoReceipts(t)

	// Tamper with the upstream receipt: change its verdict, write back.
	raw, err := os.ReadFile(pathA)
	if err != nil {
		t.Fatalf("read A: %v", err)
	}
	tampered := strings.ReplaceAll(string(raw), `"verdict": "PASS"`, `"verdict": "BLOCK"`)
	if tampered == string(raw) {
		tampered = strings.ReplaceAll(string(raw), `"verdict": "INCONCLUSIVE"`, `"verdict": "PASS"`)
	}
	if tampered == string(raw) {
		t.Fatalf("seed receipt has no PASS or INCONCLUSIVE to mutate; raw:\n%s", string(raw))
	}
	if err := os.WriteFile(pathA, []byte(tampered), 0o644); err != nil {
		t.Fatalf("write tampered: %v", err)
	}

	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptInputAttestationFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--format", "json",
		"--input-attestation", pathA,
	})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("tampered input-attestation must be rejected at chain construction")
	}
	if !strings.Contains(err.Error(), "input-attestation") && !strings.Contains(err.Error(), "fingerprint") {
		t.Errorf("error must name the chain-construction failure; got %v", err)
	}
}

func TestReceiptVerify_InputAttestation_MissingFile_Errors(t *testing.T) {
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptInputAttestationFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--format", "json",
		"--input-attestation", "/tmp/does-not-exist.receipt.json",
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("missing input-attestation file must error")
	}
}
