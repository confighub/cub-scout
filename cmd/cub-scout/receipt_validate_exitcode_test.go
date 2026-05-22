// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestReceiptValidate_ExitCodes_RunEReturn verifies that runReceiptValidate
// returns an *exitCodeError with the documented exit code for each error
// class. main.go's errors.As dispatcher then maps those to the actual
// process exit code (0 OK, 1 mismatch, 2 I/O).
//
// These are unit-level tests against the RunE return value rather than
// subprocess-level tests because subprocess-based testing in this
// package would require building a full binary; the contract surface
// is "RunE returns an exitCodeError with the right code", which is
// what main.go's dispatcher consumes.
func TestReceiptValidate_ExitCode_FingerprintMismatch_Is1(t *testing.T) {
	resetReceiptBatch3Flags(t)
	path := seedReceiptOnDisk(t)

	// Tamper the verdict so the fingerprint no longer matches.
	tamperReceiptVerdict(t, path)

	rootCmd.SetArgs([]string{"receipt", "validate", path})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error on tampered receipt")
	}
	if !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Errorf("error must say 'fingerprint mismatch'; got %v", err)
	}

	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) {
		t.Fatal("error must wrap an exitCodeError so main.go dispatches the right exit code")
	}
	if got := ec.ExitCode(); got != 1 {
		t.Errorf("fingerprint mismatch must signal exit code 1; got %d", got)
	}
}

func TestReceiptValidate_ExitCode_MissingFile_Is2(t *testing.T) {
	resetReceiptBatch3Flags(t)

	rootCmd.SetArgs([]string{"receipt", "validate", "/tmp/this/does/not/exist.receipt.json"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected I/O error for missing file")
	}

	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) {
		t.Fatal("error must wrap an exitCodeError")
	}
	if got := ec.ExitCode(); got != 2 {
		t.Errorf("I/O / missing-file error must signal exit code 2; got %d", got)
	}
}

func TestReceiptValidate_ExitCode_OK_NoError(t *testing.T) {
	resetReceiptBatch3Flags(t)
	path := seedReceiptOnDisk(t)

	rootCmd.SetArgs([]string{"receipt", "validate", path})
	captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("validate must succeed on a fresh seed; got %v", err)
		}
	})
	// No error → cobra propagates exit 0. No exitCodeError needed.
}

func TestReceiptValidate_ExitCode_BadFormat_Is1(t *testing.T) {
	// Usage errors (invalid flag value) fall through to the default
	// cobra/main path → exit code 1. They are NOT wrapped in
	// exitCodeError because they're not part of the documented 0/1/2
	// validate-specific contract.
	resetReceiptBatch3Flags(t)
	path := seedReceiptOnDisk(t)

	rootCmd.SetArgs([]string{"receipt", "validate", path, "--format", "yaml"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --format")
	}

	var ec interface{ ExitCode() int }
	if errors.As(err, &ec) {
		t.Errorf("usage errors must NOT wrap exitCodeError (they fall through to default exit 1); got ExitCode=%d", ec.ExitCode())
	}
}

// tamperReceiptVerdict reads a receipt JSON, swaps a substring, and
// writes it back — mirroring TestReceiptValidate_TamperedReceipt_Errors
// in receipt_batch3_test.go but pulled into a helper so multiple tests
// can use it.
func tamperReceiptVerdict(t *testing.T, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	data := string(raw)
	tampered := strings.ReplaceAll(data, `"verdict": "PASS"`, `"verdict": "BLOCK"`)
	if tampered == data {
		tampered = strings.ReplaceAll(data, `"verdict": "INCONCLUSIVE"`, `"verdict": "PASS"`)
	}
	if tampered == data {
		t.Fatalf("seed receipt has no PASS or INCONCLUSIVE verdict to mutate; raw:\n%s", data)
	}
	if err := os.WriteFile(path, []byte(tampered), 0o644); err != nil {
		t.Fatalf("write tampered: %v", err)
	}
}
