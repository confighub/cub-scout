# Import Patterns: GitOps Architectures → ConfigHub

**Archive status:** Historical reference only (not canonical for current releases).

Use these maintained docs first:

- [GitOps Patterns Reference](../reference/gitops-patterns.md)
- [View Hierarchies with tree](../howto/tree-hierarchies.md)
- [Pipeline Source Resolution](../reference/pipeline-source-resolution.md)
- [Import Docs Crosswalk](../reference/import-docs-crosswalk.md)

**Pattern reference** - How App-of-Apps, ApplicationSet, Flux Tenancy, and Mono-repo patterns map to Hub -> App Space -> Unit.

**Historical prerequisites:** Read [IMPORT-FROM-SOURCES.md](IMPORT-FROM-SOURCES.md) for LIVE vs GIT capabilities.

---

## Quick Reference

| Pattern | Deployer | TUI Detects | GUI Adds |
|---------|----------|-------------|----------|
| App-of-Apps | Argo CD | Child Applications | Parent→child relationships |
| ApplicationSet | Argo CD | Generated Applications | Generator config, cluster list |
| Flux Tenancy | Flux CD | Tenant Kustomizations | Tenant isolation rules |
| Mono-repo | Both | Per-cluster deployers | Cross-cluster correlation |
| Helm Umbrella | Helm/Flux | HelmRelease, sub-charts | Chart dependency tree |

---

## The Mapping Rule

| What in Git/Cluster | Maps To in ConfigHub |
|---------------------|----------------------|
| Git repo URL | **Source** (with pattern metadata) |
| `base/` folders | **Base Unit** in Hub Catalog |
| Each deployed Application/Kustomization | **Unit** in App Space |
| Overlays/variants | **Labels** on Unit (`variant=prod`) |
| Tenant folders | **App Space** per tenant |
| Orchestration parents (App-of-Apps root) | **Nothing** — deployer mechanism, not config |

---

## Pattern 1: Argo CD App-of-Apps

### Repo Structure

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
│
└── apps/order-service/
    └── ...
```

### What TUI Detects from LIVE

```
Namespace: argocd
├── Application/root-app (parent - manages child apps)
├── Application/payment-api (spec.source.path: apps/payment-api/overlays/prod)
├── Application/order-service (spec.source.path: apps/order-service/overlays/prod)
└── Application/redis

Namespace: payment-prod
└── Deployment/payment-api (owned by Application/payment-api)
```

**TUI knows:**
- Each Application and its source path
- Which namespace each deploys to
- Sync status (Synced/OutOfSync)

**TUI doesn't know:**
- That root-app is the parent
- The full overlay structure (dev/staging/prod)
- Which overlays aren't deployed

### ConfigHub Mapping

```
Org: acme-corp
└─ Platform Hub: platform-team
   │
   ├─ Hub Catalog:
   │  ├─ payment-api (from apps/payment-api/base)
   │  ├─ order-service (from apps/order-service/base)
   │  └─ redis (from apps/redis/base)
   │
   └─ App Space: checkout-team (deployer: ArgoCD)
      │
      ├─ Unit: payment-api-prod
      │  ├─ Labels: app=payment-api, variant=prod
      │  ├─ Source: apps/payment-api/overlays/prod
      │  └─ Target: prod-cluster
      │
      ├─ Unit: payment-api-staging
      │  ├─ Labels: app=payment-api, variant=staging
      │  ├─ Source: apps/payment-api/overlays/staging
      │  └─ Target: staging-cluster
      │
      └─ Unit: order-service-prod
         ├─ Labels: app=order-service, variant=prod
         └─ ...
```

### Detection Commands

```bash
# TUI: Detect from running cluster
./cub-scout import -n payment-prod
# Detects: Application/payment-api, Deployment/payment-api
# Infers: variant=prod (from overlays/prod path)

# TUI: Trace ownership
cub-scout trace --app payment-api
# Shows: Source → Application → Deployment chain
```

---

## Pattern 2: Argo CD ApplicationSet

### Repo Structure

```
├── applicationsets/
│   └── payment-api.yaml           # ApplicationSet with generators
│
├── apps/payment-api/
│   ├── base/
│   │   ├── deployment.yaml
│   │   └── kustomization.yaml
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

### What TUI Detects from LIVE

```
Namespace: argocd
├── ApplicationSet/payment-api (the generator)
├── Application/payment-api-prod-east (generated)
├── Application/payment-api-prod-west (generated)
└── Application/payment-api-staging (generated)
```

