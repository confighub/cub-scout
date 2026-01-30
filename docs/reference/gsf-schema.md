# GitOps State Format (GSF) Schema

The agent outputs GSF — a JSON format representing cluster state with ownership, drift, and relations.

## Quick Start

```bash
# Output to stdout
cub-scout snapshot

# Output to file
cub-scout snapshot -o state.json

# Include resource relations (owns, selects, mounts, references)
cub-scout snapshot --relations

# Pipe to jq
cub-scout snapshot | jq '.entries[] | select(.owner.type == "flux")'

# Filter by namespace
cub-scout snapshot --namespace prod

# Filter by kind
cub-scout snapshot --kind Deployment
```

## Output Format

```json
{
  "version": "gsf/v1",
  "generatedAt": "2025-12-29T12:00:00Z",
  "cluster": "prod-east",
  "entries": [
    {
      "id": "prod-east/prod/apps/Deployment/backend",
      "cluster": "prod-east",
      "namespace": "prod",
      "kind": "Deployment",
      "name": "backend",
      "apiVersion": "apps/v1",
      "owner": {
        "type": "flux",
        "subType": "kustomization",
        "name": "apps",
        "namespace": "flux-system"
      },
      "labels": {
        "app": "backend"
      }
    }
  ],
  "relations": [],
  "summary": {
    "total": 45,
    "byKind": { "Deployment": 12, "Service": 15, "ConfigMap": 18 },
    "byOwner": { "flux": 30, "argo": 10, "unknown": 5 },
    "drifted": 0
  }
}
```

## Entry Schema

Each entry represents a Kubernetes resource with ownership metadata:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique ID: `{cluster}/{namespace}/{group}/{kind}/{name}` |
| `cluster` | string | Cluster name (from `CLUSTER_NAME` env or "default") |
| `namespace` | string | Namespace (empty for cluster-scoped) |
| `kind` | string | Resource kind |
| `name` | string | Resource name |
| `apiVersion` | string | API version |
| `owner` | object | Ownership information (see below) |
| `drift` | object | Drift information if detected |
| `labels` | object | Resource labels |

## Owner Types

| Type | Description | Detection Method |
|------|-------------|------------------|
| `flux` | Managed by Flux | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| `argo` | Managed by Argo CD | `argocd.argoproj.io/instance` label or tracking annotation |
| `helm` | Direct Helm release | `app.kubernetes.io/managed-by: Helm` label |
| `terraform` | Managed by Terraform | `app.terraform.io/*` annotations |
| `confighub` | Managed by ConfigHub | `confighub.com/UnitSlug` label |
| `crossplane` | Managed by Crossplane | `crossplane.io/*` labels or XR owner references |
| `k8s` | Kubernetes native | OwnerReferences only (no GitOps tool) |
| `unknown` | No ownership detected | Fallback |

## Owner SubTypes

| Type | SubType | Meaning |
|------|---------|---------|
| `flux` | `kustomization` | Deployed via Flux Kustomization |
| `flux` | `helmrelease` | Deployed via Flux HelmRelease |
| `argo` | `application` | Deployed via Argo CD Application |
| `helm` | `release` | Direct Helm release |
| `crossplane` | `claim` | Created from a Crossplane Claim |
| `crossplane` | `composite` | Created from a Composite Resource (XR) |
| `crossplane` | `managed-resource` | Managed resource in a Composition |

## Detection Priority

When a resource has multiple ownership markers, detection follows this order:

1. **Flux** (Kustomization, then HelmRelease)
2. **Argo CD**
3. **Helm**
4. **Terraform**
5. **ConfigHub**
6. **Crossplane**
7. **Kubernetes native** (`k8s`)
8. **Unknown** (fallback)

> **Note:** ConfigHub labels may coexist with Flux/Argo labels. In standalone mode, the GitOps deployer takes precedence. In connected mode, both `owner` and `deployer` fields are populated.

## Owner Detection Examples

### Flux (Kustomization)
```yaml
labels:
  kustomize.toolkit.fluxcd.io/name: "apps"
  kustomize.toolkit.fluxcd.io/namespace: "flux-system"
```
```json
{ "type": "flux", "subType": "kustomization", "name": "apps", "namespace": "flux-system" }
```

### Flux (HelmRelease)
```yaml
labels:
  helm.toolkit.fluxcd.io/name: "podinfo"
  helm.toolkit.fluxcd.io/namespace: "flux-system"
```
```json
{ "type": "flux", "subType": "helmrelease", "name": "podinfo", "namespace": "flux-system" }
```

### Argo CD
```yaml
labels:
  argocd.argoproj.io/instance: "guestbook"
```
```json
{ "type": "argo", "subType": "application", "name": "guestbook" }
```

### Helm (Direct)
```yaml
labels:
  app.kubernetes.io/managed-by: "Helm"
annotations:
  meta.helm.sh/release-name: "my-release"
  meta.helm.sh/release-namespace: "default"
```
```json
{ "type": "helm", "subType": "release", "name": "my-release", "namespace": "default" }
```

