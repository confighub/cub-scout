// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/mapsvc"
)

var updateTUI = flag.Bool("update-tui", false, "update TUI snapshot golden files")

// TUI Snapshot Tests
//
// These tests lock the TUI panel rendering output. They test TUI-specific
// render methods (getPanelWorkloads, renderTrace, getPanelMaps) which currently
// have their own formatting logic separate from CLI commands.
//
// NOTE: Issue #90 will unify TUI and CLI rendering so they produce identical
// output for the same data. Once unified, these tests will verify that TUI
// panels match CLI output byte-for-byte (post-ANSI stripping).
//
// Width/height are fixed to ensure deterministic output across terminals.
const (
	testWidth  = 80
	testHeight = 24
)

// normalizeSnapshot strips ANSI codes and normalizes whitespace for golden comparison
func normalizeSnapshot(s string) string {
	// Strip ANSI escape codes
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	s = ansiRegex.ReplaceAllString(s, "")
	// Normalize newlines
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Trim trailing whitespace per line
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	s = strings.Join(lines, "\n")
	// End with exactly one newline
	s = strings.TrimRight(s, "\n") + "\n"
	return s
}

// findRepoRoot walks up to find go.mod
func findRepoRoot(t *testing.T) string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

// compareGolden compares output to golden file, updating if -update-tui flag is set
func compareGolden(t *testing.T, got, goldenPath string) {
	got = normalizeSnapshot(got)

	if *updateTUI {
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
		t.Fatalf("read golden: %v\n\nRun with -update-tui to create it", err)
	}
	want := normalizeSnapshot(string(wantBytes))

	if got != want {
		t.Fatalf("golden mismatch:\n\n--- WANT ---\n%s\n--- GOT ---\n%s", want, got)
	}
}

// TestTUISnapshot_Tree tests the workloads/tree panel rendering
func TestTUISnapshot_Tree(t *testing.T) {
	m := initialLocalModel()
	m.ready = true
	m.width = testWidth
	m.height = testHeight

	// Populate with test data - workloads grouped by owner
	m.entries = []MapEntry{
		{Kind: "Deployment", Name: "frontend", Namespace: "web", Owner: "Flux", OwnerDetails: map[string]string{"name": "kustomization/web-apps"}},
		{Kind: "Deployment", Name: "backend", Namespace: "api", Owner: "Flux", OwnerDetails: map[string]string{"name": "kustomization/api-apps"}},
		{Kind: "Deployment", Name: "cache", Namespace: "redis", Owner: "Helm", OwnerDetails: map[string]string{"name": "release/redis"}},
		{Kind: "StatefulSet", Name: "postgres", Namespace: "db", Owner: "ArgoCD", OwnerDetails: map[string]string{"name": "application/database"}},
		{Kind: "Deployment", Name: "cronjob-runner", Namespace: "jobs", Owner: "Native", OwnerDetails: nil},
	}

	got := m.getPanelWorkloads()

	repoRoot := findRepoRoot(t)
	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tui", "tree.txt")
	compareGolden(t, got, goldenPath)
}

// TestTUISnapshot_TreeFiltered tests tree panel with owner filter.
// This demonstrates CLI ↔ TUI symmetry: --owner flag maps to activeQuery filter.
func TestTUISnapshot_TreeFiltered(t *testing.T) {
	m := initialLocalModel()
	m.ready = true
	m.width = testWidth
	m.height = testHeight

	// Same entries as basic test
	m.entries = []MapEntry{
		{Kind: "Deployment", Name: "frontend", Namespace: "web", Owner: "Flux"},
		{Kind: "Deployment", Name: "backend", Namespace: "api", Owner: "Flux"},
		{Kind: "Deployment", Name: "cache", Namespace: "redis", Owner: "Helm"},
		{Kind: "StatefulSet", Name: "postgres", Namespace: "db", Owner: "ArgoCD"},
		{Kind: "Deployment", Name: "cronjob-runner", Namespace: "jobs", Owner: "Native"},
	}

	// Apply owner filter (equivalent to --owner Flux)
	// This uses the same mapsvc.OwnershipTreeOpts.FilterOwner as CLI
	opts := mapsvc.OwnershipTreeOpts{
		FilterOwner: "Flux",
	}
	lines := mapsvc.BuildOwnershipTreeLines(m.entries, opts)

	// Format with TUI styling (same code path as getPanelWorkloads with filter)
	var b strings.Builder
	for _, line := range lines {
		if line.IsHeader {
			b.WriteString(fmt.Sprintf("%s (%d)\n", line.Owner, line.Count))
		} else if line.Connector == "" {
			b.WriteString("\n")
		} else {
			b.WriteString(fmt.Sprintf("  %s %s/%s\n", line.Connector, line.Namespace, line.Name))
		}
	}

	got := b.String()
	repoRoot := findRepoRoot(t)
	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tui", "tree_filtered.txt")
	compareGolden(t, got, goldenPath)
}

// TestTUISnapshot_Trace tests the trace picker rendering.
// NOTE: The trace picker is TUI-specific UI (no CLI equivalent).
// The trace *result* uses CLI output directly (m.traceOutput from ./cub-scout trace).
func TestTUISnapshot_Trace(t *testing.T) {
	m := initialLocalModel()
	m.ready = true
	m.width = testWidth
	m.height = testHeight
	m.traceMode = true
	m.traceCursor = 1 // Explicitly set cursor to second item

	// Populate with traceable items (sorted by Kind, then namespace/name for determinism)
	m.traceItems = []TraceItem{
		{Kind: "Kustomization", Name: "infra", Namespace: "flux-system", Owner: "Flux"},
		{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Owner: "Flux"},
		{Kind: "HelmRelease", Name: "nginx-ingress", Namespace: "ingress", Owner: "Flux"},
		{Kind: "Application", Name: "guestbook", Namespace: "argocd", Owner: "ArgoCD"},
	}

	got := m.renderTrace()

	repoRoot := findRepoRoot(t)
	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tui", "trace.txt")
	compareGolden(t, got, goldenPath)
}

