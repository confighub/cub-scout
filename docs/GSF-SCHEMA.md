# GitOps State Format (GSF) Schema

The agent outputs GSF — a JSON format representing cluster state with ownership, drift, and relations.

## Output Format

```json
{
  "version": "gsf/v1",
  "generatedAt": "2025-12-29T12:00:00Z",
  "cluster": "kind-atk",
  "entries": [
    {
      "id": "kind-atk/prod/apps/v1/Deployment/backend",
      "cluster": "kind-atk",
      "namespace": "prod",
      "kind": "Deployment",
      "name": "backend",
      "apiVersion": "apps/v1",
      "owner": {
        "type": "ConfigHub",
        "resource": { "kind": "Unit", "name": "backend", "namespace": "prod" },
        "details": {
          "spaceId": "550e8400-e29b-41d4-a716-446655440000",
          "revision": "42"
        }
      },
      "drift": null
    }
  ],
  "relations": [
    {
      "from": "kind-atk/prod/apps/v1/Deployment/backend",
      "to": "kind-atk/prod/v1/Service/backend",
      "type": "selects"
    }
  ],
  "summary": {
    "total": 45,
    "byKind": { "Deployment": 12, "Service": 15, "ConfigMap": 18 },
    "byOwner": { "flux": 30, "argo": 10, "unknown": 5 },
    "drifted": 2
  }
}
```

## Entry Schema

Each entry represents a Kubernetes resource with ownership metadata:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique ID: `{cluster}/{namespace}/{group}/{version}/{kind}/{name}` |
| `cluster` | string | Cluster name |
| `namespace` | string | Namespace (empty for cluster-scoped) |
| `kind` | string | Resource kind |
| `name` | string | Resource name |
| `apiVersion` | string | API version |
| `owner` | object | Ownership information (see below) |
| `drift` | object | Drift information if detected |
| `labels` | object | Resource labels |
| `annotations` | object | Resource annotations |

## Owner Types

| Type | Description | Detection Method |
|------|-------------|------------------|
| `ConfigHub` | Managed by ConfigHub | `confighub.com/UnitSlug` label |
| `Flux` | Managed by Flux | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| `ArgoCD` | Managed by Argo CD | `argocd.argoproj.io/instance` label or tracking annotation |
| `Helm` | Direct Helm release | `app.kubernetes.io/managed-by: Helm` label |
| `Terraform` | Managed by Terraform | `app.terraform.io/*` annotations |
| `Native` | Plain Kubernetes | OwnerReferences only |

## Owner Detection

Ownership is detected by examining labels and annotations:

### ConfigHub
```yaml
labels:
  confighub.com/UnitSlug: "backend"
annotations:
  confighub.com/UnitSlug: "backend"
  confighub.com/SpaceID: "550e8400-e29b-41d4-a716-446655440000"
  confighub.com/RevisionNum: "42"
```

### Flux (Kustomization)
```yaml
labels:
  kustomize.toolkit.fluxcd.io/name: "podinfo"
  kustomize.toolkit.fluxcd.io/namespace: "flux-system"
```

### Flux (HelmRelease)
```yaml
labels:
  helm.toolkit.fluxcd.io/name: "podinfo"
  helm.toolkit.fluxcd.io/namespace: "flux-system"
```

### Argo CD
```yaml
labels:
  argocd.argoproj.io/instance: "guestbook"
# or
annotations:
  argocd.argoproj.io/tracking-id: "..."
```

### Helm (Direct)
```yaml
labels:
  app.kubernetes.io/managed-by: "Helm"
annotations:
  meta.helm.sh/release-name: "my-release"
  meta.helm.sh/release-namespace: "default"
```

## Detection Priority

When an entry has multiple ownership markers, detection follows this priority:

1. **ConfigHub** - Checked first (may coexist with deployers like Flux)
2. **Flux Kustomization**
3. **Flux HelmRelease**
4. **Argo CD**
5. **Helm**
6. **Terraform**
7. **Native** (fallback)

This allows ConfigHub to track entries deployed via Flux or Argo while still showing the deployment mechanism.

## Drift Schema

When drift is detected, the `drift` field contains:

```json
{
  "type": "modified",
  "changes": [
    { "path": "spec.replicas", "from": 2, "to": 3 }
  ],
  "summary": "replicas: 2 → 3",
  "detectedAt": "2025-12-29T12:00:00Z"
}
```

| Field | Description |
|-------|-------------|
| `type` | `modified`, `missing`, or `extra` |
| `changes` | List of field-level changes |
| `summary` | Human-readable summary |
| `detectedAt` | When drift was detected |

## Relation Types

| Type | Description | Example |
|------|-------------|---------|
| `owns` | K8s OwnerReference | ReplicaSet → Pod |
| `selects` | Label selector match | Service → Pod |
| `mounts` | Volume reference | Pod → ConfigMap |
| `references` | envFrom reference | Pod → Secret |

## Flux CRD Entries

Flux custom resources are included:

| Kind | Owner | Details |
|------|-------|---------|
| `GitRepository` | Flux | `url` |
| `HelmRepository` | Flux | `url` |
| `Kustomization` | Flux | `sourceRef` |
| `HelmRelease` | Flux | `chart` |

## Argo CD Application Entries

```json
{
  "id": "kind-atk/argocd/argoproj.io/v1alpha1/Application/guestbook",
  "kind": "Application",
  "name": "guestbook",
  "owner": {
    "type": "ArgoCD",
    "details": {
      "repoURL": "https://github.com/argoproj/argocd-example-apps",
      "destinationNamespace": "default",
      "syncStatus": "Synced",
      "healthStatus": "Healthy"
    }
  }
}
```

## Extended Format (Connected Mode)

When connected to ConfigHub API, entries include additional fields:

```json
{
  "id": "prod-east/default/apps/v1/Deployment/backend",
  "confighub": {
    "org": "payments",
    "space": "payments-prod",
    "unit": "backend",
    "revision": 42
  },
  "owner": {
    "type": "ConfigHub"
  },
  "deployer": {
    "type": "Flux",
    "resource": { "kind": "Kustomization", "name": "payments" }
  },
  "drift": { "type": "modified", "summary": "replicas: 2 → 3" },
  "ccves": ["CCVE-DRIFT-002"]
}
```

Extended fields:
- `confighub` — Full ConfigHub hierarchy (Org → Space → Unit)
- `deployer` — Separate from `owner` when ConfigHub manages via Flux/Argo
- `ccves` — Active Config CVEs affecting this entry
