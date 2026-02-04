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

// Replay tests

func TestBundleReplay_DriftJSON(t *testing.T) {
	// Create a bundle with drift findings
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "replay-drift-bundle")

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
			{
				ID:             "drift:Deployment:prod/api:spec.replicas",
				ObjectID:       "Deployment:prod/api",
				Path:           "spec.replicas",
				Classification: agent.DriftCapacity,
				Severity:       agent.DriftSeverityWarning,
			},
		},
	}

	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read and build report
	reader := agent.NewBundleReader()
	readBundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	report := agent.DriftReport{
		Command:  "bundle replay",
		Findings: readBundle.DriftFindings,
		Summary:  buildDriftSummaryFromFindings(readBundle.DriftFindings),
	}

	// Verify report structure
	if report.Command != "bundle replay" {
		t.Errorf("Command = %s, want bundle replay", report.Command)
	}
	if len(report.Findings) != 1 {
		t.Errorf("Findings count = %d, want 1", len(report.Findings))
	}
	if report.Summary.TotalFindings != 1 {
		t.Errorf("Summary.TotalFindings = %d, want 1", report.Summary.TotalFindings)
	}
}

func TestBundleReplay_DriftASCII_Deterministic(t *testing.T) {
	// Create a bundle with drift findings
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "replay-ascii-bundle")

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
			{
				ID:             "drift:Deployment:prod/api:spec.replicas",
				ObjectID:       "Deployment:prod/api",
				Path:           "spec.replicas",
				Classification: agent.DriftCapacity,
				Severity:       agent.DriftSeverityWarning,
			},
			{
				ID:             "drift:Deployment:prod/api:spec.template.spec.containers[0].image",
				ObjectID:       "Deployment:prod/api",
				Path:           "spec.template.spec.containers[0].image",
				Classification: agent.DriftImage,
				Severity:       agent.DriftSeverityCritical,
			},
		},
	}

	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read and render multiple times
	reader := agent.NewBundleReader()
	var outputs []string

	for i := 0; i < 3; i++ {
		readBundle, err := reader.Read(bundleDir)
		if err != nil {
			t.Fatalf("Read %d failed: %v", i, err)
		}

		report := agent.DriftReport{
			Command:  "bundle replay",
			Findings: readBundle.DriftFindings,
			Summary:  buildDriftSummaryFromFindings(readBundle.DriftFindings),
		}

		output := agent.RenderDriftASCII(report)
		outputs = append(outputs, output)
	}

	// All outputs should be identical
	for i := 1; i < len(outputs); i++ {
		if outputs[i] != outputs[0] {
			t.Errorf("ASCII output %d differs from output 0", i)
		}
	}
}

func TestBundleReplay_Correlation(t *testing.T) {
	// Create a bundle with drift and events
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "replay-corr-bundle")

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
			{
				ID:             "drift:1",
				Path:           "spec.replicas",
				Classification: agent.DriftCapacity,
			},
		},
		Events: []agent.TimelineEvent{
			{
				ResourceKind: "Deployment",
				ResourceName: "api",
				K8sEvent:     agent.K8sEvent{Type: "Warning", Reason: "FailedScheduling"},
			},
		},
	}

	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read and build correlation
	reader := agent.NewBundleReader()
	readBundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	corr := agent.DriftCorrelation{
		ObjectID:   "Deployment:prod/api",
		Findings:   readBundle.DriftFindings,
		Events:     readBundle.Events,
		Logs:       readBundle.Logs,
		HasDrift:   len(readBundle.DriftFindings) > 0,
		HasFailure: hasFailureSignalsInBundle(readBundle.Events, readBundle.Logs),
	}

	// Verify correlation structure
	if !corr.HasDrift {
		t.Error("HasDrift should be true")
	}
	if !corr.HasFailure {
		t.Error("HasFailure should be true (Warning event)")
	}

	// Render and check for expected content
	output := agent.RenderCorrelationASCII(corr)
	if !strings.Contains(output, "Drift-Debug Correlation") {
		t.Error("Output should contain header")
	}
	if !strings.Contains(output, "spec.replicas") {
		t.Error("Output should contain drift path")
	}
}

