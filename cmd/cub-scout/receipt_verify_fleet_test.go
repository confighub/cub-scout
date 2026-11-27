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

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

func resetReceiptVerifyFleetFlags(t *testing.T) {
	t.Helper()
	origNow := receiptVerifyFleetNow
	t.Cleanup(func() {
		receiptVerifyFleetInputs = nil
		receiptVerifyFleetPubKey = ""
		receiptVerifyFleetRevision = ""
		receiptVerifyFleetRequireClusters = nil
		receiptVerifyFleetMaxAge = 0
		receiptVerifyFleetFormat = "ascii"
		receiptVerifyFleetNow = origNow
		rootCmd.SetOut(nil)
		rootCmd.SetIn(nil)
		rootCmd.SetArgs(nil)
	})
	receiptVerifyFleetInputs = nil
	receiptVerifyFleetPubKey = ""
	receiptVerifyFleetRevision = ""
	receiptVerifyFleetRequireClusters = nil
	receiptVerifyFleetMaxAge = 0
	receiptVerifyFleetFormat = "ascii"
}

func writeSignedLandedReceipt(t *testing.T, dir, filename, cluster, revision, state string, observedAt time.Time, privPEM []byte) string {
	t.Helper()
	payload := map[string]interface{}{
		"context": cluster,
		"target": map[string]interface{}{
			"kind":      "Deployment",
			"namespace": "prod",
			"name":      "api",
		},
		"anchor": map[string]interface{}{
			"kind":  "git-sha",
			"value": revision,
		},
		"answer": map[string]interface{}{
			"state":      state,
			"reason":     "test-" + state,
			"summary":    cluster + " is " + state,
			"observedAt": observedAt.UTC().Format(time.RFC3339),
			"cluster": map[string]interface{}{
				"context":  cluster,
				"idSource": "not-read",
			},
		},
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env, err := agent.SignReceiptAt(rawPayload, privPEM, observedAt)
	if err != nil {
		t.Fatalf("SignReceiptAt: %v", err)
	}
	rawEnv, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatalf("marshal signed envelope: %v", err)
	}
	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, rawEnv, 0o644); err != nil {
		t.Fatalf("write signed receipt %s: %v", fullPath, err)
	}
	return fullPath
}

func TestReceiptVerifyFleet_FourClusterAllLanded_Pass(t *testing.T) {
	resetReceiptVerifyFleetFlags(t)
	tmp := t.TempDir()
	receiptsDir := filepath.Join(tmp, "receipts")
	if err := os.MkdirAll(receiptsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	fixedNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	receiptVerifyFleetNow = func() time.Time { return fixedNow }

	clusters := []string{"dev", "staging", "prod-east", "prod-west"}
	for _, c := range clusters {
		writeSignedLandedReceipt(t, receiptsDir, c+".signed.json", c, "a1b2c3d", agent.LandingStateLanded, fixedNow.Add(-2*time.Minute), privPEM)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "verify-fleet",
		"--inputs", receiptsDir,
		"--pubkey", pubPath,
		"--revision", "a1b2c3d",
		"--require-clusters", "dev,staging,prod-east,prod-west",
		"--max-age", "10m",
		"--format", "json",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected exit 0 on 4-cluster all-landed fleet, got err=%v\nout=%s", err, out.String())
	}

	var res fleetReceiptVerificationOutput
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal fleet json: %v\nraw: %s", err, out.String())
	}
	if res.Answer == nil {
		t.Fatal("expected non-nil answer envelope")
	}
	if res.Answer.State != agent.LandingStateLanded {
		t.Errorf("answer.state = %q, want %q", res.Answer.State, agent.LandingStateLanded)
	}
	if res.Summary.Total != 4 || res.Summary.Landed != 4 || res.Summary.ValidSignatures != 4 {
		t.Errorf("unexpected summary counts: %+v", res.Summary)
	}
	if len(res.Answer.Unknowns) != 0 {
		t.Errorf("expected 0 unknowns, got %+v", res.Answer.Unknowns)
	}

	// Compact answer envelope byte ceiling check (<= 700 bytes).
	ansBytes, err := json.Marshal(res.Answer)
	if err != nil {
		t.Fatalf("marshal answer: %v", err)
	}
	if len(ansBytes) > 700 {
		t.Errorf("answer envelope is %d bytes, exceeds 700-byte ceiling: %s", len(ansBytes), string(ansBytes))
	}
}

