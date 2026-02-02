# Graph Contract Reference (v0.6)

This document defines the stable graph export contract for cub-scout v0.6+.

> **v0.6 contract surface.** This does not modify any v0.5 contracts.

---

## Schema Version

```
schema_version: "graph.v1"
```

Changes to the schema require a new version. The schema version is included
in every export to enable consumers to verify compatibility.

---

## Export Format

```bash
cub-scout graph export --json
```

### Top-Level Fields

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Always `"graph.v1"` for this contract |
| `generated_at` | string | ISO 8601 timestamp (UTC) |
| `cluster` | string | Cluster name (from kubectl context) |
| `nodes` | array | Resource nodes |
| `edges` | array | Relationship edges |

### Node Schema

Each node represents a Kubernetes resource.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Canonical ID: `<cluster>/<namespace>/<kind>/<name>` |
| `cluster` | string | yes | Cluster name |
| `namespace` | string | no | Namespace (omitted for cluster-scoped) |
| `kind` | string | yes | Kubernetes resource kind |
| `name` | string | yes | Resource name |
| `api_version` | string | yes | Kubernetes API version (e.g., `apps/v1`) |
| `labels` | object | no | Resource labels |

### Edge Schema

Each edge represents a relationship between two resources.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | yes | Source node ID |
| `to` | string | yes | Target node ID |
| `type` | string | yes | Relationship type |
| `evidence` | object | yes | How this relationship was detected |

### Edge Types

| Type | Description |
|------|-------------|
| `owns` | Source owns target (via ownerReferences) |
| `selects` | Source selects target (via label selector) |
| `managed_by` | Source is managed by target |
| `references` | Source references target |

### Evidence Schema

Every edge includes evidence explaining how it was detected.

| Field | Type | Description |
|-------|------|-------------|
| `field` | string | Field path that established the relationship |
| `reason` | string | Human-readable explanation |

---

## Deterministic Output

Graph export is **deterministic**: same input produces byte-identical output.

### Ordering Rules

1. **Nodes** are sorted by `id` (lexicographic)
2. **Edges** are sorted by `(from, to, type)` (lexicographic tuple)
3. **Labels** within nodes are sorted by key (Go's `json.Marshal` behavior)

### Timestamp Normalization

For testing, set `CUB_SCOUT_TEST_TIME` environment variable to override
`generated_at` with a fixed timestamp:

```bash
CUB_SCOUT_TEST_TIME=2026-01-01T00:00:00Z cub-scout graph export --json
```

---

## Example Output

```json
{
  "schema_version": "graph.v1",
  "generated_at": "2026-01-01T00:00:00Z",
  "cluster": "prod-east",
  "nodes": [
    {
      "id": "prod-east/default/Deployment/nginx",
      "cluster": "prod-east",
      "namespace": "default",
      "kind": "Deployment",
      "name": "nginx",
      "api_version": "apps/v1",
      "labels": {
        "app": "nginx"
      }
    },
    {
      "id": "prod-east/default/ReplicaSet/nginx-abc123",
      "cluster": "prod-east",
      "namespace": "default",
      "kind": "ReplicaSet",
      "name": "nginx-abc123",
      "api_version": "apps/v1"
    }
  ],
  "edges": [
    {
      "from": "prod-east/default/Deployment/nginx",
      "to": "prod-east/default/ReplicaSet/nginx-abc123",
      "type": "owns",
      "evidence": {
        "field": "metadata.ownerReferences[nginx]",
        "reason": "ReplicaSet nginx-abc123 has ownerReference to Deployment nginx"
      }
    }
  ]
}
```

---

## Versioning Policy

- **graph.v1** is the current stable schema
- Backwards-compatible additions (new optional fields) stay in v1
- Breaking changes require a new schema version (graph.v2)
- Consumers should check `schema_version` before parsing

---

## Collection Scope (v0.6)

The v0.6 collector ingests:

- Deployments
- ReplicaSets
- Pods

With ownership edges via `metadata.ownerReferences`.

Future versions may add:
- GitOps CRDs (Flux, ArgoCD) when present
- Services, ConfigMaps, Secrets
- Additional relationship types

---

## See Also

- [commands.md](commands.md) — CLI reference
- [cli-contract.md](cli-contract.md) — v0.5 CLI contracts (unmodified)
