// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAttributionGraphASCII_Header(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.SetSource("test-bundle", time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), "v0.16.0")
	builder.AddNode(AttributionNode{
		Type:    NodeXR,
		Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-abc", UID: "uid-xr"},
		Present: true,
	})

	graph := builder.Build()
	output := RenderAttributionGraphASCII(graph)

	if !strings.Contains(output, "Attribution Graph") {
		t.Error("Output missing 'Attribution Graph' header")
	}
	if !strings.Contains(output, AttributionGraphSchemaVersion) {
		t.Errorf("Output missing schema version %s", AttributionGraphSchemaVersion)
	}
	if !strings.Contains(output, "test-bundle") {
		t.Error("Output missing bundle ID")
	}
}

func TestRenderAttributionGraphASCII_Summary(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{Type: NodeXR, Ref: AttributionRef{UID: "xr1"}, Present: true})
	builder.AddNode(AttributionNode{Type: NodeMR, Ref: AttributionRef{UID: "mr1"}, Present: true})
	builder.AddNode(AttributionNode{Type: NodeMR, Ref: AttributionRef{UID: "mr2"}, Present: true})
	builder.AddEdge(AttributionEdge{Type: EdgeOwns, From: "xr:uid:xr1", To: "mr:uid:mr1", Evidence: EvidenceOwnerReference})
	builder.AddEdge(AttributionEdge{Type: EdgeOwns, From: "xr:uid:xr1", To: "mr:uid:mr2", Evidence: EvidenceOwnerReference})
	builder.IncrementUnattributed()

	graph := builder.Build()
	output := RenderAttributionGraphASCII(graph)

	if !strings.Contains(output, "Nodes:        3") {
		t.Error("Output missing correct node count")
	}
	if !strings.Contains(output, "Edges:        2") {
		t.Error("Output missing correct edge count")
	}
	if !strings.Contains(output, "Unattributed: 1") {
		t.Error("Output missing unattributed count")
	}
}

func TestRenderAttributionGraphASCII_NodesByType(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{Type: NodeXR, Ref: AttributionRef{UID: "xr1"}, Present: true})
	builder.AddNode(AttributionNode{Type: NodeMR, Ref: AttributionRef{UID: "mr1"}, Present: true})
	builder.AddNode(AttributionNode{Type: NodeMR, Ref: AttributionRef{UID: "mr2"}, Present: true})

	graph := builder.Build()
	output := RenderAttributionGraphASCII(graph)

	if !strings.Contains(output, "Nodes by Type") {
		t.Error("Output missing 'Nodes by Type' section")
	}
	if !strings.Contains(output, "xr") {
		t.Error("Output missing xr node type")
	}
	if !strings.Contains(output, "mr") {
		t.Error("Output missing mr node type")
	}
}

func TestRenderAttributionGraphASCII_EdgesByType(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{Type: NodeXR, Ref: AttributionRef{UID: "xr1"}, Present: true})
	builder.AddNode(AttributionNode{Type: NodeMR, Ref: AttributionRef{UID: "mr1"}, Present: true})
	builder.AddEdge(AttributionEdge{Type: EdgeOwns, From: "xr:uid:xr1", To: "mr:uid:mr1", Evidence: EvidenceOwnerReference})

	graph := builder.Build()
	output := RenderAttributionGraphASCII(graph)

	if !strings.Contains(output, "Edges by Type") {
		t.Error("Output missing 'Edges by Type' section")
	}
	if !strings.Contains(output, "owns") {
		t.Error("Output missing owns edge type")
	}
}

func TestRenderAttributionGraphASCII_NodeList(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{
		Type:    NodeXR,
		Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-abc", UID: "uid-xr"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeMR,
		Ref:     AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system", UID: "uid-mr"},
		Present: true,
	})

	graph := builder.Build()
	output := RenderAttributionGraphASCII(graph)

	if !strings.Contains(output, "Nodes") {
		t.Error("Output missing 'Nodes' section")
	}
	if !strings.Contains(output, "XPostgreSQLInstance/xpostgresql-abc") {
		t.Error("Output missing XR reference")
	}
	if !strings.Contains(output, "Instance:crossplane-system/staging-db") {
		t.Error("Output missing MR reference with namespace")
	}
}

func TestRenderAttributionGraphASCII_EdgeList(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{Type: NodeXR, Ref: AttributionRef{UID: "xr1"}, Present: true})
	builder.AddNode(AttributionNode{Type: NodeMR, Ref: AttributionRef{UID: "mr1"}, Present: true})
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "xr:uid:xr1",
		To:       "mr:uid:mr1",
		Evidence: EvidenceOwnerReference,
	})

	graph := builder.Build()
	output := RenderAttributionGraphASCII(graph)

	if !strings.Contains(output, "Edges") {
		t.Error("Output missing 'Edges' section")
	}
	if !strings.Contains(output, "→") {
		t.Error("Output missing edge arrow")
	}
	if !strings.Contains(output, "owner_reference") {
		t.Error("Output missing evidence")
	}
}

func TestRenderAttributionGraphASCII_NotPresentNode(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{
		Type:    NodeXR,
		Ref:     AttributionRef{Kind: "XR", Name: "missing"},
		Present: false,
	})

	graph := builder.Build()
	output := RenderAttributionGraphASCII(graph)

	if !strings.Contains(output, "?") {
		t.Error("Output should show '?' for not-present node")
	}
}

func TestRenderAttributionGraphASCII_Deterministic(t *testing.T) {
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{Type: NodeXR, Ref: AttributionRef{UID: "xr1"}, Present: true})
	builder.AddNode(AttributionNode{Type: NodeMR, Ref: AttributionRef{UID: "mr1"}, Present: true})
	builder.AddEdge(AttributionEdge{Type: EdgeOwns, From: "xr:uid:xr1", To: "mr:uid:mr1", Evidence: EvidenceOwnerReference})

	graph := builder.Build()

	first := RenderAttributionGraphASCII(graph)
	for i := 0; i < 5; i++ {
		output := RenderAttributionGraphASCII(graph)
		if output != first {
			t.Errorf("Iteration %d: output differs from first render", i)
		}
	}
}

func TestFormatAttributionRef_WithNamespace(t *testing.T) {
	ref := AttributionRef{
		Kind:      "Instance",
		Namespace: "crossplane-system",
		Name:      "staging-db",
	}

	result := formatAttributionRef(ref)
	expected := "Instance:crossplane-system/staging-db"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestFormatAttributionRef_WithoutNamespace(t *testing.T) {
	ref := AttributionRef{
		Kind: "Composition",
		Name: "postgres-aws",
	}

	result := formatAttributionRef(ref)
	expected := "Composition/postgres-aws"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short-id", "short-id"},
		{"xr:uid:abc-123", "xr:uid:abc-123"},
		{"this-is-a-very-long-id-that-exceeds-forty-characters-and-should-be-truncated", "this-is-a-very-long-id-that-exceeds-f..."},
	}

	for _, tt := range tests {
		result := truncateID(tt.input)
		if result != tt.expected {
			t.Errorf("truncateID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
