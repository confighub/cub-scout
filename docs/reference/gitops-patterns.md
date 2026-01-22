# GitOps Patterns Reference

How common GitOps patterns (App-of-Apps, ApplicationSet, Flux Tenancy, Mono-repo) are detected and mapped.

## Quick Reference

| Pattern | Deployer | TUI Detects | GUI Adds |
|---------|----------|-------------|----------|
| App-of-Apps | Argo CD | Child Applications | Parent→child relationships |
| ApplicationSet | Argo CD | Generated Applications | Generator config, cluster list |
| Flux Tenancy | Flux CD | Tenant Kustomizations | Tenant isolation rules |
| Mono-repo | Both | Per-cluster deployers | Cross-cluster correlation |
| Helm Umbrella | Helm/Flux | HelmRelease, sub-charts | Chart dependency tree |

---

## Pattern Detection Cheat Sheet

| You See In Cluster | Pattern | Typical Path |
|-------------------|---------|--------------|
| ApplicationSet + multiple Applications | Argo ApplicationSet | `applicationsets/*.yaml` |
| Application managing other Applications | Argo App-of-Apps | `apps/` root |
| Kustomization with `tenants/` path | Flux Tenancy | `tenants/{team}/` |
| Kustomization with `apps/{env}` path | Flux Mono-repo | `apps/{staging,prod}/` |
| HelmRelease with dependencies | Helm Umbrella | Chart with subcharts |

---

## Pattern 1: Argo CD App-of-Apps

### Typical Repo Structure

```
├── apps/                          # App-of-Apps parent
│   ├── Chart.yaml                 # or kustomization.yaml
│   └── templates/
│       ├── payment-api.yaml       # Application pointing to apps/payment-api
│       ├── order-service.yaml     # Application pointing to apps/order-service
│       └── redis.yaml             # Application pointing to apps/redis
│
├── apps/payment-api/              # Individual app
│   ├── base/
│   │   ├── deployment.yaml
│   │   └── kustomization.yaml
│   └── overlays/
│       ├── dev/
│       ├── staging/
│       └── prod/
```

### What cub-scout Detects

```
Namespace: argocd
├── Application/root-app (parent - manages child apps)
├── Application/payment-api (spec.source.path: apps/payment-api/overlays/prod)
├── Application/order-service (spec.source.path: apps/order-service/overlays/prod)
└── Application/redis

Namespace: payment-prod
└── Deployment/payment-api (owned by Application/payment-api)
```

**Detects:**
- Each Application and its source path
- Which namespace each deploys to
- Sync status (Synced/OutOfSync)

**Does not detect:**
- That root-app is the parent (needs Git)
- The full overlay structure (dev/staging/prod)
- Which overlays aren't deployed

### Detection Commands

```bash
# Detect from running cluster
cub-scout import -n payment-prod
# Detects: Application/payment-api, Deployment/payment-api
# Infers: variant=prod (from overlays/prod path)

# Trace ownership
cub-scout trace deploy/payment-api -n payment-prod
# Shows: Source → Application → Deployment chain
```

---

## Pattern 2: Argo CD ApplicationSet

### Typical Repo Structure

```
├── applicationsets/
│   └── payment-api.yaml           # ApplicationSet with generators
│
├── apps/payment-api/
│   ├── base/
│   └── overlays/
│       ├── dev/
│       ├── staging/
│       └── prod/
│
└── clusters/                       # Cluster configs for generators
    ├── dev.yaml
    ├── staging.yaml
    └── prod.yaml
```

### ApplicationSet Example

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: payment-api
  namespace: argocd
spec:
  generators:
    - clusters:
        selector:
          matchLabels:
            env: prod
  template:
    metadata:
      name: 'payment-api-{{name}}'
    spec:
      source:
        repoURL: https://github.com/acme/gitops
        path: 'apps/payment-api/overlays/{{metadata.labels.env}}'
      destination:
        server: '{{server}}'
        namespace: payment
