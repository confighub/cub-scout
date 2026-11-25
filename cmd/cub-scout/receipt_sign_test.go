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

	"github.com/confighub/cub-scout/v3/pkg/agent"
)

func resetReceiptSignFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		receiptSignInput = ""
		receiptSignKey = ""
		receiptSignOutput = ""
		receiptVerifySignedInput = ""
		receiptVerifySignedPubKey = ""
		receiptKeygenOutPrivate = ""
		receiptKeygenOutPublic = ""
		receiptFormat = "ascii"
		rootCmd.SetOut(nil)
		rootCmd.SetIn(nil)
		rootCmd.SetArgs(nil)
	})
	receiptSignInput = ""
	receiptSignKey = ""
	receiptSignOutput = ""
	receiptVerifySignedInput = ""
	receiptVerifySignedPubKey = ""
	receiptKeygenOutPrivate = ""
	receiptKeygenOutPublic = ""
	receiptFormat = "ascii"
}

func TestReceiptSignCLI_KeygenSignVerify_RoundTrip(t *testing.T) {
	resetReceiptSignFlags(t)
	tmp := t.TempDir()
	privPath := filepath.Join(tmp, "priv.pem")
	pubPath := filepath.Join(tmp, "pub.pem")
	payloadPath := filepath.Join(tmp, "receipt.json")
	signedPath := filepath.Join(tmp, "receipt.signed.json")

	// 1. Keygen
	var keygenOut bytes.Buffer
	rootCmd.SetOut(&keygenOut)
	rootCmd.SetArgs([]string{"receipt", "keygen", "--out-private", privPath, "--out-public", pubPath})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("receipt keygen failed: %v", err)
	}
	if !strings.Contains(keygenOut.String(), "keyId:") {
		t.Errorf("expected keyId in keygen output, got: %s", keygenOut.String())
	}
	info, err := os.Stat(privPath)
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("private key perm = %o, want 0600", info.Mode().Perm())
	}

	// 2. Write a receipt/v1 payload
	payload := []byte(`{"_type":"https://in-toto.io/Statement/v1","predicateType":"https://cub-scout.dev/receipt/v1","predicate":{"verdict":"PASS","version":"v1"}}`)
	if err := os.WriteFile(payloadPath, payload, 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	// 3. Sign to output file
	var signOut bytes.Buffer
	rootCmd.SetOut(&signOut)
	rootCmd.SetArgs([]string{"receipt", "sign", "--input", payloadPath, "--key", privPath, "-o", signedPath})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("receipt sign failed: %v", err)
	}
	if _, err := os.Stat(signedPath); err != nil {
		t.Fatalf("expected signed output file at %s: %v", signedPath, err)
	}

	// 4. Verify offline (ASCII)
	var verifyASCII bytes.Buffer
	rootCmd.SetOut(&verifyASCII)
	rootCmd.SetArgs([]string{"receipt", "verify", "--input", signedPath, "--pubkey", pubPath, "--format", "ascii"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("receipt verify (ascii) failed: %v", err)
	}
	if !strings.Contains(verifyASCII.String(), "Signature OK:") {
		t.Errorf("expected 'Signature OK:' in ascii output, got: %s", verifyASCII.String())
	}

	// 5. Verify offline (JSON)
	var verifyJSON bytes.Buffer
	rootCmd.SetOut(&verifyJSON)
	rootCmd.SetArgs([]string{"receipt", "verify", "--input", signedPath, "--pubkey", pubPath, "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("receipt verify (json) failed: %v", err)
	}
	var res agent.SignedReceiptVerification
	if err := json.Unmarshal(verifyJSON.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal verify JSON: %v\nraw: %s", err, verifyJSON.String())
	}
	if !res.Valid || res.Algorithm != "ed25519" || res.SchemaVersion != "signed-receipt/v1" {
		t.Errorf("unexpected verification result: %+v", res)
	}
}

func TestReceiptSignCLI_StdinAndStdout_LandedPayload(t *testing.T) {
	resetReceiptSignFlags(t)
	tmp := t.TempDir()
	privPath := filepath.Join(tmp, "priv.pem")
	pubPath := filepath.Join(tmp, "pub.pem")

	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		t.Fatalf("write priv: %v", err)
	}
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		t.Fatalf("write pub: %v", err)
	}

	landedPayload := `{"command":"landed","answer":{"state":"landed","summary":"ok"}}`

	// Sign from stdin to stdout
	var signedBuf bytes.Buffer
	rootCmd.SetIn(strings.NewReader(landedPayload))
	rootCmd.SetOut(&signedBuf)
	rootCmd.SetArgs([]string{"receipt", "sign", "--input", "-", "--key", privPath})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("receipt sign stdin failed: %v", err)
	}

	// Verify from stdin
	var verifyBuf bytes.Buffer
	rootCmd.SetIn(bytes.NewReader(signedBuf.Bytes()))
	rootCmd.SetOut(&verifyBuf)
	rootCmd.SetArgs([]string{"receipt", "verify", "--input", "-", "--pubkey", pubPath, "--format", "json"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("receipt verify stdin failed: %v", err)
	}
	var res agent.SignedReceiptVerification
	if err := json.Unmarshal(verifyBuf.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal verify result: %v", err)
	}
	if !res.Valid {
		t.Fatalf("expected valid=true, got %+v", res)
	}
}

