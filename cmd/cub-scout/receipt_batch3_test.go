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

// resetReceiptBatch3Flags zeros the show/validate/list flags so
// consecutive tests don't bleed state.
func resetReceiptBatch3Flags(t *testing.T) {
	t.Helper()
	prev := struct {
		showFmt, valFmt, listDir, listFmt string
		save                              bool
		saveDir                           string
	}{
		showFmt: receiptShowFormat,
		valFmt:  receiptValidateFormat,
		listDir: receiptListDir,
		listFmt: receiptListFormat,
		save:    receiptSave,
		saveDir: receiptSaveDir,
	}
	receiptShowFormat = "ascii"
	receiptValidateFormat = "ascii"
	receiptListDir = ""
	receiptListFormat = "ascii"
	receiptSave = false
	receiptSaveDir = ""
	t.Cleanup(func() {
		receiptShowFormat = prev.showFmt
		receiptValidateFormat = prev.valFmt
		receiptListDir = prev.listDir
		receiptListFormat = prev.listFmt
		receiptSave = prev.save
		receiptSaveDir = prev.saveDir
	})
}

// seedReceiptOnDisk runs `receipt verify --out` against a fake live
// object and returns the path of the receipt on disk.
func seedReceiptOnDisk(t *testing.T) string {
	t.Helper()
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "seed.receipt.json")
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--format", "json",
			"--out", outPath,
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("seed verify: %v", err)
		}
	})
	return outPath
}

// --- receipt show -----------------------------------------------------

func TestReceiptShow_ASCII_RendersFromDisk(t *testing.T) {
	resetReceiptBatch3Flags(t)
	path := seedReceiptOnDisk(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "show", path})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt show: %v", err)
		}
	})
	for _, want := range []string{
		"Receipt: ",
		"Verdict: ",
		"Scope:   Deployment/api in prod",
		"Fingerprint: ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in show ASCII; got:\n%s", want, out)
		}
	}
}

func TestReceiptShow_JSON_RendersFromDisk(t *testing.T) {
	resetReceiptBatch3Flags(t)
	path := seedReceiptOnDisk(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "show", path, "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt show --format json: %v", err)
		}
	})
	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("show json output is not a valid Statement: %v\nraw:\n%s", err, out)
	}
	if stmt.Type != agent.StatementType {
		t.Errorf("Statement type: got %q, want %q", stmt.Type, agent.StatementType)
	}
}

func TestReceiptShow_MissingFile_Errors(t *testing.T) {
	resetReceiptBatch3Flags(t)
	rootCmd.SetArgs([]string{"receipt", "show", "/tmp/does-not-exist-receipt.json"})
	if err := rootCmd.Execute(); err == nil {
		t.Error("expected error for missing file")
	}
}

// --- receipt validate -------------------------------------------------

func TestReceiptValidate_HappyPath_OK(t *testing.T) {
	resetReceiptBatch3Flags(t)
	path := seedReceiptOnDisk(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "validate", path})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt validate: %v", err)
		}
	})
	if !strings.Contains(out, "Fingerprint OK") {
		t.Errorf("expected 'Fingerprint OK' in validate output; got:\n%s", out)
	}
}

func TestReceiptValidate_TamperedReceipt_Errors(t *testing.T) {
	resetReceiptBatch3Flags(t)
	path := seedReceiptOnDisk(t)

	// Read, mutate the verdict, write back. Fingerprint must mismatch.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	tampered := strings.ReplaceAll(string(data), `"verdict": "PASS"`, `"verdict": "BLOCK"`)
	if tampered == string(data) {
		// The seed receipt might be INCONCLUSIVE; tamper that instead.
		tampered = strings.ReplaceAll(string(data), `"verdict": "INCONCLUSIVE"`, `"verdict": "PASS"`)
	}
	if tampered == string(data) {
		t.Fatalf("seed receipt verdict not mutateable; raw:\n%s", string(data))
	}
	if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
		t.Fatalf("write tampered: %v", err)
	}

	rootCmd.SetArgs([]string{"receipt", "validate", path})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected fingerprint mismatch error on tampered receipt")
	}
	if !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Errorf("error must mention fingerprint mismatch; got %v", err)
	}
}