```

### What cub-scout Detects

```
Namespace: argocd
├── ApplicationSet/payment-api (the generator)
├── Application/payment-api-prod-east (generated)
├── Application/payment-api-prod-west (generated)
└── Application/payment-api-staging (generated)
```

**Detects:**
- Each generated Application exists
- Its source path and destination
- Current sync status

**Does not detect:**
- The generator pattern used (needs Git)
- How to regenerate if cluster list changes
- Which clusters COULD be targets but aren't

---

## Pattern 3: Flux Multi-Tenancy

### Typical Repo Structure

```
├── clusters/
│   ├── production/
│   │   ├── flux-system/           # Flux controllers
│   │   └── tenants.yaml           # Kustomization for tenants
│   └── staging/
│
├── infrastructure/
│   ├── controllers/               # Shared infrastructure
│   └── configs/
│
└── tenants/
    ├── team-a/
    │   ├── base/
    │   ├── staging/
    │   └── production/
    │       ├── kustomization.yaml
    │       ├── payment-api/
    │       └── order-service/
    │
    └── team-b/
```

### What cub-scout Detects

```
Namespace: flux-system
├── GitRepository/flux-system
├── Kustomization/infrastructure
└── Kustomization/tenants

Namespace: team-a-prod
├── Kustomization/team-a-apps (spec.path: tenants/team-a/production)
├── Deployment/payment-api (owned by Kustomization)
└── Deployment/order-service
```

**Detects:**
- Kustomization paths (tenants/team-a/production)
- Which namespace each tenant uses
- Resource ownership

**Does not detect:**
- Tenant isolation boundaries (needs Git)
- Cross-tenant dependencies
- Which tenants exist but aren't deployed here

---

## Pattern 4: Flux Mono-Repo

### Typical Repo Structure

```
├── clusters/
│   ├── staging/
│   │   ├── apps.yaml              # Kustomization: path: ./apps/staging
│   │   └── infrastructure.yaml
│   └── production/
│       ├── apps.yaml              # Kustomization: path: ./apps/production
│       └── infrastructure.yaml
│
├── apps/
│   ├── base/
│   │   └── podinfo/
│   ├── staging/
│   │   ├── kustomization.yaml     # resources: ../base/podinfo
│   │   └── podinfo-values.yaml
│   └── production/
│       ├── kustomization.yaml
│       └── podinfo-values.yaml
│
└── infrastructure/
    ├── controllers/
    └── configs/
```

### What cub-scout Detects

```
Namespace: flux-system
├── GitRepository/flux-system
├── Kustomization/apps (spec.path: ./apps/production)
└── Kustomization/infrastructure

Namespace: podinfo
└── Deployment/podinfo (owned by Kustomization/apps)
```

**Detects:**
- Kustomization path (./apps/production → variant=production)
- Deployed resources and ownership
- Current revision

---

## Variant Inference from Paths

cub-scout infers variant from the deployer's source path:

| Deployer Path | Inferred Variant |
|---------------|------------------|
| `./staging` | `staging` |
| `./production` | `prod` |
| `apps/prod/payment` | `prod` |
| `tenants/checkout/overlays/dev` | `dev` |
| `clusters/us-east/apps` | `us-east` |

The path is stored in the cluster by the deployer (`Kustomization.spec.path` or `Application.spec.source.path`).

---

## Query Examples

Query across patterns once detected:

```bash
# All prod variants, any pattern
cub-scout map list -q "variant=prod"

# All Argo-managed resources
cub-scout map list -q "owner=ArgoCD"

# All Flux-managed resources
cub-scout map list -q "owner=Flux"

# Resources from overlays/prod paths
cub-scout map list -q "path=*overlays/prod*"
```

---

## See Also

- [Live Cluster Inference](../concepts/live-cluster-inference.md) — How detection works without Git
- [TUI vs GUI](../concepts/tui-vs-gui.md) — What TUI detects vs GUI adds
- [Ownership Detection](../howto/ownership-detection.md) — How to use ownership detection