func TestBundleReplay_FailOnSeverity(t *testing.T) {
	findings := []agent.DriftFinding{
		{Severity: agent.DriftSeverityInfo},
		{Severity: agent.DriftSeverityWarning},
	}

	tests := []struct {
		name     string
		failOn   string
		wantExit int
	}{
		{"no fail-on", "", BundleReplayExitOK},
		{"fail-on info, has warning", "info", BundleReplayExitFailure},
		{"fail-on warning, has warning", "warning", BundleReplayExitFailure},
		{"fail-on critical, has warning", "critical", BundleReplayExitOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeReplayExitCode(findings, tt.failOn)
			if got != tt.wantExit {
				t.Errorf("computeReplayExitCode() = %d, want %d", got, tt.wantExit)
			}
		})
	}
}

func TestBundleReplay_NoFindings(t *testing.T) {
	findings := []agent.DriftFinding{}

	// Even with fail-on, empty findings should return OK
	got := computeReplayExitCode(findings, "info")
	if got != BundleReplayExitOK {
		t.Errorf("computeReplayExitCode(empty, info) = %d, want %d", got, BundleReplayExitOK)
	}
}

func TestBuildDriftSummaryFromFindings(t *testing.T) {
	findings := []agent.DriftFinding{
		{Severity: agent.DriftSeverityCritical, Classification: agent.DriftImage},
		{Severity: agent.DriftSeverityWarning, Classification: agent.DriftCapacity},
		{Severity: agent.DriftSeverityWarning, Classification: agent.DriftCapacity},
		{Severity: agent.DriftSeverityInfo, Classification: agent.DriftConfig},
	}

	summary := buildDriftSummaryFromFindings(findings)

	if summary.TotalFindings != 4 {
		t.Errorf("TotalFindings = %d, want 4", summary.TotalFindings)
	}

	// Check severity counts
	severityMap := make(map[agent.DriftSeverity]int)
	for _, sc := range summary.BySeverity {
		severityMap[sc.Severity] = sc.Count
	}

	if severityMap[agent.DriftSeverityCritical] != 1 {
		t.Errorf("Critical count = %d, want 1", severityMap[agent.DriftSeverityCritical])
	}
	if severityMap[agent.DriftSeverityWarning] != 2 {
		t.Errorf("Warning count = %d, want 2", severityMap[agent.DriftSeverityWarning])
	}
	if severityMap[agent.DriftSeverityInfo] != 1 {
		t.Errorf("Info count = %d, want 1", severityMap[agent.DriftSeverityInfo])
	}

	// Check classification counts
	classMap := make(map[agent.DriftClassification]int)
	for _, cc := range summary.ByClassification {
		classMap[cc.Classification] = cc.Count
	}

	if classMap[agent.DriftImage] != 1 {
		t.Errorf("Image count = %d, want 1", classMap[agent.DriftImage])
	}
	if classMap[agent.DriftCapacity] != 2 {
		t.Errorf("Capacity count = %d, want 2", classMap[agent.DriftCapacity])
	}
	if classMap[agent.DriftConfig] != 1 {
		t.Errorf("Config count = %d, want 1", classMap[agent.DriftConfig])
	}
}

