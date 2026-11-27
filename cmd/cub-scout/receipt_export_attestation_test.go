// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v3/pkg/agent"
)

func resetReceiptExportAttestationFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		receiptExportAttestationInput = ""
		receiptExportAttestationPubKey = ""
		receiptExportAttestationCluster = ""
		receiptExportAttestationTarget = ""
		receiptExportAttestationRelease = ""
		receiptExportAttestationOutput = ""
		rootCmd.SetOut(nil)
		rootCmd.SetIn(nil)
		rootCmd.SetArgs(nil)
	})
	receiptExportAttestationInput = ""
	receiptExportAttestationPubKey = ""
	receiptExportAttestationCluster = ""
	receiptExportAttestationTarget = ""
	receiptExportAttestationRelease = ""
	receiptExportAttestationOutput = ""
}

func TestReceiptExportAttestation_ValidWithPubKey(t *testing.T) {
	resetReceiptExportAttestationFlags(t)
	tmp := t.TempDir()

	privPEM, pubPEM, keyID, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	observedAt := time.Date(2026, 11, 27, 8, 30, 0, 0, time.UTC)
	signedPath := writeSignedLandedReceipt(t, tmp, "prod-east.signed.json", "prod-east", "a1b2c3d4", agent.LandingStateLanded, observedAt, privPEM)

	// Read envelope back to compute expected canonical SHA-256 hex digest.
	rawEnv, err := os.ReadFile(signedPath)
	if err != nil {
		t.Fatal(err)
	}
	var env agent.SignedReceiptEnvelope
	if err := json.Unmarshal(rawEnv, &env); err != nil {
		t.Fatal(err)
	}
	wantHex := strings.TrimPrefix(env.PayloadDigest, "sha256:")

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "export-attestation",
		"--input", signedPath,
		"--pubkey", pubPath,
		"--release", "rel-2026-11-27",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("receipt export-attestation failed: %v", err)
	}

	var stmt landedAttestationStatement
	if err := json.Unmarshal(out.Bytes(), &stmt); err != nil {
		t.Fatalf("unmarshal attestation statement: %v\nraw:\n%s", err, out.String())
	}

	if stmt.Type != agent.StatementType {
		t.Errorf("_type = %q, want %q", stmt.Type, agent.StatementType)
	}
	if stmt.PredicateType != landedAttestationPredicateType {
		t.Errorf("predicateType = %q, want %q", stmt.PredicateType, landedAttestationPredicateType)
	}
	if len(stmt.Subject) != 1 {
		t.Fatalf("len(subject) = %d, want 1", len(stmt.Subject))
	}
	if stmt.Subject[0].Name != "prod-east/Deployment/prod/api" {
		t.Errorf("subject[0].name = %q, want %q", stmt.Subject[0].Name, "prod-east/Deployment/prod/api")
	}
	if stmt.Subject[0].Digest["sha256"] != wantHex {
		t.Errorf("subject[0].digest.sha256 = %q, want %q", stmt.Subject[0].Digest["sha256"], wantHex)
	}

	p := stmt.Predicate
	if !p.Verified {
		t.Errorf("predicate.verified = false, want true")
	}
	if p.Cluster != "prod-east" {
		t.Errorf("predicate.cluster = %q, want %q", p.Cluster, "prod-east")
	}
	if p.Target != "Deployment/prod/api" {
		t.Errorf("predicate.target = %q, want %q", p.Target, "Deployment/prod/api")
	}
	if p.Release != "rel-2026-11-27" {
		t.Errorf("predicate.release = %q, want %q", p.Release, "rel-2026-11-27")
	}
	if p.Revision != "a1b2c3d4" {
		t.Errorf("predicate.revision = %q, want %q", p.Revision, "a1b2c3d4")
	}
	if p.State != agent.LandingStateLanded {
		t.Errorf("predicate.state = %q, want %q", p.State, agent.LandingStateLanded)
	}
	if p.ObservedAt != observedAt.Format(time.RFC3339) {
		t.Errorf("predicate.observedAt = %q, want %q", p.ObservedAt, observedAt.Format(time.RFC3339))
	}
	if p.SignedAt != env.SignedAt {
		t.Errorf("predicate.signedAt = %q, want %q", p.SignedAt, env.SignedAt)
	}
	if p.KeyID != keyID {
		t.Errorf("predicate.keyId = %q, want %q", p.KeyID, keyID)
	}
	if p.PayloadDigest != env.PayloadDigest {
		t.Errorf("predicate.payloadDigest = %q, want %q", p.PayloadDigest, env.PayloadDigest)
	}
	if p.SignatureAlgorithm != agent.SignedReceiptAlgorithm {
		t.Errorf("predicate.signatureAlgorithm = %q, want %q", p.SignatureAlgorithm, agent.SignedReceiptAlgorithm)
	}

	// Deterministic output: a second run produces byte-identical JSON.
	var out2 bytes.Buffer
	rootCmd.SetOut(&out2)
	rootCmd.SetArgs([]string{
		"receipt", "export-attestation",
		"--input", signedPath,
		"--pubkey", pubPath,
		"--release", "rel-2026-11-27",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("second export-attestation failed: %v", err)
	}
	if !bytes.Equal(out.Bytes(), out2.Bytes()) {
		t.Errorf("export-attestation output is not deterministic:\nrun1:\n%s\nrun2:\n%s", out.String(), out2.String())
	}
}

