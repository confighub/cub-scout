// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"sort"
)

// MergeAttributionGraphs merges two attribution graphs into one.
// The result is deterministic: nodes and edges are deduplicated by ID,
// sorted, and summary is recomputed.
//
// Nil-safe: returns the non-nil graph if one is nil, or nil if both are nil.
func MergeAttributionGraphs(a, b *AttributionGraph) *AttributionGraph {
	// Nil-safe handling
	if a == nil && b == nil {
		return nil
	}
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	// Merge nodes (dedupe by ID, keep first)
	nodeMap := make(map[string]AttributionNode)
	for _, n := range a.Nodes {
		nodeMap[n.ID] = n
	}
	for _, n := range b.Nodes {
		if _, exists := nodeMap[n.ID]; !exists {
			nodeMap[n.ID] = n
		}
	}

	// Merge edges (dedupe by ID, keep first)
	edgeMap := make(map[string]AttributionEdge)
	for _, e := range a.Edges {
		edgeMap[e.ID] = e
	}
	for _, e := range b.Edges {
		if _, exists := edgeMap[e.ID]; !exists {
			edgeMap[e.ID] = e
		}
	}

	// Convert to slices and sort
	nodes := make([]AttributionNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	edges := make([]AttributionEdge, 0, len(edgeMap))
	for _, e := range edgeMap {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].ID < edges[j].ID
	})

	// Recompute summary from merged data
	summary := AttributionSummary{
		NodesByType: make(map[AttributionNodeType]int),
		EdgesByType: make(map[AttributionEdgeType]int),
	}
	for _, n := range nodes {
		summary.NodesByType[n.Type]++
	}
	for _, e := range edges {
		summary.EdgesByType[e.Type]++
	}

	// Merge unattributed/ambiguous counts (take max, since they may overlap)
	summary.UnattributedCount = maxInt(a.Summary.UnattributedCount, b.Summary.UnattributedCount)
	summary.AmbiguousCount = maxInt(a.Summary.AmbiguousCount, b.Summary.AmbiguousCount)

	// Use source from first graph if present
	var source *AttributionSource
	if a.GeneratedFrom != nil {
		source = a.GeneratedFrom
	} else if b.GeneratedFrom != nil {
		source = b.GeneratedFrom
	}

	return &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		GeneratedFrom: source,
		Nodes:         nodes,
		Edges:         edges,
		Summary:       summary,
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
