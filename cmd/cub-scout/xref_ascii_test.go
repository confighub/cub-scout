// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var updateXref = flag.Bool("update-xref", false, "update xref golden files")

func TestXrefOverlay_Basic(t *testing.T) {
	m := initialLocalModel()
	m.xrefMode = true
	m.xrefSourceType = "workload"
	m.xrefSourceName = "my-deploy"
	m.xrefCursor = 0
	m.xrefItems = []xrefItem{
		{Type: "workload", Kind: "Service", Name: "web", Namespace: "default", Relation: "exposes", TargetView: viewDashboard},
		{Type: "gitops", Kind: "Kustomization", Name: "my-app", Namespace: "flux-system", Relation: "managed by", TargetView: viewPipelines},
	}

	got := normalizeXref(m.renderXref())

	// Find repo root by walking up
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}

	goldenPath := filepath.Join(dir, "test", "ascii", "xref", "basic.txt")

	if *updateXref {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s", goldenPath)
		return
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v\n\nRun with -update-xref to create it", err)
	}
	want := normalizeXref(string(wantBytes))

	if got != want {
		t.Fatalf("golden mismatch:\n\n--- WANT ---\n%s\n--- GOT ---\n%s", want, got)
	}
}

func normalizeXref(s string) string {
	// Strip ANSI escape codes
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	s = ansiRegex.ReplaceAllString(s, "")
	// Normalize newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Trim trailing whitespace
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	s = strings.Join(lines, "\n")
	// End with exactly one newline
	s = strings.TrimRight(s, "\n") + "\n"
	return s
}
