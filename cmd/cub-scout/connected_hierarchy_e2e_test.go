//go:build e2e
// +build e2e

package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
)

// TestE2EConnectedHierarchyRendering exercises the connected hierarchy view
// rendering with real cluster data loaded into the model.
func TestE2EConnectedHierarchyRendering(t *testing.T) {
	// Load real cluster data
	msg := loadLocalClusterData()
	loaded, ok := msg.(localDataLoadedMsg)
	if !ok {
		t.Fatal("loadLocalClusterData did not return localDataLoadedMsg")
	}
	if loaded.err != nil {
		t.Fatalf("loadLocalClusterData failed: %v", loaded.err)
	}
	if len(loaded.entries) == 0 {
		t.Fatal("no entries loaded from cluster")
	}
	t.Logf("Loaded %d entries from real cluster", len(loaded.entries))

	// Extract namespaces from entries
	nsSet := map[string]bool{}
	for _, e := range loaded.entries {
		nsSet[e.Namespace] = true
	}
	var namespaces []string
	for ns := range nsSet {
		namespaces = append(namespaces, ns)
	}

	// Build model with real data
	vp := viewport.New(80, 24)
	s := spinner.New()
	m := LocalClusterModel{
		entries:     loaded.entries,
		gitops:      loaded.gitops,
		gitSources:  loaded.gitSources,
		width:       120,
		height:      40,
		ready:       true,
		loading:     false,
		view:        viewDashboard,
		spinner:     s,
		keymap:      defaultLocalKeyMap(),
		clusterName: "e2e-test",
		panelPane:   vp,
		namespaces:  namespaces,
	}

	// --- Test 1: Standalone mode renders without panic ---
	t.Run("StandaloneAppHierarchy", func(t *testing.T) {
		m.connectionMode = "online"
		content := m.getPanelAppHierarchy()
		if content == "" {
			t.Fatal("getPanelAppHierarchy returned empty in standalone mode")
		}
		if !strings.Contains(content, "This is TUI's interpretation") {
			t.Error("standalone mode should show disclaimer")
		}
		if !strings.Contains(content, "What ConfigHub provides") {
			t.Error("standalone mode should show marketing box")
		}
		if !strings.Contains(content, "Inferred ConfigHub Model") {
			t.Error("standalone mode should show 'Inferred' header")
		}
		t.Logf("Standalone view: %d chars", len(content))
	})

	// --- Test 2: Connected mode renders without panic ---
	t.Run("ConnectedAppHierarchy", func(t *testing.T) {
		m.connectionMode = "connected"
		m.connectedEmail = "e2e@test.com"
		m.workerName = "e2e-worker"
		m.workerStatus = "connected"
		content := m.getPanelAppHierarchy()
		if content == "" {
			t.Fatal("getPanelAppHierarchy returned empty in connected mode")
		}
		if strings.Contains(content, "This is TUI's interpretation") {
			t.Error("connected mode should NOT show disclaimer")
		}
		if strings.Contains(content, "What ConfigHub provides") {
			t.Error("connected mode should NOT show marketing box")
		}
		if !strings.Contains(content, "ConfigHub Connected") {
			t.Error("connected mode should show 'ConfigHub Connected' header")
		}
		if !strings.Contains(content, "e2e@test.com") {
			t.Error("connected mode should show email")
		}
		if !strings.Contains(content, "e2e-worker") {
			t.Error("connected mode should show worker name")
		}
		t.Logf("Connected view: %d chars", len(content))
	})

	// --- Test 3: Compact dashboard shows ConfigHub section when connected ---
	t.Run("ConnectedDashboardCompact", func(t *testing.T) {
		m.connectionMode = "connected"
		content := m.renderDashboardCompact()
		if !strings.Contains(content, "CONFIGHUB") {
			t.Error("connected compact dashboard should show CONFIGHUB section")
		}
		if !strings.Contains(content, "Connected") {
			t.Error("connected compact dashboard should show Connected indicator")
		}
		// All resources are Native on this cluster
		if !strings.Contains(content, "importable") {
			t.Error("should show importable count for Native resources")
		}
		t.Logf("Connected compact dashboard: %d chars", len(content))
	})

	// --- Test 4: Standalone dashboard does NOT show ConfigHub section ---
	t.Run("StandaloneDashboardCompact", func(t *testing.T) {
		m.connectionMode = "online"
		content := m.renderDashboardCompact()
		if strings.Contains(content, "CONFIGHUB") {
			t.Error("standalone compact dashboard should NOT show CONFIGHUB section")
		}
	})

	// --- Test 5: Panel title decoration ---
	t.Run("PanelTitle", func(t *testing.T) {
		m.connectionMode = "connected"
		m.panelView = viewAppHierarchy
		title := m.getPanelTitle()
		if title != "APP HIERARCHY (Connected)" {
			t.Errorf("expected 'APP HIERARCHY (Connected)', got %q", title)
		}

		m.connectionMode = "online"
		title = m.getPanelTitle()
		if title != "APP HIERARCHY" {
			t.Errorf("expected 'APP HIERARCHY', got %q", title)
		}
	})

	// --- Test 6: Query presets mode-aware ---
	t.Run("QueryPresets", func(t *testing.T) {
		m.connectionMode = "connected"
		cq := m.getEffectiveQueries()
		if cq[1].Name != "confighub" {
			t.Errorf("connected: expected confighub as #2, got %s", cq[1].Name)
		}

		m.connectionMode = "online"
		sq := m.getEffectiveQueries()
		if sq[1].Name != "orphans" {
			t.Errorf("standalone: expected orphans as #2, got %s", sq[1].Name)
		}
	})

	// --- Test 7: Auto-nav on connectionStatusMsg ---
	t.Run("AutoNavOnConnect", func(t *testing.T) {
		m.connectionMode = ""
		m.panelMode = false
		m.userHasNavigated = false
		updated, _ := m.Update(connectionStatusMsg{
			mode:         "connected",
			email:        "e2e@test.com",
			workerName:   "e2e-worker",
			workerStatus: "connected",
		})
		fm := updated.(LocalClusterModel)
		if !fm.panelMode {
			t.Error("should auto-navigate to panel mode on connect")
		}
		if fm.panelView != viewAppHierarchy {
			t.Errorf("should auto-nav to App Hierarchy, got %d", fm.panelView)
		}
	})

	// --- Test 8: Full View() render without panic ---
	t.Run("FullViewRender", func(t *testing.T) {
		m.connectionMode = "connected"
		m.panelMode = true
		m.panelView = viewAppHierarchy
		m.updatePanelContent()
		view := m.View()
		if view == "" {
			t.Fatal("View() returned empty")
		}
		t.Logf("Full view: %d chars", len(view))
	})
}
