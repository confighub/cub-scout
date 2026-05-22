// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

// resetReceiptFailOnFlag clears the --fail-on flag between tests.
func resetReceiptFailOnFlag(t *testing.T) {
	t.Helper()
	prev := receiptFailOn
	receiptFailOn = ""
	t.Cleanup(func() { receiptFailOn = prev })
}

// --- parseReceiptFailOn unit tests ---

func TestParseReceiptFailOn_AnyNonPass(t *testing.T) {
	set, err := parseReceiptFailOn("any-non-pass")
	if err != nil {
		t.Fatalf("any-non-pass should parse: %v", err)
	}
	for _, v := range []agent.ReceiptVerdict{agent.VerdictWATCH, agent.VerdictBLOCK, agent.VerdictINCONCLUSIVE} {
		if !set[v] {
			t.Errorf("any-non-pass must include %q in fail set", v)
		}
	}
	if set[agent.VerdictPASS] {
		t.Error("any-non-pass must NOT include PASS")
	}
}

func TestParseReceiptFailOn_SingleVerdict(t *testing.T) {
	cases := []struct {
		in      string
		matches agent.ReceiptVerdict
		others  []agent.ReceiptVerdict
	}{
		{"WATCH", agent.VerdictWATCH, []agent.ReceiptVerdict{agent.VerdictBLOCK, agent.VerdictINCONCLUSIVE}},
		{"BLOCK", agent.VerdictBLOCK, []agent.ReceiptVerdict{agent.VerdictWATCH, agent.VerdictINCONCLUSIVE}},
		{"INCONCLUSIVE", agent.VerdictINCONCLUSIVE, []agent.ReceiptVerdict{agent.VerdictWATCH, agent.VerdictBLOCK}},
	}
	for _, tc := range cases {
		set, err := parseReceiptFailOn(tc.in)
		if err != nil {
			t.Errorf("%s: parse failed: %v", tc.in, err)
			continue
		}
		if !set[tc.matches] {
			t.Errorf("%s: must include %q", tc.in, tc.matches)
		}
		for _, other := range tc.others {
			if set[other] {
				t.Errorf("%s: must NOT include %q", tc.in, other)
			}
		}
	}
}

func TestParseReceiptFailOn_CommaList(t *testing.T) {
	set, err := parseReceiptFailOn("WATCH,BLOCK")
	if err != nil {
		t.Fatalf("WATCH,BLOCK should parse: %v", err)
	}
	if !set[agent.VerdictWATCH] {
		t.Error("WATCH,BLOCK must include WATCH")
	}
	if !set[agent.VerdictBLOCK] {
		t.Error("WATCH,BLOCK must include BLOCK")
	}
	if set[agent.VerdictINCONCLUSIVE] {
		t.Error("WATCH,BLOCK must NOT include INCONCLUSIVE")
	}
}

func TestParseReceiptFailOn_CaseInsensitive(t *testing.T) {
	for _, in := range []string{"watch", "Watch", "WATCH", " watch "} {
		set, err := parseReceiptFailOn(in)
		if err != nil {
			t.Errorf("%q must parse case-insensitively: %v", in, err)
			continue
		}
		if !set[agent.VerdictWATCH] {
			t.Errorf("%q must resolve to WATCH", in)
		}
	}
}

func TestParseReceiptFailOn_RejectsPASS(t *testing.T) {
	_, err := parseReceiptFailOn("PASS")
	if err == nil {
		t.Fatal("--fail-on PASS must be rejected")
	}
	if !strings.Contains(err.Error(), "PASS") {
		t.Errorf("error must name the problem; got %v", err)
	}
}

func TestParseReceiptFailOn_RejectsUnknownVerdict(t *testing.T) {
	_, err := parseReceiptFailOn("MAYBE")
	if err == nil {
		t.Fatal("unknown verdict must be rejected")
	}
	if !strings.Contains(err.Error(), "MAYBE") {
		t.Errorf("error must echo the bad input; got %v", err)
	}
}

func TestParseReceiptFailOn_EmptyAfterParse_Errors(t *testing.T) {
	// "," alone resolves to empty after trim; not a valid gate spec.
	_, err := parseReceiptFailOn(",")
	if err == nil {
		t.Fatal(",-only input must be rejected")
	}
}

// --- end-to-end CLI integration tests ---

func TestReceiptVerify_FailOn_BlockTriggersExit2(t *testing.T) {
	// Force a BLOCK verdict by injecting a strategy-mismatched
	// source-truth evidence. The receipt builds successfully; --fail-on
	// then turns the verdict into an exit-code-2 error wrap.
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	// Evidence says git-argo; caller says confighub-oci-argo →
	// strategy mismatch → BLOCK.
	withFakeSourceTruth(t, &agent.SourceTruthEvidence{
		DeclaredStrategy: agent.StrategyGitArgo.Human(),
		Status:           agent.StatusPASS,
	})
	receiptFailOn = "BLOCK"

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--strategy", "confighub-oci-argo",
		"--format", "json",
	})

	captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("BLOCK verdict + --fail-on BLOCK must error")
		}
		if !strings.Contains(err.Error(), "fail-on") {
			t.Errorf("error must mention --fail-on; got %v", err)
		}
		var ec interface{ ExitCode() int }
		if !errors.As(err, &ec) {
			t.Fatal("error must wrap exitCodeError so main.go exits 2")
		}
		if got := ec.ExitCode(); got != 2 {
			t.Errorf("--fail-on BLOCK on BLOCK verdict must signal exit 2; got %d", got)
		}
	})
}

