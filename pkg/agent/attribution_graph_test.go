// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAttributionGraphBuilder_DeterministicOutput(t *testing.T) {
	// Build graph with nodes/edges added in random order
	builder := NewAttributionGraphBuilder()

	// Add nodes out of order
	builder.AddNode(AttributionNode{
		Type:    NodeMR,
		Ref:     AttributionRef{Kind: "Instance", Name: "staging-db", UID: "uid-mr"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeXR,
		Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-abc", UID: "uid-xr"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeClaim,
		Ref:     AttributionRef{Kind: "PostgreSQLInstance", Name: "db", Namespace: "prod", UID: "uid-claim"},
		Present: true,
	})

	// Add edges
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "xr:uid:uid-xr",
		To:       "mr:uid:uid-mr",
		Evidence: EvidenceOwnerReference,
	})
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "claim:uid:uid-claim",
		To:       "xr:uid:uid-xr",
		Evidence: EvidenceOwnerReference,
	})

	// Build multiple times and verify identical output
	first := builder.Build()
	firstJSON, _ := json.Marshal(first)

	for i := 0; i < 5; i++ {
		graph := builder.Build()
		graphJSON, _ := json.Marshal(graph)
		if string(graphJSON) != string(firstJSON) {
			t.Errorf("Iteration %d: output differs from first build", i)
		}
	}
}

func TestAttributionGraphBuilder_NodesSortedByID(t *testing.T) {
	builder := NewAttributionGraphBuilder()

	// Add nodes with IDs that would be out of order alphabetically
	builder.AddNode(AttributionNode{
		Type:    NodeMR,
		Ref:     AttributionRef{Kind: "Instance", Name: "z-last", UID: "uid-z"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeXR,
		Ref:     AttributionRef{Kind: "XR", Name: "a-first", UID: "uid-a"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeClaim,
		Ref:     AttributionRef{Kind: "Claim", Name: "m-middle", UID: "uid-m"},
		Present: true,
	})

	graph := builder.Build()

	// Verify sorted order
	for i := 1; i < len(graph.Nodes); i++ {
		if graph.Nodes[i].ID < graph.Nodes[i-1].ID {
			t.Errorf("Nodes not sorted: %s comes after %s", graph.Nodes[i].ID, graph.Nodes[i-1].ID)
		}
	}
}

func TestAttributionGraphBuilder_EdgesSortedByID(t *testing.T) {
	builder := NewAttributionGraphBuilder()

	builder.AddNode(AttributionNode{
		Type: NodeXR,
		Ref:  AttributionRef{Kind: "XR", Name: "xr", UID: "uid-xr"},
	})
	builder.AddNode(AttributionNode{
		Type: NodeMR,
		Ref:  AttributionRef{Kind: "MR", Name: "mr1", UID: "uid-mr1"},
	})
	builder.AddNode(AttributionNode{
		Type: NodeMR,
		Ref:  AttributionRef{Kind: "MR", Name: "mr2", UID: "uid-mr2"},
	})

	// Add edges in reverse order they should appear
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "xr:uid:uid-xr",
		To:       "mr:uid:uid-mr2",
		Evidence: EvidenceOwnerReference,
	})
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "xr:uid:uid-xr",
		To:       "mr:uid:uid-mr1",
		Evidence: EvidenceOwnerReference,
	})

	graph := builder.Build()

	// Verify sorted order
	for i := 1; i < len(graph.Edges); i++ {
		if graph.Edges[i].ID < graph.Edges[i-1].ID {
			t.Errorf("Edges not sorted: %s comes after %s", graph.Edges[i].ID, graph.Edges[i-1].ID)
		}
	}
}

func TestGenerateNodeID_WithUID(t *testing.T) {
	ref := AttributionRef{
		APIVersion: "rds.aws.upbound.io/v1beta1",
		Kind:       "Instance",
		Name:       "staging-db",
		UID:        "abc-123-def",
	}

	id := GenerateNodeID(NodeMR, ref)

	if id != "mr:uid:abc-123-def" {
		t.Errorf("Expected 'mr:uid:abc-123-def', got %q", id)
	}
}

func TestGenerateNodeID_WithoutUID_Namespaced(t *testing.T) {
	ref := AttributionRef{
		APIVersion: "example.org/v1",
		Kind:       "PostgreSQLInstance",
		Namespace:  "prod",
		Name:       "db",
	}

	id := GenerateNodeID(NodeClaim, ref)

	expected := "claim:ref:example.org/v1/PostgreSQLInstance:prod/db"
	if id != expected {
		t.Errorf("Expected %q, got %q", expected, id)
	}
}