func TestReceiptSignCLI_TamperedPayload_ExitsNonZero(t *testing.T) {
	resetReceiptSignFlags(t)
	tmp := t.TempDir()
	privPath := filepath.Join(tmp, "priv.pem")
	pubPath := filepath.Join(tmp, "pub.pem")
	signedPath := filepath.Join(tmp, "signed.json")

	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair: %v", err)
	}
	_ = os.WriteFile(privPath, privPEM, 0o600)
	_ = os.WriteFile(pubPath, pubPEM, 0o644)

	env, err := agent.SignReceipt([]byte(`{"verdict":"PASS"}`), privPEM)
	if err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	env.Payload = json.RawMessage(`{"verdict":"BLOCK"}`)
	raw, _ := json.MarshalIndent(env, "", "  ")
	_ = os.WriteFile(signedPath, raw, 0o644)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"receipt", "verify", "--input", signedPath, "--pubkey", pubPath, "--format", "json"})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero error on tampered signed receipt")
	}

	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 1 {
		t.Errorf("expected exit code 1 on invalid signature, got err=%v", err)
	}

	var res agent.SignedReceiptVerification
	if uErr := json.Unmarshal(out.Bytes(), &res); uErr != nil {
		t.Fatalf("unmarshal failure JSON: %v\nraw: %s", uErr, out.String())
	}
	if res.Valid {
		t.Errorf("expected valid=false in JSON output, got true")
	}
	if res.Error == "" {
		t.Errorf("expected non-empty error field in JSON output")
	}
}

func TestReceiptSignCLI_WrongPubKey_ExitsNonZero(t *testing.T) {
	resetReceiptSignFlags(t)
	tmp := t.TempDir()
	pub2Path := filepath.Join(tmp, "pub2.pem")
	signedPath := filepath.Join(tmp, "signed.json")

	priv1, _, _, _ := agent.GenerateEd25519KeyPair()
	_, pub2, _, _ := agent.GenerateEd25519KeyPair()
	_ = os.WriteFile(pub2Path, pub2, 0o644)

	env, err := agent.SignReceipt([]byte(`{"state":"landed"}`), priv1)
	if err != nil {
		t.Fatalf("SignReceipt: %v", err)
	}
	raw, _ := json.Marshal(env)
	_ = os.WriteFile(signedPath, raw, 0o644)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"receipt", "verify", "--input", signedPath, "--pubkey", pub2Path})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when verifying with wrong public key")
	}
	if !strings.Contains(out.String(), "Signature INVALID:") {
		t.Errorf("expected 'Signature INVALID:' in ascii output, got: %s", out.String())
	}
}

func TestReceiptSignCLI_MissingRequiredFlags(t *testing.T) {
	resetReceiptSignFlags(t)
	rootCmd.SetArgs([]string{"receipt", "verify", "--input", "some.json"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "--pubkey is required") {
		t.Errorf("expected --pubkey is required error, got: %v", err)
	}

	resetReceiptSignFlags(t)
	rootCmd.SetArgs([]string{"receipt", "verify", "--pubkey", "some.pem"})
	if err := rootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "--input is required") {
		t.Errorf("expected --input is required error, got: %v", err)
	}
}