func TestHasFailureSignalsInBundle(t *testing.T) {
	tests := []struct {
		name   string
		events []agent.TimelineEvent
		logs   []agent.ContainerLogResult
		want   bool
	}{
		{
			name:   "no signals",
			events: []agent.TimelineEvent{{K8sEvent: agent.K8sEvent{Type: "Normal"}}},
			logs:   []agent.ContainerLogResult{},
			want:   false,
		},
		{
			name:   "warning event",
			events: []agent.TimelineEvent{{K8sEvent: agent.K8sEvent{Type: "Warning"}}},
			logs:   []agent.ContainerLogResult{},
			want:   true,
		},
		{
			name:   "error severity",
			events: []agent.TimelineEvent{{Severity: "error"}},
			logs:   []agent.ContainerLogResult{},
			want:   true,
		},
		{
			name:   "log error",
			events: []agent.TimelineEvent{},
			logs:   []agent.ContainerLogResult{{Error: "connection refused"}},
			want:   true,
		},
		{
			name:   "log pattern error",
			events: []agent.TimelineEvent{},
			logs:   []agent.ContainerLogResult{{Patterns: []agent.LogPattern{{Type: "error"}}}},
			want:   true,
		},
		{
			name:   "log pattern panic",
			events: []agent.TimelineEvent{},
			logs:   []agent.ContainerLogResult{{Patterns: []agent.LogPattern{{Type: "panic"}}}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasFailureSignalsInBundle(tt.events, tt.logs)
			if got != tt.want {
				t.Errorf("hasFailureSignalsInBundle() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Bundle Diff tests

func TestBundleDiff_JSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two bundles
	bundleDirA := filepath.Join(tmpDir, "bundle-a")
	bundleDirB := filepath.Join(tmpDir, "bundle-b")

	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	writer := agent.NewBundleWriter("v0.14.6-test")

	bundleA := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label: "before",
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift-1", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
		},
	}
	// Manually set time for determinism
	bundleA.Metadata.CreatedAt = t1

	bundleB := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label: "after",
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift-1", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
			{ID: "drift-2", ObjectID: "Deployment:prod/api", Path: "spec.image"},
		},
	}
	bundleB.Metadata.CreatedAt = t2

	if err := writer.Write(bundleA, bundleDirA); err != nil {
		t.Fatalf("Write A failed: %v", err)
	}
	if err := writer.Write(bundleB, bundleDirB); err != nil {
		t.Fatalf("Write B failed: %v", err)
	}

	// Read bundles and diff
	reader := agent.NewBundleReader()
	readA, _ := reader.Read(bundleDirA)
	readB, _ := reader.Read(bundleDirB)

	diff, err := agent.DiffBundles(readA, readB, agent.JoinObjectID)
	if err != nil {
		t.Fatalf("DiffBundles failed: %v", err)
	}

	// Marshal to JSON
	jsonBytes, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	// Parse back and verify structure
	var parsed agent.BundleDiff
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if parsed.SchemaVersion != agent.BundleDiffSchemaVersion {
		t.Errorf("SchemaVersion = %s, want %s", parsed.SchemaVersion, agent.BundleDiffSchemaVersion)
	}
	if parsed.JoinMode != agent.JoinObjectID {
		t.Errorf("JoinMode = %s, want object_id", parsed.JoinMode)
	}
}

func TestBundleDiff_ASCII(t *testing.T) {
	tmpDir := t.TempDir()

	bundleDirA := filepath.Join(tmpDir, "bundle-a")
	bundleDirB := filepath.Join(tmpDir, "bundle-b")

	writer := agent.NewBundleWriter("v0.14.6-test")

	bundleA := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label: "before",
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
	}

	bundleB := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label: "after",
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
	}

	if err := writer.Write(bundleA, bundleDirA); err != nil {
		t.Fatalf("Write A failed: %v", err)
	}
	if err := writer.Write(bundleB, bundleDirB); err != nil {
		t.Fatalf("Write B failed: %v", err)
	}

	// Read and diff
	reader := agent.NewBundleReader()
	readA, _ := reader.Read(bundleDirA)
	readB, _ := reader.Read(bundleDirB)

	diff, err := agent.DiffBundles(readA, readB, agent.JoinObjectID)
	if err != nil {
		t.Fatalf("DiffBundles failed: %v", err)
	}

	// Render ASCII
	output := agent.RenderBundleDiffASCII(diff)

	// Verify expected content
	if !strings.Contains(output, "Bundle Diff") {
		t.Error("Output missing 'Bundle Diff' header")
	}
	if !strings.Contains(output, "From:") {
		t.Error("Output missing 'From:' line")
	}
	if !strings.Contains(output, "To:") {
		t.Error("Output missing 'To:' line")
	}
	if !strings.Contains(output, "Join: object_id") {
		t.Error("Output missing 'Join: object_id'")
	}
	if !strings.Contains(output, "Summary") {
		t.Error("Output missing 'Summary' section")
	}
}

func TestBundleDiff_JoinCompositeNotImplemented(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "bundle")

	writer := agent.NewBundleWriter("v0.14.6-test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{Kind: "Deployment", Name: "api"},
		},
	}
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader := agent.NewBundleReader()
	readBundle, _ := reader.Read(bundleDir)

	_, err := agent.DiffBundles(readBundle, readBundle, agent.JoinComposite)
	if err == nil {
		t.Fatal("Expected error for composite join")
	}

	expected := "join mode 'composite' not yet implemented"
	if err.Error() != expected {
		t.Errorf("Error = %q, want exactly %q", err.Error(), expected)
	}
}

func TestBundleDiff_JoinNoneNotImplemented(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "bundle")

	writer := agent.NewBundleWriter("v0.14.6-test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{Kind: "Deployment", Name: "api"},
		},
	}
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader := agent.NewBundleReader()
	readBundle, _ := reader.Read(bundleDir)

	_, err := agent.DiffBundles(readBundle, readBundle, agent.JoinNone)
	if err == nil {
		t.Fatal("Expected error for none join")
	}

	expected := "join mode 'none' not yet implemented"
	if err.Error() != expected {
		t.Errorf("Error = %q, want exactly %q", err.Error(), expected)
	}
}

