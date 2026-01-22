# View Hierarchies with tree

The `cub-scout tree` command shows different hierarchical perspectives on your infrastructure.

## Quick Start

```bash
# Default: Runtime hierarchy (Deployment → ReplicaSet → Pod)
cub-scout tree

# Resources grouped by owner
cub-scout tree ownership

# Suggested Hub/AppSpace organization
cub-scout tree suggest
```

---

## Available Views

### Runtime Hierarchy (default)

Shows the Kubernetes runtime tree: Deployment → ReplicaSet → Pod.

```bash
cub-scout tree
cub-scout tree runtime
```

**Example output:**

```
RUNTIME HIERARCHY (51 Deployments)
════════════════════════════════════════════════════════════════════

NAMESPACE: boutique
────────────────────────────────────────────────────────────────────
├── cart [Flux: apps/boutique] 2/2 ready
│   └── ReplicaSet cart-86f68db776 [2/2]
│       ├── Pod cart-86f68db776-hzqgf  ✓ Running  10.244.0.15  node-1
│       └── Pod cart-86f68db776-mp8kz  ✓ Running  10.244.0.16  node-2
│
├── checkout [Flux: apps/boutique] 1/1 ready
│   └── ReplicaSet checkout-5d8f9c7b4 [1/1]
│       └── Pod checkout-5d8f9c7b4-abc12  ✓ Running  10.244.0.17  node-1
│
├── frontend [Flux: apps/boutique] 3/3 ready
│   └── ReplicaSet frontend-8e6f7a9c2 [3/3]
│       ├── Pod frontend-8e6f7a9c2-def34  ✓ Running  10.244.0.18  node-1
│       ├── Pod frontend-8e6f7a9c2-ghi56  ✓ Running  10.244.0.19  node-2
│       └── Pod frontend-8e6f7a9c2-jkl78  ✓ Running  10.244.0.20  node-3
│
└── payment-api [Flux: apps/boutique] 2/2 ready
    └── ReplicaSet payment-api-7d4b8c [2/2]
        ├── Pod payment-api-7d4b8c-mno90  ✓ Running  10.244.0.21  node-2
        └── Pod payment-api-7d4b8c-pqr12  ✓ Running  10.244.0.22  node-3

NAMESPACE: monitoring
────────────────────────────────────────────────────────────────────
├── prometheus [Helm: kube-prometheus] 1/1 ready
│   └── ReplicaSet prometheus-7d4b8c [1/1]
│       └── Pod prometheus-7d4b8c-xyz99  ✓ Running  10.244.0.25  node-1
│
└── grafana [Helm: kube-prometheus] 1/1 ready
    └── ReplicaSet grafana-6c5d7b [1/1]
        └── Pod grafana-6c5d7b-stu34  ✓ Running  10.244.0.26  node-2

NAMESPACE: temp-test  ⚠ ORPHANS DETECTED
────────────────────────────────────────────────────────────────────
└── debug-nginx [Native] 1/1 ready
    └── ReplicaSet debug-nginx-9a8b7c [1/1]
        └── Pod debug-nginx-9a8b7c-vwx56  ⚠ Pending  (no node assigned)

════════════════════════════════════════════════════════════════════
Summary: 51 Deployments │ 189 Pods │ 186 Running │ 3 Pending
         Flux(28) ArgoCD(12) Helm(5) ConfigHub(4) Native(2)
```

---

### Ownership Hierarchy

Groups resources by GitOps owner (Flux, ArgoCD, Helm, ConfigHub, Native).

```bash
cub-scout tree ownership
```

**Example output:**

