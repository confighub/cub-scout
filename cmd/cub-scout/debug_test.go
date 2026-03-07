// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestGenerateBundleDirName(t *testing.T) {
	tests := []struct {
		name   string
		target ResourceRef
		want   string
	}{
		{
			name:   "with namespace",
			target: ResourceRef{Kind: "Deployment", Name: "api", Namespace: "production"},
			want:   "deployment-production-api",
		},
		{
			name:   "without namespace",
			target: ResourceRef{Kind: "Namespace", Name: "default"},
			want:   "namespace-default",
		},
		{
			name:   "crossplane resource",
			target: ResourceRef{Kind: "XPostgreSQLInstance", Name: "staging-db", Namespace: "crossplane-system"},
			want:   "xpostgresqlinstance-crossplane-system-staging-db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateBundleDirName(tt.target)
			if got != tt.want {
				t.Errorf("generateBundleDirName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDebugBundleFromSession(t *testing.T) {
	session := &DebugSession{
		Target: ResourceRef{
			Kind:      "Deployment",
			Name:      "api",
			Namespace: "production",
		},
		StartedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2024, 1, 15, 10, 5, 0, 0, time.UTC),
		WorkloadStatus: &WorkloadHealthStatus{
			Kind:          "Deployment",
			Name:          "api",
			Namespace:     "production",
			Replicas:      3,
			ReadyReplicas: 2,
			PodIssues: []PodIssue{
				{PodName: "api-abc123", Phase: "Running", ContainerStatus: "CrashLoopBackOff"},
			},
		},
		RootCause: &RootCauseAnalysis{
			Category:   "crash_loop",
			Stage:      "workload",
			Confidence: "high",
			Summary:    "Pod is in CrashLoopBackOff",
		},
	}

	bundle := buildDebugBundleFromSession(session)

	// Verify bundle target
	if bundle.Metadata.Target.Kind != "Deployment" {
		t.Errorf("Target.Kind = %s, want Deployment", bundle.Metadata.Target.Kind)
	}
	if bundle.Metadata.Target.Name != "api" {
		t.Errorf("Target.Name = %s, want api", bundle.Metadata.Target.Name)
	}

	// Verify session data
	if bundle.Session == nil {
		t.Fatal("Session should not be nil")
	}
	if !bundle.Session.StartedAt.Equal(session.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", bundle.Session.StartedAt, session.StartedAt)
	}

	// Verify workload health
	if bundle.Session.WorkloadHealth == nil {
		t.Fatal("WorkloadHealth should not be nil")
	}
	if bundle.Session.WorkloadHealth.ReadyReplicas != 2 {
		t.Errorf("ReadyReplicas = %d, want 2", bundle.Session.WorkloadHealth.ReadyReplicas)
	}
	if len(bundle.Session.WorkloadHealth.PodIssues) != 1 {
		t.Errorf("PodIssues count = %d, want 1", len(bundle.Session.WorkloadHealth.PodIssues))
	}

	// Verify root cause
	if bundle.Session.RootCause == nil {
		t.Fatal("RootCause should not be nil")
	}
	if bundle.Session.RootCause.Category != "crash_loop" {
		t.Errorf("RootCause.Category = %s, want crash_loop", bundle.Session.RootCause.Category)
	}
}

func TestPopulateAttributionFromFixture_WithCrossplaneResource(t *testing.T) {
	// Create a test bundle
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Instance",
				Name:      "staging-db-abc123",
				Namespace: "crossplane-system",
			},
		},
	}

	// Use the Crossplane fixture
	fixturePath := filepath.Join("../../test/fixtures/crossplane/managed_with_composite_label.json")

	// Skip if fixture doesn't exist (CI may not have fixtures)
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("Fixture file not found, skipping test")
	}

	err := populateAttributionFromFixture(fixturePath, bundle)
	if err != nil {
		t.Fatalf("populateAttributionFromFixture failed: %v", err)
	}

	// The fixture has crossplane.io/composite label, so attribution should be populated
	if bundle.Attribution == nil {
		t.Fatal("Attribution should be populated for Crossplane resource")
	}
	if bundle.Attribution.SchemaVersion != agent.AttributionGraphSchemaVersion {
		t.Errorf("SchemaVersion = %s, want %s", bundle.Attribution.SchemaVersion, agent.AttributionGraphSchemaVersion)
	}
	if len(bundle.Attribution.Nodes) < 2 {
		t.Errorf("Nodes count = %d, want at least 2 (MR and XR)", len(bundle.Attribution.Nodes))
	}
}

func TestPopulateAttributionFromFixture_WithoutCrossplaneResource(t *testing.T) {
	// Create a test bundle
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api-server",
				Namespace: "production",
			},
		},
	}

	// Use the non-Crossplane fixture
	fixturePath := filepath.Join("../../test/fixtures/crossplane/deployment_no_crossplane.json")

	// Skip if fixture doesn't exist
	if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
		t.Skip("Fixture file not found, skipping test")
	}

	err := populateAttributionFromFixture(fixturePath, bundle)
	if err != nil {
		t.Fatalf("populateAttributionFromFixture failed: %v", err)
	}

	// Non-Crossplane resource should have no attribution
	if bundle.Attribution != nil && !bundle.Attribution.IsEmpty() {
		t.Error("Attribution should be nil or empty for non-Crossplane resource")
	}
}

