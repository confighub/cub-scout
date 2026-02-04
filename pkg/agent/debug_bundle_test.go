// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBundleWriter_Write(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "test-bundle")

	writer := NewBundleWriter("v0.14.6-test")

	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			Label: "test-bundle",
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		Session: &DebugSessionData{
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
			StartedAt: time.Now().UTC(),
		},
		DriftFindings: []DriftFinding{
			{
				ID:             "drift:1",
				Path:           "spec.replicas",
				Classification: DriftCapacity,
			},
		},
		Events: []TimelineEvent{
			{
				ResourceKind: "Deployment",
				ResourceName: "api",
				K8sEvent:     K8sEvent{Type: "Warning", Reason: "FailedScheduling"},
			},
		},
		Logs: []ContainerLogResult{
			{
				ContainerName: "app",
				Lines:         []string{"log line 1", "log line 2"},
			},
		},
	}

	err := writer.Write(bundle, bundleDir)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify all expected files exist
	expectedFiles := []string{
		"metadata.json",
		"session.json",
		"drift.json",
		"events.json",
		"logs.json",
		"README.md",
	}

	for _, file := range expectedFiles {
		path := filepath.Join(bundleDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s to exist", file)
		}
	}

	// Verify metadata has correct values
	if bundle.Metadata.FormatVersion != BundleFormatVersion {
		t.Errorf("FormatVersion = %s, want %s", bundle.Metadata.FormatVersion, BundleFormatVersion)
	}
	if bundle.Metadata.CubScoutVersion != "v0.14.6-test" {
		t.Errorf("CubScoutVersion = %s, want v0.14.6-test", bundle.Metadata.CubScoutVersion)
	}
	if !bundle.Metadata.Contents.HasSession {
		t.Error("Contents.HasSession should be true")
	}
	if !bundle.Metadata.Contents.HasDrift {
		t.Error("Contents.HasDrift should be true")
	}
	if !bundle.Metadata.Contents.HasEvents {
		t.Error("Contents.HasEvents should be true")
	}
	if !bundle.Metadata.Contents.HasLogs {
		t.Error("Contents.HasLogs should be true")
	}
}

func TestBundleWriter_WriteMinimal(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "minimal-bundle")

	writer := NewBundleWriter("v0.14.6-test")

	// Minimal bundle with only metadata
	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			Target: BundleTarget{
				Kind: "Deployment",
				Name: "api",
			},
		},
	}

	err := writer.Write(bundle, bundleDir)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Only metadata and README should exist
	mustExist := []string{"metadata.json", "README.md"}
	mustNotExist := []string{"session.json", "drift.json", "events.json", "logs.json"}

	for _, file := range mustExist {
		path := filepath.Join(bundleDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file %s to exist", file)
		}
	}

	for _, file := range mustNotExist {
		path := filepath.Join(bundleDir, file)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("File %s should not exist for minimal bundle", file)
		}
	}

	// Verify contents flags are false
	if bundle.Metadata.Contents.HasSession {
		t.Error("Contents.HasSession should be false")
	}
	if bundle.Metadata.Contents.HasDrift {
		t.Error("Contents.HasDrift should be false")
	}
}