```
OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════

Flux (28 resources)
────────────────────────────────────────────────────────────────────
  Managed by: kustomize.toolkit.fluxcd.io labels

  STATUS  NAMESPACE       NAME              KIND         READY
  ✓       boutique        cart              Deployment   2/2
  ✓       boutique        checkout          Deployment   1/1
  ✓       boutique        frontend          Deployment   3/3
  ✓       boutique        payment-api       Deployment   2/2
  ✓       boutique        shipping          Deployment   1/1
  ✓       boutique        product-catalog   Deployment   2/2
  ✓       ingress         nginx-ingress     Deployment   2/2
  ✓       flux-system     source-controller Deployment   1/1
  ✓       flux-system     kustomize-ctrl    Deployment   1/1
  └── ... (19 more resources)

ArgoCD (12 resources)
────────────────────────────────────────────────────────────────────
  Managed by: argocd.argoproj.io/instance label

  STATUS  NAMESPACE       NAME              KIND         READY
  ✓       cert-manager    cert-manager      Deployment   1/1
  ✓       cert-manager    cainjector        Deployment   1/1
  ✓       cert-manager    webhook           Deployment   1/1
  ✓       argocd          argocd-server     Deployment   1/1
  ✓       argocd          argocd-repo       Deployment   1/1
  └── ... (7 more resources)

Helm (5 resources)
────────────────────────────────────────────────────────────────────
  Managed by: app.kubernetes.io/managed-by: Helm label

  STATUS  NAMESPACE       NAME              KIND         READY
  ✓       monitoring      prometheus        StatefulSet  1/1
  ✓       monitoring      grafana           Deployment   1/1
  ✓       monitoring      alertmanager      StatefulSet  1/1
  ✓       kube-system     metrics-server    Deployment   1/1
  ✓       kube-system     coredns           Deployment   2/2

ConfigHub (4 resources)
────────────────────────────────────────────────────────────────────
  Managed by: confighub.com/UnitSlug label

  STATUS  NAMESPACE       NAME              KIND         UNIT
  ✓       payments        payment-gateway   Deployment   payments/payment-gateway
  ✓       payments        fraud-detector    Deployment   payments/fraud-detector
  ✓       orders          order-processor   Deployment   orders/order-processor
  ✓       orders          order-api         Deployment   orders/order-api

Native (2 resources)  ⚠ ORPHANS — not managed by GitOps
────────────────────────────────────────────────────────────────────
  No GitOps labels detected — likely kubectl-applied

  STATUS  NAMESPACE       NAME              KIND         AGE
  ⚠       temp-test       debug-nginx       Deployment   3d
  ⚠       default         test-pod          Pod          1d

════════════════════════════════════════════════════════════════════
Ownership Distribution:

  Flux       ████████████████████████████░░░░░░░░░░░░  56%
  ArgoCD     ████████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░  24%
  Helm       █████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  10%
  ConfigHub  ████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   8%
  Native     █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   2%

→ To import orphans: cub-scout import -n temp-test
→ To trace any resource: cub-scout trace <kind>/<name> -n <ns>
```

---

### Git Source Hierarchy

Shows Git repository structure from Flux GitRepositories and ArgoCD Applications.

```bash
cub-scout tree git
```

**Example output:**

```
GIT SOURCE HIERARCHY
════════════════════════════════════════════════════════════════════

GitRepository: platform-config (flux-system)
────────────────────────────────────────────────────────────────────
  URL: git@github.com:acme/platform-config.git
  Branch: main
  Revision: main@sha1:abc123f
  Status: ✓ Artifact is up to date

  Paths referenced:
  ├── ./clusters/prod/apps/
  │   └── Kustomization: apps (12 resources)
  ├── ./clusters/prod/infrastructure/
  │   └── Kustomization: infrastructure (8 resources)
  └── ./clusters/prod/monitoring/
      └── Kustomization: monitoring (5 resources)

GitRepository: app-manifests (flux-system)
────────────────────────────────────────────────────────────────────
  URL: git@github.com:acme/app-manifests.git
  Branch: main
  Revision: main@sha1:def456g
  Status: ✓ Artifact is up to date

  Paths referenced:
  └── ./services/payment/
      └── Kustomization: payment-api (3 resources)

ArgoCD Application: cert-manager (argocd)
────────────────────────────────────────────────────────────────────
  Repo URL: https://charts.jetstack.io
  Chart: cert-manager
  Version: v1.14.0
  Status: ✓ Synced
  Resources: 12

════════════════════════════════════════════════════════════════════
Summary: 2 GitRepositories │ 1 ArgoCD Application │ 40 resources
```

---

### Patterns Hierarchy

Detects named GitOps patterns from the Flux community.

```bash
cub-scout tree patterns
```

**Example output:**

```
GITOPS PATTERNS DETECTED
════════════════════════════════════════════════════════════════════

Primary Pattern: "Control Plane" (D2-style)
────────────────────────────────────────────────────────────────────
  Named after the Flux CD community reference architecture.

  Characteristics detected:
  ✓ clusters/ directory structure
  ✓ infrastructure/ for platform components
  ✓ apps/ for tenant applications
  ✓ Multi-environment overlays (prod, staging)

  Repository structure:
  platform-config/
  ├── clusters/
  │   ├── prod/
  │   │   ├── apps/
  │   │   ├── infrastructure/
  │   │   └── flux-system/
  │   └── staging/
  │       ├── apps/
  │       └── infrastructure/
  └── base/
      └── shared components

Secondary Pattern: "Arnie" (Environment-per-folder)
────────────────────────────────────────────────────────────────────
  Named after the Argo CD community pattern.

  Characteristics detected:
  ✓ Environment folders at top level
  ✓ Same apps deployed across environments

  app-manifests/
  ├── dev/
  │   └── payment-api/
  ├── staging/
  │   └── payment-api/
  └── prod/
      └── payment-api/

ENVIRONMENT CHAINS
────────────────────────────────────────────────────────────────────
  Same application deployed across environments:

  payment-api:
    dev      →  staging  →  prod
    (1 pod)     (2 pods)    (3 pods)

  frontend:
    dev      →  staging  →  prod
    (1 pod)     (2 pods)    (5 pods)

════════════════════════════════════════════════════════════════════
Pattern Reference:
  D2:    Flux CD "Control Plane" — clusters/infra/apps structure
  Arnie: Argo CD style — env-per-folder (dev/staging/prod)
  Banko: Cluster-per-directory — one cluster per folder
  Fluxy: Multi-repo fleet — Kustomizations reference external repos
```

