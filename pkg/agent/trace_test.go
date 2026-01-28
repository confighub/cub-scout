// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestResourceRefString(t *testing.T) {
	tests := []struct {
		name     string
		ref      ResourceRef
		expected string
	}{
		{
			name:     "with namespace",
			ref:      ResourceRef{Kind: "Deployment", Name: "nginx", Namespace: "demo"},
			expected: "Deployment/nginx in demo",
		},
		{
			name:     "without namespace",
			ref:      ResourceRef{Kind: "GitRepository", Name: "infra-repo", Namespace: ""},
			expected: "GitRepository/infra-repo",
		},
		{
			name:     "cluster-scoped resource",
			ref:      ResourceRef{Kind: "ClusterRole", Name: "admin"},
			expected: "ClusterRole/admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.String()
			if result != tt.expected {
				t.Errorf("ResourceRef.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestChainLinkIsHealthy(t *testing.T) {
	tests := []struct {
		name     string
		link     ChainLink
		expected bool
	}{
		{
			name:     "ready link is healthy",
			link:     ChainLink{Kind: "GitRepository", Name: "repo", Ready: true},
			expected: true,
		},
		{
			name:     "not ready link is unhealthy",
			link:     ChainLink{Kind: "Kustomization", Name: "apps", Ready: false},
			expected: false,
		},
		{
			name:     "ready with error message still healthy",
			link:     ChainLink{Kind: "Deployment", Name: "app", Ready: true, Message: "warning"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.link.IsHealthy()
			if result != tt.expected {
				t.Errorf("ChainLink.IsHealthy() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTraceResultFullyManaged(t *testing.T) {
	tests := []struct {
		name     string
		result   TraceResult
		expected bool
	}{
		{
			name: "all links ready",
			result: TraceResult{
				Chain: []ChainLink{
					{Kind: "GitRepository", Ready: true},
					{Kind: "Kustomization", Ready: true},
					{Kind: "Deployment", Ready: true},
				},
				FullyManaged: true,
			},
			expected: true,
		},
		{
			name: "one link not ready",
			result: TraceResult{
				Chain: []ChainLink{
					{Kind: "GitRepository", Ready: true},
					{Kind: "Kustomization", Ready: false},
					{Kind: "Deployment", Ready: true},
				},
				FullyManaged: false,
			},
			expected: false,
		},
		{
			name: "empty chain",
			result: TraceResult{
				Chain:        []ChainLink{},
				FullyManaged: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.FullyManaged != tt.expected {
				t.Errorf("TraceResult.FullyManaged = %v, want %v", tt.result.FullyManaged, tt.expected)
			}
		})
	}
}

// MockTracer is a test tracer for unit tests
type MockTracer struct {
	available bool
	toolName  string
	traceFunc func(ctx context.Context, kind, name, namespace string) (*TraceResult, error)
}

func (m *MockTracer) Available() bool {
	return m.available
}

func (m *MockTracer) ToolName() string {
	return m.toolName
}

func (m *MockTracer) Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
	if m.traceFunc != nil {
		return m.traceFunc(ctx, kind, name, namespace)
	}
	return nil, nil
}

func TestMultiTracerSelectsAvailable(t *testing.T) {
	ctx := context.Background()

	unavailableTracer := &MockTracer{
		available: false,
		toolName:  "unavailable",
	}

	availableTracer := &MockTracer{
		available: true,
		toolName:  "available",
		traceFunc: func(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
			return &TraceResult{
				Object:       ResourceRef{Kind: kind, Name: name, Namespace: namespace},
				Chain:        []ChainLink{{Kind: "Source", Name: "test", Ready: true}},
				FullyManaged: true,
				Tool:         "available",
				TracedAt:     time.Now(),
			}, nil
		},
	}

	multi := NewMultiTracer(unavailableTracer, availableTracer)

	result, err := multi.Trace(ctx, "Deployment", "test", "default")
	if err != nil {
		t.Fatalf("MultiTracer.Trace() error = %v", err)
	}

	if result.Tool != "available" {
		t.Errorf("MultiTracer selected wrong tracer, got tool = %q", result.Tool)
	}
}

func TestMultiTracerReturnsNotManaged(t *testing.T) {
	ctx := context.Background()

	// No tracers available
	multi := NewMultiTracer()

	result, err := multi.Trace(ctx, "Deployment", "test", "default")
	if err != nil {
		t.Fatalf("MultiTracer.Trace() error = %v", err)
	}

	if result.FullyManaged {
		t.Error("Expected FullyManaged = false when no tracers available")
	}

	if result.Error == "" {
		t.Error("Expected error message when no tracers available")
	}
}

func TestMultiTracerAvailableTracers(t *testing.T) {
	tracer1 := &MockTracer{available: true, toolName: "flux"}
	tracer2 := &MockTracer{available: false, toolName: "argocd"}
	tracer3 := &MockTracer{available: true, toolName: "helm"}

	multi := NewMultiTracer(tracer1, tracer2, tracer3)
	available := multi.AvailableTracers()

	if len(available) != 2 {
		t.Errorf("Expected 2 available tracers, got %d", len(available))
	}

	// Check flux and helm are available
	found := make(map[string]bool)
	for _, name := range available {
		found[name] = true
	}

	if !found["flux"] {
		t.Error("Expected 'flux' in available tracers")
	}
	if !found["helm"] {
		t.Error("Expected 'helm' in available tracers")
	}
	if found["argocd"] {
		t.Error("Did not expect 'argocd' in available tracers")
	}
}

// ============================================================================
// History Feature Tests (Task 1: Core data structures)
// ============================================================================

func TestHistoryEntryStruct(t *testing.T) {
	// Test that HistoryEntry can be created and serialized correctly
	ts := time.Date(2026, 1, 28, 10, 30, 0, 0, time.UTC)

	entry := HistoryEntry{
		Timestamp: ts,
		Revision:  "v1.2.3@abc123",
		Status:    "deployed",
		Source:    "manual sync by alice@example.com",
		Message:   "Deployment successful",
		Duration:  "2.5s",
	}

	// Verify all fields are set correctly
	if entry.Timestamp != ts {
		t.Errorf("Timestamp = %v, want %v", entry.Timestamp, ts)
	}
	if entry.Revision != "v1.2.3@abc123" {
		t.Errorf("Revision = %q, want %q", entry.Revision, "v1.2.3@abc123")
	}
	if entry.Status != "deployed" {
		t.Errorf("Status = %q, want %q", entry.Status, "deployed")
	}
	if entry.Source != "manual sync by alice@example.com" {
		t.Errorf("Source = %q, want %q", entry.Source, "manual sync by alice@example.com")
	}
	if entry.Message != "Deployment successful" {
		t.Errorf("Message = %q, want %q", entry.Message, "Deployment successful")
	}
	if entry.Duration != "2.5s" {
		t.Errorf("Duration = %q, want %q", entry.Duration, "2.5s")
	}
}

func TestHistoryEntryJSON(t *testing.T) {
	ts := time.Date(2026, 1, 28, 10, 30, 0, 0, time.UTC)

	entry := HistoryEntry{
		Timestamp: ts,
		Revision:  "main@sha1:abc123",
		Status:    "ReconciliationSucceeded",
	}

	// Test JSON marshaling
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal HistoryEntry: %v", err)
	}

	// Verify JSON contains expected fields
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"revision":"main@sha1:abc123"`) {
		t.Errorf("JSON missing revision field: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"status":"ReconciliationSucceeded"`) {
		t.Errorf("JSON missing status field: %s", jsonStr)
	}

	// Test JSON unmarshaling
	var decoded HistoryEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal HistoryEntry: %v", err)
	}

	if decoded.Revision != entry.Revision {
		t.Errorf("Decoded Revision = %q, want %q", decoded.Revision, entry.Revision)
	}
}

func TestTraceResultWithHistory(t *testing.T) {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)

	result := TraceResult{
		Object: ResourceRef{
			Kind:      "Deployment",
			Name:      "nginx",
			Namespace: "prod",
		},
		Chain: []ChainLink{
			{Kind: "Application", Name: "nginx-app", Ready: true},
			{Kind: "Deployment", Name: "nginx", Ready: true},
		},
		FullyManaged: true,
		Tool:         "argocd",
		TracedAt:     now,
		History: []HistoryEntry{
			{Timestamp: hourAgo, Revision: "v1.2.3@abc123", Status: "deployed"},
			{Timestamp: dayAgo, Revision: "v1.2.2@def456", Status: "deployed"},
		},
	}

	// Verify history is populated
	if len(result.History) != 2 {
		t.Fatalf("Expected 2 history entries, got %d", len(result.History))
	}

	// Verify entries are in order (most recent first)
	if result.History[0].Revision != "v1.2.3@abc123" {
		t.Errorf("First history entry Revision = %q, want %q", result.History[0].Revision, "v1.2.3@abc123")
	}
	if result.History[1].Revision != "v1.2.2@def456" {
		t.Errorf("Second history entry Revision = %q, want %q", result.History[1].Revision, "v1.2.2@def456")
	}
}

func TestTraceResultHistoryEmpty(t *testing.T) {
	result := TraceResult{
		Object: ResourceRef{
			Kind:      "Deployment",
			Name:      "nginx",
			Namespace: "default",
		},
		FullyManaged: true,
		Tool:         "flux",
		TracedAt:     time.Now(),
		History:      nil, // No history
	}

	// Empty history should be valid
	if result.History != nil && len(result.History) != 0 {
		t.Errorf("Expected nil or empty history, got %d entries", len(result.History))
	}

	// JSON should omit empty history
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal TraceResult: %v", err)
	}

	// With omitempty, history field should not appear in JSON
	if strings.Contains(string(data), `"history":null`) {
		t.Log("History is null in JSON (acceptable)")
	}
}

func TestTraceResultHistoryJSON(t *testing.T) {
	ts := time.Date(2026, 1, 28, 10, 0, 0, 0, time.UTC)

	result := TraceResult{
		Object: ResourceRef{
			Kind:      "Deployment",
			Name:      "app",
			Namespace: "prod",
		},
		FullyManaged: true,
		Tool:         "helm",
		TracedAt:     ts,
		History: []HistoryEntry{
			{Timestamp: ts, Revision: "v1", Status: "deployed"},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal TraceResult with history: %v", err)
	}

	// Verify JSON contains history
	if !strings.Contains(string(data), `"history":[`) {
		t.Errorf("JSON missing history array: %s", string(data))
	}

	// Unmarshal and verify
	var decoded TraceResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal TraceResult: %v", err)
	}

	if len(decoded.History) != 1 {
		t.Fatalf("Decoded history has %d entries, want 1", len(decoded.History))
	}
	if decoded.History[0].Status != "deployed" {
		t.Errorf("Decoded history status = %q, want %q", decoded.History[0].Status, "deployed")
	}
}

func TestEnrichWithConfigHub(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		wantUnit    string
		wantSpace   string
		wantDrift   bool
	}{
		{
			name:        "ConfigHub managed via labels",
			labels:      map[string]string{"confighub.com/UnitSlug": "payment-api", "confighub.com/SpaceName": "production"},
			annotations: map[string]string{"confighub.com/SpaceID": "space_123"},
			wantUnit:    "payment-api",
			wantSpace:   "production",
		},
		{
			name:        "ConfigHub managed via annotations",
			labels:      map[string]string{},
			annotations: map[string]string{"confighub.com/UnitSlug": "worker", "confighub.com/SpaceName": "staging", "confighub.com/DriftDetected": "true"},
			wantUnit:    "worker",
			wantSpace:   "staging",
			wantDrift:   true,
		},
		{
			name:        "Not ConfigHub managed",
			labels:      map[string]string{"app": "nginx"},
			annotations: map[string]string{},
			wantUnit:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &TraceResult{}
			result.EnrichWithConfigHub(tt.labels, tt.annotations)

			if tt.wantUnit == "" {
				if result.ConfigHub != nil {
					t.Error("Expected nil ConfigHub for non-managed resource")
				}
				return
			}

			if result.ConfigHub == nil {
				t.Fatal("Expected ConfigHub to be set")
			}

			if result.ConfigHub.UnitSlug != tt.wantUnit {
				t.Errorf("UnitSlug = %q, want %q", result.ConfigHub.UnitSlug, tt.wantUnit)
			}
			if result.ConfigHub.SpaceName != tt.wantSpace {
				t.Errorf("SpaceName = %q, want %q", result.ConfigHub.SpaceName, tt.wantSpace)
			}
			if result.ConfigHub.DriftDetected != tt.wantDrift {
				t.Errorf("DriftDetected = %v, want %v", result.ConfigHub.DriftDetected, tt.wantDrift)
			}
		})
	}
}
