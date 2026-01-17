// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// Journey tests simulate real user workflows through the TUI.
// These require a running Kubernetes cluster with test workloads.
// Run: ./test/e2e/setup-multi-tool-cluster.sh first.

// skipIfNoCluster skips test if no cluster is available
func skipIfNoCluster(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "cluster-info")
	if err := cmd.Run(); err != nil {
		t.Skip("No Kubernetes cluster available - run ./test/e2e/setup-multi-tool-cluster.sh")
	}
}

// skipIfNoContext skips test if kubectl context doesn't match expected
func skipIfNoContext(t *testing.T, expected string) {
	t.Helper()
	cmd := exec.Command("kubectl", "config", "current-context")
	out, err := cmd.Output()
	if err != nil {
		t.Skip("Cannot get kubectl context")
	}
	ctx := strings.TrimSpace(string(out))
	if !strings.Contains(ctx, expected) {
		t.Skipf("Test requires context containing %q, got %q", expected, ctx)
	}
}

// ===========================================================================
// Journey 1: "What's running?" - Dashboard exploration
// ===========================================================================

func TestJourney_WhatsRunning_LaunchAndExplore(t *testing.T) {
	// Use mock model for journey tests (doesn't require cluster)
	m := testLocalModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// User presses ? to see help
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	time.Sleep(50 * time.Millisecond)

	// Press escape to close help
	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	// Navigate through resources with j/k
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	// Switch to Issues view
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	time.Sleep(50 * time.Millisecond)

	// Switch to Cluster Data view (4)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	time.Sleep(50 * time.Millisecond)

	// Back to Dashboard
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestJourney_WhatsRunning_FilterByOwner(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// User wants to filter to see only Flux-managed resources
	// Type / to enter search/filter mode
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	time.Sleep(50 * time.Millisecond)

	// Type filter query
	for _, r := range "owner=Flux" {
		tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to apply filter
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(100 * time.Millisecond)

	// Clear filter with Escape
	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestJourney_WhatsRunning_OwnerBreakdown(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// Check Cluster Data tab shows owner breakdown
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := finalModel.(LocalClusterModel)

	// Check we're in panel mode with Cluster Data view
	if !fm.panelMode || fm.panelView != viewClusterData {
		t.Errorf("Expected Cluster Data view, got panelMode=%v panelView=%d", fm.panelMode, fm.panelView)
	}
}

// ===========================================================================
// Journey 2: "Find the problem" - Crash/issue investigation
// ===========================================================================

func TestJourney_FindProblem_NavigateToIssues(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// User goes directly to Issues view
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	time.Sleep(50 * time.Millisecond)

	// Navigate through issues
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := finalModel.(LocalClusterModel)

	if !fm.panelMode || fm.panelView != viewIssues {
		t.Errorf("Expected Issues view, got panelMode=%v panelView=%d", fm.panelMode, fm.panelView)
	}
}

func TestJourney_FindProblem_CheckCrashes(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// Go to crashes view
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := finalModel.(LocalClusterModel)

	if !fm.panelMode || fm.panelView != viewCrashes {
		t.Errorf("Expected Crashes view, got panelMode=%v panelView=%d", fm.panelMode, fm.panelView)
	}
}

// ===========================================================================
// Journey 3: "Audit GitOps" - Coverage analysis
// ===========================================================================

func TestJourney_AuditGitOps_FindOrphans(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// Go to orphans view (Native/unmanaged resources)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	time.Sleep(50 * time.Millisecond)

	// Check Cluster Data for owner breakdown
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := finalModel.(LocalClusterModel)

	if !fm.panelMode || fm.panelView != viewClusterData {
		t.Errorf("Expected Cluster Data view, got panelMode=%v panelView=%d", fm.panelMode, fm.panelView)
	}
}

func TestJourney_AuditGitOps_ViewPipelines(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// Go to pipelines view (shows GitOps resources)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	time.Sleep(50 * time.Millisecond)

	// Navigate through pipelines
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := finalModel.(LocalClusterModel)

	if !fm.panelMode || fm.panelView != viewPipelines {
		t.Errorf("Expected Pipelines view, got panelMode=%v panelView=%d", fm.panelMode, fm.panelView)
	}
}

// ===========================================================================
// Journey 4: "Import to ConfigHub" - Connected mode workflow
// ===========================================================================

func TestJourney_ImportWorkflow_SelectResource(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// Navigate to a resource
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}}) // Go to workloads view
	time.Sleep(50 * time.Millisecond)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Press 'I' to initiate import (should show wizard or message)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	time.Sleep(100 * time.Millisecond)

	// Press Escape to cancel
	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
}

// ===========================================================================
// Journey 5: "Check drift" - Sync status verification
// ===========================================================================

func TestJourney_CheckDrift_ViewDriftStatus(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// Go to drift view
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	time.Sleep(50 * time.Millisecond)

	// Navigate through drift entries
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := finalModel.(LocalClusterModel)

	if !fm.panelMode || fm.panelView != viewDrift {
		t.Errorf("Expected Drift view, got panelMode=%v panelView=%d", fm.panelMode, fm.panelView)
	}
}

func TestJourney_CheckDrift_ScanForIssues(t *testing.T) {
	m := testLocalModel()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	time.Sleep(50 * time.Millisecond)

	// Press 'S' to run scan
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	time.Sleep(100 * time.Millisecond)

	// Escape to exit scan view
	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
}

// ===========================================================================
// Journey 6: "Hub navigation" - Hierarchy exploration (connected mode)
// ===========================================================================

func TestJourney_HubNavigation_ToggleFilter(t *testing.T) {
	// This tests the Hub view filter toggle feature
	m := testModelWithHubData()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	// Press 'a' to toggle between cluster filter and all units
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Should now show all units
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
	fm := finalModel.(Model)

	if !fm.showAllUnits {
		t.Error("Expected showAllUnits=true after pressing 'a'")
	}
}

func TestJourney_HubNavigation_ExpandCollapse(t *testing.T) {
	m := testModelWithHubData()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	// Navigate to a space node
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	time.Sleep(50 * time.Millisecond)

	// Expand
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(50 * time.Millisecond)

	// Navigate into children
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	time.Sleep(50 * time.Millisecond)

	// Collapse with backspace
	tm.Send(tea.KeyMsg{Type: tea.KeyBackspace})
	time.Sleep(50 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestJourney_HubNavigation_SearchUnits(t *testing.T) {
	m := testModelWithHubData()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))
	defer tm.Quit()

	// Enter search mode
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	time.Sleep(50 * time.Millisecond)

	// Search for something
	for _, r := range "test" {
		tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Apply search
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(100 * time.Millisecond)

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
}

// ===========================================================================
// Helper functions
// ===========================================================================

// testModelWithHubData creates a Model with mock Hub hierarchy data
func testModelWithHubData() Model {
	m := testModel()
	m.contextName = "kind-tui-e2e"
	m.currentCluster = "tui-e2e"
	m.showAllUnits = false

	return m
}
