// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

func TestBuildAttributionGraphFromCrossplaneLineage_Basic(t *testing.T) {
	lineage := &CrossplaneLineage{
		Managed: CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
			Present: true,
		},
		Composite: CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
			Present: false,
		},
		Evidence: []string{"label:crossplane.io/composite"},
	}

	graph := BuildAttributionGraphFromCrossplaneLineage(lineage, "test-bundle")

	if graph == nil {
		t.Fatal("Expected non-nil graph")
	}
	if graph.SchemaVersion != AttributionGraphSchemaVersion {
		t.Errorf("SchemaVersion = %s, want %s", graph.SchemaVersion, AttributionGraphSchemaVersion)
	}
	if len(graph.Nodes) != 2 {
		t.Errorf("Nodes count = %d, want 2", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Errorf("Edges count = %d, want 1", len(graph.Edges))
	}
}

func TestBuildAttributionGraphFromCrossplaneLineage_WithClaim(t *testing.T) {
	lineage := &CrossplaneLineage{
		Managed: CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
			Present: true,
		},
		Composite: CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
			Present: true,
		},
		Claim: &CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "PostgreSQLInstance", Name: "staging-db", Namespace: "app-team"},
			Present: true,
		},
		Evidence: []string{"label:crossplane.io/composite", "label:crossplane.io/claim-name"},
	}

	graph := BuildAttributionGraphFromCrossplaneLineage(lineage, "test-bundle")

	if graph == nil {
		t.Fatal("Expected non-nil graph")
	}
	if len(graph.Nodes) != 3 {
		t.Errorf("Nodes count = %d, want 3 (MR, XR, Claim)", len(graph.Nodes))
	}
	if len(graph.Edges) != 2 {
		t.Errorf("Edges count = %d, want 2 (XR->MR, Claim->XR)", len(graph.Edges))
	}

	// Check node types present
	nodeTypes := make(map[AttributionNodeType]int)
	for _, n := range graph.Nodes {
		nodeTypes[n.Type]++
	}
	if nodeTypes[NodeMR] != 1 {
		t.Errorf("MR nodes = %d, want 1", nodeTypes[NodeMR])
	}
	if nodeTypes[NodeXR] != 1 {
		t.Errorf("XR nodes = %d, want 1", nodeTypes[NodeXR])
	}
	if nodeTypes[NodeClaim] != 1 {
		t.Errorf("Claim nodes = %d, want 1", nodeTypes[NodeClaim])
	}
}

func TestBuildAttributionGraphFromCrossplaneLineage_Nil(t *testing.T) {
	graph := BuildAttributionGraphFromCrossplaneLineage(nil, "test-bundle")
	if graph != nil {
		t.Error("Expected nil graph for nil lineage")
	}
}

func TestBuildAttributionGraphFromCrossplaneLineage_EvidenceMapping(t *testing.T) {
	tests := []struct {
		name     string
		evidence []string
		want     AttributionEvidence
	}{
		{
			name:     "composite label",
			evidence: []string{"label:crossplane.io/composite"},
			want:     EvidenceCompositeLabel,
		},
		{
			name:     "owner reference",
			evidence: []string{"ownerRef:database.example.org/v1alpha1/XPostgreSQLInstance"},
			want:     EvidenceOwnerReference,
		},
		{
			name:     "claim label",
			evidence: []string{"label:crossplane.io/claim-name"},
			want:     EvidenceClaimLabel,
		},
		{
			name:     "unknown defaults to owner reference",
			evidence: []string{"unknown:signal"},
			want:     EvidenceOwnerReference,
		},
		{
			name:     "empty defaults to owner reference",
			evidence: []string{},
			want:     EvidenceOwnerReference,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evidenceFromLineageEvidence(tt.evidence)
			if got != tt.want {
				t.Errorf("evidenceFromLineageEvidence(%v) = %s, want %s", tt.evidence, got, tt.want)
			}
		})
	}
}

func TestResourceRefToAttributionRef(t *testing.T) {
	tests := []struct {
		name string
		ref  ResourceRef
		want AttributionRef
	}{
		{
			name: "full ref with group and version",
			ref: ResourceRef{
				Kind:      "Instance",
				Name:      "staging-db",
				Namespace: "crossplane-system",
				Group:     "rds.aws.upbound.io",
				Version:   "v1beta1",
			},
			want: AttributionRef{
				APIVersion: "rds.aws.upbound.io/v1beta1",
				Kind:       "Instance",
				Namespace:  "crossplane-system",
				Name:       "staging-db",
			},
		},
		{
			name: "ref with version only",
			ref: ResourceRef{
				Kind:    "ConfigMap",
				Name:    "my-config",
				Version: "v1",
			},
			want: AttributionRef{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "my-config",
			},
		},
		{
			name: "minimal ref",
			ref: ResourceRef{
				Kind: "CompositeResource",
				Name: "xpostgresql-staging",
			},
			want: AttributionRef{
				Kind: "CompositeResource",
				Name: "xpostgresql-staging",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourceRefToAttributionRef(tt.ref)
			if got.APIVersion != tt.want.APIVersion {
				t.Errorf("APIVersion = %q, want %q", got.APIVersion, tt.want.APIVersion)
			}
			if got.Kind != tt.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.want.Kind)
			}
			if got.Namespace != tt.want.Namespace {
				t.Errorf("Namespace = %q, want %q", got.Namespace, tt.want.Namespace)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
		})
	}
}

func TestBuildAttributionGraphFromCrossplaneLineage_Deterministic(t *testing.T) {
	lineage := &CrossplaneLineage{
		Managed: CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
			Present: true,
		},
		Composite: CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
			Present: false,
		},
		Evidence: []string{"label:crossplane.io/composite"},
	}

	// Build multiple times and compare node/edge IDs (not full graph due to timestamps)
	var firstNodeIDs, firstEdgeIDs []string
	for i := 0; i < 5; i++ {
		graph := BuildAttributionGraphFromCrossplaneLineage(lineage, "test-bundle")

		var nodeIDs, edgeIDs []string
		for _, n := range graph.Nodes {
			nodeIDs = append(nodeIDs, n.ID)
		}
		for _, e := range graph.Edges {
			edgeIDs = append(edgeIDs, e.ID)
		}

		if i == 0 {
			firstNodeIDs = nodeIDs
			firstEdgeIDs = edgeIDs
		} else {
			// Compare
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

func TestBuildAttributionGraphFromCrossplaneLineage_NodePresent(t *testing.T) {
	lineage := &CrossplaneLineage{
		Managed: CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "Instance", Name: "staging-db"},
			Present: true,
		},
		Composite: CrossplaneLineageNode{
			Ref:     ResourceRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
			Present: false, // XR not found
		},
		Evidence: []string{"label:crossplane.io/composite"},
	}

	graph := BuildAttributionGraphFromCrossplaneLineage(lineage, "")

	// Check present flags
	for _, n := range graph.Nodes {
		switch n.Type {
		case NodeMR:
			if !n.Present {
				t.Error("MR should be present")
			}
		case NodeXR:
			if n.Present {
				t.Error("XR should not be present")
			}
		}
	}
}