func TestReceiptVerifyFleet_PartialRolloutConverging(t *testing.T) {
	resetReceiptVerifyFleetFlags(t)
	tmp := t.TempDir()
	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	_ = os.WriteFile(pubPath, pubPEM, 0o644)

	fixedNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	receiptVerifyFleetNow = func() time.Time { return fixedNow }

	receiptsDir := filepath.Join(tmp, "receipts")
	_ = os.MkdirAll(receiptsDir, 0o755)
	writeSignedLandedReceipt(t, receiptsDir, "dev.signed.json", "dev", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)
	writeSignedLandedReceipt(t, receiptsDir, "staging.signed.json", "staging", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)
	writeSignedLandedReceipt(t, receiptsDir, "prod-east.signed.json", "prod-east", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)
	writeSignedLandedReceipt(t, receiptsDir, "prod-west.signed.json", "prod-west", "a1b2c3d", agent.LandingStateConverging, fixedNow, privPEM)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "verify-fleet",
		"--inputs", receiptsDir,
		"--pubkey", pubPath,
		"--format", "json",
	})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit when prod-west is converging")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got err=%v", err)
	}

	var res fleetReceiptVerificationOutput
	if uErr := json.Unmarshal(out.Bytes(), &res); uErr != nil {
		t.Fatalf("unmarshal fleet json: %v", uErr)
	}
	if res.Answer.State != agent.LandingStateConverging {
		t.Errorf("answer.state = %q, want %q", res.Answer.State, agent.LandingStateConverging)
	}
	if res.Summary.Converging != 1 || res.Summary.Landed != 3 {
		t.Errorf("unexpected summary: %+v", res.Summary)
	}
}

func TestReceiptVerifyFleet_TamperedPayload_FailsClosed(t *testing.T) {
	resetReceiptVerifyFleetFlags(t)
	tmp := t.TempDir()
	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	_ = os.WriteFile(pubPath, pubPEM, 0o644)

	fixedNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	receiptVerifyFleetNow = func() time.Time { return fixedNow }

	receiptsDir := filepath.Join(tmp, "receipts")
	_ = os.MkdirAll(receiptsDir, 0o755)
	writeSignedLandedReceipt(t, receiptsDir, "dev.signed.json", "dev", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)
	writeSignedLandedReceipt(t, receiptsDir, "staging.signed.json", "staging", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)
	writeSignedLandedReceipt(t, receiptsDir, "prod-east.signed.json", "prod-east", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)
	tamperedPath := writeSignedLandedReceipt(t, receiptsDir, "prod-west.signed.json", "prod-west", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)

	// Tamper with prod-west payload after signing.
	raw, _ := os.ReadFile(tamperedPath)
	var env agent.SignedReceiptEnvelope
	_ = json.Unmarshal(raw, &env)
	env.Payload = json.RawMessage(`{"context":"prod-west","answer":{"state":"landed","summary":"forged"}}`)
	tamperedBytes, _ := json.MarshalIndent(env, "", "  ")
	_ = os.WriteFile(tamperedPath, tamperedBytes, 0o644)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "verify-fleet",
		"--inputs", receiptsDir,
		"--pubkey", pubPath,
		"--format", "json",
	})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit on tampered receipt")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got err=%v", err)
	}

	var res fleetReceiptVerificationOutput
	if uErr := json.Unmarshal(out.Bytes(), &res); uErr != nil {
		t.Fatalf("unmarshal fleet json: %v", uErr)
	}
	if res.Answer.State != agent.LandingStateNotLanded {
		t.Errorf("answer.state = %q, want %q", res.Answer.State, agent.LandingStateNotLanded)
	}
	if res.Summary.InvalidReceipts != 1 || res.Summary.ValidSignatures != 3 {
		t.Errorf("unexpected summary: %+v", res.Summary)
	}
}

func TestReceiptVerifyFleet_WrongPubKey_FailsClosed(t *testing.T) {
	resetReceiptVerifyFleetFlags(t)
	tmp := t.TempDir()
	priv1, _, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, pub2, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pub2Path := filepath.Join(tmp, "wrong-pub.pem")
	_ = os.WriteFile(pub2Path, pub2, 0o644)

	fixedNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	receiptVerifyFleetNow = func() time.Time { return fixedNow }

	f1 := writeSignedLandedReceipt(t, tmp, "dev.signed.json", "dev", "a1b2c3d", agent.LandingStateLanded, fixedNow, priv1)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "verify-fleet",
		"--inputs", f1,
		"--pubkey", pub2Path,
		"--format", "json",
	})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit with wrong public key")
	}
	var res fleetReceiptVerificationOutput
	if uErr := json.Unmarshal(out.Bytes(), &res); uErr != nil {
		t.Fatalf("unmarshal fleet json: %v", uErr)
	}
	if res.Answer.State != agent.LandingStateNotLanded {
		t.Errorf("answer.state = %q, want %q", res.Answer.State, agent.LandingStateNotLanded)
	}
	if res.Summary.InvalidReceipts != 1 {
		t.Errorf("expected 1 invalid receipt, got %+v", res.Summary)
	}
}

