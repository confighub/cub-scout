// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides Attribution Report v1 for human-readable ownership explanations.
//
// An attribution report is derived strictly from attribution-graph.v1 and produces
// a ranked list of ownership explanations for each object in the graph.
//
// Contract:
// - Report is derived from graph JSON (no cluster access)
// - Scoring is deterministic: owner_reference > composite_label > claim_label
// - Tie-breaks use stable ordering (alphabetical by ID)
// - ASCII = f(JSON) + g (renderer consumes only report JSON)
package agent

import (
	"sort"
	"time"
)

// AttributionReportSchemaVersion is the schema version for attribution reports.
const AttributionReportSchemaVersion = "attribution-report.v1"

// Evidence scoring constants (higher = stronger signal)
const (
	ScoreOwnerReference   = 100
	ScoreKustomizeOrigin  = 90 // Reserved for annotation-based detection (future)
	ScoreCompositeLabel   = 80
	ScoreKustomizeOverlay = 75
	ScoreClaimLabel       = 60
)

// AttributionReport is the top-level report result.
// Schema: attribution-report.v1
type AttributionReport struct {
	// SchemaVersion identifies the report format
	SchemaVersion string `json:"schema_version"`

	// GeneratedAt is when the report was generated
	GeneratedAt time.Time `json:"generated_at"`

	// Items contains one entry per managed resource or composed object
	Items []AttributionReportItem `json:"items"`

	// Summary provides aggregate counts
	Summary AttributionReportSummary `json:"summary"`
}

// AttributionReportItem describes ownership for a single object.
type AttributionReportItem struct {
	// Ref identifies the object
	Ref AttributionRef `json:"ref"`

	// BestOwner is the top-ranked owner (nil if unattributed)
	BestOwner *OwnerCandidate `json:"best_owner,omitempty"`

	// OwnerCandidates lists all owners ranked by score (descending)
	OwnerCandidates []OwnerCandidate `json:"owner_candidates"`

	// Reason is a structured code explaining the attribution (not free text)
	Reason AttributionReason `json:"reason"`
}

// OwnerCandidate represents a potential owner with its score.
type OwnerCandidate struct {
	// Ref identifies the owner
	Ref AttributionRef `json:"ref"`

	// NodeType is the owner's node type (xr, claim, composition, etc.)
	NodeType AttributionNodeType `json:"node_type"`

	// Score is the computed ownership score
	Score int `json:"score"`

	// Evidence describes how ownership was determined
	Evidence AttributionEvidence `json:"evidence"`
}

// AttributionReason is a structured code explaining attribution.
type AttributionReason string

const (
	// ReasonOwnedViaOwnerRef indicates ownership via Kubernetes ownerReference
	ReasonOwnedViaOwnerRef AttributionReason = "owned_via_owner_ref"

	// ReasonOwnedViaLabel indicates ownership via Crossplane label
	ReasonOwnedViaLabel AttributionReason = "owned_via_label"

	// ReasonOwnedViaKustomize indicates ownership via user-provided kustomize context
	ReasonOwnedViaKustomize AttributionReason = "owned_via_kustomize"

	// ReasonUnattributed indicates no owner found
	ReasonUnattributed AttributionReason = "unattributed"

	// ReasonAmbiguous indicates multiple equally-ranked owners
	ReasonAmbiguous AttributionReason = "ambiguous"
)

// AttributionReportSummary provides aggregate counts.
type AttributionReportSummary struct {
	// TotalItems is the number of objects in the report
	TotalItems int `json:"total_items"`

	// OwnedCount is the number of objects with a clear owner
	OwnedCount int `json:"owned_count"`

	// UnattributedCount is the number of objects without any owner
	UnattributedCount int `json:"unattributed_count"`

	// AmbiguousCount is the number of objects with ambiguous ownership
	AmbiguousCount int `json:"ambiguous_count"`
}

