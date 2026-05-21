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
// compares its output to the committed files under
// examples/receipts/applied-matches-spec. Drift fails the build — the
// example receipts are a contract surface, not loose docs.
//
// To regenerate after an intentional change to the generator or to a
// pkg/agent type that changes the wire format:
//
//	go run ./tools/gen-receipt-examples examples/receipts/applied-matches-spec
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

	committedDir := filepath.Join(repoRoot, "examples", "receipts", "applied-matches-spec")
	committedEntries, err := os.ReadDir(committedDir)
	if err != nil {
		t.Fatalf("read committed dir %s: %v", committedDir, err)
	}

	var committedJSON []string
	for _, e := range committedEntries {
		if strings.HasSuffix(e.Name(), ".json") {
			committedJSON = append(committedJSON, e.Name())
		}
	}
	if len(committedJSON) == 0 {
		t.Fatalf("no .json files in %s; the example directory must contain receipts", committedDir)
	}

	regenEntries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("read tmp output dir: %v", err)
	}
	regenSet := map[string]bool{}
	for _, e := range regenEntries {
		if strings.HasSuffix(e.Name(), ".json") {
			regenSet[e.Name()] = true
		}
	}

	// Same set of files (no rename / addition / deletion drift).
	for _, name := range committedJSON {
		if !regenSet[name] {
			t.Errorf("committed file %q is no longer produced by the generator; regenerate or update the generator", name)
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
			t.Errorf("generator produces %q but it is not committed; run `go run ./tools/gen-receipt-examples examples/receipts/applied-matches-spec`", name)
		}
	}

	// Byte-for-byte content match.
	for _, name := range committedJSON {
		got, err := os.ReadFile(filepath.Join(tmp, name))
		if err != nil {
			continue // file-set mismatch already reported above
		}
		want, err := os.ReadFile(filepath.Join(committedDir, name))
		if err != nil {
			t.Errorf("read committed %s: %v", name, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("committed %s differs from generator output; run `go run ./tools/gen-receipt-examples examples/receipts/applied-matches-spec`", name)
		}
	}
}

// findRepoRoot is defined in tui_snapshot_test.go and reused here.