func TestReceiptVerifyFleet_MissingRequiredCluster_NotObservable(t *testing.T) {
	resetReceiptVerifyFleetFlags(t)
	tmp := t.TempDir()
	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	_ = os.WriteFile(pubPath, pubPEM, 0o644)

	fixedNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	receiptVerifyFleetNow = func() time.Time { return fixedNow }

	f1 := writeSignedLandedReceipt(t, tmp, "dev.signed.json", "dev", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)
	f2 := writeSignedLandedReceipt(t, tmp, "staging.signed.json", "staging", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "verify-fleet",
		"--inputs", f1 + "," + f2,
		"--pubkey", pubPath,
		"--require-clusters", "dev,staging,prod-east",
		"--format", "json",
	})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit when required cluster prod-east is missing")
	}

	var res fleetReceiptVerificationOutput
	if uErr := json.Unmarshal(out.Bytes(), &res); uErr != nil {
		t.Fatalf("unmarshal fleet json: %v", uErr)
	}
	if res.Answer.State != agent.LandingStateNotObservable {
		t.Errorf("answer.state = %q, want %q", res.Answer.State, agent.LandingStateNotObservable)
	}
	if res.Summary.MissingClusters != 1 {
		t.Errorf("expected MissingClusters=1, got %+v", res.Summary)
	}
	foundUnknown := false
	for _, u := range res.Answer.Unknowns {
		if u.Kind == mapsvc.UnknownNotObservable && strings.Contains(u.What, "prod-east") {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Errorf("expected explicit not-observable unknown for prod-east, got %+v", res.Answer.Unknowns)
	}
}

func TestReceiptVerifyFleet_RevisionMismatch_NotLanded(t *testing.T) {
	resetReceiptVerifyFleetFlags(t)
	tmp := t.TempDir()
	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	_ = os.WriteFile(pubPath, pubPEM, 0o644)

	fixedNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	receiptVerifyFleetNow = func() time.Time { return fixedNow }

	f1 := writeSignedLandedReceipt(t, tmp, "dev.signed.json", "dev", "a1b2c3d", agent.LandingStateLanded, fixedNow, privPEM)
	f2 := writeSignedLandedReceipt(t, tmp, "prod.signed.json", "prod", "9999999", agent.LandingStateLanded, fixedNow, privPEM)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{
		"receipt", "verify-fleet",
		"--inputs", f1,
		"--inputs", f2,
		"--pubkey", pubPath,
		"--revision", "a1b2c3d",
		"--format", "json",
	})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit on revision mismatch")
	}

	var res fleetReceiptVerificationOutput
	if uErr := json.Unmarshal(out.Bytes(), &res); uErr != nil {
		t.Fatalf("unmarshal fleet json: %v", uErr)
	}
	if res.Answer.State != agent.LandingStateNotLanded {
		t.Errorf("answer.state = %q, want %q", res.Answer.State, agent.LandingStateNotLanded)
	}
	if res.Summary.RevisionMismatch != 1 {
		t.Errorf("expected RevisionMismatch=1, got %+v", res.Summary)
	}
}

func TestReceiptVerifyFleet_MaxAgeFlagsStaleUnknown_AndASCIIOutput(t *testing.T) {
	resetReceiptVerifyFleetFlags(t)
	tmp := t.TempDir()
	privPEM, pubPEM, _, err := agent.GenerateEd25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubPath := filepath.Join(tmp, "pub.pem")
	_ = os.WriteFile(pubPath, pubPEM, 0o644)

	fixedNow := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	receiptVerifyFleetNow = func() time.Time { return fixedNow }

	f1 := writeSignedLandedReceipt(t, tmp, "dev.signed.json", "dev", "a1b2c3d", agent.LandingStateLanded, fixedNow.Add(-2*time.Hour), privPEM)

	var jsonOut bytes.Buffer
	rootCmd.SetOut(&jsonOut)
	rootCmd.SetArgs([]string{
		"receipt", "verify-fleet",
		"--inputs", f1,
		"--pubkey", pubPath,
		"--max-age", "30m",
		"--format", "json",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var res fleetReceiptVerificationOutput
	if err := json.Unmarshal(jsonOut.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if res.Summary.StaleReceipts != 1 {
		t.Errorf("expected StaleReceipts=1, got %+v", res.Summary)
	}
	foundStale := false
	for _, u := range res.Answer.Unknowns {
		if u.Kind == mapsvc.UnknownStale {
			foundStale = true
		}
	}
	if !foundStale {
		t.Errorf("expected explicit stale unknown, got %+v", res.Answer.Unknowns)
	}

	// Verify compact answer envelope byte ceiling even with a stale unknown.
	ansBytes, err := json.Marshal(res.Answer)
	if err != nil {
		t.Fatalf("marshal answer: %v", err)
	}
	if len(ansBytes) > 700 {
		t.Errorf("answer envelope with stale unknown is %d bytes, exceeds 700-byte ceiling: %s", len(ansBytes), string(ansBytes))
	}

	// Verify ASCII rendering works cleanly too.
	resetReceiptVerifyFleetFlags(t)
	receiptVerifyFleetNow = func() time.Time { return fixedNow }
	var asciiOut bytes.Buffer
	rootCmd.SetOut(&asciiOut)
	rootCmd.SetArgs([]string{
		"receipt", "verify-fleet",
		"--inputs", f1,
		"--pubkey", pubPath,
		"--max-age", "30m",
		"--format", "ascii",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ascii verify-fleet failed: %v", err)
	}
	if !strings.Contains(asciiOut.String(), "FLEET VERIFY: landed") || !strings.Contains(asciiOut.String(), "[STALE]") {
		t.Errorf("unexpected ascii output:\n%s", asciiOut.String())
	}
}
