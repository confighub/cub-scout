# Repo Pattern: Monorepo with Folders

## The Problem

You have one repo with 3 apps × 3 environments = 9 deployment paths.
A new engineer asks: *"Which Kustomization deploys payment-api to prod?"*

With `kubectl` alone you'd cross-reference namespaces, labels, and directory structures manually.

**cub-scout answers this in one command:**

```
$ ./cub-scout trace deployment/payment-api -n payments-prod

  Deployment/payment-api (payments-prod)
  ├── Owner: Flux
  ├── Kustomization: payment-api-prod (flux-system)
  └── Source: GitRepository/platform-configs → acme/platform-configs@main
```

## Structure

```
platform-configs/
├── apps/
│   ├── payment-api/
│   │   ├── base/
│   │   │   ├── deployment.yaml
│   │   │   ├── service.yaml
│   │   │   └── kustomization.yaml
│   │   └── overlays/
│   │       ├── dev/
│   │       │   └── kustomization.yaml
│   │       ├── staging/
│   │       │   └── kustomization.yaml
│   │       └── prod/
│   │           └── kustomization.yaml
│   ├── order-api/
│   │   └── ... (same structure)
│   └── inventory-api/
│       └── ... (same structure)
│
└── argocd/
    └── apps.yaml  # App-of-apps or ApplicationSet
```

## How cub-scout Sees This

```bash
# Parse the repo structure offline
$ ./cub-scout parse-repo --path ./platform-configs

  Detected: monorepo (kustomize overlays)
  Skeleton: flux-tenant-mono
  Apps:     payment-api, order-api, inventory-api
  Envs:     dev, staging, prod

# See all workloads and their owners
$ ./cub-scout map list -q "namespace=payments-*"

  STATUS  NAMESPACE        NAME          OWNER  MANAGED-BY
  ✓       payments-dev     payment-api   Flux   payment-api-dev
  ✓       payments-staging payment-api   Flux   payment-api-staging
  ✓       payments-prod    payment-api   Flux   payment-api-prod

# Find all prod workloads across all apps
$ ./cub-scout map list -q "namespace=*-prod"

  STATUS  NAMESPACE       NAME           OWNER  MANAGED-BY
  ✓       payments-prod   payment-api    Flux   payment-api-prod
  ✓       orders-prod     order-api      Flux   order-api-prod
  ✓       inventory-prod  inventory-api  Flux   inventory-api-prod
```

## Ownership Tree

```
./cub-scout tree ownership

OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════

Flux (9 resources)
────────────────────────────────────────────────────────────────────
  GitRepository/platform-configs (flux-system)
  │
  ├── payments
  │   ├── Kustomization/payment-api-dev      → Deployment/payment-api (payments-dev)      ✓
  │   ├── Kustomization/payment-api-staging   → Deployment/payment-api (payments-staging)  ✓
  │   └── Kustomization/payment-api-prod      → Deployment/payment-api (payments-prod)     ✓
  │
  ├── orders
  │   ├── Kustomization/order-api-dev         → Deployment/order-api (orders-dev)          ✓
  │   ├── Kustomization/order-api-staging     → Deployment/order-api (orders-staging)      ✓
  │   └── Kustomization/order-api-prod        → Deployment/order-api (orders-prod)         ✓
  │
  └── inventory
      ├── Kustomization/inventory-api-dev     → Deployment/inventory-api (inventory-dev)    ✓
      ├── Kustomization/inventory-api-staging → Deployment/inventory-api (inventory-staging) ✓
      └── Kustomization/inventory-api-prod    → Deployment/inventory-api (inventory-prod)   ✓

════════════════════════════════════════════════════════════════════
Summary: 1 GitRepository │ 9 Kustomizations │ 9 Deployments
         3 apps × 3 envs = 9 paths, all from one repo
```

## Skeleton Classification

| Dimension | Value |
|-----------|-------|
| Tool | Argo CD / Flux |
| Repo Count | Monorepo |
| Env Strategy | Overlays (Kustomize) |
| Orchestration | ApplicationSet or Kustomizations |

**Skeleton ID:** `flux-tenant-mono` or `argo-appset-mono`

## See Also

- [Flux Boutique](../../../flux-boutique/) — Working single-repo Flux example
- [Apptique Flux Monorepo](../../../apptique-examples/flux-monorepo/) — Multi-env overlays with real manifests
- [Platform Example](../../../platform-example/) — Full env-per-folder (Arnie) pattern
