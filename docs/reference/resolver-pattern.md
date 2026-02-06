# Lineage Resolution in cub-scout

This document explains the **resolver pattern** for explaining generated resources in cub-scout.

---

## When Is a Resolver Needed?

cub-scout has two levels of resource analysis:

### 1. Ownership Detection (Simple)

**Location:** `pkg/agent/ownership.go`

Ownership detection answers: *"Who manages this resource?"*

It examines a **single resource** and checks labels/annotations:
- Flux: `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*`
- ArgoCD: `argocd.argoproj.io/instance`
- Helm: `app.kubernetes.io/managed-by: Helm`
- Crossplane: `crossplane.io/*` labels or ownerRefs to `*.crossplane.io`

This is sufficient for most GitOps tools where the relationship is direct.

### 2. Lineage Resolution (Complex)

**Location:** `pkg/agent/crossplane_lineage.go`

A resolver is needed when:
- Resources are **generated** by a composition system
- The ownership chain has **multiple hops** (Managed → XR → Claim)
- Relationships must be **resolved across objects**, not just read from one

**Example:** Crossplane creates a chain:
```
Claim (user intent)
  └── Composite Resource (XR)
        └── Managed Resource (cloud resource)
```

To explain this chain, cub-scout must:
1. Find the Managed Resource's XR via labels or ownerRefs
2. Find the XR's Claim via labels
3. Handle missing objects gracefully (partial lineage)

---

## Resolver Responsibilities

A resolver must:

### 1. Consume Only Kubernetes Metadata

```go
func ResolveCrossplaneLineage(
    target *unstructured.Unstructured,    // The resource to explain
    objects []*unstructured.Unstructured, // Available objects to search
) (*CrossplaneLineage, bool)
```

Resolvers receive:
- The target object to explain
- A set of related objects (already fetched)

Resolvers **never** call external APIs or the Kubernetes API directly.

### 2. Return Lineage + Evidence

```go
type CrossplaneLineage struct {
    Managed   CrossplaneLineageNode  `json:"managed"`
    Composite CrossplaneLineageNode  `json:"composite"`
    Claim     *CrossplaneLineageNode `json:"claim,omitempty"`
    Evidence  []string               `json:"evidence,omitempty"`
}

type CrossplaneLineageNode struct {
    Ref     ResourceRef `json:"ref"`
    Present bool        `json:"present"` // Was the object found?
}
```

Key properties:
- **Evidence** explains which signals were used (labels, ownerRefs)
- **Present** indicates if the referenced object was found
- **Partial lineage** is valid (missing objects are first-class)

### 3. Support Partial Lineage

When a referenced object isn't in the object set:

```go
if xrObj != nil {
    xrRef = resourceRefFromUnstructured(xrObj)
    xrPresent = true
} else {
    // Best-effort: preserve the name, mark as not present
    xrRef = ResourceRef{Kind: "CompositeResource", Name: compName}
    xrPresent = false
}
```

Partial lineage is explicitly surfaced in output:
```
xr:       PostgreSQLInstance/my-db (partial lineage)
```

### 4. Avoid Guessing

Resolvers use only explicit signals:

| Signal | Usage |
|--------|-------|
| `crossplane.io/composite` label | Primary XR identification |
| `crossplane.io/claim-name` label | Claim identification |
| OwnerReferences to `*.crossplane.io` | Fallback XR identification |

If no signal is found, the resolver returns `false`:
```go
if xrRef.Name == "" {
    lineage.Evidence = append(lineage.Evidence, "xr:unresolved")
    return lineage, true // Valid but incomplete
}
```

---

## How Resolver Output Is Consumed

### In `trace`

```go
// trace.go:1071
if lineage, ok := agent.ResolveCrossplaneLineage(result.Objects[0], result.Objects); ok {
    fmt.Print(renderCrossplaneLineageHuman(lineage))
}
```

The trace command shows lineage as enrichment after ownership:
```
Crossplane lineage:
  managed:   RDSInstance/my-db-xyz
  xr:        PostgreSQLInstance/my-db
  claim:     PostgreSQLClaim/my-db
  evidence:  label:crossplane.io/composite, label:crossplane.io/claim-*
```

### In `tree composition`

```go
// tree_composition.go:126
lineage, ok := agent.ResolveCrossplaneLineage(obj, objs)
if !ok || lineage == nil {
    continue
}
// Group by XR
byXR[xrKey].Managed = append(byXR[xrKey].Managed, lineage.Managed)
```

The tree command groups resources by their XR:
```
PostgreSQLInstance/my-db
├── RDSInstance/my-db-xyz
├── SecurityGroup/my-db-sg
└── SubnetGroup/my-db-subnets
```

### In Attribution Graphs

```go
// attribution_crossplane.go
func BuildAttributionGraphFromCrossplaneLineage(lineage *CrossplaneLineage, bundleID string) *AttributionGraph
```

Lineage is converted to the `attribution-graph.v1` schema for bundle storage:
- Nodes: `mr`, `xr`, `claim`
- Edges: `owns` with evidence types

---

## Reference Implementation

The Crossplane resolver (`pkg/agent/crossplane_lineage.go`) is the reference implementation.

### Code Structure

```
pkg/agent/
├── ownership.go              # Simple ownership detection
├── crossplane_lineage.go     # Lineage resolver
├── attribution_crossplane.go # Attribution graph conversion
```

### Key Functions

| Function | Purpose |
|----------|---------|
| `ResolveCrossplaneLineage()` | Build lineage chain for a Crossplane resource |
| `BuildAttributionGraphFromCrossplaneLineage()` | Convert lineage to attribution graph |
| `AttributionGraphForTarget()` | Combined entry point |

### Tests

```
cmd/cub-scout/trace_crossplane_test.go  # Integration tests
pkg/agent/crossplane_lineage_test.go    # Unit tests (if present)
```

---

## Adding a New Resolver

To support a new composition platform (e.g., kro):

### 1. Define the Lineage Structure

```go
type KroLineage struct {
    Instance  KroLineageNode  `json:"instance"`
    Blueprint KroLineageNode  `json:"blueprint"`
    Evidence  []string        `json:"evidence,omitempty"`
}
```

### 2. Implement the Resolver

```go
func ResolveKroLineage(
    target *unstructured.Unstructured,
    objects []*unstructured.Unstructured,
) (*KroLineage, bool) {
    // 1. Check if this is kro-managed
    // 2. Find related objects via labels/ownerRefs
    // 3. Return lineage with evidence
    // 4. Handle partial lineage
}
```

### 3. Integrate with Commands

- `trace.go`: Add rendering for kro lineage
- `tree_composition.go`: Add kro grouping (or new `tree kro` view)
- `attribution_*.go`: Add attribution graph conversion

### 4. Add Tests

- Unit tests with fixture objects
- Golden tests for output format
- Partial lineage test cases

---

## Principles

1. **Resolvers are read-only** — They never modify objects or call APIs
2. **Partial is valid** — Missing objects produce partial lineage, not errors
3. **Evidence is required** — Every relationship must cite its source
4. **No guessing** — If a signal isn't present, don't infer it
5. **Deterministic** — Same inputs always produce same outputs
