// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrationGuide30_BeforeAndAfterCommands builds the cub-scout binary and
// executes every before/after migration pair from
// docs/releases/v3.0.0-migration.md (#595).
//
// Every "before" command (2.x surface removed in 3.0) must fail with a
// non-zero exit code, mention "removed in 3.0", and name its replacement.
// Every offline-safe "after" command must succeed.
func TestMigrationGuide30_BeforeAndAfterCommands(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	guidePath := filepath.Join(root, "docs", "releases", "v3.0.0-migration.md")
	raw, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("read %s: %v", guidePath, err)
	}
	guide := string(raw)

	bin := filepath.Join(t.TempDir(), "cub-scout")
	build := exec.Command("go", "build", "-o", bin, "./cmd/cub-scout")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build cub-scout: %v\n%s", err, out)
	}

	beforeCases := []struct {
		docSnippet  string
		args        []string
		wantReplace string
	}{
		{
			docSnippet:  "`cub scout trace deploy/app -n demo --json`",
			args:        []string{"trace", "deploy/app", "-n", "demo", "--json"},
			wantReplace: "--format json",
		},
		{
			docSnippet:  "`cub scout tree --json`",
			args:        []string{"tree", "--json"},
			wantReplace: "--format json",
		},
		{
			docSnippet:  "`cub scout graph export --empty --json`",
			args:        []string{"graph", "export", "--empty", "--json"},
			wantReplace: "--format json",
		},
		{
			docSnippet:  "`cub scout gitops status --json`",
			args:        []string{"gitops", "status", "--json"},
			wantReplace: "--format json",
		},
		{
			docSnippet:  "`cub scout connect --go`",
			args:        []string{"connect", "--go"},
			wantReplace: "--map",
		},
		{
			docSnippet:  "`cub scout app create demo --set-context`",
			args:        []string{"app", "create", "demo", "--set-context"},
			wantReplace: "--space",
		},
		{
			docSnippet:  "`cub scout hierarchy`",
			args:        []string{"hierarchy"},
			wantReplace: "map --hub",
		},
		{
			docSnippet:  "`cub scout fleet outliers`",
			args:        []string{"fleet", "outliers"},
			wantReplace: "docs/migration-3.0.md",
		},
	}

	for _, tc := range beforeCases {
		t.Run("before/"+strings.Join(tc.args, "_"), func(t *testing.T) {
			if !strings.Contains(guide, tc.docSnippet) {
				t.Fatalf("docs/releases/v3.0.0-migration.md missing before command %s", tc.docSnippet)
			}
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("before command %v exited 0; want non-zero error\noutput:\n%s", tc.args, out)
			}
			text := string(out)
			if !strings.Contains(text, "removed in 3.0") || !strings.Contains(text, tc.wantReplace) {
				t.Fatalf("before command %v output = %q; want both %q and %q", tc.args, text, "removed in 3.0", tc.wantReplace)
			}
		})
	}

	// Verify the "after" replacements documented in the guide work against the
	// built binary (offline-safe executions and --help flag checks).
	afterHelpCases := []struct {
		docSnippet string
		args       []string
		wantFlag   string
	}{
		{
			docSnippet: "`cub scout trace deploy/app -n demo --format json`",
			args:       []string{"trace", "--help"},
			wantFlag:   "--format",
		},
		{
			docSnippet: "`cub scout tree --format json`",
			args:       []string{"tree", "--help"},
			wantFlag:   "--format",
		},
		{
			docSnippet: "`cub scout gitops status --format json`",
			args:       []string{"gitops", "status", "--help"},
			wantFlag:   "--format",
		},
		{
			docSnippet: "`cub scout connect --map`",
			args:       []string{"connect", "--help"},
			wantFlag:   "--map",
		},
		{
			docSnippet: "`cub scout app create demo`",
			args:       []string{"app", "create", "--help"},
			wantFlag:   "create",
		},
		{
			docSnippet: "`cub scout map hub`",
			args:       []string{"map", "hub", "--help"},
			wantFlag:   "hub",
		},
	}

	for _, tc := range afterHelpCases {
		t.Run("after/"+strings.Join(tc.args, "_"), func(t *testing.T) {
			if !strings.Contains(guide, tc.docSnippet) {
				t.Fatalf("docs/releases/v3.0.0-migration.md missing after command %s", tc.docSnippet)
			}
			cmd := exec.Command(bin, tc.args...)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("after command %v failed: %v\n%s", tc.args, err, out)
			}
			if !strings.Contains(string(out), tc.wantFlag) {
				t.Fatalf("after command %v output missing %q\n%s", tc.args, tc.wantFlag, out)
			}
		})
	}

	t.Run("after/graph_export_empty_format_json", func(t *testing.T) {
		const snippet = "`cub scout graph export --empty --format json`"
		if !strings.Contains(guide, snippet) {
			t.Fatalf("docs/releases/v3.0.0-migration.md missing %s", snippet)
		}
		cmd := exec.Command(bin, "graph", "export", "--empty", "--format", "json")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("graph export --empty --format json failed: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("graph export --empty --format json did not emit valid JSON: %v\n%s", err, out)
		}
	})
}
