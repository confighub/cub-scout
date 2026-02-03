// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package golden provides helpers for ASCII golden tests.
//
// NOTE: These goldens lock user-facing ASCII output.
// Changes here should be reviewed as UX changes.
//
// To update goldens: go test ./test/ascii/... -update
package golden

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

// Normalize makes CLI output stable across platforms and minor environment differences.
func Normalize(s string) string {
	// Strip ANSI escape codes (colors)
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	s = ansiRegex.ReplaceAllString(s, "")

	// Normalize newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")

	// Trim trailing whitespace on each line
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	s = strings.Join(lines, "\n")

	// Ensure the file ends with exactly one newline
	s = strings.TrimRight(s, "\n") + "\n"

	// Scrub obviously dynamic tokens if they ever appear.
	// Keep this list short; prefer fixing determinism in production code.
	replacements := []struct {
		re   *regexp.Regexp
		with string
	}{
		// Pod UIDs (5-char suffix)
		{regexp.MustCompile(`-[a-z0-9]{5}$`), "-<UID>"},
		// Timestamps in various formats
		{regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`), "<TIMESTAMP>"},
		// Relative times like "12m ago", "2h ago"
		{regexp.MustCompile(`\d+[smhd] ago`), "<AGO>"},
	}
	for _, r := range replacements {
		s = r.re.ReplaceAllString(s, r.with)
	}

	return s
}

// AssertGolden compares output against a golden file.
// If -update flag is set, updates the golden file instead.
func AssertGolden(t *testing.T, got string, goldenPath string) {
	t.Helper()

	got = Normalize(got)

	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(goldenPath), err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		t.Logf("updated golden: %s", goldenPath)
		return
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v\n\nRun with -update to create it:\n  go test ./test/ascii/... -update", goldenPath, err)
	}
	want := Normalize(string(wantBytes))

	if got != want {
		t.Fatalf("golden mismatch: %s\n\n--- WANT ---\n%s\n--- GOT ---\n%s",
			goldenPath, trimForError(want), trimForError(got))
	}
}

func trimForError(s string) string {
	// Keep errors readable if the output is huge
	const max = 8000
	if len(s) <= max {
		return s
	}
	var buf bytes.Buffer
	buf.WriteString(s[:max])
	buf.WriteString("\n…(truncated)…\n")
	return buf.String()
}
