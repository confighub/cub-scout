// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestMergeAttributionGraphs_BothNil(t *testing.T) {
	result := MergeAttributionGraphs(nil, nil)
	if result != nil {
		t.Error("Expected nil for both nil inputs")
	}
}

func TestMergeAttributionGraphs_OneNil(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node1", Type: NodeMR, Ref: AttributionRef{Kind: "Instance", Name: "db"}},
		},
	}

	// a is nil
	result := MergeAttributionGraphs(nil, graph)
	if result != graph {
		t.Error("Expected graph when a is nil")
	}

	// b is nil
	result = MergeAttributionGraphs(graph, nil)
	if result != graph {
		t.Error("Expected graph when b is nil")
	}
}

func TestMergeAttributionGraphs_DedupeNodes(t *testing.T) {
	graphA := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node1", Type: NodeMR, Ref: AttributionRef{Kind: "Instance", Name: "db1"}},
			{ID: "node2", Type: NodeXR, Ref: AttributionRef{Kind: "XDB", Name: "xdb1"}},
		},
	}

	graphB := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node2", Type: NodeXR, Ref: AttributionRef{Kind: "XDB", Name: "xdb1"}}, // duplicate
			{ID: "node3", Type: NodeClaim, Ref: AttributionRef{Kind: "DBClaim", Name: "claim1"}},
		},
	}

	result := MergeAttributionGraphs(graphA, graphB)

	if len(result.Nodes) != 3 {
		t.Errorf("Nodes count = %d, want 3", len(result.Nodes))
	}

	// Check no duplicates
	nodeIDs := make(map[string]bool)
	for _, n := range result.Nodes {
		if nodeIDs[n.ID] {
			t.Errorf("Duplicate node ID: %s", n.ID)
		}
		nodeIDs[n.ID] = true
	}
}

func TestMergeAttributionGraphs_DedupeEdges(t *testing.T) {
	graphA := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node1", Type: NodeMR},
			{ID: "node2", Type: NodeXR},
		},
		Edges: []AttributionEdge{
			{ID: "edge1", Type: EdgeOwns, From: "node2", To: "node1"},
		},
	}

	graphB := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node2", Type: NodeXR},
			{ID: "node3", Type: NodeKustomizeOverlay},
		},
		Edges: []AttributionEdge{
			{ID: "edge1", Type: EdgeOwns, From: "node2", To: "node1"}, // duplicate
			{ID: "edge2", Type: EdgeOwns, From: "node3", To: "node1"},
		},
	}

	result := MergeAttributionGraphs(graphA, graphB)

	if len(result.Edges) != 2 {
		t.Errorf("Edges count = %d, want 2", len(result.Edges))
	}

	// Check no duplicates
	edgeIDs := make(map[string]bool)
	for _, e := range result.Edges {
		if edgeIDs[e.ID] {
			t.Errorf("Duplicate edge ID: %s", e.ID)
		}
		edgeIDs[e.ID] = true
	}
}

func TestMergeAttributionGraphs_SortsByID(t *testing.T) {
	graphA := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "zzzz", Type: NodeMR},
			{ID: "aaaa", Type: NodeXR},
		},
		Edges: []AttributionEdge{
			{ID: "edge-z", Type: EdgeOwns},
			{ID: "edge-a", Type: EdgeOwns},
		},
	}

	graphB := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "mmmm", Type: NodeClaim},
		},
		Edges: []AttributionEdge{
			{ID: "edge-m", Type: EdgeOwns},
		},
	}

	result := MergeAttributionGraphs(graphA, graphB)

	// Check nodes are sorted
	for i := 1; i < len(result.Nodes); i++ {
		if result.Nodes[i].ID < result.Nodes[i-1].ID {
			t.Error("Nodes not sorted by ID")
		}
	}

	// Check edges are sorted
	for i := 1; i < len(result.Edges); i++ {
		if result.Edges[i].ID < result.Edges[i-1].ID {
			t.Error("Edges not sorted by ID")
		}
	}
}