func TestGenerateNodeID_WithoutUID_ClusterScoped(t *testing.T) {
	ref := AttributionRef{
		APIVersion: "apiextensions.crossplane.io/v1",
		Kind:       "Composition",
		Name:       "postgres-aws",
	}

	id := GenerateNodeID(NodeComposition, ref)

	expected := "composition:ref:apiextensions.crossplane.io/v1/Composition:postgres-aws"
	if id != expected {
		t.Errorf("Expected %q, got %q", expected, id)
	}
}

func TestGenerateEdgeID_Deterministic(t *testing.T) {
	id1 := GenerateEdgeID(EdgeOwns, "xr:uid:abc", "mr:uid:def", EvidenceOwnerReference)
	id2 := GenerateEdgeID(EdgeOwns, "xr:uid:abc", "mr:uid:def", EvidenceOwnerReference)

	if id1 != id2 {
		t.Errorf("Same inputs produced different IDs: %q vs %q", id1, id2)
	}

	// Different inputs should produce different IDs
	id3 := GenerateEdgeID(EdgeOwns, "xr:uid:abc", "mr:uid:ghi", EvidenceOwnerReference)
	if id1 == id3 {
		t.Errorf("Different inputs produced same ID: %q", id1)
	}
}

func TestAttributionGraph_Validate_Valid(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{
		Type:    NodeXR,
		Ref:     AttributionRef{Kind: "XR", Name: "xr", UID: "uid-xr"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeMR,
		Ref:     AttributionRef{Kind: "MR", Name: "mr", UID: "uid-mr"},
		Present: true,
	})
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "xr:uid:uid-xr",
		To:       "mr:uid:uid-mr",
		Evidence: EvidenceOwnerReference,
	})

	graph := builder.Build()
	errors := graph.Validate()

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got: %v", errors)
	}
}

func TestAttributionGraph_Validate_MissingEdgeEndpoint(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "xr:uid:abc", Type: NodeXR},
		},
		Edges: []AttributionEdge{
			{ID: "edge:123", Type: EdgeOwns, From: "xr:uid:abc", To: "mr:uid:missing", Evidence: EvidenceOwnerReference},
		},
		Summary: AttributionSummary{
			NodesByType: make(map[AttributionNodeType]int),
			EdgesByType: make(map[AttributionEdgeType]int),
		},
	}

	errors := graph.Validate()

	found := false
	for _, err := range errors {
		if err == "edge edge:123 references unknown to node: mr:uid:missing" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected error about missing endpoint, got: %v", errors)
	}
}

func TestAttributionGraph_Validate_WrongSchemaVersion(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: "wrong-version",
		Summary: AttributionSummary{
			NodesByType: make(map[AttributionNodeType]int),
			EdgesByType: make(map[AttributionEdgeType]int),
		},
	}

	errors := graph.Validate()

	found := false
	for _, err := range errors {
		if err == `unexpected schema_version: "wrong-version" (expected "attribution-graph.v1")` {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected error about schema version, got: %v", errors)
	}
}

func TestAttributionGraph_Validate_DuplicateNodeID(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "xr:uid:abc", Type: NodeXR},
			{ID: "xr:uid:abc", Type: NodeXR}, // duplicate
		},
		Summary: AttributionSummary{
			NodesByType: make(map[AttributionNodeType]int),
			EdgesByType: make(map[AttributionEdgeType]int),
		},
	}

	errors := graph.Validate()

	found := false
	for _, err := range errors {
		if err == "duplicate node ID: xr:uid:abc" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected error about duplicate node ID, got: %v", errors)
	}
}

func TestAttributionGraph_IsEmpty(t *testing.T) {
	empty := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes:         []AttributionNode{},
		Edges:         []AttributionEdge{},
	}

	if !empty.IsEmpty() {
		t.Error("Expected empty graph to return true for IsEmpty()")
	}

	nonEmpty := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{ID: "test", Type: NodeXR},
		},
	}

	if nonEmpty.IsEmpty() {
		t.Error("Expected non-empty graph to return false for IsEmpty()")
	}
}

