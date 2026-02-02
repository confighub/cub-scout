package graph

import (
	"fmt"
	"sort"
	"strings"
)

// RelatedEdge represents an edge in the explain output.
type RelatedEdge struct {
	Type     EdgeType
	Target   string // node ID (to_id for outgoing, from_id for incoming)
	Evidence []Evidence
}

// Explanation holds the explain output for a resource.
type Explanation struct {
	Node     Node
	Outgoing []RelatedEdge
	Incoming []RelatedEdge
}

// Explain returns explanation for a resource in the graph.
// Returns the rendered output string and an exit code (0=success, 3=not found).
func Explain(g *Graph, cluster, namespace, kind, name string) (string, int) {
	targetID := NodeID(cluster, namespace, kind, name)

	// Build node lookup map
	nodeMap := make(map[string]Node)
	for _, node := range g.Nodes {
		nodeMap[node.ID] = node
	}

	// Find target node
	node, found := nodeMap[targetID]
	if !found {
		return fmt.Sprintf("Error: target not found: %s\n", targetID), 3
	}

	// Filter and collect edges
	var outgoing, incoming []RelatedEdge

	for _, edge := range g.Edges {
		if edge.From == targetID {
			outgoing = append(outgoing, RelatedEdge{
				Type:     edge.Type,
				Target:   edge.To,
				Evidence: edge.Evidence,
			})
		}
		if edge.To == targetID {
			incoming = append(incoming, RelatedEdge{
				Type:     edge.Type,
				Target:   edge.From,
				Evidence: edge.Evidence,
			})
		}
	}

	// Sort edges deterministically
	sortRelatedEdges(outgoing)
	sortRelatedEdges(incoming)

	// Build explanation
	exp := Explanation{
		Node:     node,
		Outgoing: outgoing,
		Incoming: incoming,
	}

	return exp.Render(cluster), 0
}

// sortRelatedEdges sorts edges by (type, target) lexicographically.
func sortRelatedEdges(edges []RelatedEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Type != edges[j].Type {
			return edges[i].Type < edges[j].Type
		}
		return edges[i].Target < edges[j].Target
	})

	// Also sort evidence within each edge
	for i := range edges {
		sortEvidence(edges[i].Evidence)
	}
}

// sortEvidence sorts evidence by (field, reason) lexicographically.
func sortEvidence(evidence []Evidence) {
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].Field != evidence[j].Field {
			return evidence[i].Field < evidence[j].Field
		}
		return evidence[i].Reason < evidence[j].Reason
	})
}

// Render formats the explanation as deterministic text.
func (e *Explanation) Render(cluster string) string {
	var sb strings.Builder

	// Header
	sb.WriteString("GRAPH EXPLAIN\n")
	sb.WriteString(fmt.Sprintf("Target: %s\n", e.Node.ID))
	sb.WriteString(fmt.Sprintf("Schema: %s\n", SchemaVersion))
	sb.WriteString("\n")

	// Node details
	sb.WriteString("Node:\n")
	sb.WriteString(fmt.Sprintf("  kind: %s\n", e.Node.Kind))
	sb.WriteString(fmt.Sprintf("  name: %s\n", e.Node.Name))
	if e.Node.Namespace != "" {
		sb.WriteString(fmt.Sprintf("  namespace: %s\n", e.Node.Namespace))
	}
	sb.WriteString(fmt.Sprintf("  api_version: %s\n", e.Node.APIVersion))
	sb.WriteString(fmt.Sprintf("  id: %s\n", e.Node.ID))

	// Labels
	sb.WriteString("  labels:\n")
	if len(e.Node.Labels) == 0 {
		sb.WriteString("    (none)\n")
	} else {
		// Sort labels by key
		keys := make([]string, 0, len(e.Node.Labels))
		for k := range e.Node.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("    %s=%s\n", k, e.Node.Labels[k]))
		}
	}
	sb.WriteString("\n")

	// Relationships
	sb.WriteString("Relationships:\n")

	// Outgoing
	sb.WriteString(fmt.Sprintf("  outgoing (%d):\n", len(e.Outgoing)))
	for _, edge := range e.Outgoing {
		sb.WriteString(fmt.Sprintf("    - %s -> %s\n", edge.Type, edge.Target))
		sb.WriteString(fmt.Sprintf("      evidence (%d):\n", len(edge.Evidence)))
		for _, ev := range edge.Evidence {
			sb.WriteString(fmt.Sprintf("        - field: %s\n", ev.Field))
			sb.WriteString(fmt.Sprintf("          reason: %s\n", ev.Reason))
		}
	}

	// Incoming
	sb.WriteString(fmt.Sprintf("  incoming (%d):\n", len(e.Incoming)))
	for _, edge := range e.Incoming {
		sb.WriteString(fmt.Sprintf("    - %s <- %s\n", edge.Type, edge.Target))
		sb.WriteString(fmt.Sprintf("      evidence (%d):\n", len(edge.Evidence)))
		for _, ev := range edge.Evidence {
			sb.WriteString(fmt.Sprintf("        - field: %s\n", ev.Field))
			sb.WriteString(fmt.Sprintf("          reason: %s\n", ev.Reason))
		}
	}
	sb.WriteString("\n")

	// Footer
	sb.WriteString("Hint: Use 'cub-scout graph export --json' for the full graph.\n")

	return sb.String()
}
