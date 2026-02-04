// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides Attribution Graph v1 for composition lineage.
//
// An attribution graph describes object lineage and ownership relationships
// captured at bundle creation time from cluster objects.
//
// Contract:
// - Attribution is captured at bundle creation (not computed at replay)
// - Graphs are deterministic (sorted nodes/edges, stable IDs)
// - Missing/ambiguous lineage is explicit (never inferred)
// - ASCII = f(JSON) + g (renderer consumes only graph JSON)
package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

// AttributionGraphSchemaVersion is the schema version for attribution graphs.
const AttributionGraphSchemaVersion = "attribution-graph.v1"

// AttributionGraph is the top-level attribution result.
// Schema: attribution-graph.v1
type AttributionGraph struct {
	// SchemaVersion identifies the graph format
	SchemaVersion string `json:"schema_version"`

	// GeneratedFrom identifies the source bundle (optional)
	GeneratedFrom *AttributionSource `json:"generated_from,omitempty"`

	// Nodes are the graph nodes, sorted by ID
	Nodes []AttributionNode `json:"nodes"`

	// Edges are the graph edges, sorted by ID
	Edges []AttributionEdge `json:"edges"`

	// Summary provides aggregate counts
	Summary AttributionSummary `json:"summary"`
}

// AttributionSource identifies where the attribution was captured from.
type AttributionSource struct {
	// BundleID is the bundle identifier
	BundleID string `json:"bundle_id,omitempty"`

	// CreatedAt is when the bundle was created
	CreatedAt time.Time `json:"created_at,omitempty"`

	// ToolVersion is the cub-scout version
	ToolVersion string `json:"tool_version,omitempty"`
}

// AttributionNodeType identifies the type of node.
type AttributionNodeType string

const (
	// NodeXR is a Composite Resource (XR)
	NodeXR AttributionNodeType = "xr"

	// NodeMR is a Managed Resource (composed resource)
	NodeMR AttributionNodeType = "mr"

	// NodeClaim is a Crossplane Claim
	NodeClaim AttributionNodeType = "claim"

	// NodeComposition is a Composition
	NodeComposition AttributionNodeType = "composition"

	// NodeCompositionRevision is a CompositionRevision
	NodeCompositionRevision AttributionNodeType = "composition_revision"

	// NodeK8sObject is a generic Kubernetes object (non-Crossplane)
	NodeK8sObject AttributionNodeType = "k8s_object"

	// NodeKustomizeOverlay is a kustomization directory that owns rendered resources
	NodeKustomizeOverlay AttributionNodeType = "kustomize_overlay"
)

// AttributionNode represents a node in the attribution graph.
type AttributionNode struct {
	// ID is a stable, deterministic identifier
	ID string `json:"id"`

	// Type identifies the node type
	Type AttributionNodeType `json:"type"`

	// Ref identifies the Kubernetes object
	Ref AttributionRef `json:"ref"`

	// Present indicates if the object was found during capture
	Present bool `json:"present"`
}

// AttributionRef identifies a Kubernetes object.
type AttributionRef struct {
	// APIVersion is the object's API version (e.g., "rds.aws.upbound.io/v1beta1")
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind is the object kind
	Kind string `json:"kind"`

	// Namespace is the object namespace (empty for cluster-scoped)
	Namespace string `json:"namespace,omitempty"`

	// Name is the object name
	Name string `json:"name"`

	// UID is the object UID (most stable identifier)
	UID string `json:"uid,omitempty"`
}

// AttributionEdgeType identifies the type of edge.
type AttributionEdgeType string

const (
	// EdgeOwns indicates ownership (XR owns MR, Claim owns XR)
	EdgeOwns AttributionEdgeType = "owns"

	// EdgeSelectedComposition indicates composition selection
	EdgeSelectedComposition AttributionEdgeType = "selected_composition"

	// EdgeSelectedCompositionRevision indicates composition revision selection
	EdgeSelectedCompositionRevision AttributionEdgeType = "selected_composition_revision"
)

// AttributionEvidence describes how an edge was determined.
type AttributionEvidence string