func TestAttributionGraphBuilder_Summary(t *testing.T) {
	builder := NewAttributionGraphBuilder()

	// Add various node types
	builder.AddNode(AttributionNode{Type: NodeXR, Ref: AttributionRef{UID: "xr1"}})
	builder.AddNode(AttributionNode{Type: NodeXR, Ref: AttributionRef{UID: "xr2"}})
	builder.AddNode(AttributionNode{Type: NodeMR, Ref: AttributionRef{UID: "mr1"}})
	builder.AddNode(AttributionNode{Type: NodeClaim, Ref: AttributionRef{UID: "claim1"}})

	// Add edges
	builder.AddEdge(AttributionEdge{Type: EdgeOwns, From: "xr:uid:xr1", To: "mr:uid:mr1", Evidence: EvidenceOwnerReference})

	builder.IncrementUnattributed()
	builder.IncrementUnattributed()
	builder.IncrementAmbiguous()

	graph := builder.Build()

	if graph.Summary.NodesByType[NodeXR] != 2 {
		t.Errorf("Expected 2 XR nodes, got %d", graph.Summary.NodesByType[NodeXR])
	}
	if graph.Summary.NodesByType[NodeMR] != 1 {
		t.Errorf("Expected 1 MR node, got %d", graph.Summary.NodesByType[NodeMR])
	}
	if graph.Summary.NodesByType[NodeClaim] != 1 {
		t.Errorf("Expected 1 Claim node, got %d", graph.Summary.NodesByType[NodeClaim])
	}
	if graph.Summary.EdgesByType[EdgeOwns] != 1 {
		t.Errorf("Expected 1 owns edge, got %d", graph.Summary.EdgesByType[EdgeOwns])
	}
	if graph.Summary.UnattributedCount != 2 {
		t.Errorf("Expected 2 unattributed, got %d", graph.Summary.UnattributedCount)
	}
	if graph.Summary.AmbiguousCount != 1 {
		t.Errorf("Expected 1 ambiguous, got %d", graph.Summary.AmbiguousCount)
	}
}

func TestAttributionGraphBuilder_SetSource(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	createdAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	builder.SetSource("bundle-123", createdAt, "v0.16.0")

	graph := builder.Build()

	if graph.GeneratedFrom == nil {
		t.Fatal("Expected GeneratedFrom to be set")
	}
	if graph.GeneratedFrom.BundleID != "bundle-123" {
		t.Errorf("Expected bundle ID 'bundle-123', got %q", graph.GeneratedFrom.BundleID)
	}
	if !graph.GeneratedFrom.CreatedAt.Equal(createdAt) {
		t.Errorf("Expected created at %v, got %v", createdAt, graph.GeneratedFrom.CreatedAt)
	}
	if graph.GeneratedFrom.ToolVersion != "v0.16.0" {
		t.Errorf("Expected tool version 'v0.16.0', got %q", graph.GeneratedFrom.ToolVersion)
	}
}

func TestAttributionGraph_JSONRoundTrip(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.SetSource("test-bundle", time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), "v0.16.0")
	builder.AddNode(AttributionNode{
		Type:    NodeXR,
		Ref:     AttributionRef{APIVersion: "example.org/v1", Kind: "XR", Name: "xr", UID: "uid-xr"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeMR,
		Ref:     AttributionRef{APIVersion: "rds.aws.upbound.io/v1beta1", Kind: "Instance", Name: "db", UID: "uid-mr"},
		Present: true,
	})
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "xr:uid:uid-xr",
		To:       "mr:uid:uid-mr",
		Evidence: EvidenceOwnerReference,
	})

	original := builder.Build()

	// Marshal to JSON
	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal back
	var restored AttributionGraph
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify key fields
	if restored.SchemaVersion != original.SchemaVersion {
		t.Errorf("SchemaVersion mismatch: %q vs %q", restored.SchemaVersion, original.SchemaVersion)
	}
	if len(restored.Nodes) != len(original.Nodes) {
		t.Errorf("Node count mismatch: %d vs %d", len(restored.Nodes), len(original.Nodes))
	}
	if len(restored.Edges) != len(original.Edges) {
		t.Errorf("Edge count mismatch: %d vs %d", len(restored.Edges), len(original.Edges))
	}

	// Validate restored graph
	errors := restored.Validate()
	if len(errors) > 0 {
		t.Errorf("Restored graph validation failed: %v", errors)
	}
}
