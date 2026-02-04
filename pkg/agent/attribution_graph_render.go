// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"strings"
)

// RenderAttributionGraphASCII renders an attribution graph as ASCII.
// This is a minimal renderer for PR1; more detailed output in PR2.
func RenderAttributionGraphASCII(graph *AttributionGraph) string {
	var b strings.Builder

	// Header
	b.WriteString("Attribution Graph\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n\n")

	// Schema info
	b.WriteString(fmt.Sprintf("Schema:   %s\n", graph.SchemaVersion))
	if graph.GeneratedFrom != nil && graph.GeneratedFrom.BundleID != "" {
		b.WriteString(fmt.Sprintf("Bundle:   %s\n", graph.GeneratedFrom.BundleID))
	}
	b.WriteString("\n")

	// Summary
	b.WriteString("Summary\n")
	b.WriteString(fmt.Sprintf("  Nodes:        %d\n", len(graph.Nodes)))
	b.WriteString(fmt.Sprintf("  Edges:        %d\n", len(graph.Edges)))
	if graph.Summary.UnattributedCount > 0 {
		b.WriteString(fmt.Sprintf("  Unattributed: %d\n", graph.Summary.UnattributedCount))
	}
	if graph.Summary.AmbiguousCount > 0 {
		b.WriteString(fmt.Sprintf("  Ambiguous:    %d\n", graph.Summary.AmbiguousCount))
	}
	b.WriteString("\n")

	// Node counts by type
	if len(graph.Summary.NodesByType) > 0 {
		b.WriteString("Nodes by Type\n")
		// Render in deterministic order
		for _, nodeType := range []AttributionNodeType{NodeClaim, NodeXR, NodeMR, NodeComposition, NodeCompositionRevision} {
			if count, ok := graph.Summary.NodesByType[nodeType]; ok && count > 0 {
				b.WriteString(fmt.Sprintf("  %-20s %d\n", nodeType, count))
			}
		}
		b.WriteString("\n")
	}

	// Edge counts by type
	if len(graph.Summary.EdgesByType) > 0 {
		b.WriteString("Edges by Type\n")
		// Render in deterministic order
		for _, edgeType := range []AttributionEdgeType{EdgeOwns, EdgeSelectedComposition, EdgeSelectedCompositionRevision} {
			if count, ok := graph.Summary.EdgesByType[edgeType]; ok && count > 0 {
				b.WriteString(fmt.Sprintf("  %-30s %d\n", edgeType, count))
			}
		}
		b.WriteString("\n")
	}

	// Show nodes (if not too many)
	if len(graph.Nodes) > 0 && len(graph.Nodes) <= 20 {
		b.WriteString("Nodes\n")
		for _, node := range graph.Nodes {
			presence := "✓"
			if !node.Present {
				presence = "?"
			}
			ref := formatAttributionRef(node.Ref)
			b.WriteString(fmt.Sprintf("  %s [%s] %s\n", presence, node.Type, ref))
		}
		b.WriteString("\n")
	} else if len(graph.Nodes) > 20 {
		b.WriteString(fmt.Sprintf("Nodes: %d (use --format json for full list)\n\n", len(graph.Nodes)))
	}

	// Show edges (if not too many)
	if len(graph.Edges) > 0 && len(graph.Edges) <= 20 {
		b.WriteString("Edges\n")
		for _, edge := range graph.Edges {
			b.WriteString(fmt.Sprintf("  %s → %s\n", truncateID(edge.From), truncateID(edge.To)))
			b.WriteString(fmt.Sprintf("    type: %s, evidence: %s\n", edge.Type, edge.Evidence))
		}
		b.WriteString("\n")
	} else if len(graph.Edges) > 20 {
		b.WriteString(fmt.Sprintf("Edges: %d (use --format json for full list)\n\n", len(graph.Edges)))
	}

	return b.String()
}

// formatAttributionRef formats a reference for display.
func formatAttributionRef(ref AttributionRef) string {
	if ref.Namespace != "" {
		return fmt.Sprintf("%s:%s/%s", ref.Kind, ref.Namespace, ref.Name)
	}
	return fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
}

// truncateID truncates a node ID for display.
func truncateID(id string) string {
	// If ID is too long, truncate and add ellipsis
	if len(id) > 40 {
		return id[:37] + "..."
	}
	return id
}