const (
	// EvidenceOwnerReference indicates edge derived from ownerReferences
	EvidenceOwnerReference AttributionEvidence = "owner_reference"

	// EvidenceCompositeLabel indicates edge derived from crossplane.io/composite label
	EvidenceCompositeLabel AttributionEvidence = "composite_label"

	// EvidenceClaimLabel indicates edge derived from crossplane.io/claim-* labels
	EvidenceClaimLabel AttributionEvidence = "claim_label"

	// EvidenceSpecCompositionRef indicates edge derived from spec.compositionRef
	EvidenceSpecCompositionRef AttributionEvidence = "spec_composition_ref"

	// EvidenceSpecCompositionRevisionRef indicates edge derived from spec.compositionRevisionRef
	EvidenceSpecCompositionRevisionRef AttributionEvidence = "spec_composition_revision_ref"

	// EvidenceKustomizeOverlay indicates ownership via user-provided kustomize context
	EvidenceKustomizeOverlay AttributionEvidence = "kustomize_overlay"
)

// AttributionEdge represents an edge in the attribution graph.
type AttributionEdge struct {
	// ID is a stable, deterministic identifier
	ID string `json:"id"`

	// Type identifies the edge type
	Type AttributionEdgeType `json:"type"`

	// From is the source node ID
	From string `json:"from"`

	// To is the target node ID
	To string `json:"to"`

	// Evidence describes how this edge was determined
	Evidence AttributionEvidence `json:"evidence"`
}

// AttributionSummary provides aggregate counts for the graph.
type AttributionSummary struct {
	// NodesByType counts nodes by type
	NodesByType map[AttributionNodeType]int `json:"nodes_by_type"`

	// EdgesByType counts edges by type
	EdgesByType map[AttributionEdgeType]int `json:"edges_by_type"`

	// UnattributedCount is the number of objects without lineage
	UnattributedCount int `json:"unattributed_count"`

	// AmbiguousCount is the number of objects with ambiguous lineage
	AmbiguousCount int `json:"ambiguous_count"`
}

// AttributionGraphBuilder builds attribution graphs with deterministic output.
type AttributionGraphBuilder struct {
	nodes   map[string]AttributionNode
	edges   map[string]AttributionEdge
	source  *AttributionSource
	unattributed int
	ambiguous    int
}

// NewAttributionGraphBuilder creates a new builder.
func NewAttributionGraphBuilder() *AttributionGraphBuilder {
	return &AttributionGraphBuilder{
		nodes: make(map[string]AttributionNode),
		edges: make(map[string]AttributionEdge),
	}
}

// SetSource sets the attribution source metadata.
func (b *AttributionGraphBuilder) SetSource(bundleID string, createdAt time.Time, toolVersion string) {
	b.source = &AttributionSource{
		BundleID:    bundleID,
		CreatedAt:   createdAt,
		ToolVersion: toolVersion,
	}
}

// AddNode adds a node to the graph.
// If a node with the same ID exists, it is replaced.
func (b *AttributionGraphBuilder) AddNode(node AttributionNode) {
	if node.ID == "" {
		node.ID = GenerateNodeID(node.Type, node.Ref)
	}
	b.nodes[node.ID] = node
}

// AddEdge adds an edge to the graph.
// If an edge with the same ID exists, it is replaced.
func (b *AttributionGraphBuilder) AddEdge(edge AttributionEdge) {
	if edge.ID == "" {
		edge.ID = GenerateEdgeID(edge.Type, edge.From, edge.To, edge.Evidence)
	}
	b.edges[edge.ID] = edge
}

// IncrementUnattributed increments the unattributed count.
func (b *AttributionGraphBuilder) IncrementUnattributed() {
	b.unattributed++
}

// IncrementAmbiguous increments the ambiguous count.
func (b *AttributionGraphBuilder) IncrementAmbiguous() {
	b.ambiguous++
}

