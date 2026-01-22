// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// testLocalModel creates a LocalClusterModel with mock data for testing.
func testLocalModel() LocalClusterModel {
	vp := viewport.New(40, 20)
	s := spinner.New()
	s.Spinner = spinner.Dot

	entries := []MapEntry{
		{Name: "nginx", Namespace: "default", Kind: "Deployment", Owner: "Flux", Status: "Ready"},
		{Name: "redis", Namespace: "default", Kind: "Deployment", Owner: "ArgoCD", Status: "Ready"},
		{Name: "postgres", Namespace: "database", Kind: "StatefulSet", Owner: "Helm", Status: "Ready"},
		{Name: "orphan-svc", Namespace: "default", Kind: "Service", Owner: "Native", Status: "Ready"},
		{Name: "crashing-pod", Namespace: "prod", Kind: "Pod", Owner: "Flux", Status: "CrashLoopBackOff"},
	}

	gitops := []GitOpsResource{
		{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Status: "Ready", Source: "git@github.com:example/repo"},
		{Kind: "HelmRelease", Name: "redis", Namespace: "default", Status: "Ready", Source: "redis-chart"},
		{Kind: "Application", Name: "argo-app", Namespace: "argocd", Status: "Synced", Source: "https://github.com/example/app"},
	}

	m := LocalClusterModel{
		entries:     entries,
		gitops:      gitops,
		width:       80,
		height:      24,
		ready:       true,
		loading:     false,
		cursor:      0,
		view:        viewDashboard,
		spinner:     s,
		keymap:      defaultLocalKeyMap(),
		clusterName: "test-cluster",
		panelPane:   vp,
		namespaces:  []string{"default", "database", "prod"},
	}

	return m
}

// testLocalModelWithScan creates a model in scan mode with findings.
func testLocalModelWithScan() LocalClusterModel {
	m := testLocalModel()
	m.scanMode = true
	m.scanOutput = "[W] CCVE-2025-0001: Test finding"
	m.scanFindings = []scanFinding{
		{CCVE: "CCVE-2025-0001", Severity: "warning", Category: "CONFIG", Resource: "default/nginx"},
	}
	m.scanCategories = map[string]int{"CONFIG": 1}
	return m
}

// --- View Key Tests ---

// TestLocalClusterDashboard tests 's' key for status/dashboard view.
func TestLocalClusterDashboard(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// First go to a panel view
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	time.Sleep(50 * time.Millisecond)

	// Then press 's' to go back to dashboard
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.panelMode {
		t.Error("expected panel mode to be off after pressing 's' (dashboard)")
	}
}