**TUI knows:**
- Each generated Application exists
- Its source path and destination
- Current sync status

**TUI doesn't know:**
- The generator pattern used
- How to regenerate if cluster list changes
- Which clusters COULD be targets but aren't

### ConfigHub Mapping

```
Org: acme-corp
└─ Platform Hub: platform-team
   │
   └─ App Space: payments-team (deployer: ArgoCD)
      │
      ├─ Unit: payment-api-prod-east
      │  ├─ Labels: app=payment-api, variant=prod, region=us-east
      │  ├─ Source: apps/payment-api/overlays/prod
      │  └─ Target: prod-east-cluster
      │
      ├─ Unit: payment-api-prod-west
      │  ├─ Labels: app=payment-api, variant=prod, region=us-west
      │  ├─ Source: apps/payment-api/overlays/prod
      │  └─ Target: prod-west-cluster
      │
      └─ Unit: payment-api-staging
         ├─ Labels: app=payment-api, variant=staging
         └─ Target: staging-cluster
```

**Key insight:** Each generated Application becomes a Unit. The ApplicationSet generator is a "DRY" template — ConfigHub stores the "WET" rendered result.

---

## Pattern 3: Flux Multi-Tenancy

### Repo Structure

```
├── clusters/
│   ├── production/
│   │   ├── flux-system/           # Flux controllers
│   │   └── tenants.yaml           # Kustomization for tenants
│   └── staging/
│       └── ...
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
        └── ...
```

### What TUI Detects from LIVE

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

**TUI knows:**
- Kustomization paths (tenants/team-a/production)
- Which namespace each tenant uses
- Resource ownership

**TUI doesn't know:**
- Tenant isolation boundaries
- Cross-tenant dependencies
- Which tenants exist but aren't deployed here

### ConfigHub Mapping

```
Org: acme-corp
└─ Platform Hub: platform-team
   │
   ├─ Hub Catalog (infrastructure):
   │  ├─ nginx-ingress
   │  ├─ cert-manager
   │  └─ external-secrets
   │
   ├─ App Space: team-a (deployer: Flux)
   │  ├─ Unit: payment-api-prod
   │  │  ├─ Labels: app=payment-api, variant=prod, team=team-a
   │  │  └─ Source: tenants/team-a/production/payment-api
   │  └─ Unit: order-service-prod
   │     └─ ...
   │
   └─ App Space: team-b (deployer: Flux)
      └─ Unit: inventory-api-prod
         └─ ...
```

**Key insight:** Each tenant → App Space. The tenant boundary is the deployer/team boundary.

---

## Pattern 4: Flux Mono-Repo

### Repo Structure

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

### What TUI Detects from LIVE

```
Namespace: flux-system
├── GitRepository/flux-system
├── Kustomization/apps (spec.path: ./apps/production)
└── Kustomization/infrastructure

Namespace: podinfo
└── Deployment/podinfo (owned by Kustomization/apps)
```

**TUI knows:**
- Kustomization path (./apps/production → variant=production)
- Deployed resources and ownership
- Current revision

### ConfigHub Mapping

```
Org: acme-corp
└─ Platform Hub: platform-team
   │
   ├─ Hub Catalog:
   │  └─ podinfo (from apps/base/podinfo)
   │
   └─ App Space: apps-team (deployer: Flux)
      │
      ├─ Unit: podinfo-staging
      │  ├─ Labels: app=podinfo, variant=staging
      │  ├─ Source: apps/staging
      │  ├─ Upstream: Hub/podinfo (tracks base)
      │  └─ Target: staging-cluster
      │
      └─ Unit: podinfo-prod
         ├─ Labels: app=podinfo, variant=prod
         ├─ Source: apps/production
         ├─ Upstream: Hub/podinfo
         └─ Target: prod-cluster
```

---

## Pattern Detection Cheat Sheet

| You See In Cluster | Pattern | ConfigHub Mapping |
|-------------------|---------|-------------------|
| ApplicationSet + multiple Applications | Argo ApplicationSet | Each generated App → Unit |
| Application managing other Applications | Argo App-of-Apps | Parent tracks children |
| Kustomization with `tenants/` path | Flux Tenancy | Tenant → App Space |
| Kustomization with `apps/{env}` path | Flux Mono-repo | Path → variant label |
| HelmRelease with dependencies | Helm Umbrella | Chart → Hub catalog |

---

## Query Examples

Once imported, query across patterns:

```bash
# All prod variants, any pattern
cub query "Labels['variant'] = 'prod'"

# All units from a specific team
cub query "Labels['team'] = 'payments'"

# All Argo-managed units
cub unit list --where "deployer = 'argocd'"

# All units from a specific source path pattern
cub query "source_path LIKE '%/overlays/prod%'"
```

---

## GUI: Visual Pattern Enhancement

### What TUI Creates → What GUI Shows

TUI detects patterns from LIVE and creates Units. GUI then provides visual enhancement:

```
┌─ GUI: Pattern Detection Results ─────────────────────────────────────┐
│                                                                       │
│  Detected: Argo App-of-Apps                                          │
│  Source: github.com/acme/gitops (connected)                          │
│                                                                       │
│  ┌─ Root Application (orchestration only) ────────────────────────┐  │
│  │  apps/root-app → NOT imported (Argo's mechanism)               │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                       │
│  ┌─ Child Applications (imported as Units) ───────────────────────┐  │
│  │                                                                 │  │
│  │  payment-api         order-service       redis                 │  │
│  │  ├─ prod ✓ Synced    ├─ prod ✓ Synced    └─ prod ✓ Synced     │  │
│  │  └─ staging ○        └─ staging ○                              │  │
│  │     (in Git, not     (in Git, not                              │  │
│  │      deployed here)   deployed here)                           │  │
│  │                                                                 │  │
│  └─────────────────────────────────────────────────────────────────┘  │
│                                                                       │
│  📊 Full variant matrix: 2 deployed here, 2 in Git only              │
│     [ Deploy staging to this cluster ]  [ View in other clusters ]   │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
```

### GUI Enhancement: LIVE + Git Combined

| What TUI Found (LIVE) | What Git Adds | GUI Shows |
|----------------------|---------------|-----------|
| payment-api (prod) deployed | staging overlay exists | "2 variants: prod ✓, staging (not here)" |
| Source path: overlays/prod | base/ folder exists | "Has Base Unit in apps/base/" |
| Synced at rev abc123 | Newer commits pending | "3 commits ahead of deployed" |

### Refinement Flow in GUI

```
Step 1: TUI/Worker discovered these patterns
────────────────────────────────────────────
  App-of-Apps: 3 child Applications found
  Flux Tenancy: 2 tenants detected

Step 2: Suggested ConfigHub structure
─────────────────────────────────────
  App Space: checkout-team (Argo)
    • payment-api [variant=prod]
    • order-service [variant=prod]

  App Space: team-a (Flux)
    • inventory-api [variant=prod]

Step 3: Refine (everything editable)
────────────────────────────────────
  [ Rename App Space ]  [ Edit Labels ]  [ Move Units ]

  💡 These are suggestions. Adjust anything before finalizing.
```

### Pattern-Specific GUI Views

| Pattern | GUI Visualization |
|---------|-------------------|
| **App-of-Apps** | Tree view: root (grayed) → children (selectable) |
| **ApplicationSet** | Generator config + generated Applications list |
| **Flux Tenancy** | Tenant folders → App Spaces, visual isolation |
| **Mono-repo** | Path tree with variant highlighting |

---

## Single-Cluster-First Verification

**Core principle:** If ConfigHub works with one cluster, it works with N clusters.

Before claiming support for a pattern, verify with single cluster:

```bash
./cub-scout map                           # See what's running
./cub-scout map -q "owner!=Native"        # Verify ownership detection
./cub-scout import -n <namespace>         # Import to ConfigHub
cub unit list --space <space>             # Verify hierarchy
cub unit update <unit> --set image.tag=X  # Make a change
```

See [REPO-SKELETON-TAXONOMY.md](planning/REPO-SKELETON-TAXONOMY.md) for full verification checklist and skeleton classification.

---

## See Also

- [planning/REPO-SKELETON-TAXONOMY.md](planning/REPO-SKELETON-TAXONOMY.md) — Skeleton classification & single-cluster-first
- [planning/reference/kostis-argocd-best-practices.md](planning/reference/kostis-argocd-best-practices.md) — ArgoCD best practices
- [TUI-GUI-notes.md](TUI-GUI-notes.md) — What TUI detects vs GUI adds
- [IMPORT-FROM-SOURCES.md](IMPORT-FROM-SOURCES.md) — Flow from TUI → GUI
- [planning/map/02-HUB-APPSPACE-MODEL.md](planning/map/02-HUB-APPSPACE-MODEL.md) — Full model documentation
- [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) — How to run import
