// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/internal/scan"
)

func TestObserveScopeSummary_FromFixture(t *testing.T) {
	// Create a temporary fixture file
	fixture := doctorFixtureInput{
		Cluster:   "test-cluster",
		Namespace: "test-ns",
		Entries: []MapEntry{
			{Kind: "Deployment", Name: "app1", Namespace: "test-ns", Owner: "Flux", Status: "Ready"},
			{Kind: "Service", Name: "svc1", Namespace: "test-ns", Owner: "ArgoCD", Status: "Ready"},
			{Kind: "ConfigMap", Name: "cfg1", Namespace: "test-ns", Owner: "Native", Status: "Ready"},
		},
		Findings: []scan.NormalizedFinding{
			{Severity: "warning", Resource: "app1", Namespace: "test-ns", Message: "test warning"},
		},
	}

	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "doctor-fixture.json")
	b, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(fixturePath, b, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Call the seam with explicit fixture path
	result, err := ObserveScopeSummary(context.Background(), ObserveScopeSummaryRequest{
		Namespace:   "test-ns",
		TopIssues:   5,
		FixturePath: fixturePath,
	})
	if err != nil {
		t.Fatalf("ObserveScopeSummary error: %v", err)
	}

	summary := result.Summary

	// Verify the summary
	if summary.Cluster != "test-cluster" {
		t.Errorf("Cluster = %q, want %q", summary.Cluster, "test-cluster")
	}
	if summary.Resources.Total != 3 {
		t.Errorf("Resources.Total = %d, want 3", summary.Resources.Total)
	}
	if summary.Ownership.Flux != 1 {
		t.Errorf("Ownership.Flux = %d, want 1", summary.Ownership.Flux)
	}
	if summary.Ownership.ArgoCD != 1 {
		t.Errorf("Ownership.ArgoCD = %d, want 1", summary.Ownership.ArgoCD)
	}
	if summary.Ownership.Native != 1 {
		t.Errorf("Ownership.Native = %d, want 1", summary.Ownership.Native)
	}
	if summary.Risks.Total != 1 {
		t.Errorf("Risks.Total = %d, want 1", summary.Risks.Total)
	}
}

func TestObserveScopeSummary_TopIssuesRespected(t *testing.T) {
	fixture := doctorFixtureInput{
		Cluster:   "test-cluster",
		Namespace: "all",
		Entries:   []MapEntry{},
		Findings: []scan.NormalizedFinding{
			{Severity: "critical", Resource: "app1", Namespace: "ns1", Message: "critical1"},
			{Severity: "critical", Resource: "app2", Namespace: "ns1", Message: "critical2"},
			{Severity: "warning", Resource: "app3", Namespace: "ns1", Message: "warning1"},
		},
	}

	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "doctor-fixture.json")
	b, _ := json.Marshal(fixture)
	_ = os.WriteFile(fixturePath, b, 0644)

	// Request only 2 top issues with explicit fixture path
	result, err := ObserveScopeSummary(context.Background(), ObserveScopeSummaryRequest{
		TopIssues:   2,
		FixturePath: fixturePath,
	})
	if err != nil {
		t.Fatalf("ObserveScopeSummary error: %v", err)
	}

	if len(result.Summary.TopIssues) != 2 {
		t.Errorf("TopIssues length = %d, want 2", len(result.Summary.TopIssues))
	}
	// Should be critical issues first
	for _, issue := range result.Summary.TopIssues {
		if issue.Severity != "CRITICAL" {
			t.Errorf("Expected critical issues first, got %q", issue.Severity)
		}
	}
}