func TestReceiptExportAttestation_TamperedWithPubKey_FailsClosed(t *testing.T) {
	resetReceiptExportAttestationFlags(t)
	tmp := t.TempDir()

	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	env, err := agent.SignReceiptAt([]byte(`{"context":"prod","answer":{"state":"landed"}}`), privPEM, time.Date(2026, 11, 27, 8, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	env.Payload = json.RawMessage(`{"context":"prod","answer":{"state":"not-landed"}}`)
	rawTampered, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tamperedPath := filepath.Join(tmp, "tampered.signed.json")
	if err := os.WriteFile(tamperedPath, rawTampered, 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(tmp, "should-not-exist.att.json")

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "export-attestation",
		"--input", tamperedPath,
		"--pubkey", pubPath,
		"-o", outPath,
	})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero error on tampered signed receipt")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got err=%v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected no stdout on failure, got: %s", out.String())
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Errorf("expected output file %s not to be created on failure", outPath)
	}
}

func TestReceiptExportAttestation_WrongPubKey_FailsClosed(t *testing.T) {
	resetReceiptExportAttestationFlags(t)
	tmp := t.TempDir()

	priv1, _, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, pub2, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pub2Path := filepath.Join(tmp, "pub2.pem")
	if err := os.WriteFile(pub2Path, pub2, 0o644); err != nil {
		t.Fatal(err)
	}

	signedPath := writeSignedLandedReceipt(t, tmp, "staging.signed.json", "staging", "deadbeef", agent.LandingStateLanded, time.Date(2026, 11, 27, 8, 30, 0, 0, time.UTC), priv1)
	outPath := filepath.Join(tmp, "wrong-key.att.json")

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "export-attestation",
		"--input", signedPath,
		"--pubkey", pub2Path,
		"-o", outPath,
	})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero error when verifying with wrong public key")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got err=%v", err)
	}
	if out.Len() != 0 {
		t.Errorf("expected no stdout on failure, got: %s", out.String())
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Errorf("expected output file %s not to be created on failure", outPath)
	}
}