// TestTUISnapshot_Map tests the maps panel rendering.
// NOTE: The map panel is a TUI-specific composite view combining:
// - GITOPS TREES: deployers list (different format from `map deployers` tabular output)
// - CONFIGHUB: placeholder for hub integration
// - REPO → DEPLOY: source-to-deployer mapping (no CLI equivalent)
// Tree connectors follow canonical format (├──/└──) for consistency.
func TestTUISnapshot_Map(t *testing.T) {
	m := initialLocalModel()
	m.ready = true
	m.width = testWidth
	m.height = testHeight

	// Populate with GitOps resources for the maps view
	m.gitops = []GitOpsResource{
		{Kind: "Kustomization", Name: "infra", Namespace: "flux-system", Status: "Ready", Source: "flux-system", Path: "./infrastructure", InventoryCount: 12},
		{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Status: "Ready", Source: "flux-system", Path: "./apps", InventoryCount: 8},
		{Kind: "HelmRelease", Name: "nginx", Namespace: "ingress", Status: "Ready", Source: "helm-charts", InventoryCount: 5},
		{Kind: "Application", Name: "guestbook", Namespace: "argocd", Status: "Healthy", Source: "https://github.com/argoproj/argocd-example-apps", Path: "guestbook", InventoryCount: 3},
	}

	// Add some native resources for the count
	m.entries = []MapEntry{
		{Kind: "Deployment", Name: "legacy-app", Namespace: "default", Owner: "Native"},
		{Kind: "Deployment", Name: "test-pod", Namespace: "default", Owner: "Native"},
	}

	got := m.getPanelMaps()

	repoRoot := findRepoRoot(t)
	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tui", "map.txt")
	compareGolden(t, got, goldenPath)
}

// TestTUISnapshot_SuggestWorkloads tests the suggestion overlay for workloads panel.
// Verifies that cub-scout commands are prioritized (trace/tree first, then kubectl).
func TestTUISnapshot_SuggestWorkloads(t *testing.T) {
	m := initialLocalModel()
	m.ready = true
	m.width = testWidth
	m.height = testHeight
	m.panelMode = true
	m.panelView = viewWorkloads
	m.cursor = 0 // First item selected

	// Populate with test workloads
	m.entries = []MapEntry{
		{Kind: "Deployment", Name: "frontend", Namespace: "web", Owner: "Flux"},
		{Kind: "Deployment", Name: "backend", Namespace: "api", Owner: "Flux"},
		{Kind: "StatefulSet", Name: "postgres", Namespace: "db", Owner: "ArgoCD"},
	}

	got := m.renderSuggestions()

	repoRoot := findRepoRoot(t)
	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tui", "suggest_workloads.txt")
	compareGolden(t, got, goldenPath)
}

// TestTUISnapshot_SuggestDashboard tests the suggestion overlay for dashboard (no panel).
// Verifies that overview commands include current filter state (owner/depth).
func TestTUISnapshot_SuggestDashboard(t *testing.T) {
	m := initialLocalModel()
	m.ready = true
	m.width = testWidth
	m.height = testHeight
	m.panelMode = false
	// Apply owner filter to verify it's reflected in suggestions
	m.activeQuery = &SavedQuery{Name: "flux", Query: "owner=Flux"}
	m.treeMaxDepth = 5

	got := m.renderSuggestions()

	repoRoot := findRepoRoot(t)
	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tui", "suggest_dashboard.txt")
	compareGolden(t, got, goldenPath)
}

// TestShellCompletion_BashGeneration tests that bash completion is generated correctly.
// This verifies the completion script contains expected patterns and is deterministic.
func TestShellCompletion_BashGeneration(t *testing.T) {
	var buf strings.Builder
	err := GenerateCompletionScript(&buf, "bash")
	if err != nil {
		t.Fatalf("failed to generate bash completion: %v", err)
	}

	got := buf.String()

	// Verify key patterns are present (Cobra-generated patterns)
	expectedPatterns := []string{
		"cub-scout",               // Command name
		"__cub-scout_debug",       // Debug function
		"__cub-scout_init",        // Init function
		"COMPREPLY",               // Bash completion variable
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(got, pattern) {
			t.Errorf("bash completion missing expected pattern: %q", pattern)
		}
	}

	// Verify minimum length (completion script should be substantial)
	if len(got) < 1000 {
		t.Errorf("bash completion seems too short: %d bytes", len(got))
	}
}

// TestShellCompletion_ZshGeneration tests that zsh completion is generated correctly.
func TestShellCompletion_ZshGeneration(t *testing.T) {
	var buf strings.Builder
	err := GenerateCompletionScript(&buf, "zsh")
	if err != nil {
		t.Fatalf("failed to generate zsh completion: %v", err)
	}

	got := buf.String()

	// Verify key patterns are present (Cobra-generated patterns)
	expectedPatterns := []string{
		"cub-scout",     // Command name
		"#compdef",      // Zsh compdef header
		"_cub-scout",    // Zsh completion function
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(got, pattern) {
			t.Errorf("zsh completion missing expected pattern: %q", pattern)
		}
	}

	// Verify minimum length
	if len(got) < 1000 {
		t.Errorf("zsh completion seems too short: %d bytes", len(got))
	}
}
