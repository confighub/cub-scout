package main

import "testing"

func TestLocalClusterSuggestions_DashboardIncludesDoctorAndQuickstart(t *testing.T) {
	m := testLocalModel()
	m.panelView = viewDashboard

	suggestions := m.getSuggestions()

	foundDoctor := false
	foundQuickstart := false
	for _, suggestion := range suggestions {
		if suggestion.Command == "./cub-scout doctor" {
			foundDoctor = true
		}
		if suggestion.Command == "./cub-scout quickstart --yes" {
			foundQuickstart = true
		}
	}

	if !foundDoctor {
		t.Fatalf("expected dashboard suggestions to include ./cub-scout doctor, got: %+v", suggestions)
	}
	if !foundQuickstart {
		t.Fatalf("expected dashboard suggestions to include ./cub-scout quickstart --yes, got: %+v", suggestions)
	}
}

func TestLocalClusterSuggestions_WorkloadsIncludesExplain(t *testing.T) {
	m := testLocalModel()
	m.panelView = viewWorkloads
	m.cursor = 0

	suggestions := m.getSuggestions()

	foundExplain := false
	for _, suggestion := range suggestions {
		if suggestion.Command == "./cub-scout explain deployment/nginx -n default" {
			foundExplain = true
			break
		}
	}

	if !foundExplain {
		t.Fatalf("expected workloads suggestions to include explain command, got: %+v", suggestions)
	}
}
