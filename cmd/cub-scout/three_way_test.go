// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
)

func TestClassifyThreeWayPattern_Agreed(t *testing.T) {
	// No mismatches = agreed
	result := compareResourceResult{
		Resource:   "Deployment/api",
		Namespace:  "prod",
		Mode:       "dry-wet-live",
		Connected:  true,
		Mismatches: []compareFieldMismatch{}, // Empty = no disagreement
	}

	d := classifyThreeWayPattern(result, "Synced", "Healthy")

	if d.Pattern != PatternAgreed {
		t.Errorf("expected PatternAgreed, got %q", d.Pattern)
	}
	if d.IsDisagreement() {
		t.Error("PatternAgreed should not be a disagreement")
	}
}

func TestClassifyThreeWayPattern_ChangeInProgress(t *testing.T) {
	// ConfigHub (WET) differs from cluster, controller is syncing
	replicas2 := int64(2)
	replicas3 := int64(3)
	result := compareResourceResult{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Mode:      "dry-wet-live",
		Connected: true,
		Wet:       &compareSideSummary{Replicas: &replicas3},
		Live:      compareSideSummary{Replicas: &replicas2},
		Mismatches: []compareFieldMismatch{
			{Field: "replicas", Wet: "3", Live: "2"},
		},
	}

	d := classifyThreeWayPattern(result, "OutOfSync", "Progressing")

	if d.Pattern != PatternChangeInProgress {
		t.Errorf("expected PatternChangeInProgress, got %q", d.Pattern)
	}
	if !d.IsDisagreement() {
		t.Error("PatternChangeInProgress should be a disagreement")
	}
	if d.Meaning == "" {
		t.Error("expected non-empty meaning")
	}
}

func TestClassifyThreeWayPattern_SyncStale(t *testing.T) {
	// ConfigHub differs from cluster, but controller thinks it's synced
	result := compareResourceResult{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Mode:      "dry-wet-live",
		Connected: true,
		Wet:       &compareSideSummary{Images: []string{"nginx:1.26"}},
		Live:      compareSideSummary{Images: []string{"nginx:1.25"}},
		Mismatches: []compareFieldMismatch{
			{Field: "images", Wet: "nginx:1.26", Live: "nginx:1.25"},
		},
	}

	d := classifyThreeWayPattern(result, "Synced", "Healthy")

	if d.Pattern != PatternSyncStale {
		t.Errorf("expected PatternSyncStale, got %q", d.Pattern)
	}
	if !d.IsDisagreement() {
		t.Error("PatternSyncStale should be a disagreement")
	}
}

func TestClassifyThreeWayPattern_RolloutPending(t *testing.T) {
	// DRY differs from WET (render in progress)
	result := compareResourceResult{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Mode:      "dry-wet-live",
		Connected: true,
		Dry:       &compareSideSummary{Images: []string{"nginx:1.27"}},
		Wet:       &compareSideSummary{Images: []string{"nginx:1.26"}},
		Live:      compareSideSummary{Images: []string{"nginx:1.26"}},
		Mismatches: []compareFieldMismatch{
			{Field: "images", Dry: "nginx:1.27", Wet: "nginx:1.26", Live: "nginx:1.26"},
		},
	}

	d := classifyThreeWayPattern(result, "Synced", "Healthy")

	if d.Pattern != PatternRolloutPending {
		t.Errorf("expected PatternRolloutPending, got %q", d.Pattern)
	}
}

func TestClassifyThreeWayPattern_MultiChange(t *testing.T) {
	// All three differ
	result := compareResourceResult{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Mode:      "dry-wet-live",
		Connected: true,
		Dry:       &compareSideSummary{Images: []string{"nginx:1.27"}},
		Wet:       &compareSideSummary{Images: []string{"nginx:1.26"}},
		Live:      compareSideSummary{Images: []string{"nginx:1.25"}},
		Mismatches: []compareFieldMismatch{
			// DRY != WET != LIVE - but no specific WET/LIVE mismatch without DRY involvement
			{Field: "images", Dry: "nginx:1.27", Wet: "", Live: "nginx:1.25"},
		},
	}

	d := classifyThreeWayPattern(result, "Unknown", "Unknown")

	// This should be multi-change since there are mismatches but no clear WET/LIVE or DRY/WET pattern
	if d.Pattern != PatternMultiChange && d.Pattern != PatternUnknown {
		t.Errorf("expected PatternMultiChange or PatternUnknown, got %q", d.Pattern)
	}
}

