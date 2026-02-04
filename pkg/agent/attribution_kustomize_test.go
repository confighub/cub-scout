// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildKustomizeOverlayOwnership_CreatesOwnsEdge(t *testing.T) {
	kustomizeDir := "overlays/prod"
	target := AttributionRef{
		Kind:      "Deployment",
		Name:      "api-server",
		Namespace: "production",
	}

	graph, err := BuildKustomizeOverlayOwnership(kustomizeDir, target, NodeK8sObject)
	if err != nil {
		t.Fatalf("BuildKustomizeOverlayOwnership failed: %v", err)
	}

	if graph == nil {
		t.Fatal("Expected non-nil graph")
	}

	// Should have 2 nodes: overlay and target
	if len(graph.Nodes) != 2 {
		t.Errorf("Nodes count = %d, want 2", len(graph.Nodes))
	}

	// Should have 1 edge: owns
	if len(graph.Edges) != 1 {
		t.Errorf("Edges count = %d, want 1", len(graph.Edges))
	}

	// Check edge type and evidence
	edge := graph.Edges[0]
	if edge.Type != EdgeOwns {
		t.Errorf("Edge.Type = %s, want %s", edge.Type, EdgeOwns)
	}
	if edge.Evidence != EvidenceKustomizeOverlay {
		t.Errorf("Edge.Evidence = %s, want %s", edge.Evidence, EvidenceKustomizeOverlay)
	}

	// Check node types
	nodeTypes := make(map[AttributionNodeType]int)
	for _, n := range graph.Nodes {
		nodeTypes[n.Type]++
	}
	if nodeTypes[NodeKustomizeOverlay] != 1 {
		t.Errorf("kustomize_overlay nodes = %d, want 1", nodeTypes[NodeKustomizeOverlay])
	}
	if nodeTypes[NodeK8sObject] != 1 {
		t.Errorf("k8s_object nodes = %d, want 1", nodeTypes[NodeK8sObject])
	}
}

func TestBuildKustomizeOverlayOwnership_PathNormalizationDeterministic(t *testing.T) {
	target := AttributionRef{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "default",
	}

	// Run multiple times with same input
	var firstOverlayName string
	for i := 0; i < 5; i++ {
		graph, err := BuildKustomizeOverlayOwnership("./overlays/prod", target, NodeK8sObject)
		if err != nil {
			t.Fatalf("Iteration %d: BuildKustomizeOverlayOwnership failed: %v", i, err)
		}

		// Find overlay node
		var overlayName string
		for _, n := range graph.Nodes {
			if n.Type == NodeKustomizeOverlay {
				overlayName = n.Ref.Name
				break
			}
		}

		if i == 0 {
			firstOverlayName = overlayName
		} else if overlayName != firstOverlayName {
			t.Errorf("Iteration %d: overlay name %q differs from first %q", i, overlayName, firstOverlayName)
		}
	}
}

func TestBuildKustomizeOverlayOwnership_RelativePathNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"overlays/prod", "overlays/prod"},
		{"./overlays/prod", "overlays/prod"},
		{"overlays/prod/", "overlays/prod"},
		{"./overlays/prod/", "overlays/prod"},
	}

	target := AttributionRef{Kind: "Deployment", Name: "api", Namespace: "default"}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			graph, err := BuildKustomizeOverlayOwnership(tt.input, target, NodeK8sObject)
			if err != nil {
				t.Fatalf("BuildKustomizeOverlayOwnership failed: %v", err)
			}

			var overlayName string
			for _, n := range graph.Nodes {
				if n.Type == NodeKustomizeOverlay {
					overlayName = n.Ref.Name
					break
				}
			}

			if overlayName != tt.expected {
				t.Errorf("overlay name = %q, want %q", overlayName, tt.expected)
			}
		})
	}
}

func TestBuildKustomizeOverlayOwnership_AbsPathDoesNotLeakFullPath(t *testing.T) {
	// Use an absolute path
	absPath := filepath.Join("/", "Users", "test", "project", "overlays", "prod")
	target := AttributionRef{Kind: "Deployment", Name: "api", Namespace: "default"}

	graph, err := BuildKustomizeOverlayOwnership(absPath, target, NodeK8sObject)
	if err != nil {
		t.Fatalf("BuildKustomizeOverlayOwnership failed: %v", err)
	}

	var overlayName string
	for _, n := range graph.Nodes {
		if n.Type == NodeKustomizeOverlay {
			overlayName = n.Ref.Name
			break
		}
	}

	// Should not contain the full path
	if strings.Contains(overlayName, "/Users/") {
		t.Errorf("overlay name should not leak full path: %s", overlayName)
	}

	// Should contain "prod" (the basename)
	if !strings.HasPrefix(overlayName, "prod") {
		t.Errorf("overlay name should start with basename 'prod': %s", overlayName)
	}

	// Should contain hash suffix
	if !strings.Contains(overlayName, "#") {
		t.Errorf("overlay name should contain hash suffix: %s", overlayName)
	}
}

func TestBuildKustomizeOverlayOwnership_EmptyPath(t *testing.T) {
	target := AttributionRef{Kind: "Deployment", Name: "api", Namespace: "default"}

	graph, err := BuildKustomizeOverlayOwnership("", target, NodeK8sObject)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if graph != nil {
		t.Error("Expected nil graph for empty kustomize path")
	}
}

func TestBuildKustomizeOverlayOwnership_UsesNodeMRWhenSpecified(t *testing.T) {
	target := AttributionRef{
		Kind:      "Instance",
		Name:      "staging-db",
		Namespace: "crossplane-system",
	}

	graph, err := BuildKustomizeOverlayOwnership("overlays/prod", target, NodeMR)
	if err != nil {
		t.Fatalf("BuildKustomizeOverlayOwnership failed: %v", err)
	}

	// Check target node type is MR
	var foundMR bool
	for _, n := range graph.Nodes {
		if n.Type == NodeMR {
			foundMR = true
			break
		}
	}
	if !foundMR {
		t.Error("Expected target node to be NodeMR")
	}
}

func TestNormalizeKustomizePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantPart string // partial match for result
	}{
		{"relative simple", "overlays/prod", "overlays/prod"},
		{"relative with dot", "./overlays/prod", "overlays/prod"},
		{"relative with trailing slash", "overlays/prod/", "overlays/prod"},
		{"absolute path", "/Users/test/project/prod", "prod#"}, // basename + hash
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeKustomizePath(tt.input)
			if !strings.Contains(got, tt.wantPart) {
				t.Errorf("normalizeKustomizePath(%q) = %q, want to contain %q", tt.input, got, tt.wantPart)
			}
		})
	}
}
