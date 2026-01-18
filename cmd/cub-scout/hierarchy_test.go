// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// skipIfNoCub skips the test if the 'cub' CLI is not available.
// This is needed because teatest-based tests trigger Init() which calls the cub CLI.
func skipIfNoCub(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("cub"); err != nil {
		t.Skip("Skipping test: 'cub' CLI not found in PATH")
	}
}

// testModel creates a Model with mock data for testing.
// This bypasses the cub CLI calls and provides a static tree structure.
func testModel() Model {
	vp := viewport.New(40, 20)
	vp.MouseWheelEnabled = true

	// Create mock tree structure
	orgNode := &TreeNode{
		ID:       "org-1",
		Name:     "test-org",
		Type:     "org",
		Status:   "ok",
		Expanded: true,
		Children: []*TreeNode{},
	}

	spaceNode := &TreeNode{
		ID:       "space-1",
		Name:     "test-space",
		Type:     "space",
		Status:   "ok",
		Parent:   orgNode,
		Expanded: false,
		Children: []*TreeNode{},
	}

	unitNode := &TreeNode{
		ID:     "unit-1",
		Name:   "test-unit",
		Type:   "unit",
		Status: "ok",
		Info:   "v1 → production",
		Parent: spaceNode,
	}

	targetNode := &TreeNode{
		ID:     "target-1",
		Name:   "test-target",
		Type:   "target",
		Status: "ok",
		Info:   "Kubernetes",
		Parent: spaceNode,
	}

	spaceNode.Children = []*TreeNode{unitNode, targetNode}
	orgNode.Children = []*TreeNode{spaceNode}

	m := Model{
		nodes:       []*TreeNode{orgNode},
		keymap:      defaultKeyMap(),
		ready:       true,
		loading:     false,
		width:       80,
		height:      24,
		detailsPane: vp,
	}

	// Build the flat list for navigation
	m.rebuildFlatList()

	return m
}

// testModelCollapsed creates a Model with collapsed nodes for testing expand/collapse.
func testModelCollapsed() Model {
	vp := viewport.New(40, 20)
	vp.MouseWheelEnabled = true

	orgNode := &TreeNode{
		ID:       "org-1",
		Name:     "test-org",
		Type:     "org",
		Status:   "ok",
		Expanded: false, // Collapsed by default
		Children: []*TreeNode{},
	}

	spaceNode := &TreeNode{
		ID:       "space-1",
		Name:     "test-space",
		Type:     "space",
		Status:   "ok",
		Parent:   orgNode,
		Expanded: false,
		Children: []*TreeNode{},
	}

	unitNode := &TreeNode{
		ID:     "unit-1",
		Name:   "test-unit",
		Type:   "unit",
		Status: "ok",
		Parent: spaceNode,
	}

	spaceNode.Children = []*TreeNode{unitNode}
	orgNode.Children = []*TreeNode{spaceNode}

	m := Model{
		nodes:       []*TreeNode{orgNode},
		keymap:      defaultKeyMap(),
		ready:       true,
		loading:     false,
		width:       80,
		height:      24,
		detailsPane: vp,
	}

	m.rebuildFlatList()
	return m
}

// testModelMultipleOrgs creates a Model with multiple organizations for navigation testing.
func testModelMultipleOrgs() Model {
	vp := viewport.New(40, 20)
	vp.MouseWheelEnabled = true

	org1 := &TreeNode{
		ID:       "org-1",
		Name:     "org-alpha",
		Type:     "org",
		Status:   "ok",
		Expanded: true,
		Data: CubOrganization{
			OrganizationID: "org-1",
			ExternalID:     "ext-1",
			DisplayName:    "Org Alpha",
			Slug:           "org-alpha",
		},
	}
	space1 := &TreeNode{
		ID:       "space-1",
		Name:     "space-one",
		Type:     "space",
		Status:   "ok",
		Parent:   org1,
		Expanded: false,
	}
	org1.Children = []*TreeNode{space1}

	org2 := &TreeNode{
		ID:       "org-2",
		Name:     "org-beta",
		Type:     "org",
		Status:   "warn",
		Expanded: true,
		Data: CubOrganization{
			OrganizationID: "org-2",
			ExternalID:     "ext-2",
			DisplayName:    "Org Beta",
			Slug:           "org-beta",
		},
	}
	space2 := &TreeNode{
		ID:       "space-2",
		Name:     "space-two",
		Type:     "space",
		Status:   "error",
		Parent:   org2,
		Expanded: false,
	}
	org2.Children = []*TreeNode{space2}

	m := Model{
		nodes:       []*TreeNode{org1, org2},
		keymap:      defaultKeyMap(),
		ready:       true,
		loading:     false,
		width:       80,
		height:      24,
		detailsPane: vp,
	}

	m.rebuildFlatList()
	return m
}

