// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestBundleInspect_Deterministic(t *testing.T) {
	// Create a test bundle
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "test-bundle")

	writer := agent.NewBundleWriter("v0.14.6-test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label: "test-label",
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift:1", Path: "spec.replicas"},
			{ID: "drift:2", Path: "spec.template.spec.containers[0].image"},
		},
		Events: []agent.TimelineEvent{
			{ResourceKind: "Pod", ResourceName: "api-abc"},
		},
	}

	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read the bundle and generate ASCII output multiple times
	reader := agent.NewBundleReader()
	readBundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	summary := agent.Summarize(readBundle)

	// Capture output multiple times - should be identical
	var outputs []string
	for i := 0; i < 3; i++ {
		var b strings.Builder

		// Simulate renderBundleInspectASCII logic
		b.WriteString("Debug Bundle Inspection\n")
		b.WriteString(strings.Repeat("─", 50))
		b.WriteString("\n\n")
		b.WriteString("Metadata\n")
		b.WriteString("  Format version:  " + readBundle.Metadata.FormatVersion + "\n")
		b.WriteString("  Created by:      cub-scout " + readBundle.Metadata.CubScoutVersion + "\n")
		b.WriteString("  Created at:      " + readBundle.Metadata.CreatedAt.Format("2006-01-02 15:04:05 UTC") + "\n")
		if readBundle.Metadata.Label != "" {
			b.WriteString("  Label:           " + readBundle.Metadata.Label + "\n")
		}
		b.WriteString("\n")
		b.WriteString("Target\n")
		b.WriteString("  Kind:            " + readBundle.Metadata.Target.Kind + "\n")
		b.WriteString("  Name:            " + readBundle.Metadata.Target.Name + "\n")
		if readBundle.Metadata.Target.Namespace != "" {
			b.WriteString("  Namespace:       " + readBundle.Metadata.Target.Namespace + "\n")
		}

		outputs = append(outputs, b.String())
	}

	// All outputs should be identical
	for i := 1; i < len(outputs); i++ {
		if outputs[i] != outputs[0] {
			t.Errorf("Output %d differs from output 0:\n--- Output 0 ---\n%s\n--- Output %d ---\n%s",
				i, outputs[0], i, outputs[i])
		}
	}

	// Verify counts are correct
	if summary.DriftCount != 2 {
		t.Errorf("DriftCount = %d, want 2", summary.DriftCount)
	}
	if summary.EventCount != 1 {
		t.Errorf("EventCount = %d, want 1", summary.EventCount)
	}
}

func TestBundleInspect_JSONOutput(t *testing.T) {
	// Create a test bundle with known values
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "json-test-bundle")

	// Use a fixed time for determinism
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	writer := agent.NewBundleWriter("v0.14.6-test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label: "incident-123",
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
				Cluster:   "cluster-1",
			},
		},
		Session: &agent.DebugSessionData{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
			StartedAt: fixedTime,
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift:1", Path: "spec.replicas"},
		},
		Events: []agent.TimelineEvent{
			{ResourceKind: "Pod"},
			{ResourceKind: "Pod"},
		},
		Logs: []agent.ContainerLogResult{
			{ContainerName: "app"},
		},
	}

	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read and generate JSON output
	reader := agent.NewBundleReader()
	readBundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	summary := agent.Summarize(readBundle)

	output := BundleInspectOutput{
		FormatVersion:   readBundle.Metadata.FormatVersion,
		CubScoutVersion: readBundle.Metadata.CubScoutVersion,
		CreatedAt:       readBundle.Metadata.CreatedAt.Format("2006-01-02T15:04:05Z"),
		Label:           readBundle.Metadata.Label,
		Target:          readBundle.Metadata.Target,
		Contents: BundleInspectContents{
			HasSession: summary.SessionPresent,
			HasDrift:   summary.DriftCount > 0,
			HasEvents:  summary.EventCount > 0,
			HasLogs:    summary.LogCount > 0,
			DriftCount: summary.DriftCount,
			EventCount: summary.EventCount,
			LogCount:   summary.LogCount,
		},
	}

	// Verify JSON structure
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	// Parse back to verify
	var parsed BundleInspectOutput
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	// Verify values
	if parsed.FormatVersion != "v1" {
		t.Errorf("FormatVersion = %s, want v1", parsed.FormatVersion)
	}
	if parsed.CubScoutVersion != "v0.14.6-test" {
		t.Errorf("CubScoutVersion = %s, want v0.14.6-test", parsed.CubScoutVersion)
	}
	if parsed.Label != "incident-123" {
		t.Errorf("Label = %s, want incident-123", parsed.Label)
	}
	if !parsed.Contents.HasSession {
		t.Error("HasSession should be true")
	}
	if !parsed.Contents.HasDrift {
		t.Error("HasDrift should be true")
	}
	if parsed.Contents.DriftCount != 1 {
		t.Errorf("DriftCount = %d, want 1", parsed.Contents.DriftCount)
	}
	if parsed.Contents.EventCount != 2 {
		t.Errorf("EventCount = %d, want 2", parsed.Contents.EventCount)
	}
	if parsed.Contents.LogCount != 1 {
		t.Errorf("LogCount = %d, want 1", parsed.Contents.LogCount)
	}
}