// Build produces the final AttributionGraph with deterministic ordering.
func (b *AttributionGraphBuilder) Build() *AttributionGraph {
	// Sort nodes by ID
	nodes := make([]AttributionNode, 0, len(b.nodes))
	for _, n := range b.nodes {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	// Sort edges by ID
	edges := make([]AttributionEdge, 0, len(b.edges))
	for _, e := range b.edges {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].ID < edges[j].ID
	})

	// Build summary
	summary := AttributionSummary{
		NodesByType:       make(map[AttributionNodeType]int),
		EdgesByType:       make(map[AttributionEdgeType]int),
		UnattributedCount: b.unattributed,
		AmbiguousCount:    b.ambiguous,
	}
	for _, n := range nodes {
		summary.NodesByType[n.Type]++
	}
	for _, e := range edges {
		summary.EdgesByType[e.Type]++
	}

	return &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		GeneratedFrom: b.source,
		Nodes:         nodes,
		Edges:         edges,
		Summary:       summary,
	}
}

// GenerateNodeID generates a stable, deterministic node ID.
// Prefers UID when available, falls back to canonical ref string.
func GenerateNodeID(nodeType AttributionNodeType, ref AttributionRef) string {
	if ref.UID != "" {
		return fmt.Sprintf("%s:uid:%s", nodeType, ref.UID)
	}
	// Fallback: canonical ref string
	if ref.Namespace != "" {
		return fmt.Sprintf("%s:ref:%s/%s:%s/%s", nodeType, ref.APIVersion, ref.Kind, ref.Namespace, ref.Name)
	}
	return fmt.Sprintf("%s:ref:%s/%s:%s", nodeType, ref.APIVersion, ref.Kind, ref.Name)
}

// GenerateEdgeID generates a stable, deterministic edge ID.
// Uses a hash of the edge components for compactness.
func GenerateEdgeID(edgeType AttributionEdgeType, from, to string, evidence AttributionEvidence) string {
	canonical := fmt.Sprintf("%s|%s|%s|%s", edgeType, from, to, evidence)
	hash := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("edge:%s", hex.EncodeToString(hash[:8]))
}

// Validate checks the graph for structural integrity.
func (g *AttributionGraph) Validate() []string {
	var errors []string

	// Check schema version
	if g.SchemaVersion != AttributionGraphSchemaVersion {
		errors = append(errors, fmt.Sprintf("unexpected schema_version: %q (expected %q)",
			g.SchemaVersion, AttributionGraphSchemaVersion))
	}

	// Build node ID set
	nodeIDs := make(map[string]bool)
	for _, n := range g.Nodes {
		if n.ID == "" {
			errors = append(errors, "node with empty ID")
		}
		if nodeIDs[n.ID] {
			errors = append(errors, fmt.Sprintf("duplicate node ID: %s", n.ID))
		}
		nodeIDs[n.ID] = true
	}

	// Check edge endpoints exist
	edgeIDs := make(map[string]bool)
	for _, e := range g.Edges {
		if e.ID == "" {
			errors = append(errors, "edge with empty ID")
		}
		if edgeIDs[e.ID] {
			errors = append(errors, fmt.Sprintf("duplicate edge ID: %s", e.ID))
		}
		edgeIDs[e.ID] = true

		if !nodeIDs[e.From] {
			errors = append(errors, fmt.Sprintf("edge %s references unknown from node: %s", e.ID, e.From))
		}
		if !nodeIDs[e.To] {
			errors = append(errors, fmt.Sprintf("edge %s references unknown to node: %s", e.ID, e.To))
		}
	}

	// Check ordering (determinism)
	for i := 1; i < len(g.Nodes); i++ {
		if g.Nodes[i].ID < g.Nodes[i-1].ID {
			errors = append(errors, "nodes not sorted by ID")
			break
		}
	}
	for i := 1; i < len(g.Edges); i++ {
		if g.Edges[i].ID < g.Edges[i-1].ID {
			errors = append(errors, "edges not sorted by ID")
			break
		}
	}

	return errors
}

// IsEmpty returns true if the graph has no nodes or edges.
func (g *AttributionGraph) IsEmpty() bool {
	return len(g.Nodes) == 0 && len(g.Edges) == 0
}