func TestReceiptExportAttestation_ValidWithoutPubKey_VerifiedFalse(t *testing.T) {
	resetReceiptExportAttestationFlags(t)
	tmp := t.TempDir()

	privPEM, _, keyID, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 11, 27, 8, 30, 0, 0, time.UTC)
	signedPath := writeSignedLandedReceipt(t, tmp, "dev.signed.json", "dev", "rev-1234", agent.LandingStateConverging, observedAt, privPEM)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "export-attestation",
		"--input", signedPath,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export-attestation without --pubkey failed: %v", err)
	}

	// Verify explicit `"verified": false` appears in raw JSON.
	if !strings.Contains(out.String(), `"verified": false`) {
		t.Errorf("expected explicit \"verified\": false in JSON output:\n%s", out.String())
	}

	var stmt landedAttestationStatement
	if err := json.Unmarshal(out.Bytes(), &stmt); err != nil {
		t.Fatalf("unmarshal statement: %v", err)
	}
	if stmt.Type != agent.StatementType {
		t.Errorf("_type = %q, want %q", stmt.Type, agent.StatementType)
	}
	if stmt.PredicateType != landedAttestationPredicateType {
		t.Errorf("predicateType = %q, want %q", stmt.PredicateType, landedAttestationPredicateType)
	}
	if stmt.Predicate.Verified {
		t.Errorf("predicate.verified = true, want false")
	}
	if stmt.Predicate.KeyID != keyID {
		t.Errorf("predicate.keyId = %q, want %q", stmt.Predicate.KeyID, keyID)
	}
	if stmt.Predicate.State != agent.LandingStateConverging {
		t.Errorf("predicate.state = %q, want %q", stmt.Predicate.State, agent.LandingStateConverging)
	}
	if stmt.Predicate.SignatureAlgorithm != agent.SignedReceiptAlgorithm {
		t.Errorf("predicate.signatureAlgorithm = %q, want %q", stmt.Predicate.SignatureAlgorithm, agent.SignedReceiptAlgorithm)
	}
	if !strings.HasPrefix(stmt.Predicate.PayloadDigest, "sha256:") {
		t.Errorf("predicate.payloadDigest = %q, want sha256: prefix", stmt.Predicate.PayloadDigest)
	}
}

func TestReceiptExportAttestation_CLIRoundTrip_OutputFileAndStdinWithOverrides(t *testing.T) {
	resetReceiptExportAttestationFlags(t)
	tmp := t.TempDir()

	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	observedAt := time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)
	signedPath := writeSignedLandedReceipt(t, tmp, "eu.signed.json", "eu-central", "cafebabe", agent.LandingStateLanded, observedAt, privPEM)
	rawSigned, err := os.ReadFile(signedPath)
	if err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tmp, "nested", "eu.att.json")

	// Stdin ('-') -> -o <file>, overriding --cluster, --target, and --release.
	var stdout bytes.Buffer
	rootCmd.SetIn(bytes.NewReader(rawSigned))
	rootCmd.SetOut(&stdout)
	rootCmd.SetArgs([]string{
		"receipt", "export-attestation",
		"--input", "-",
		"--pubkey", pubPath,
		"--cluster", "prod-eu",
		"--target", "payments-api",
		"--release", "v3.2.0-rc1",
		"-o", outPath,
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("export-attestation stdin -> -o failed: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout when -o is set, got: %s", stdout.String())
	}

	written, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read written attestation file: %v", err)
	}
	var stmt landedAttestationStatement
	if err := json.Unmarshal(written, &stmt); err != nil {
		t.Fatalf("unmarshal written attestation: %v\nraw:\n%s", err, string(written))
	}
	if len(stmt.Subject) != 1 || stmt.Subject[0].Name != "prod-eu/payments-api" {
		t.Errorf("subject = %+v, want single subject named prod-eu/payments-api", stmt.Subject)
	}
	if stmt.Predicate.Cluster != "prod-eu" || stmt.Predicate.Target != "payments-api" || stmt.Predicate.Release != "v3.2.0-rc1" {
		t.Errorf("unexpected predicate overrides: %+v", stmt.Predicate)
	}
	if !stmt.Predicate.Verified {
		t.Errorf("expected verified=true, got false")
	}
}

func TestReceiptExportAttestation_MissingInputAndMalformedEnvelope(t *testing.T) {
	resetReceiptExportAttestationFlags(t)
	rootCmd.SetArgs([]string{"receipt", "export-attestation"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "--input is required") {
		t.Errorf("expected --input is required error, got: %v", err)
	}

	resetReceiptExportAttestationFlags(t)
	rootCmd.SetIn(strings.NewReader(`{"schemaVersion":"unknown/v0","algorithm":"ed25519"}`))
	rootCmd.SetArgs([]string{"receipt", "export-attestation", "--input", "-"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "unsupported schemaVersion") {
		t.Errorf("expected unsupported schemaVersion error, got: %v", err)
	}
}