func TestThreeWayDisagreement_IsDisagreement(t *testing.T) {
	tests := []struct {
		pattern ThreeWayPattern
		want    bool
	}{
		{PatternAgreed, false},
		{PatternDisconnected, false},
		{PatternUnlinked, false},
		{PatternChangeInProgress, true},
		{PatternSyncStale, true},
		{PatternRolloutPending, true},
		{PatternMultiChange, true},
		{PatternUnknown, true},
	}

	for _, tc := range tests {
		d := ThreeWayDisagreement{Pattern: tc.pattern}
		if got := d.IsDisagreement(); got != tc.want {
			t.Errorf("IsDisagreement() for %q = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

func TestBuildConfigHubStateSummary(t *testing.T) {
	tests := []struct {
		name     string
		dry      *compareSideSummary
		wet      *compareSideSummary
		contains string
	}{
		{
			name:     "wet with image",
			wet:      &compareSideSummary{Images: []string{"nginx:1.26"}},
			contains: "nginx:1.26",
		},
		{
			name:     "dry with image when wet missing",
			dry:      &compareSideSummary{Images: []string{"nginx:1.25"}},
			wet:      nil,
			contains: "nginx:1.25",
		},
		{
			name:     "wet with replicas",
			wet:      &compareSideSummary{Replicas: ptrInt64(3)},
			contains: "replicas=3",
		},
		{
			name:     "fallback to available",
			dry:      &compareSideSummary{},
			wet:      &compareSideSummary{},
			contains: "available",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildConfigHubStateSummary(tc.dry, tc.wet)
			if got == "" || (tc.contains != "" && !containsSubstring(got, tc.contains)) {
				t.Errorf("buildConfigHubStateSummary() = %q, want containing %q", got, tc.contains)
			}
		})
	}
}

func TestBuildClusterStateSummary(t *testing.T) {
	tests := []struct {
		name     string
		live     compareSideSummary
		contains string
	}{
		{
			name:     "with image",
			live:     compareSideSummary{Images: []string{"nginx:1.27"}},
			contains: "nginx:1.27",
		},
		{
			name:     "with replicas",
			live:     compareSideSummary{Replicas: ptrInt64(2)},
			contains: "replicas=2",
		},
		{
			name:     "fallback to running",
			live:     compareSideSummary{},
			contains: "running",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildClusterStateSummary(tc.live)
			if !containsSubstring(got, tc.contains) {
				t.Errorf("buildClusterStateSummary() = %q, want containing %q", got, tc.contains)
			}
		})
	}
}

func TestBuildControllerStateSummary(t *testing.T) {
	tests := []struct {
		syncStatus   string
		healthStatus string
		want         string
	}{
		{"Synced", "Healthy", "Synced, Healthy"},
		{"OutOfSync", "", "OutOfSync"},
		{"", "Degraded", "Degraded"},
		{"", "", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.syncStatus+"_"+tc.healthStatus, func(t *testing.T) {
			got := buildControllerStateSummary(tc.syncStatus, tc.healthStatus)
			if got != tc.want {
				t.Errorf("buildControllerStateSummary(%q, %q) = %q, want %q",
					tc.syncStatus, tc.healthStatus, got, tc.want)
			}
		})
	}
}

func TestPatternToLabel(t *testing.T) {
	// Ensure all patterns have labels
	patterns := []ThreeWayPattern{
		PatternAgreed,
		PatternChangeInProgress,
		PatternSyncStale,
		PatternRolloutPending,
		PatternMultiChange,
		PatternDisconnected,
		PatternUnlinked,
		PatternUnknown,
	}

	for _, p := range patterns {
		label := patternToLabel(p)
		if label == "" {
			t.Errorf("patternToLabel(%q) returned empty string", p)
		}
	}
}

func TestFormatThreeWaySection_NoDisagreement(t *testing.T) {
	d := &ThreeWayDisagreement{
		Pattern: PatternAgreed,
	}

	output := formatThreeWaySection(d)
	if output != "" {
		t.Errorf("expected empty output for PatternAgreed, got %q", output)
	}
}

func TestFormatThreeWaySection_WithDisagreement(t *testing.T) {
	d := &ThreeWayDisagreement{
		Resource:        "Deployment/api",
		Namespace:       "prod",
		Pattern:         PatternChangeInProgress,
		Meaning:         "ConfigHub change is being applied.",
		ConfigHubState:  "image=nginx:1.26",
		ControllerState: "OutOfSync, Progressing",
		ClusterState:    "image=nginx:1.25",
		FieldMismatches: []ThreeWayFieldMismatch{
			{Field: "images", ConfigHub: "nginx:1.26", Cluster: "nginx:1.25"},
		},
	}

	output := formatThreeWaySection(d)

	// Should contain key elements
	checks := []string{
		"THREE-WAY STATUS",
		"Change in progress",
		"ConfigHub:",
		"Controller:",
		"Cluster:",
		"images",
	}

	for _, check := range checks {
		if !containsSubstring(output, check) {
			t.Errorf("formatThreeWaySection output missing %q\nGot: %s", check, output)
		}
	}
}

func TestExtractFieldMismatches(t *testing.T) {
	mismatches := []compareFieldMismatch{
		{Field: "images", Dry: "nginx:1.27", Wet: "nginx:1.26", Live: "nginx:1.25"},
		{Field: "replicas", Wet: "3", Live: "2"},
	}

	result := extractFieldMismatches(mismatches)

	if len(result) != 2 {
		t.Fatalf("expected 2 mismatches, got %d", len(result))
	}

	// First mismatch should prefer WET over DRY
	if result[0].ConfigHub != "nginx:1.26" {
		t.Errorf("expected ConfigHub=nginx:1.26 (from WET), got %q", result[0].ConfigHub)
	}
	if result[0].Cluster != "nginx:1.25" {
		t.Errorf("expected Cluster=nginx:1.25, got %q", result[0].Cluster)
	}

	// Second mismatch - only WET available
	if result[1].ConfigHub != "3" {
		t.Errorf("expected ConfigHub=3 (from WET), got %q", result[1].ConfigHub)
	}
}

// Helper functions

func ptrInt64(v int64) *int64 {
	return &v
}

// Note: containsSubstring is defined in map_secret_issues_test.go