func TestReceiptVerify_FailOn_PassDoesNotTrigger(t *testing.T) {
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	withFakeSourceTruth(t, &agent.SourceTruthEvidence{
		DeclaredStrategy: agent.StrategyGitArgo.Human(),
		Status:           agent.StatusPASS,
		SourceTruth:      agent.VerdictAGREED,
	})
	receiptFailOn = "any-non-pass"

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--strategy", "git-argo",
		"--format", "json",
	})

	captureStdout(t, func() {
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("PASS verdict + --fail-on any-non-pass must succeed (exit 0); got %v", err)
		}
	})
}

func TestReceiptVerify_FailOn_InconclusiveOnly(t *testing.T) {
	// Default applied-matches-spec with no Argo tracer wired = INCONCLUSIVE.
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	receiptFailOn = "INCONCLUSIVE"

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--format", "json",
	})

	captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("INCONCLUSIVE verdict + --fail-on INCONCLUSIVE must error")
		}
		var ec interface{ ExitCode() int }
		if !errors.As(err, &ec) || ec.ExitCode() != 2 {
			t.Errorf("INCONCLUSIVE + --fail-on INCONCLUSIVE must signal exit 2; got %v", err)
		}
	})
}

func TestReceiptVerify_FailOn_BadValue_NoExit2(t *testing.T) {
	// Bad --fail-on value is a usage error (operational), not a verdict
	// gate. Falls through to exit 1, not exit 2.
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	receiptFailOn = "MAYBE"

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--format", "json",
	})

	captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("bad --fail-on value must error")
		}
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			t.Errorf("bad --fail-on must NOT wrap exitCodeError (it's a usage error → default exit 1); got ExitCode=%d", ec.ExitCode())
		}
	})
}

// TestReceiptVerify_FailOn_BadValue_NoSideEffects verifies the Codex
// round-6 P2 fix: an invalid --fail-on value rejects the command
// BEFORE any side effect (stdout print, --out write, --save to store).
// The prior order parsed --fail-on after artifact emission, which left
// a footgun in CI: a typo would error out (correctly with exit 1) but
// the artifact had already been printed and persisted with the wrong
// gate semantics.
func TestReceiptVerify_FailOn_BadValue_NoSideEffects(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	saveDir := t.TempDir()
	outDir := t.TempDir() // separate from saveDir so neither read pollutes the other
	outPath := outDir + "/should-not-be-written.receipt.json"
	receiptSave = true
	receiptSaveDir = saveDir

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--format", "json",
		"--out", outPath,
		"--fail-on", "GARBAGE-VALUE",
	})

	out := captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("bad --fail-on must reject upfront")
		}
		var ec interface{ ExitCode() int }
		if errors.As(err, &ec) {
			t.Errorf("bad --fail-on must NOT wrap exitCodeError; got ExitCode=%d", ec.ExitCode())
		}
	})

	// stdout must be empty — the receipt was never printed.
	if strings.Contains(out, "\"_type\":") {
		t.Errorf("expected NO receipt on stdout when --fail-on is invalid; got:\n%s", out)
	}

	// --out file must not exist — the receipt was never written.
	if _, err := os.Stat(outPath); err == nil {
		t.Errorf("--out file %q must NOT be written when --fail-on is invalid", outPath)
	}

	// --save store must be empty — the receipt was never persisted.
	entries, err := os.ReadDir(saveDir)
	if err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".receipt.json") {
				t.Errorf("save store must be empty when --fail-on is invalid; found %s", e.Name())
			}
		}
	}
}

// TestReceiptVerify_FailOn_PreservesArtifact verifies that --fail-on
// triggering exit 2 still produces the receipt as stdout + --save +
// --out output. The artifact is a postmortem requirement; the gate
// triggers AFTER the artifact is durable.
func TestReceiptVerify_FailOn_PreservesArtifact(t *testing.T) {
	resetReceiptBatch3Flags(t)
	resetReceiptBatch2Flags(t)
	resetReceiptFailOnFlag(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())
	storeDir := t.TempDir()
	receiptSave = true
	receiptSaveDir = storeDir
	receiptFailOn = "any-non-pass"

	rootCmd.SetArgs([]string{
		"receipt", "verify", "deploy/api",
		"-n", "prod",
		"--format", "json",
	})

	out := captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("INCONCLUSIVE verdict + --fail-on any-non-pass must error")
		}
		// We want the failure exit code, but ALSO the artifact.
	})

	// stdout should still contain the JSON receipt.
	if !strings.Contains(out, "\"_type\": \"https://in-toto.io/Statement/v1\"") {
		t.Errorf("expected JSON receipt on stdout despite --fail-on; got:\n%s", out)
	}

	// --save should still have written the artifact to the store.
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
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected receipt saved to store despite --fail-on; store entries: %v", names)
	}
}