func TestMergeAttributionGraphs_RecomputesSummary(t *testing.T) {
	graphA := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node1", Type: NodeMR},
			{ID: "node2", Type: NodeXR},
		},
		Edges: []AttributionEdge{
			{ID: "edge1", Type: EdgeOwns},
		},
		Summary: AttributionSummary{
			NodesByType: map[AttributionNodeType]int{NodeMR: 1, NodeXR: 1},
			EdgesByType: map[AttributionEdgeType]int{EdgeOwns: 1},
		},
	}

	graphB := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node3", Type: NodeKustomizeOverlay},
		},
		Edges: []AttributionEdge{
			{ID: "edge2", Type: EdgeOwns},
		},
		Summary: AttributionSummary{
			NodesByType: map[AttributionNodeType]int{NodeKustomizeOverlay: 1},
			EdgesByType: map[AttributionEdgeType]int{EdgeOwns: 1},
		},
	}

	result := MergeAttributionGraphs(graphA, graphB)

	// Check summary was recomputed
	if result.Summary.NodesByType[NodeMR] != 1 {
		t.Errorf("NodesByType[NodeMR] = %d, want 1", result.Summary.NodesByType[NodeMR])
	}
	if result.Summary.NodesByType[NodeXR] != 1 {
		t.Errorf("NodesByType[NodeXR] = %d, want 1", result.Summary.NodesByType[NodeXR])
	}
	if result.Summary.NodesByType[NodeKustomizeOverlay] != 1 {
		t.Errorf("NodesByType[NodeKustomizeOverlay] = %d, want 1", result.Summary.NodesByType[NodeKustomizeOverlay])
	}
	if result.Summary.EdgesByType[EdgeOwns] != 2 {
		t.Errorf("EdgesByType[EdgeOwns] = %d, want 2", result.Summary.EdgesByType[EdgeOwns])
	}
}

func TestMergeAttributionGraphs_PreservesSource(t *testing.T) {
	graphA := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		GeneratedFrom: &AttributionSource{BundleID: "bundle-a"},
		Nodes:         []AttributionNode{},
	}

	graphB := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		GeneratedFrom: &AttributionSource{BundleID: "bundle-b"},
		Nodes:         []AttributionNode{},
	}

	// First graph's source should be preserved
	result := MergeAttributionGraphs(graphA, graphB)
	if result.GeneratedFrom == nil || result.GeneratedFrom.BundleID != "bundle-a" {
		t.Error("Expected source from first graph to be preserved")
	}
}

func TestMergeAttributionGraphs_Deterministic(t *testing.T) {
	graphA := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node1", Type: NodeMR, Ref: AttributionRef{Kind: "Instance", Name: "db1"}},
		},
		Edges: []AttributionEdge{
			{ID: "edge1", Type: EdgeOwns, From: "xr1", To: "node1"},
		},
	}

	graphB := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "node2", Type: NodeKustomizeOverlay, Ref: AttributionRef{Kind: "kustomize", Name: "overlay1"}},
		},
		Edges: []AttributionEdge{
			{ID: "edge2", Type: EdgeOwns, From: "node2", To: "node1"},
		},
	}

	// Run multiple times
	var firstNodeIDs, firstEdgeIDs []string
	for i := 0; i < 5; i++ {
		result := MergeAttributionGraphs(graphA, graphB)

		var nodeIDs, edgeIDs []string
		for _, n := range result.Nodes {
			nodeIDs = append(nodeIDs, n.ID)
		}
		for _, e := range result.Edges {
			edgeIDs = append(edgeIDs, e.ID)
		}

		if i == 0 {
			firstNodeIDs = nodeIDs
			firstEdgeIDs = edgeIDs
		} else {
			if len(nodeIDs) != len(firstNodeIDs) {
				t.Errorf("Iteration %d: node count differs", i)
				continue
			}
			for j, id := range nodeIDs {
				if id != firstNodeIDs[j] {
					t.Errorf("Iteration %d: node ID %d differs: %s vs %s", i, j, id, firstNodeIDs[j])
				}
			}
			if len(edgeIDs) != len(firstEdgeIDs) {
				t.Errorf("Iteration %d: edge count differs", i)
				continue
			}
			for j, id := range edgeIDs {
				if id != firstEdgeIDs[j] {
					t.Errorf("Iteration %d: edge ID %d differs: %s vs %s", i, j, id, firstEdgeIDs[j])
				}
			}
		}
	}
}