### ConfigHub
```yaml
labels:
  confighub.com/UnitSlug: "backend"
annotations:
  confighub.com/SpaceID: "550e8400-e29b-41d4-a716-446655440000"
```
```json
{ "type": "confighub", "name": "backend" }
```

### Crossplane (Claim)
```yaml
labels:
  crossplane.io/claim-name: "my-database"
  crossplane.io/claim-namespace: "prod"
```
```json
{ "type": "crossplane", "subType": "claim", "name": "my-database", "namespace": "prod" }
```

### Crossplane (Composite)
```yaml
labels:
  crossplane.io/composite: "my-xr-abc123"
```
```json
{ "type": "crossplane", "subType": "composite", "name": "my-xr-abc123" }
```

## Drift Schema

When drift is detected, the `drift` field contains:

```json
{
  "type": "modified",
  "summary": "replicas: 2 → 3",
  "detectedAt": "2025-12-29T12:00:00Z"
}
```

| Field | Description |
|-------|-------------|
| `type` | `modified`, `missing`, or `extra` |
| `summary` | Human-readable summary |
| `detectedAt` | When drift was detected |

## Summary Schema

```json
{
  "total": 45,
  "byKind": { "Deployment": 12, "Service": 15, "ConfigMap": 18 },
  "byOwner": { "flux": 30, "argo": 10, "unknown": 5 },
  "drifted": 2
}
```

## Scanned Resources

The snapshot command scans these resource types:

| Kind | Group | Version |
|------|-------|---------|
| Deployment | apps | v1 |
| ReplicaSet | apps | v1 |
| StatefulSet | apps | v1 |
| DaemonSet | apps | v1 |
| Pod | core | v1 |
| Service | core | v1 |
| ConfigMap | core | v1 |
| Secret | core | v1 |
| Ingress | networking.k8s.io | v1 |
| GitRepository | source.toolkit.fluxcd.io | v1 |
| Kustomization | kustomize.toolkit.fluxcd.io | v1 |
| HelmRelease | helm.toolkit.fluxcd.io | v2 |
| Application | argoproj.io | v1alpha1 |

## Third-Party Integration

GSF is designed for tool integration. Example uses:

```bash
# Count resources by owner
cub-scout snapshot | jq '.summary.byOwner'

# List all Flux-managed deployments
cub-scout snapshot | jq '.entries[] | select(.owner.type == "flux" and .kind == "Deployment") | .name'

# Find resources without ownership
cub-scout snapshot | jq '.entries[] | select(.owner == null) | {kind, name, namespace}'

# Export for external dashboard
cub-scout snapshot -o /var/lib/dashboard/cluster-state.json
```

### Programmatic Access (Go)

```go
import "github.com/confighub/cub-scout/pkg/agent"

// Detect ownership of any unstructured resource
ownership := agent.DetectOwnership(resource)
fmt.Printf("Owner: %s (%s)\n", ownership.Type, ownership.SubType)
```

## Relation Types

Relations describe dependencies between resources. Use `--relations` flag to include them:

```bash
cub-scout snapshot --relations
```

| Type | Description | Example |
|------|-------------|---------|
| `owns` | K8s OwnerReference | ReplicaSet → Pod |
| `selects` | Label selector match | Service → Pod |
| `mounts` | Volume reference | Pod → ConfigMap |
| `references` | envFrom reference | Pod → Secret |

### Relation Schema

```json
{
  "relations": [
    {
      "from": "default/apps/ReplicaSet/nginx-abc123",
      "to": "default//Pod/nginx-abc123-xyz",
      "type": "owns"
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `from` | string | Source resource ID |
| `to` | string | Target resource ID |
| `type` | string | Relation type (owns, selects, mounts, references) |

### Relation Examples

```bash
# List all ownership relations
cub-scout snapshot --relations | jq '.relations[] | select(.type == "owns")'

# Find what a Service selects
cub-scout snapshot --relations | jq '.relations[] | select(.from | contains("Service/nginx"))'

# Find what mounts a specific ConfigMap
cub-scout snapshot --relations | jq '.relations[] | select(.to | contains("ConfigMap/app-config"))'
```

## Extended Format (Connected Mode)

When connected to ConfigHub API, entries include additional fields:

```json
{
  "id": "prod-east/default/apps/Deployment/backend",
  "confighub": {
    "org": "acme",
    "space": "payments-prod",
    "unit": "backend",
    "revision": 42
  },
  "owner": {
    "type": "confighub"
  },
  "deployer": {
    "type": "flux",
    "subType": "kustomization",
    "name": "payments"
  },
  "ccves": ["CCVE-DRIFT-002"]
}
```

Extended fields:
- `confighub` — Full ConfigHub hierarchy (Org → Space → Unit)
- `deployer` — Separate from `owner` when ConfigHub manages via Flux/Argo
- `ccves` — Active Config CVEs affecting this entry

---

## See Also

- [Command Reference](commands.md)
- [Query Syntax](query-syntax.md)
- [Architecture](../ARCHITECTURE.md)