func TestBundleReader_Read(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "read-test-bundle")

	// Write a bundle first
	writer := NewBundleWriter("v0.14.6-test")
	originalBundle := &DebugBundle{
		Metadata: BundleMetadata{
			Label: "read-test",
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
				Cluster:   "cluster-1",
			},
		},
		Session: &DebugSessionData{
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
			StartedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		},
		DriftFindings: []DriftFinding{
			{
				ID:             "drift:1",
				Path:           "spec.replicas",
				Classification: DriftCapacity,
				Severity:       DriftSeverityWarning,
			},
		},
		Events: []TimelineEvent{
			{
				ResourceKind: "Deployment",
				ResourceName: "api",
				K8sEvent:     K8sEvent{Type: "Warning", Reason: "FailedScheduling"},
			},
		},
		Logs: []ContainerLogResult{
			{
				ContainerName: "app",
				Lines:         []string{"log line 1"},
			},
		},
	}

	if err := writer.Write(originalBundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read it back
	reader := NewBundleReader()
	readBundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify metadata
	if readBundle.Metadata.Label != "read-test" {
		t.Errorf("Label = %s, want read-test", readBundle.Metadata.Label)
	}
	if readBundle.Metadata.Target.Kind != "Deployment" {
		t.Errorf("Target.Kind = %s, want Deployment", readBundle.Metadata.Target.Kind)
	}
	if readBundle.Metadata.Target.Namespace != "prod" {
		t.Errorf("Target.Namespace = %s, want prod", readBundle.Metadata.Target.Namespace)
	}

	// Verify session
	if readBundle.Session == nil {
		t.Fatal("Session should not be nil")
	}
	if readBundle.Session.Target.Name != "api" {
		t.Errorf("Session.Target.Name = %s, want api", readBundle.Session.Target.Name)
	}

	// Verify drift findings
	if len(readBundle.DriftFindings) != 1 {
		t.Fatalf("Expected 1 drift finding, got %d", len(readBundle.DriftFindings))
	}
	if readBundle.DriftFindings[0].Path != "spec.replicas" {
		t.Errorf("DriftFindings[0].Path = %s, want spec.replicas", readBundle.DriftFindings[0].Path)
	}

	// Verify events
	if len(readBundle.Events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(readBundle.Events))
	}

	// Verify logs
	if len(readBundle.Logs) != 1 {
		t.Fatalf("Expected 1 log result, got %d", len(readBundle.Logs))
	}
}

func TestBundleReader_ReadMissingMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	reader := NewBundleReader()
	_, err := reader.Read(tmpDir)
	if err == nil {
		t.Error("Read should fail when metadata.json is missing")
	}
}

func TestBundleReader_ReadPartial(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "partial-bundle")

	// Write only metadata
	writer := NewBundleWriter("v0.14.6-test")
	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			Target: BundleTarget{
				Kind: "Deployment",
				Name: "api",
			},
		},
	}

	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read it back - should succeed with nil optional fields
	reader := NewBundleReader()
	readBundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if readBundle.Session != nil {
		t.Error("Session should be nil")
	}
	if len(readBundle.DriftFindings) != 0 {
		t.Error("DriftFindings should be empty")
	}
	if len(readBundle.Events) != 0 {
		t.Error("Events should be empty")
	}
	if len(readBundle.Logs) != 0 {
		t.Error("Logs should be empty")
	}
}

func TestSummarize(t *testing.T) {
	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			FormatVersion:   BundleFormatVersion,
			CubScoutVersion: "v0.14.6",
			CreatedAt:       time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			Label:           "incident-123",
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
		},
		Session: &DebugSessionData{},
		DriftFindings: []DriftFinding{
			{ID: "drift:1"},
			{ID: "drift:2"},
		},
		Events: []TimelineEvent{
			{ResourceKind: "Pod"},
			{ResourceKind: "Pod"},
			{ResourceKind: "Pod"},
		},
		Logs: []ContainerLogResult{
			{ContainerName: "app"},
		},
	}

	summary := Summarize(bundle)

	if summary.FormatVersion != BundleFormatVersion {
		t.Errorf("FormatVersion = %s, want %s", summary.FormatVersion, BundleFormatVersion)
	}
	if summary.CubScoutVersion != "v0.14.6" {
		t.Errorf("CubScoutVersion = %s, want v0.14.6", summary.CubScoutVersion)
	}
	if summary.Label != "incident-123" {
		t.Errorf("Label = %s, want incident-123", summary.Label)
	}
	if summary.Target != "Deployment/api in prod" {
		t.Errorf("Target = %s, want Deployment/api in prod", summary.Target)
	}
	if !summary.SessionPresent {
		t.Error("SessionPresent should be true")
	}
	if summary.DriftCount != 2 {
		t.Errorf("DriftCount = %d, want 2", summary.DriftCount)
	}
	if summary.EventCount != 3 {
		t.Errorf("EventCount = %d, want 3", summary.EventCount)
	}
	if summary.LogCount != 1 {
		t.Errorf("LogCount = %d, want 1", summary.LogCount)
	}
}

func TestSummarize_NoNamespace(t *testing.T) {
	bundle := &DebugBundle{
		Metadata: BundleMetadata{
			Target: BundleTarget{
				Kind: "Namespace",
				Name: "prod",
			},
		},
	}

	summary := Summarize(bundle)

	// Should not include "in <namespace>" when namespace is empty
	if summary.Target != "Namespace/prod" {
		t.Errorf("Target = %s, want Namespace/prod", summary.Target)
	}
}

func TestRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	bundleDir := filepath.Join(tmpDir, "roundtrip-bundle")

	// Create a complete bundle
	original := &DebugBundle{
		Metadata: BundleMetadata{
			Label: "roundtrip-test",
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
				Cluster:   "cluster-1",
			},
		},
		Session: &DebugSessionData{
			Target: BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "prod",
			},
			StartedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			CompletedAt: time.Date(2024, 1, 15, 10, 5, 0, 0, time.UTC),
			WorkloadHealth: &WorkloadHealthSnapshot{
				Kind:          "Deployment",
				Name:          "api",
				Namespace:     "prod",
				Replicas:      3,
				ReadyReplicas: 2,
				PodIssues: []PodIssue{
					{
						PodName:      "api-abc123",
						Phase:        "Running",
						RestartCount: 5,
					},
				},
			},
			OwnershipChain: &OwnershipSnapshot{
				Owner: "flux",
				K8sChain: []ChainLink{
					{Kind: "Deployment", Name: "api", Ready: true},
				},
			},
			DeployerStatus: &DeployerSnapshot{
				Kind:      "Kustomization",
				Name:      "api-deploy",
				Namespace: "flux-system",
				Ready:     true,
			},
			SourceStatus: &SourceSnapshot{
				Kind:      "GitRepository",
				Name:      "main-repo",
				Namespace: "flux-system",
				Ready:     true,
				URL:       "https://github.com/example/repo",
			},
			RootCause: &RootCauseSnapshot{
				Category:       "workload",
				Stage:          "runtime",
				Confidence:     "high",
				Summary:        "Pod restart loop",
				ProbableCauses: []string{"OOM", "Crash"},
				SuggestedFixes: []string{"Increase memory limits"},
			},
		},
		DriftFindings: []DriftFinding{
			{
				ID:             "drift:1",
				Path:           "spec.replicas",
				Classification: DriftCapacity,
				Severity:       DriftSeverityWarning,
				Desired:        int32(3),
				Live:           int32(5),
			},
		},
		Events: []TimelineEvent{
			{
				ResourceKind: "Deployment",
				ResourceName: "api",
				K8sEvent:     K8sEvent{Type: "Warning", Reason: "FailedScheduling"},
			},
		},
		Logs: []ContainerLogResult{
			{
				ContainerName: "app",
				Lines:         []string{"log line 1", "log line 2"},
				Patterns:      []LogPattern{{Type: "error", Match: "connection refused"}},
			},
		},
	}

	// Write
	writer := NewBundleWriter("v0.14.6-test")
	if err := writer.Write(original, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read back
	reader := NewBundleReader()
	restored, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify key fields survived round-trip
	if restored.Metadata.Label != original.Metadata.Label {
		t.Errorf("Label mismatch: %s vs %s", restored.Metadata.Label, original.Metadata.Label)
	}
	if restored.Session.RootCause.Summary != original.Session.RootCause.Summary {
		t.Errorf("RootCause.Summary mismatch: %s vs %s",
			restored.Session.RootCause.Summary, original.Session.RootCause.Summary)
	}
	if len(restored.Session.RootCause.ProbableCauses) != len(original.Session.RootCause.ProbableCauses) {
		t.Errorf("RootCause.ProbableCauses length mismatch")
	}
	// Test that pod issues came through
	if len(restored.Session.WorkloadHealth.PodIssues) != 1 {
		t.Errorf("PodIssues length mismatch")
	}
	if restored.Session.WorkloadHealth.PodIssues[0].RestartCount != original.Session.WorkloadHealth.PodIssues[0].RestartCount {
		t.Errorf("PodIssues[0].RestartCount mismatch")
	}
	if restored.DriftFindings[0].Path != original.DriftFindings[0].Path {
		t.Errorf("DriftFindings[0].Path mismatch: %s vs %s",
			restored.DriftFindings[0].Path, original.DriftFindings[0].Path)
	}
}

func TestYesNo(t *testing.T) {
	if yesNo(true) != "Yes" {
		t.Error("yesNo(true) should return Yes")
	}
	if yesNo(false) != "No" {
		t.Error("yesNo(false) should return No")
	}
}