// TestHierarchyInitialView tests that the initial view renders correctly.
func TestHierarchyInitialView(t *testing.T) {
	m := testModel()

	// Directly test the View() function without going through teatest
	// since we're testing the rendered content
	view := m.View()

	// Verify the output contains expected elements
	if !bytes.Contains([]byte(view), []byte("CONFIGHUB HIERARCHY")) {
		t.Errorf("expected view to contain 'CONFIGHUB HIERARCHY' header, got: %s", view[:min(len(view), 200)])
	}

	if !bytes.Contains([]byte(view), []byte("test-org")) {
		t.Errorf("expected view to contain 'test-org' organization, got: %s", view[:min(len(view), 500)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestHierarchyNavigationDown tests j/down key navigation.
func TestHierarchyNavigationDown(t *testing.T) {
	skipIfNoCub(t)
	m := testModelMultipleOrgs()

	// Verify initial state
	if m.cursor != 0 {
		t.Fatalf("expected initial cursor at 0, got %d", m.cursor)
	}
	if m.flatList[0].Name != "org-alpha" {
		t.Fatalf("expected first node to be org-alpha, got %s", m.flatList[0].Name)
	}

	// Test navigation with j key
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Move down with 'j'
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	time.Sleep(50 * time.Millisecond)

	// Move down again
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	time.Sleep(50 * time.Millisecond)

	// Quit and get final model
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	// Check that cursor moved
	fm := finalModel.(Model)
	if fm.cursor < 1 {
		t.Errorf("expected cursor to have moved down, got cursor=%d", fm.cursor)
	}
}

// TestHierarchyNavigationUp tests k/up key navigation.
func TestHierarchyNavigationUp(t *testing.T) {
	skipIfNoCub(t)
	m := testModelMultipleOrgs()
	m.cursor = 2 // Start at 3rd item

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Move up with 'k'
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	time.Sleep(50 * time.Millisecond)

	// Quit and get final model
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if fm.cursor != 1 {
		t.Errorf("expected cursor at 1 after moving up, got cursor=%d", fm.cursor)
	}
}

// TestHierarchyExpandCollapse tests 'l' key for expand and 'h' key for collapse.
func TestHierarchyExpandCollapse(t *testing.T) {
	skipIfNoCub(t)
	m := testModelCollapsed()

	// Verify initial state - org is collapsed
	if m.nodes[0].Expanded {
		t.Fatal("expected org to start collapsed")
	}
	if len(m.flatList) != 1 {
		t.Fatalf("expected 1 item in flat list when collapsed, got %d", len(m.flatList))
	}

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'l' to expand (vim-style right arrow)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	time.Sleep(50 * time.Millisecond)

	// Quit and get final model
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if !fm.nodes[0].Expanded {
		t.Error("expected org to be expanded after pressing 'l'")
	}
	if len(fm.flatList) < 2 {
		t.Errorf("expected more items in flat list after expanding, got %d", len(fm.flatList))
	}
}

// TestHierarchyCollapseExpanded tests collapsing an expanded node with 'h' key.
func TestHierarchyCollapseExpanded(t *testing.T) {
	skipIfNoCub(t)
	m := testModel() // Starts with org expanded

	// Verify initial state - org is expanded
	if !m.nodes[0].Expanded {
		t.Fatal("expected org to start expanded")
	}
	initialCount := len(m.flatList)
	if initialCount < 2 {
		t.Fatalf("expected multiple items when expanded, got %d", initialCount)
	}

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'h' to collapse (vim-style left arrow)
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	time.Sleep(50 * time.Millisecond)

	// Quit and get final model
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if fm.nodes[0].Expanded {
		t.Error("expected org to be collapsed after pressing 'h'")
	}
	if len(fm.flatList) >= initialCount {
		t.Errorf("expected fewer items after collapsing, got %d (was %d)", len(fm.flatList), initialCount)
	}
}

// TestHierarchyQuit tests that 'q' quits the program.
func TestHierarchyQuit(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Send quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Should finish within timeout
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyLoadingState tests that loading state renders correctly.
func TestHierarchyLoadingState(t *testing.T) {
	vp := viewport.New(40, 20)
	s := spinner.New()
	s.Spinner = spinner.Dot
	m := Model{
		keymap:      defaultKeyMap(),
		ready:       true,
		loading:     true, // Loading state
		width:       80,
		height:      24,
		detailsPane: vp,
		spinner:     s,
	}

	view := m.View()
	// View now includes spinner prefix, so check for the text content
	if !bytes.Contains([]byte(view), []byte("Loading ConfigHub data...")) {
		t.Errorf("expected loading message to contain 'Loading ConfigHub data...', got: %s", view)
	}
}

// TestHierarchyNotReadyState tests that not-ready state renders correctly.
func TestHierarchyNotReadyState(t *testing.T) {
	vp := viewport.New(40, 20)
	s := spinner.New()
	s.Spinner = spinner.Dot
	m := Model{
		keymap:      defaultKeyMap(),
		ready:       false, // Not ready
		loading:     false,
		width:       80,
		height:      24,
		detailsPane: vp,
		spinner:     s,
	}

	view := m.View()
	// View now includes spinner prefix, so check for the text content
	if !bytes.Contains([]byte(view), []byte("Initializing...")) {
		t.Errorf("expected initializing message to contain 'Initializing...', got: %s", view)
	}
}

// TestHierarchySearchMode tests entering and exiting search mode.
func TestHierarchySearchMode(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	if m.searchMode {
		t.Fatal("expected search mode to be off initially")
	}

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Enter search mode with '/'
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	time.Sleep(50 * time.Millisecond)

	// Type a search query
	tm.Type("test")
	time.Sleep(50 * time.Millisecond)

	// Exit search mode with escape
	tm.Send(tea.KeyMsg{Type: tea.KeyEscape})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if fm.searchMode {
		t.Error("expected search mode to be off after escape")
	}
}

// TestHierarchyGoldenOutput tests the view output against a golden file.
// Run with -update flag to update golden files.
func TestHierarchyGoldenOutput(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(100 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(2*time.Second)))
	if err != nil {
		t.Fatalf("failed to read final output: %v", err)
	}

	// Use RequireEqualOutput to compare with golden file
	// This will create/update testdata/TestHierarchyGoldenOutput.golden when run with -update
	teatest.RequireEqualOutput(t, out)
}

// TestHierarchyTabSwitchFocus tests tab key switching focus between panes.
func TestHierarchyTabSwitchFocus(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	if m.detailsFocused {
		t.Fatal("expected details pane to not be focused initially")
	}

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press tab to switch focus
	tm.Send(tea.KeyMsg{Type: tea.KeyTab})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if !fm.detailsFocused {
		t.Error("expected details pane to be focused after tab")
	}
}

// TestHierarchyRefresh tests the 'r' key for refresh.
func TestHierarchyRefresh(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'r' to refresh - this triggers a command but won't complete in test
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyCtrlC tests that Ctrl+C quits the program.
func TestHierarchyCtrlC(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Send Ctrl+C
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})

	// Should finish within timeout
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyOrgSwitch tests that 'O' key opens org selector when multiple orgs exist.
func TestHierarchyOrgSwitch(t *testing.T) {
	skipIfNoCub(t)
	m := testModelMultipleOrgs()

	// Verify we have 2 orgs
	orgs := m.getOrgList()
	if len(orgs) != 2 {
		t.Fatalf("expected 2 orgs, got %d", len(orgs))
	}

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'O' to open org selector
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	time.Sleep(50 * time.Millisecond)

	// Press Esc to close
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if fm.orgSelectMode {
		t.Error("expected org select mode to be closed after Esc")
	}
}

// TestHierarchyCommandPalette tests that ':' key enters command mode.
func TestHierarchyCommandPalette(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press ':' to open command palette
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{':'}})
	time.Sleep(50 * time.Millisecond)

	// Press Esc to close
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if fm.cmdMode {
		t.Error("expected command mode to be closed after Esc")
	}
}

// TestHierarchyHelp tests that '?' key opens help overlay.
func TestHierarchyHelp(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press '?' to open help
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	time.Sleep(50 * time.Millisecond)

	// Press any key to close (help dismisses on any key)
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if fm.helpMode {
		t.Error("expected help mode to be closed")
	}
}

// TestHierarchySearchNextPrev tests 'n' and 'N' keys for search navigation.
func TestHierarchySearchNextPrev(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()
	m.searchQuery = "test"
	m.searchMatches = []int{0, 1} // Indices of matching nodes in flatList
	m.searchIndex = 0

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'n' for next match
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	time.Sleep(50 * time.Millisecond)

	// Press 'N' for previous match
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyToggleFilter tests 'f' key for filter toggle.
func TestHierarchyToggleFilter(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()
	m.searchQuery = "test"
	m.filterActive = false

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'f' to toggle filter
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))

	fm := finalModel.(Model)
	if !fm.filterActive {
		t.Error("expected filter to be active after pressing 'f'")
	}
}

// TestHierarchyActivity tests 'a' key for activity view.
func TestHierarchyActivity(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'a' for activity view
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	time.Sleep(50 * time.Millisecond)

	// Press Esc to close
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyMaps tests 'M' key for maps view.
func TestHierarchyMaps(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'M' for maps view
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'M'}})
	time.Sleep(50 * time.Millisecond)

	// Press Esc to close
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyCreate tests 'c' key for create mode.
func TestHierarchyCreate(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'c' for create
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	time.Sleep(50 * time.Millisecond)

	// Press Esc to cancel
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyImport tests 'i' key for import mode.
func TestHierarchyImport(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'i' for import
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	time.Sleep(50 * time.Millisecond)

	// Press Esc to cancel
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyOpenWeb tests 'o' key for open in browser.
func TestHierarchyOpenWeb(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'o' for open web - this may trigger browser opening
	// In test mode it should be a no-op or handled gracefully
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyLocalCluster tests 'L' key for local cluster switch.
func TestHierarchyLocalCluster(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'L' for local cluster - this triggers quit to switch mode
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	time.Sleep(50 * time.Millisecond)

	// Should finish (triggers quit for mode switch)
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyEnter tests Enter key for details view.
func TestHierarchyEnter(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press Enter to view details
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	time.Sleep(50 * time.Millisecond)

	// Press Esc to close details
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHierarchyDelete tests 'd' key for delete.
func TestHierarchyDelete(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	time.Sleep(50 * time.Millisecond)

	// Press 'd' for delete - should prompt for confirmation
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	time.Sleep(50 * time.Millisecond)

	// Press 'n' to cancel delete
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	time.Sleep(50 * time.Millisecond)

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// TestHubSnapshotSaveLoad tests that Hub TUI state is saved and restored.
func TestHubSnapshotSaveLoad(t *testing.T) {
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
	m := testModel()
	m.cursor = 2
	m.mapsMode = true
	m.currentOrg = "test-org"
	// Expand a node
	m.nodes[0].Expanded = true

	// Save snapshot
	saveHubSnapshot(&m)

	// Verify file was created
	snapPath := filepath.Join(sessionsDir, "hub-snapshot.json")
	if _, err := os.Stat(snapPath); os.IsNotExist(err) {
		t.Fatal("snapshot file was not created")
	}

	// Load snapshot
	snap := loadHubSnapshot()
	if snap == nil {
		t.Fatal("snapshot was not loaded")
	}

	// Verify state was preserved
	if snap.Cursor != 2 {
		t.Errorf("expected cursor 2, got %d", snap.Cursor)
	}
	if !snap.MapsMode {
		t.Error("expected maps mode to be true")
	}
	if snap.CurrentOrg != "test-org" {
		t.Errorf("expected current org 'test-org', got '%s'", snap.CurrentOrg)
	}
	if len(snap.ExpandedPaths) == 0 {
		t.Error("expected expanded paths to be saved")
	}
}

// TestHubSnapshotExpiry tests that old snapshots are not loaded.
func TestHubSnapshotExpiry(t *testing.T) {
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
	oldSnap := HubSnapshot{
		Version:    hubSnapshotVersion,
		UpdatedAt:  time.Now().Add(-25 * time.Hour), // 25 hours ago
		Cursor:     5,
		CurrentOrg: "old-org",
	}

	// Write the old snapshot
	snapPath := filepath.Join(sessionsDir, "hub-snapshot.json")
	data, _ := json.MarshalIndent(oldSnap, "", "  ")
	if err := os.WriteFile(snapPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Load should return nil for expired snapshot
	snap := loadHubSnapshot()
	if snap != nil {
		t.Error("expected nil for expired snapshot, but got a snapshot")
	}
}

// TestExtractClusterName tests cluster name extraction from various context formats
func TestExtractClusterName(t *testing.T) {
	tests := []struct {
		name     string
		context  string
		expected string
	}{
		{
			name:     "AWS EKS ARN",
			context:  "arn:aws:eks:us-east-1:123456789012:cluster/prod-east",
			expected: "prod-east",
		},
		{
			name:     "GKE context",
			context:  "gke_myproject_us-central1-a_prod-east",
			expected: "prod-east",
		},
		{
			name:     "kind context",
			context:  "kind-prod-east",
			expected: "prod-east",
		},
		{
			name:     "docker-desktop",
			context:  "docker-desktop",
			expected: "docker-desktop",
		},
		{
			name:     "minikube",
			context:  "minikube",
			expected: "minikube",
		},
		{
			name:     "empty",
			context:  "",
			expected: "",
		},
		{
			name:     "unknown",
			context:  "unknown",
			expected: "unknown",
		},
		{
			name:     "custom context",
			context:  "my-cluster",
			expected: "my-cluster",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractClusterName(tc.context)
			if result != tc.expected {
				t.Errorf("extractClusterName(%q) = %q, want %q", tc.context, result, tc.expected)
			}
		})
	}
}

// TestMatchesCluster tests cluster matching logic
func TestMatchesCluster(t *testing.T) {
	tests := []struct {
		name           string
		targetCluster  string
		currentCluster string
		expected       bool
	}{
		{
			name:           "exact match",
			targetCluster:  "prod-east",
			currentCluster: "prod-east",
			expected:       true,
		},
		{
			name:           "partial match - target contains current",
			targetCluster:  "eks-prod-east",
			currentCluster: "prod-east",
			expected:       true,
		},
		{
			name:           "partial match - current contains target",
			targetCluster:  "prod",
			currentCluster: "prod-east",
			expected:       true,
		},
		{
			name:           "case insensitive match",
			targetCluster:  "PROD-EAST",
			currentCluster: "prod-east",
			expected:       true,
		},
		{
			name:           "no match",
			targetCluster:  "staging",
			currentCluster: "prod-east",
			expected:       false,
		},
		{
			name:           "empty target",
			targetCluster:  "",
			currentCluster: "prod-east",
			expected:       false,
		},
		{
			name:           "empty current",
			targetCluster:  "prod-east",
			currentCluster: "",
			expected:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := matchesCluster(tc.targetCluster, tc.currentCluster)
			if result != tc.expected {
				t.Errorf("matchesCluster(%q, %q) = %v, want %v", tc.targetCluster, tc.currentCluster, result, tc.expected)
			}
		})
	}
}

// TestHubViewFilterToggle tests the 'a' key toggles showAllUnits
func TestHubViewFilterToggle(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()

	// Initial state: showAllUnits should be false
	if m.showAllUnits {
		t.Error("expected showAllUnits to be false initially")
	}

	// Simulate 'a' key press using teatest
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Press 'a' to toggle filter
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Get final model
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
	fm := finalModel.(Model)

	// After pressing 'a', showAllUnits should be true
	if !fm.showAllUnits {
		t.Error("expected showAllUnits to be true after pressing 'a'")
	}

	// Status message should indicate "Showing all units"
	if fm.statusMsg != "Showing all units" {
		t.Errorf("expected status message 'Showing all units', got %q", fm.statusMsg)
	}
}

// TestHubViewFilterToggleBack tests toggling back to "this cluster only"
func TestHubViewFilterToggleBack(t *testing.T) {
	skipIfNoCub(t)
	m := testModel()
	m.showAllUnits = true // Start with all units shown
	m.currentCluster = "test-cluster"

	// Simulate 'a' key press using teatest
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	// Press 'a' to toggle filter back
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	// Quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Get final model
	finalModel := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second))
	fm := finalModel.(Model)

	// After pressing 'a', showAllUnits should be false
	if fm.showAllUnits {
		t.Error("expected showAllUnits to be false after pressing 'a' twice")
	}

	// Status message should indicate cluster filtering
	expected := "Showing units on cluster: test-cluster"
	if fm.statusMsg != expected {
		t.Errorf("expected status message %q, got %q", expected, fm.statusMsg)
	}
}

// TestHubViewModeHeader tests the mode header renders correctly
func TestHubViewModeHeader(t *testing.T) {
	m := testModel()
	m.currentCluster = "prod-east"
	m.contextName = "eks-prod-east"

	// Test filter active (this cluster only)
	m.showAllUnits = false
	header := m.renderModeHeader()
	if header == "" {
		t.Error("expected mode header to be non-empty")
	}
	// Should contain "Connected"
	if !containsString(header, "Connected") {
		t.Error("mode header should contain 'Connected'")
	}
	// Should contain cluster name
	if !containsString(header, "prod-east") {
		t.Error("mode header should contain cluster name")
	}
	// Should contain "This cluster only"
	if !containsString(header, "This cluster only") {
		t.Error("mode header should contain 'This cluster only' when filtered")
	}

	// Test all units (no filter)
	m.showAllUnits = true
	header = m.renderModeHeader()
	// Should contain "All Units"
	if !containsString(header, "All Units") {
		t.Error("mode header should contain 'All Units' when not filtered")
	}
}