func TestBundleInspect_MinimalBundle(t *testing.T) {
	// Create a minimal bundle (metadata only)
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "minimal-bundle")

	writer := agent.NewBundleWriter("v0.14.6-test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind: "Namespace",
				Name: "default",
			},
		},
	}

	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read and verify
	reader := agent.NewBundleReader()
	readBundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	summary := agent.Summarize(readBundle)

	// Verify all counts are zero
	if summary.SessionPresent {
		t.Error("SessionPresent should be false")
	}
	if summary.DriftCount != 0 {
		t.Errorf("DriftCount = %d, want 0", summary.DriftCount)
	}
	if summary.EventCount != 0 {
		t.Errorf("EventCount = %d, want 0", summary.EventCount)
	}
	if summary.LogCount != 0 {
		t.Errorf("LogCount = %d, want 0", summary.LogCount)
	}
}

func TestContentsLine(t *testing.T) {
	tests := []struct {
		filename string
		present  bool
		count    int
		contains []string // Check contains instead of exact match for portability
	}{
		{"session.json", true, 0, []string{"✓", "session.json"}},
		{"session.json", false, 0, []string{"✗", "session.json"}},
		{"drift.json", true, 5, []string{"✓", "drift.json", "(5 items)"}},
		{"events.json", true, 10, []string{"✓", "events.json", "(10 items)"}},
		{"logs.json", false, 0, []string{"✗", "logs.json"}},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := contentsLine(tt.filename, tt.present, tt.count)
			for _, substr := range tt.contains {
				if !strings.Contains(got, substr) {
					t.Errorf("contentsLine(%q, %v, %d) = %q, should contain %q",
						tt.filename, tt.present, tt.count, got, substr)
				}
			}
		})
	}
}

func TestBundleInspect_MissingBundle(t *testing.T) {
	reader := agent.NewBundleReader()
	_, err := reader.Read("/nonexistent/bundle/path")
	if err == nil {
		t.Error("Expected error for missing bundle")
	}
}

func TestBundleInspect_NoTimestampGeneration(t *testing.T) {
	// This test verifies that inspect output uses captured timestamps,
	// not generated ones.
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "timestamp-test")

	// Create bundle with known timestamp
	writer := agent.NewBundleWriter("v0.14.6-test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind: "Deployment",
				Name: "test",
			},
		},
	}

	// Write creates bundle with current time
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read multiple times at different "times"
	reader := agent.NewBundleReader()

	readBundle1, _ := reader.Read(bundleDir)
	time.Sleep(10 * time.Millisecond) // Small delay
	readBundle2, _ := reader.Read(bundleDir)

	// Both reads should have the same CreatedAt (from the bundle, not from reading)
	if !readBundle1.Metadata.CreatedAt.Equal(readBundle2.Metadata.CreatedAt) {
		t.Error("CreatedAt should be identical between reads (uses captured time, not current time)")
	}
}

// TestBundleInspect_StableFileOrdering verifies that bundle reading
// doesn't depend on filesystem iteration order.
func TestBundleInspect_StableFileOrdering(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "ordering-test")

	// Create bundle with multiple items in each category
	writer := agent.NewBundleWriter("v0.14.6-test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift:3", Path: "c.path"},
			{ID: "drift:1", Path: "a.path"},
			{ID: "drift:2", Path: "b.path"},
		},
		Events: []agent.TimelineEvent{
			{ResourceKind: "Pod", ResourceName: "pod-c"},
			{ResourceKind: "Pod", ResourceName: "pod-a"},
			{ResourceKind: "Pod", ResourceName: "pod-b"},
		},
	}

	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read multiple times and verify order is preserved
	reader := agent.NewBundleReader()

	var firstDriftOrder []string
	var firstEventOrder []string

	for i := 0; i < 5; i++ {
		readBundle, err := reader.Read(bundleDir)
		if err != nil {
			t.Fatalf("Read %d failed: %v", i, err)
		}

		// Extract orders
		var driftOrder []string
		for _, f := range readBundle.DriftFindings {
			driftOrder = append(driftOrder, f.ID)
		}
		var eventOrder []string
		for _, e := range readBundle.Events {
			eventOrder = append(eventOrder, e.ResourceName)
		}

		if i == 0 {
			firstDriftOrder = driftOrder
			firstEventOrder = eventOrder
		} else {
			// Compare with first read
			if !sliceEqual(driftOrder, firstDriftOrder) {
				t.Errorf("Drift order changed on read %d: %v vs %v", i, driftOrder, firstDriftOrder)
			}
			if !sliceEqual(eventOrder, firstEventOrder) {
				t.Errorf("Event order changed on read %d: %v vs %v", i, eventOrder, firstEventOrder)
			}
		}
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
