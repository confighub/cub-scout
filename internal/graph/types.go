// Package graph provides the resource graph model for cub-scout v0.6+.
//
// The graph captures cluster-visible relationships between Kubernetes resources
// with evidence-backed edges. This is a new v0.6 contract surface that does not
// modify any v0.5 CLI contracts.
package graph

import "time"

// SchemaVersion is the current graph export schema version.
// Changes to the schema require a new version.
const SchemaVersion = "graph.v1"

// Graph represents the complete resource graph for a cluster.
type Graph struct {
	// SchemaVersion identifies the graph schema version for compatibility.
	SchemaVersion string `json:"schema_version"`

	// GeneratedAt is the timestamp when this graph was generated.
	GeneratedAt time.Time `json:"generated_at"`

	// Cluster is the name of the cluster this graph represents.
	Cluster string `json:"cluster"`

	// Nodes are the resources in the graph.
	Nodes []Node `json:"nodes"`

	// Edges are the relationships between resources.
	Edges []Edge `json:"edges"`
}

// Node represents a Kubernetes resource in the graph.
type Node struct {
	// ID is the unique identifier for this node.
	// Format: <cluster>/<namespace>/<kind>/<name> (or <cluster>/-/<kind>/<name> for cluster-scoped)
	ID string `json:"id"`

	// Cluster is the cluster this resource belongs to.
	Cluster string `json:"cluster"`

	// Namespace is the namespace of the resource (empty for cluster-scoped).
	Namespace string `json:"namespace,omitempty"`

	// Kind is the Kubernetes resource kind.
	Kind string `json:"kind"`

	// Name is the resource name.
	Name string `json:"name"`

	// APIVersion is the Kubernetes API version (e.g., "apps/v1").
	APIVersion string `json:"api_version"`

	// Labels are the resource labels.
	Labels map[string]string `json:"labels,omitempty"`
}

// Edge represents a relationship between two resources.
type Edge struct {
	// From is the source node ID.
	From string `json:"from"`

	// To is the target node ID.
	To string `json:"to"`

	// Type is the relationship type.
	Type EdgeType `json:"type"`

	// Evidence describes how this relationship was detected.
	Evidence Evidence `json:"evidence"`
}

// EdgeType represents the type of relationship between resources.
type EdgeType string

const (
	// EdgeTypeOwns indicates the source owns the target (via ownerReferences).
	EdgeTypeOwns EdgeType = "owns"

	// EdgeTypeSelects indicates the source selects the target (via label selector).
	EdgeTypeSelects EdgeType = "selects"

	// EdgeTypeManagedBy indicates the source is managed by the target.
	EdgeTypeManagedBy EdgeType = "managed_by"

	// EdgeTypeReferences indicates the source references the target.
	EdgeTypeReferences EdgeType = "references"
)

// Evidence describes how a relationship was detected.
type Evidence struct {
	// Field is the field path that established this relationship.
	// Examples: "metadata.ownerReferences[0]", "spec.selector.matchLabels"
	Field string `json:"field"`

	// Reason is a human-readable explanation.
	Reason string `json:"reason"`
}

// NewGraph creates a new empty graph with the current schema version.
func NewGraph(cluster string) *Graph {
	return &Graph{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Cluster:       cluster,
		Nodes:         []Node{},
		Edges:         []Edge{},
	}
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(node Node) {
	g.Nodes = append(g.Nodes, node)
}

// AddEdge adds an edge to the graph.
func (g *Graph) AddEdge(edge Edge) {
	g.Edges = append(g.Edges, edge)
}

// NodeID generates a canonical node ID.
func NodeID(cluster, namespace, kind, name string) string {
	if namespace == "" {
		return cluster + "/-/" + kind + "/" + name
	}
	return cluster + "/" + namespace + "/" + kind + "/" + name
}
