# Graph Explain Contract (v0.6)

This document defines the `graph explain` command contract for cub-scout v0.6+.

> **v0.6 contract surface.** This does not modify any v0.5 contracts.

---

## Command

```bash
cub-scout graph explain <kind>/<name> -n <namespace>
```

### Inputs

| Argument | Required | Description |
|----------|----------|-------------|
| `<kind>/<name>` | yes | Target resource selector (case-sensitive Kind as shown in graph export) |
| `-n, --namespace` | yes | Namespace of the resource |

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (missing args / missing namespace) |
| 3 | Target not found |

---

## Output Format (Deterministic Text)

### Header

```
GRAPH EXPLAIN
Target: <cluster>/<namespace>/<kind>/<name>
Schema: graph.v1
```

### Node Details Section

Always shown:

```
Node:
  kind: <Kind>
  name: <name>
  namespace: <namespace>        # omit line entirely for cluster-scoped
  api_version: <api_version>
  id: <id>
  labels:
    <k1>=<v1>
    <k2>=<v2>
```

Rules:
- `labels:` block:
  - If no labels: show `labels: (none)`
  - Otherwise sort keys lexicographically
- Field names are lowercase with underscores exactly as above

### Relationships Section

Always shown:

```
Relationships:
  outgoing (<N>):
    - <type> -> <to_id>
      evidence (<M>):
        - field: <field>
          reason: <reason>
  incoming (<N>):
    - <type> <- <from_id>
      evidence (<M>):
        - field: <field>
          reason: <reason>
```

Rules:
- Show both `outgoing` and `incoming` blocks even if count is 0
- Ordering:
  - Outgoing edges sorted by `(type, to_id)` lexicographically
  - Incoming edges sorted by `(type, from_id)` lexicographically
  - Evidence entries sorted by `(field, reason)` lexicographically
- Evidence formatting:
  - Always print `evidence (<M>):`
  - For each evidence entry print two lines as shown
- If an edge refers to a node ID that isn't present in the graph, still print the ID (don't error)

### Footer

```
Hint: Use 'cub-scout graph export --json' for the full graph.
```

---

## Determinism Requirements

- No timestamps in output
- Output must be byte-identical for the same underlying graph
- Sorting rules above are mandatory

---

## Error Behavior

If node not found, exit code 3 with a one-line message:

```
Error: target not found: <cluster>/<ns>/<kind>/<name>
```

Keep error line deterministic.

---

## Golden Examples

### Golden 1: Deployment with outgoing owns edge

**File:** `test/golden/graph-explain/testdata/deployment.golden.txt`

```
GRAPH EXPLAIN
Target: test-cluster/default/Deployment/nginx
Schema: graph.v1

Node:
  kind: Deployment
  name: nginx
  namespace: default
  api_version: apps/v1
  id: test-cluster/default/Deployment/nginx
  labels:
    app=nginx

Relationships:
  outgoing (1):
    - owns -> test-cluster/default/ReplicaSet/nginx-abc123
      evidence (1):
        - field: metadata.ownerReferences
          reason: ReplicaSet nginx-abc123 has ownerReference to Deployment nginx
  incoming (0):

Hint: Use 'cub-scout graph export --json' for the full graph.
```

### Golden 2: Pod with incoming owns edge

**File:** `test/golden/graph-explain/testdata/pod.golden.txt`

```
GRAPH EXPLAIN
Target: test-cluster/default/Pod/nginx-abc123-xyz789
Schema: graph.v1

Node:
  kind: Pod
  name: nginx-abc123-xyz789
  namespace: default
  api_version: v1
  id: test-cluster/default/Pod/nginx-abc123-xyz789
  labels:
    app=nginx

Relationships:
  outgoing (0):
  incoming (1):
    - owns <- test-cluster/default/ReplicaSet/nginx-abc123
      evidence (1):
        - field: metadata.ownerReferences
          reason: Pod nginx-abc123-xyz789 has ownerReference to ReplicaSet nginx-abc123

Hint: Use 'cub-scout graph export --json' for the full graph.
```

### Golden 3: GitOps CRD node with no edges

**File:** `test/golden/graph-explain/testdata/crd-no-edges.golden.txt`

```
GRAPH EXPLAIN
Target: test-cluster/argocd/Application/my-app
Schema: graph.v1

Node:
  kind: Application
  name: my-app
  namespace: argocd
  api_version: argoproj.io/v1alpha1
  id: test-cluster/argocd/Application/my-app
  labels:
    app.kubernetes.io/name=my-app

Relationships:
  outgoing (0):
  incoming (0):

Hint: Use 'cub-scout graph export --json' for the full graph.
```

---

## Implementation Notes

### Suggested Code Flow

1. Build graph using existing collectors (same as export)
2. Resolve target node ID from args (`cluster`, `namespace`, `kind`, `name`)
3. Find node in `g.Nodes` (map by ID)
4. Filter edges into:
   - outgoing: `edge.From == targetID`
   - incoming: `edge.To == targetID`
5. Sort outgoing/incoming as specified
6. Render using `strings.Builder`

### Suggested File Structure

```
cmd/cub-scout/graph_explain.go          # CLI command
internal/graph/explain.go               # Core explain logic
internal/graph/explain_test.go          # Unit tests
test/golden/graph-explain/              # Golden tests
  graph_explain_test.go
  testdata/
    deployment.golden.txt
    pod.golden.txt
    crd-no-edges.golden.txt
    not-found.golden.txt
```

### Core Types

```go
// Explanation holds the explain output for a resource.
type Explanation struct {
    Node     Node
    Outgoing []RelatedEdge
    Incoming []RelatedEdge
}

// RelatedEdge represents an edge in the explain output.
type RelatedEdge struct {
    Type     EdgeType
    Target   string     // node ID (to_id for outgoing, from_id for incoming)
    Evidence []Evidence
}

// Explain returns explanation for a resource in the graph.
func (g *Graph) Explain(nodeID string) (*Explanation, error)

// Render formats the explanation as deterministic text.
func (e *Explanation) Render(cluster string) string
```

---

## See Also

- [graph-contract.md](graph-contract.md) — Graph export contract
- [commands.md](commands.md) — CLI reference