func TestReceiptValidate_JSON_StructuredOutput(t *testing.T) {
	resetReceiptBatch3Flags(t)
	path := seedReceiptOnDisk(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "validate", path, "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("validate --format json: %v", err)
		}
	})
	var result receiptValidateResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("validate json is not valid receiptValidateResult: %v\nraw:\n%s", err, out)
	}
	if !result.Valid {
		t.Errorf("expected Valid=true; got %+v", result)
	}
	if result.Fingerprint == "" {
		t.Error("expected fingerprint in JSON output")
	}
}

// --- receipt list -----------------------------------------------------

func TestReceiptList_EmptyStore_PrintsZeroState(t *testing.T) {
	resetReceiptBatch3Flags(t)
	tmp := t.TempDir()
	receiptListDir = tmp

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "list", "--dir", tmp})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt list: %v", err)
		}
	})
	if !strings.Contains(out, "(no receipts found)") {
		t.Errorf("empty store must print zero-state message; got:\n%s", out)
	}
}

func TestReceiptList_PopulatedStore_PrintsRows(t *testing.T) {
	resetReceiptBatch3Flags(t)
	storeDir := t.TempDir()

	// Save two receipts via --save into the store.
	for i := 0; i < 2; i++ {
		resetReceiptFlags(t)
		withFakeReceiptLoader(t, makeReceiptArgoLive())
		receiptSave = true
		receiptSaveDir = storeDir
		// Different namespaces so the second save doesn't dedupe.
		ns := "prod"
		if i == 1 {
			ns = "staging"
		}
		captureStdout(t, func() {
			rootCmd.SetArgs([]string{
				"receipt", "verify", "deploy/api",
				"-n", ns,
				"--format", "json",
			})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("seed verify %d: %v", i, err)
			}
		})
	}
	// receiptSave / receiptSaveDir are cleared by the inner reset
	// each iteration; reset them here for the list call.
	resetReceiptBatch3Flags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "list", "--dir", storeDir})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt list: %v", err)
		}
	})
	if !strings.Contains(out, "VERDICT") || !strings.Contains(out, "PREDICATE") {
		t.Errorf("list ASCII must include header; got:\n%s", out)
	}
	// Two entries → both namespaces present.
	if !strings.Contains(out, "Deployment/api in prod") || !strings.Contains(out, "Deployment/api in staging") {
		t.Errorf("expected both scopes in list output; got:\n%s", out)
	}
}

func TestReceiptList_JSON_RoundTrips(t *testing.T) {
	resetReceiptBatch3Flags(t)
	storeDir := t.TempDir()

	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	receiptSave = true
	receiptSaveDir = storeDir
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("seed: %v", err)
		}
	})
	resetReceiptBatch3Flags(t)

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "list", "--dir", storeDir, "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("list --format json: %v", err)
		}
	})
	var entries []agent.ReceiptListEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("list JSON output is not []ReceiptListEntry: %v\nraw:\n%s", err, out)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry; got %d", len(entries))
	}
}

// --- receipt verify --save --------------------------------------------

func TestReceiptVerify_Save_WritesToStore(t *testing.T) {
	resetReceiptBatch3Flags(t)
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	storeDir := t.TempDir()
	receiptSave = true
	receiptSaveDir = storeDir

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --save: %v", err)
		}
	})

	entries, err := os.ReadDir(storeDir)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".receipt.json") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a .receipt.json under %s; got entries: %v", storeDir, entries)
	}
}

func TestReceiptVerify_Save_DefaultStoreHonorsEnv(t *testing.T) {
	resetReceiptBatch3Flags(t)
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	// Point the default store env var at a tempdir; --save (no --save-dir)
	// must land there.
	tmp := t.TempDir()
	t.Setenv("CUB_SCOUT_RECEIPTS_DIR", tmp)
	receiptSave = true

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --save: %v", err)
		}
	})

	entries, _ := os.ReadDir(tmp)
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".receipt.json") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("CUB_SCOUT_RECEIPTS_DIR must be honored by --save; got entries: %v", entries)
	}
}