// BuildAttributionReport produces an attribution report from a graph.
// The graph must be a valid attribution-graph.v1.
func BuildAttributionReport(graph *AttributionGraph) (*AttributionReport, error) {
	if graph == nil {
		return nil, nil
	}

	// Build node index for quick lookup
	nodeIndex := make(map[string]AttributionNode)
	for _, n := range graph.Nodes {
		nodeIndex[n.ID] = n
	}

	// Build reverse edge index: target -> list of (source, edge)
	// This finds "who owns this target"
	incomingEdges := make(map[string][]incomingEdgeInfo)
	for _, e := range graph.Edges {
		if e.Type == EdgeOwns {
			incomingEdges[e.To] = append(incomingEdges[e.To], incomingEdgeInfo{
				sourceID: e.From,
				evidence: e.Evidence,
			})
		}
	}

	// Process each target node (MR or K8sObject) - these are what we report on
	var items []AttributionReportItem
	for _, n := range graph.Nodes {
		if n.Type != NodeMR && n.Type != NodeK8sObject {
			continue
		}

		item := buildReportItem(n, incomingEdges, nodeIndex)
		items = append(items, item)
	}

	// Sort items deterministically by ref (kind, namespace, name)
	sort.Slice(items, func(i, j int) bool {
		return refSortKey(items[i].Ref) < refSortKey(items[j].Ref)
	})

	// Build summary
	summary := AttributionReportSummary{
		TotalItems: len(items),
	}
	for _, item := range items {
		switch item.Reason {
		case ReasonOwnedViaOwnerRef, ReasonOwnedViaLabel:
			summary.OwnedCount++
		case ReasonUnattributed:
			summary.UnattributedCount++
		case ReasonAmbiguous:
			summary.AmbiguousCount++
		}
	}

	return &AttributionReport{
		SchemaVersion: AttributionReportSchemaVersion,
		GeneratedAt:   time.Now(),
		Items:         items,
		Summary:       summary,
	}, nil
}

// incomingEdgeInfo tracks an incoming ownership edge.
type incomingEdgeInfo struct {
	sourceID string
	evidence AttributionEvidence
}

// buildReportItem builds a report item for a single node.
func buildReportItem(node AttributionNode, incomingEdges map[string][]incomingEdgeInfo, nodeIndex map[string]AttributionNode) AttributionReportItem {
	edges := incomingEdges[node.ID]

	// Build candidates from incoming edges
	var candidates []OwnerCandidate
	for _, e := range edges {
		sourceNode, ok := nodeIndex[e.sourceID]
		if !ok {
			continue
		}

		candidates = append(candidates, OwnerCandidate{
			Ref:      sourceNode.Ref,
			NodeType: sourceNode.Type,
			Score:    scoreForEvidence(e.evidence),
			Evidence: e.evidence,
		})
	}

	// Sort candidates by score (descending), then by ref for tie-break
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return refSortKey(candidates[i].Ref) < refSortKey(candidates[j].Ref)
	})

	item := AttributionReportItem{
		Ref:             node.Ref,
		OwnerCandidates: candidates,
	}

	// Determine reason and best owner
	if len(candidates) == 0 {
		item.Reason = ReasonUnattributed
	} else if len(candidates) > 1 && candidates[0].Score == candidates[1].Score {
		// Ambiguous: top two have same score
		item.Reason = ReasonAmbiguous
		// Still pick first (deterministic tie-break by ref)
		item.BestOwner = &candidates[0]
	} else {
		// Clear winner
		item.BestOwner = &candidates[0]
		item.Reason = reasonForEvidence(candidates[0].Evidence)
	}

	return item
}

// scoreForEvidence returns the score for an evidence type.
func scoreForEvidence(evidence AttributionEvidence) int {
	switch evidence {
	case EvidenceOwnerReference:
		return ScoreOwnerReference
	case EvidenceCompositeLabel:
		return ScoreCompositeLabel
	case EvidenceKustomizeOverlay:
		return ScoreKustomizeOverlay
	case EvidenceClaimLabel:
		return ScoreClaimLabel
	default:
		// Unknown evidence gets a low score
		return 10
	}
}

// reasonForEvidence returns the appropriate reason code for an evidence type.
func reasonForEvidence(evidence AttributionEvidence) AttributionReason {
	switch evidence {
	case EvidenceOwnerReference:
		return ReasonOwnedViaOwnerRef
	case EvidenceKustomizeOverlay:
		return ReasonOwnedViaKustomize
	default:
		// composite_label, claim_label, etc.
		return ReasonOwnedViaLabel
	}
}

// refSortKey returns a stable sort key for a ref.
func refSortKey(ref AttributionRef) string {
	return ref.Kind + "/" + ref.Namespace + "/" + ref.Name
}