func TestBundleDiff_BadPath(t *testing.T) {
	reader := agent.NewBundleReader()
	_, err := reader.Read("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

// Golden test A: No changes
func TestBundleDiff_GoldenA_NoChanges(t *testing.T) {
	tmpDir := t.TempDir()

	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	bundleDirA := filepath.Join(tmpDir, "bundle-a")
	bundleDirB := filepath.Join(tmpDir, "bundle-b")

	writer := agent.NewBundleWriter("v0.14.6-test")

	// Create identical bundles
	bundleA := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label: "snapshot-1",
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift-1", ObjectID: "Deployment:prod/api", Path: "spec.replicas", Desired: 3, Live: 2},
		},
	}
	bundleA.Metadata.CreatedAt = t1

	bundleB := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label: "snapshot-2",
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift-1", ObjectID: "Deployment:prod/api", Path: "spec.replicas", Desired: 3, Live: 2},
		},
	}
	bundleB.Metadata.CreatedAt = t2

	writer.Write(bundleA, bundleDirA)
	writer.Write(bundleB, bundleDirB)

	reader := agent.NewBundleReader()
	readA, _ := reader.Read(bundleDirA)
	readB, _ := reader.Read(bundleDirB)

	diff, _ := agent.DiffBundles(readA, readB, agent.JoinObjectID)

	// Verify: all unchanged
	if diff.Summary.Unchanged != 1 {
		t.Errorf("Summary.Unchanged = %d, want 1", diff.Summary.Unchanged)
	}
	if diff.Summary.Changed != 0 {
		t.Errorf("Summary.Changed = %d, want 0", diff.Summary.Changed)
	}
	if diff.Summary.Added != 0 {
		t.Errorf("Summary.Added = %d, want 0", diff.Summary.Added)
	}
	if diff.Summary.Removed != 0 {
		t.Errorf("Summary.Removed = %d, want 0", diff.Summary.Removed)
	}

	// Verify object status
	for _, obj := range diff.Objects {
		if obj.Status != agent.StatusUnchanged {
			t.Errorf("Object %s status = %v, want unchanged", obj.Identity, obj.Status)
		}
	}
}

// Golden test B: Added/removed + drift changed
func TestBundleDiff_GoldenB_AddedRemovedChanged(t *testing.T) {
	tmpDir := t.TempDir()

	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	bundleDirA := filepath.Join(tmpDir, "bundle-a")
	bundleDirB := filepath.Join(tmpDir, "bundle-b")

	writer := agent.NewBundleWriter("v0.14.6-test")

	// Bundle A: obj-1 (drift count 2), obj-2
	bundleA := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label:  "before",
			Target: agent.BundleTarget{Kind: "Deployment", Name: "api", Namespace: "prod"},
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift-1a", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
			{ID: "drift-1b", ObjectID: "Deployment:prod/api", Path: "spec.image"},
			{ID: "drift-2a", ObjectID: "Deployment:prod/worker", Path: "spec.replicas"},
		},
	}
	bundleA.Metadata.CreatedAt = t1

	// Bundle B: obj-1 (drift count 5 - changed), obj-2 removed, obj-3 added
	bundleB := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label:  "after",
			Target: agent.BundleTarget{Kind: "Deployment", Name: "api", Namespace: "prod"},
		},
		DriftFindings: []agent.DriftFinding{
			{ID: "drift-1a", ObjectID: "Deployment:prod/api", Path: "spec.replicas"},
			{ID: "drift-1b", ObjectID: "Deployment:prod/api", Path: "spec.image"},
			{ID: "drift-1c", ObjectID: "Deployment:prod/api", Path: "spec.resources"},
			{ID: "drift-1d", ObjectID: "Deployment:prod/api", Path: "spec.env"},
			{ID: "drift-1e", ObjectID: "Deployment:prod/api", Path: "spec.labels"},
			{ID: "drift-3a", ObjectID: "Deployment:prod/frontend", Path: "spec.replicas"},
		},
	}
	bundleB.Metadata.CreatedAt = t2

	writer.Write(bundleA, bundleDirA)
	writer.Write(bundleB, bundleDirB)

	reader := agent.NewBundleReader()
	readA, _ := reader.Read(bundleDirA)
	readB, _ := reader.Read(bundleDirB)

	diff, _ := agent.DiffBundles(readA, readB, agent.JoinObjectID)

	// Verify counts
	if diff.Summary.Changed != 1 {
		t.Errorf("Summary.Changed = %d, want 1", diff.Summary.Changed)
	}
	if diff.Summary.Removed != 1 {
		t.Errorf("Summary.Removed = %d, want 1", diff.Summary.Removed)
	}
	if diff.Summary.Added != 1 {
		t.Errorf("Summary.Added = %d, want 1", diff.Summary.Added)
	}

	// Verify ordering is deterministic: api, frontend, worker
	expectedOrder := []string{
		"Deployment:prod/api",
		"Deployment:prod/frontend",
		"Deployment:prod/worker",
	}
	for i, obj := range diff.Objects {
		if obj.Identity != expectedOrder[i] {
			t.Errorf("Objects[%d].Identity = %q, want %q", i, obj.Identity, expectedOrder[i])
		}
	}

	// Verify api shows drift:changed
	var apiObj *agent.ObjectDiff
	for i := range diff.Objects {
		if diff.Objects[i].Identity == "Deployment:prod/api" {
			apiObj = &diff.Objects[i]
			break
		}
	}
	if apiObj == nil {
		t.Fatal("api object not found")
	}

	var driftSection *agent.SectionDiff
	for i := range apiObj.Sections {
		if apiObj.Sections[i].Name == "drift" {
			driftSection = &apiObj.Sections[i]
			break
		}
	}
	if driftSection == nil {
		t.Fatal("drift section not found")
	}
	if driftSection.Status != agent.SectionChanged {
		t.Errorf("drift section status = %v, want changed", driftSection.Status)
	}
}

