// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReceiptExamplesAreFresh re-runs the gen-receipt-examples tool and
// compares its output to the committed files under examples/receipts/.
// Drift fails the build — the example receipts are a contract surface,
// not loose docs. Both batch 1 (applied-matches-spec) and batch 2
// (source-truth-pass, no-manual-edits-since) subdirectories are covered.
//
// To regenerate after an intentional change to the generator or to a
// pkg/agent type that changes the wire format:
//
//	go run ./tools/gen-receipt-examples examples/receipts
func TestReceiptExamplesAreFresh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("go run via exec is awkward on Windows; the linux/macos CI matrix catches drift")
	}

	repoRoot := findRepoRoot(t)

	tmp := t.TempDir()
	cmd := exec.Command("go", "run", "./tools/gen-receipt-examples", tmp)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("regenerate examples: %v\noutput:\n%s", err, out)
	}

	// Each receipt-predicate subdirectory under examples/receipts/ is a
	// contract surface. The generator writes the same layout into the
	// tmp dir; walk both and diff.
	predicateDirs := []string{
		"applied-matches-spec",
		"source-truth-pass",
		"no-manual-edits-since",
	}

	for _, predicateDir := range predicateDirs {
		committedDir := filepath.Join(repoRoot, "examples", "receipts", predicateDir)
		regenDir := filepath.Join(tmp, predicateDir)

		committedEntries, err := os.ReadDir(committedDir)
		if err != nil {
			t.Errorf("read committed dir %s: %v", committedDir, err)
			continue
		}
		var committedJSON []string
		for _, e := range committedEntries {
			if strings.HasSuffix(e.Name(), ".json") {
				committedJSON = append(committedJSON, e.Name())
			}
		}
		if len(committedJSON) == 0 {
			t.Errorf("no .json files in %s; the example directory must contain receipts", committedDir)
			continue
		}

		regenEntries, err := os.ReadDir(regenDir)
		if err != nil {
			t.Errorf("read regen dir %s: %v", regenDir, err)
			continue
		}
		regenSet := map[string]bool{}
		for _, e := range regenEntries {
			if strings.HasSuffix(e.Name(), ".json") {
				regenSet[e.Name()] = true
			}
		}

		for _, name := range committedJSON {
			if !regenSet[name] {
				t.Errorf("[%s] committed file %q is no longer produced by the generator; regenerate or update the generator", predicateDir, name)
			}
		}
		for name := range regenSet {
			found := false
			for _, c := range committedJSON {
				if c == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("[%s] generator produces %q but it is not committed; run `go run ./tools/gen-receipt-examples examples/receipts`", predicateDir, name)
			}
		}

		for _, name := range committedJSON {
			got, err := os.ReadFile(filepath.Join(regenDir, name))
			if err != nil {
				continue
			}
			want, err := os.ReadFile(filepath.Join(committedDir, name))
			if err != nil {
				t.Errorf("read committed %s/%s: %v", predicateDir, name, err)
				continue
			}
			if !bytes.Equal(got, want) {
				t.Errorf("committed %s/%s differs from generator output; run `go run ./tools/gen-receipt-examples examples/receipts`", predicateDir, name)
			}
		}
	}
}

// findRepoRoot is defined in tui_snapshot_test.go and reused here.