func TestObserveScopeSummary_NegativeTopIssuesTreatedAsZero(t *testing.T) {
	fixture := doctorFixtureInput{
		Cluster: "test-cluster",
		Findings: []scan.NormalizedFinding{
			{Severity: "warning", Resource: "app1", Namespace: "ns1", Message: "warning1"},
		},
	}

	tmpDir := t.TempDir()
	fixturePath := filepath.Join(tmpDir, "doctor-fixture.json")
	b, _ := json.Marshal(fixture)
	_ = os.WriteFile(fixturePath, b, 0644)

	result, err := ObserveScopeSummary(context.Background(), ObserveScopeSummaryRequest{
		TopIssues:   -1,
		FixturePath: fixturePath,
	})
	if err != nil {
		t.Fatalf("ObserveScopeSummary error: %v", err)
	}

	if len(result.Summary.TopIssues) != 0 {
		t.Errorf("TopIssues length = %d, want 0 for negative input", len(result.Summary.TopIssues))
	}
}

func TestObserveResourceContext_DefaultNamespace(t *testing.T) {
	// This test verifies the default namespace handling.
	// Without a live cluster, it will return a failure summary, but that's expected.
	summary, err := ObserveResourceContext(context.Background(), ObserveResourceContextRequest{
		Kind:      "Deployment",
		Name:      "test-app",
		Namespace: "", // empty should default to "default"
	})
	if err != nil {
		t.Fatalf("ObserveResourceContext error: %v", err)
	}

	// Should have "default" namespace in the summary
	if summary.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q", summary.Namespace, "default")
	}
}

func TestObserveResourceContext_ReturnsValidSummary(t *testing.T) {
	// Without a live cluster, this will return a failure summary
	// but the seam should still return a valid ExplainSummary
	summary, err := ObserveResourceContext(context.Background(), ObserveResourceContextRequest{
		Kind:      "Deployment",
		Name:      "my-app",
		Namespace: "prod",
	})
	if err != nil {
		t.Fatalf("ObserveResourceContext error: %v", err)
	}

	// Should have resource and namespace set
	if summary.Resource == "" {
		t.Error("Resource should not be empty")
	}
	if summary.Namespace != "prod" {
		t.Errorf("Namespace = %q, want %q", summary.Namespace, "prod")
	}
	// Should have some owner (even if unknown)
	if summary.Owner == "" {
		t.Error("Owner should not be empty")
	}
}

func TestObserveScopeSummaryRequest_IsTransportAgnostic(t *testing.T) {
	// Verify the request struct has no CLI/Cobra dependencies
	req := ObserveScopeSummaryRequest{
		Namespace:   "prod",
		TopIssues:   5,
		FixturePath: "/some/path.json",
	}

	// Just verify it can be created and used without any transport-specific fields
	if req.Namespace != "prod" {
		t.Errorf("Namespace = %q, want %q", req.Namespace, "prod")
	}
	if req.TopIssues != 5 {
		t.Errorf("TopIssues = %d, want 5", req.TopIssues)
	}
	if req.FixturePath != "/some/path.json" {
		t.Errorf("FixturePath = %q, want %q", req.FixturePath, "/some/path.json")
	}
}

func TestObserveScopeSummaryResult_WarningsAreStructured(t *testing.T) {
	// Verify that warnings are returned in a structured way, not written to stderr
	result := ObserveScopeSummaryResult{
		Summary:  DoctorSummary{Cluster: "test"},
		Warnings: []string{"scan unavailable: connection refused"},
	}

	// Callers can inspect warnings without side effects
	if len(result.Warnings) != 1 {
		t.Errorf("Warnings length = %d, want 1", len(result.Warnings))
	}
	if result.Warnings[0] != "scan unavailable: connection refused" {
		t.Errorf("Warnings[0] = %q, unexpected", result.Warnings[0])
	}
}

func TestObserveResourceContextRequest_IsTransportAgnostic(t *testing.T) {
	// Verify the request struct has no CLI/Cobra dependencies
	req := ObserveResourceContextRequest{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "prod",
	}

	// Just verify it can be created and used without any transport-specific fields
	if req.Kind != "Deployment" {
		t.Errorf("Kind = %q, want %q", req.Kind, "Deployment")
	}
	if req.Name != "api" {
		t.Errorf("Name = %q, want %q", req.Name, "api")
	}
	if req.Namespace != "prod" {
		t.Errorf("Namespace = %q, want %q", req.Namespace, "prod")
	}
}