func TestSaveDebugBundle_Integration(t *testing.T) {
	// This test verifies the full bundle save flow using test hooks
	tmpDir := t.TempDir()

	session := &DebugSession{
		Target: ResourceRef{
			Kind:      "Deployment",
			Name:      "api",
			Namespace: "production",
		},
		StartedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2024, 1, 15, 10, 5, 0, 0, time.UTC),
		WorkloadStatus: &WorkloadHealthStatus{
			Kind:          "Deployment",
			Name:          "api",
			Namespace:     "production",
			Replicas:      3,
			ReadyReplicas: 3,
		},
	}

	// Build bundle manually (without Kubernetes client)
	bundle := buildDebugBundleFromSession(session)

	// Write the bundle
	bundleDir := filepath.Join(tmpDir, generateBundleDirName(session.Target))
	writer := agent.NewBundleWriter("test-version")
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify bundle was written
	if _, err := os.Stat(filepath.Join(bundleDir, "metadata.json")); os.IsNotExist(err) {
		t.Error("metadata.json should exist")
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "session.json")); os.IsNotExist(err) {
		t.Error("session.json should exist")
	}

	// Read back and verify
	reader := agent.NewBundleReader()
	readBundle, err := reader.Read(bundleDir)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if readBundle.Metadata.Target.Kind != "Deployment" {
		t.Errorf("Target.Kind = %s, want Deployment", readBundle.Metadata.Target.Kind)
	}
	if readBundle.Session == nil {
		t.Fatal("Session should not be nil")
	}
	if readBundle.Session.WorkloadHealth == nil {
		t.Fatal("WorkloadHealth should not be nil")
	}
}

func TestSaveDebugBundle_WithAttribution(t *testing.T) {
	// Test that attribution is properly saved when present
	tmpDir := t.TempDir()

	session := &DebugSession{
		Target: ResourceRef{
			Kind:      "Instance",
			Name:      "staging-db",
			Namespace: "crossplane-system",
		},
		StartedAt: time.Now(),
	}

	// Build bundle
	bundle := buildDebugBundleFromSession(session)

	// Manually add attribution (simulating what populateAttribution would do)
	builder := agent.NewAttributionGraphBuilder()
	builder.AddNode(agent.AttributionNode{
		Type:    agent.NodeMR,
		Ref:     agent.AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
		Present: true,
	})
	builder.AddNode(agent.AttributionNode{
		Type:    agent.NodeXR,
		Ref:     agent.AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
		Present: false,
	})
	builder.AddEdge(agent.AttributionEdge{
		Type:     agent.EdgeOwns,
		From:     agent.GenerateNodeID(agent.NodeXR, agent.AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"}),
		To:       agent.GenerateNodeID(agent.NodeMR, agent.AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"}),
		Evidence: agent.EvidenceCompositeLabel,
	})
	bundle.Attribution = builder.Build()

	// Write
	bundleDir := filepath.Join(tmpDir, "test-bundle")
	writer := agent.NewBundleWriter("test-version")
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Verify attribution.json exists
	attrPath := filepath.Join(bundleDir, "attribution.json")
	if _, err := os.Stat(attrPath); os.IsNotExist(err) {
		t.Fatal("attribution.json should exist")
	}

	// Read and verify
	data, _ := os.ReadFile(attrPath)
	var graph agent.AttributionGraph
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatalf("Failed to parse attribution.json: %v", err)
	}

	if graph.SchemaVersion != agent.AttributionGraphSchemaVersion {
		t.Errorf("SchemaVersion = %s, want %s", graph.SchemaVersion, agent.AttributionGraphSchemaVersion)
	}
	if len(graph.Nodes) != 2 {
		t.Errorf("Nodes count = %d, want 2", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Errorf("Edges count = %d, want 1", len(graph.Edges))
	}
}

func TestLoadAndRenderDebugFromJSON_SaveBundle(t *testing.T) {
	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "debug.json")
	session := DebugSession{
		Target: ResourceRef{
			Kind:      "Deployment",
			Name:      "api-server",
			Namespace: "production",
		},
		StartedAt:   time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 3, 7, 12, 1, 0, 0, time.UTC),
	}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if err := os.WriteFile(fixturePath, data, 0644); err != nil {
		t.Fatalf("Write fixture failed: %v", err)
	}

	oldFormat := debugFormat
	oldSaveDir := debugSaveBundleDir
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer func() {
		debugFormat = oldFormat
		debugSaveBundleDir = oldSaveDir
		os.Stdout = oldStdout
		_ = r.Close()
	}()

	debugFormat = "json"
	debugSaveBundleDir = filepath.Join(tmpDir, "bundles")
	os.Stdout = w

	err = loadAndRenderDebugFromJSON(context.Background(), fixturePath)
	_ = w.Close()
	_, _ = io.Copy(io.Discard, r)
	if err != nil {
		t.Fatalf("loadAndRenderDebugFromJSON failed: %v", err)
	}

	bundleDir := filepath.Join(debugSaveBundleDir, generateBundleDirName(session.Target))
	if _, err := os.Stat(filepath.Join(bundleDir, "metadata.json")); err != nil {
		t.Fatalf("expected metadata.json in saved bundle: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "session.json")); err != nil {
		t.Fatalf("expected session.json in saved bundle: %v", err)
	}
}
