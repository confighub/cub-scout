package graph

import (
	"encoding/json"
	"sort"
)

// Export serializes the graph to JSON with deterministic ordering.
// The output is stable: same input produces byte-identical output.
func (g *Graph) Export() ([]byte, error) {
	// Sort for deterministic output
	g.Sort()

	return json.MarshalIndent(g, "", "  ")
}

// Sort sorts nodes and edges for deterministic output.
// Nodes are sorted by ID. Edges are sorted by (from, to, type).
func (g *Graph) Sort() {
	// Sort nodes by ID
	sort.Slice(g.Nodes, func(i, j int) bool {
		return g.Nodes[i].ID < g.Nodes[j].ID
	})

	// Sort edges by (from, to, type)
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return g.Edges[i].From < g.Edges[j].From
		}
		if g.Edges[i].To != g.Edges[j].To {
			return g.Edges[i].To < g.Edges[j].To
		}
		return g.Edges[i].Type < g.Edges[j].Type
	})

	// Sort labels within each node for deterministic output
	// (Go's json.Marshal already sorts map keys, but we document the contract)
}