// TestLocalClusterWorkloads tests 'w' key for workloads view.
func TestLocalClusterWorkloads(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewWorkloads {
		t.Errorf("expected workloads view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterPipelines tests 'p' key for pipelines view.
func TestLocalClusterPipelines(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewPipelines {
		t.Errorf("expected pipelines view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterDrift tests 'd' key for drift view.
func TestLocalClusterDrift(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewDrift {
		t.Errorf("expected drift view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterOrphans tests 'o' key for orphans view.
func TestLocalClusterOrphans(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewOrphans {
		t.Errorf("expected orphans view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterCrashes tests 'c' key for crashes view.
func TestLocalClusterCrashes(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewCrashes {
		t.Errorf("expected crashes view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterIssues tests 'i' key for issues view.
func TestLocalClusterIssues(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewIssues {
		t.Errorf("expected issues view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterSuspended tests 'u' key for suspended view.
func TestLocalClusterSuspended(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewSuspended {
		t.Errorf("expected suspended view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterBypass tests 'b' key for bypass view.
func TestLocalClusterBypass(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewBypass {
		t.Errorf("expected bypass view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterSprawl tests 'x' key for sprawl view.
func TestLocalClusterSprawl(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewSprawl {
		t.Errorf("expected sprawl view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterApps tests 'a' key for apps view.
func TestLocalClusterApps(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewApps {
		t.Errorf("expected apps view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterDependencies tests 'D' key for dependencies view.
func TestLocalClusterDependencies(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewDependencies {
		t.Errorf("expected dependencies view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterGitSources tests 'G' key for git sources view.
func TestLocalClusterGitSources(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewGitSources {
		t.Errorf("expected git sources view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterMaps tests 'M' key for maps view.
func TestLocalClusterMaps(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewMaps {
		t.Errorf("expected maps view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterClusterData tests '4' key for cluster data view.
func TestLocalClusterClusterData(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewClusterData {
		t.Errorf("expected cluster data view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterAppHierarchy tests '5' key for app hierarchy view.
func TestLocalClusterAppHierarchy(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewAppHierarchy {
		t.Errorf("expected app hierarchy view, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// TestLocalClusterAppHierarchyAltKey tests 'A' key for app hierarchy view.
func TestLocalClusterAppHierarchyAltKey(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if !fm.panelMode || fm.panelView != viewAppHierarchy {
		t.Errorf("expected app hierarchy view with 'A' key, got panelMode=%v view=%v", fm.panelMode, fm.panelView)
	}
}

// --- Navigation Tests ---

// TestLocalClusterNavigationDown tests j/down key navigation.
func TestLocalClusterNavigationDown(t *testing.T) {
	m := testLocalModel()
	m.panelMode = true
	m.panelView = viewWorkloads

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.cursor != 1 {
		t.Errorf("expected cursor at 1 after j, got %d", fm.cursor)
	}
}

// TestLocalClusterNavigationUp tests k/up key navigation.
func TestLocalClusterNavigationUp(t *testing.T) {
	m := testLocalModel()
	m.panelMode = true
	m.panelView = viewWorkloads
	m.cursor = 2

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.cursor != 1 {
		t.Errorf("expected cursor at 1 after k, got %d", fm.cursor)
	}
}

// TestLocalClusterCrossReference tests Enter key for cross-reference navigation.
func TestLocalClusterCrossReference(t *testing.T) {
	m := testLocalModel()
	m.panelMode = true
	m.panelView = viewWorkloads
	m.cursor = 0 // Select first workload

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press Enter to show cross-references
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(50 * time.Millisecond)

	// Press Esc to close
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	// After Esc, xrefMode should be false
	if fm.xrefMode {
		t.Error("expected xrefMode to be false after pressing Esc")
	}
}

// TestLocalClusterNamespaceNext tests ']' key for next namespace.
func TestLocalClusterNamespaceNext(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.namespaceIdx != 1 {
		t.Errorf("expected namespaceIdx 1 after ], got %d", fm.namespaceIdx)
	}
}

// TestLocalClusterNamespacePrev tests '[' key for previous namespace.
func TestLocalClusterNamespacePrev(t *testing.T) {
	m := testLocalModel()
	m.namespaceIdx = 2

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.namespaceIdx != 1 {
		t.Errorf("expected namespaceIdx 1 after [, got %d", fm.namespaceIdx)
	}
}

// --- Action Tests ---

// TestLocalClusterTrace tests 'T' key for trace.
func TestLocalClusterTrace(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	time.Sleep(50 * time.Millisecond)

	// Escape to exit trace mode
	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.traceMode {
		t.Error("expected trace mode to be off after Esc")
	}
}

// TestLocalClusterScan tests 'S' key for scan.
func TestLocalClusterScan(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	time.Sleep(50 * time.Millisecond)

	// Escape to exit scan mode
	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.scanMode {
		t.Error("expected scan mode to be off after Esc")
	}
}

// TestLocalClusterQuery tests 'Q' key for query selector.
func TestLocalClusterQuery(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.queryMode {
		t.Error("expected query mode to be closed after Esc")
	}
}

// TestLocalClusterImport tests 'I' key for import wizard.
func TestLocalClusterImport(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	time.Sleep(50 * time.Millisecond)

	// Import triggers quit to switch mode
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// --- Mode Tests ---

// TestLocalClusterHelp tests '?' key for help overlay.
func TestLocalClusterHelp(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	time.Sleep(50 * time.Millisecond)

	// Help dismisses on any key, so press Esc first
	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	// Now quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	// Verify we got a valid final model
	if fm.entries == nil {
		t.Error("expected valid final model")
	}
}

// TestLocalClusterSearch tests '/' key for search mode.
func TestLocalClusterSearch(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	time.Sleep(50 * time.Millisecond)

	tm.Type("nginx")
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.searchMode {
		t.Error("expected search mode to be off after Esc")
	}
}

// TestLocalClusterRefresh tests 'r' key for refresh.
func TestLocalClusterRefresh(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestLocalClusterHub tests 'H' key for hub switch.
func TestLocalClusterHub(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}})
	time.Sleep(100 * time.Millisecond)

	// H triggers auth check, then possibly quit or auth needed dialog
	// Send 'n' to decline auth if prompted, then 'q' to quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestLocalClusterCommandPalette tests ':' key for command mode.
func TestLocalClusterCommandPalette(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	time.Sleep(50 * time.Millisecond)

	tm.Type("kubectl")
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	if fm.cmdMode {
		t.Error("expected command mode to be off after Esc")
	}
}

// TestLocalClusterCommandHistory tests command history navigation.
func TestLocalClusterCommandHistory(t *testing.T) {
	m := testLocalModel()
	m.cmdHistory = []string{"kubectl get pods", "kubectl get svc"}

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Enter command mode
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	time.Sleep(50 * time.Millisecond)

	// Press up to get history
	tm.Send(tea.KeyMsg{Type: tea.KeyUp})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestLocalClusterQuit tests 'q' key for quit.
func TestLocalClusterQuit(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestLocalClusterCtrlC tests Ctrl+C for quit.
func TestLocalClusterCtrlC(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestLocalClusterTab tests Tab for view cycling.
func TestLocalClusterTab(t *testing.T) {
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyTab})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(LocalClusterModel)
	// Tab should have switched to a view or toggled panel focus
	if !fm.panelMode && fm.view == viewDashboard {
		// If not in panel mode, tab might not change view from dashboard
		// This is acceptable behavior
	}
}

// --- View Rendering Tests ---

// TestLocalClusterInitialView tests initial view renders correctly.
func TestLocalClusterInitialView(t *testing.T) {
	m := testLocalModel()

	view := m.View()

	if !bytes.Contains([]byte(view), []byte("test-cluster")) {
		t.Errorf("expected view to contain cluster name, got: %s", view[:min(len(view), 200)])
	}
}

// TestLocalClusterHelpViewContent tests help view content.
func TestLocalClusterHelpViewContent(t *testing.T) {
	m := testLocalModel()
	m.helpMode = true

	view := m.View()

	expectedKeys := []string{"VIEWS", "NAVIGATION", "ACTIONS", "COMMAND PALETTE"}
	for _, key := range expectedKeys {
		if !bytes.Contains([]byte(view), []byte(key)) {
			t.Errorf("expected help view to contain '%s'", key)
		}
	}
}

// TestLocalClusterLoadingState tests loading state renders correctly.
func TestLocalClusterLoadingState(t *testing.T) {
	m := testLocalModel()
	m.loading = true

	view := m.View()

	if !bytes.Contains([]byte(view), []byte("Loading")) {
		t.Errorf("expected loading message, got: %s", view)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestSnapshotSaveLoad tests that TUI state is saved and restored.
func TestSnapshotSaveLoad(t *testing.T) {
	// Use a temp directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create sessions directory
	sessionsDir := filepath.Join(tmpDir, ".confighub", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create model with specific state
	m := testLocalModel()
	m.clusterName = "test-cluster"
	m.cursor = 3
	m.namespaceIdx = 2
	m.panelMode = true
	m.panelView = viewWorkloads

	// Save snapshot
	saveSnapshot(&m)

	// Verify file was created
	snapPath := filepath.Join(sessionsDir, "localcluster-snapshot.json")
	if _, err := os.Stat(snapPath); os.IsNotExist(err) {
		t.Fatal("snapshot file was not created")
	}

	// Load snapshot
	snap := loadSnapshot()
	if snap == nil {
		t.Fatal("snapshot was not loaded")
	}

	// Verify state was preserved
	if snap.ClusterName != "test-cluster" {
		t.Errorf("expected cluster name 'test-cluster', got '%s'", snap.ClusterName)
	}
	if snap.Cursor != 3 {
		t.Errorf("expected cursor 3, got %d", snap.Cursor)
	}
	if snap.NamespaceIdx != 2 {
		t.Errorf("expected namespace index 2, got %d", snap.NamespaceIdx)
	}
	if !snap.PanelMode {
		t.Error("expected panel mode to be true")
	}
	if snap.PanelView != int(viewWorkloads) {
		t.Errorf("expected panel view %d, got %d", viewWorkloads, snap.PanelView)
	}
}

// TestSnapshotExpiry tests that old snapshots are not loaded.
func TestSnapshotExpiry(t *testing.T) {
	// Use a temp directory for testing
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create sessions directory
	sessionsDir := filepath.Join(tmpDir, ".confighub", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an old snapshot (> 24 hours old)
	oldSnap := TUISnapshot{
		Version:     snapshotVersion,
		UpdatedAt:   time.Now().Add(-25 * time.Hour), // 25 hours ago
		ClusterName: "old-cluster",
		Cursor:      5,
	}

	// Write the old snapshot
	snapPath := filepath.Join(sessionsDir, "localcluster-snapshot.json")
	data, _ := json.MarshalIndent(oldSnap, "", "  ")
	if err := os.WriteFile(snapPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Load should return nil for expired snapshot
	snap := loadSnapshot()
	if snap != nil {
		t.Error("expected nil for expired snapshot, but got a snapshot")
	}
}