// Golden test C: Correlation changed
func TestBundleDiff_GoldenC_CorrelationChanged(t *testing.T) {
	tmpDir := t.TempDir()

	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC)

	bundleDirA := filepath.Join(tmpDir, "bundle-a")
	bundleDirB := filepath.Join(tmpDir, "bundle-b")

	writer := agent.NewBundleWriter("v0.14.6-test")

	// Bundle A with correlation
	bundleA := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label:  "before",
			Target: agent.BundleTarget{Kind: "Deployment", Name: "api", Namespace: "prod"},
		},
		Session: &agent.DebugSessionData{
			Target: agent.BundleTarget{Kind: "Deployment", Name: "api", Namespace: "prod"},
			RootCause: &agent.RootCauseSnapshot{
				Category:   "deployment",
				Stage:      "rollout",
				Confidence: "high",
				Summary:    "Rollout stuck due to insufficient resources",
			},
		},
	}
	bundleA.Metadata.CreatedAt = t1

	// Bundle B with different correlation
	bundleB := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Label:  "after",
			Target: agent.BundleTarget{Kind: "Deployment", Name: "api", Namespace: "prod"},
		},
		Session: &agent.DebugSessionData{
			Target: agent.BundleTarget{Kind: "Deployment", Name: "api", Namespace: "prod"},
			RootCause: &agent.RootCauseSnapshot{
				Category:   "deployment",
				Stage:      "running",
				Confidence: "high",
				Summary:    "Rollout completed successfully", // Changed!
			},
		},
	}
	bundleB.Metadata.CreatedAt = t2

	writer.Write(bundleA, bundleDirA)
	writer.Write(bundleB, bundleDirB)

	reader := agent.NewBundleReader()
	readA, _ := reader.Read(bundleDirA)
	readB, _ := reader.Read(bundleDirB)

	diff, _ := agent.DiffBundles(readA, readB, agent.JoinObjectID)

	// Verify object is changed
	if diff.Summary.Changed != 1 {
		t.Errorf("Summary.Changed = %d, want 1", diff.Summary.Changed)
	}

	// Find the api object and verify correlation changed
	var apiObj *agent.ObjectDiff
	for i := range diff.Objects {
		if diff.Objects[i].Identity == "Deployment:prod/api" {
			apiObj = &diff.Objects[i]
			break
		}
	}
	if apiObj == nil {
		t.Fatal("api object not found")
	}
	if apiObj.Status != agent.StatusChanged {
		t.Errorf("api status = %v, want changed", apiObj.Status)
	}

	// Find correlation section
	var corrSection *agent.SectionDiff
	for i := range apiObj.Sections {
		if apiObj.Sections[i].Name == "correlation" {
			corrSection = &apiObj.Sections[i]
			break
		}
	}
	if corrSection == nil {
		t.Fatal("correlation section not found")
	}
	if corrSection.Status != agent.SectionChanged {
		t.Errorf("correlation section status = %v, want changed", corrSection.Status)
	}
}

// Golden test D: Error strings (already covered by TestBundleDiff_JoinCompositeNotImplemented and TestBundleDiff_JoinNoneNotImplemented)
