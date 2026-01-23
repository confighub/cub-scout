// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
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