---

### ConfigHub Hierarchy

Wraps `cub unit tree` to show Unit relationships.

```bash
# Clone relationships (configuration inheritance)
cub-scout tree config --space boutique-prod

# Link relationships (dependencies)
cub-scout tree config --space boutique-prod --edge link

# Across all spaces
cub-scout tree config --space "*"
```

**Example output (clone edge):**

```
CONFIGHUB UNIT TREE: boutique-prod (clone relationships)
════════════════════════════════════════════════════════════════════

  boutique-base                    (base template)
  ├─▶ boutique-dev                 (clone, dev values)
  ├─▶ boutique-staging             (clone, staging values)
  └─▶ boutique-prod                (clone, prod values)
      ├── cart                     (component)
      ├── checkout                 (component)
      ├── frontend                 (component)
      └── payment-api              (component)

════════════════════════════════════════════════════════════════════
Clone edges show configuration inheritance.
Use --edge link to see dependencies instead.
```

**Example output (link edge):**

```
CONFIGHUB UNIT TREE: boutique-prod (link relationships)
════════════════════════════════════════════════════════════════════

  payment-api
  ├─▶ redis-cache                  (depends on)
  └─▶ postgres-db                  (depends on)

  frontend
  ├─▶ payment-api                  (depends on)
  ├─▶ cart                         (depends on)
  └─▶ checkout                     (depends on)

  checkout
  └─▶ payment-api                  (depends on)

════════════════════════════════════════════════════════════════════
Link edges show runtime dependencies.
Use --edge clone to see inheritance instead.
```

---

### Suggested Organization

Analyzes cluster workloads and suggests Hub/AppSpace organization.

```bash
cub-scout tree suggest
```

**Example output:**

```
HUB/APPSPACE SUGGESTION
════════════════════════════════════════════════════════════════════

Detected Pattern: "Control Plane" (D2-style)
  └── clusters/prod, clusters/staging structure found

SUGGESTED STRUCTURE
────────────────────────────────────────────────────────────────────

Hub: acme-platform
│
├── Space: boutique-prod
│   │
│   │  Inferred from: namespace=boutique, owner=Flux
│   │  Kustomization: apps/boutique → 6 Deployments
│   │
│   ├── Unit: cart
│   │   └── Deployment boutique/cart (2 replicas)
│   │
│   ├── Unit: checkout
│   │   └── Deployment boutique/checkout (1 replica)
│   │
│   ├── Unit: frontend
│   │   └── Deployment boutique/frontend (3 replicas)
│   │
│   ├── Unit: payment-api
│   │   └── Deployment boutique/payment-api (2 replicas)
│   │
│   ├── Unit: product-catalog
│   │   └── Deployment boutique/product-catalog (2 replicas)
│   │
│   └── Unit: shipping
│       └── Deployment boutique/shipping (1 replica)
│
├── Space: platform
│   │
│   │  Inferred from: system namespaces, shared infrastructure
│   │
│   ├── Unit: nginx-ingress
│   │   └── Deployment ingress/nginx-ingress (2 replicas)
│   │
│   ├── Unit: cert-manager
│   │   └── Deployment cert-manager/cert-manager (1 replica)
│   │
│   └── Unit: monitoring-stack
│       ├── StatefulSet monitoring/prometheus (1 replica)
│       └── Deployment monitoring/grafana (1 replica)
│
└── Space: payments  (existing ConfigHub space detected)
    │
    │  Already managed by ConfigHub
    │
    ├── Unit: payment-gateway ✓
    └── Unit: fraud-detector ✓

ORPHANS (not suggested for import)
────────────────────────────────────────────────────────────────────
  temp-test/debug-nginx — appears to be temporary debug resource
  default/test-pod — appears to be test resource

════════════════════════════════════════════════════════════════════
Next steps:
  1. Review the suggested structure above
  2. Import workloads: cub-scout import -n boutique --space boutique-prod
  3. View in ConfigHub: cub unit tree --space boutique-prod
  4. Clean up orphans: kubectl delete deploy debug-nginx -n temp-test
```

---

## Relationship to cub unit tree

These commands are complementary:

| Command | Perspective | Shows |
|---------|-------------|-------|
| `cub-scout tree` | Cluster | What's deployed in THIS cluster |
| `cub unit tree` | ConfigHub | How Units relate ACROSS your fleet |

Use `cub-scout tree` to understand your cluster, then `cub unit tree` to see cross-cluster relationships after importing to ConfigHub.

---

## Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Filter by namespace |
| `-A, --all` | Include system namespaces |
| `--space` | ConfigHub space (for config view) |
| `--edge` | clone (inheritance) or link (dependencies) |
| `--json` | JSON output |

---

## See Also

- [Fleet Queries](fleet-queries.md) - Multi-cluster queries with ConfigHub
- [Import to ConfigHub](../map/howto/import-to-confighub.md) - Import workloads
- [CLI Reference](../../CLI-GUIDE.md) - Full command reference
