# Demo Data: App-Deployment-Target Pattern

This example demonstrates the **App-Deployment-Target** model using a subset of
the [demo-data dataset](https://github.com/confighubai/examples-internal/tree/main/demo-data).

## What This Shows

The App-Deployment-Target model organizes workloads as:

```
App (eshop)
├── Deployment: dev     (1 replica, debug logging)
├── Deployment: staging (2 replicas, info logging)
└── Deployment: prod    (3 replicas, warn logging, resource limits)
```

Each Deployment maps to a ConfigHub Space. Each component (api, worker)
maps to a Unit within that Space.

## Fixtures

| File | App | Environment | Owner | Version Skew |
|------|-----|-------------|-------|-------------|
| `dev-eshop.yaml` | eshop | dev | Helm | api:4.2.1 |
| `prod-eshop.yaml` | eshop | prod | Helm | api:**4.2.0** (behind dev!) |
| `prod-website.yaml` | website | prod | ArgoCD | frontend:2.1.0 |

**Intentional version skew**: `us-prod-eshop` has `eshop/api:4.2.0` while
`dev-eshop` has `eshop/api:4.2.1`. This is a common pattern where prod
lags behind dev after a deploy freeze.

## Labels

All resources carry ConfigHub labels (prefixed `confighub.com/` on cluster
resources) for multi-dimensional querying:

| Cluster Label | ConfigHub Label | Values | Use |
|---------------|-----------------|--------|-----|
| `confighub.com/App` | `App` | eshop, website | Group by application |
| `confighub.com/AppOwner` | `AppOwner` | Product, Marketing | Group by team |
| `confighub.com/TargetRole` | `TargetRole` | dev, prod | Filter by environment |
| `confighub.com/TargetRegion` | `TargetRegion` | us-east | Filter by region |

## Try It

### Scan for risks (works today, no cluster needed)

```bash
./cub-scout scan --file examples/demo-data-adt/fixtures/dev-eshop.yaml
./cub-scout scan --file examples/demo-data-adt/fixtures/prod-eshop.yaml
```

### Import discovery (needs cluster)

```bash
# Deploy fixtures
kubectl create namespace dev-eshop
kubectl create namespace us-prod-eshop
kubectl apply -f examples/demo-data-adt/fixtures/dev-eshop.yaml
kubectl apply -f examples/demo-data-adt/fixtures/prod-eshop.yaml

# Discover and propose App structure
./cub-scout import -n dev-eshop --dry-run --json
./cub-scout import -n us-prod-eshop --dry-run --json
```

### Ownership detection

```bash
# See who manages what
./cub-scout map list -n dev-eshop
./cub-scout map list -n us-prod-eshop
./cub-scout map list -n us-prod-website
```

## Connected Mode: Fleet Queries and Promotion

After importing into ConfigHub, use the SDK `cub` CLI for cross-environment
queries and promotion workflows.

> **Ownership:** `cub` commands come from the [ConfigHub SDK](https://github.com/confighub/sdk) (`cmd/cub`).
> cub-scout discovers and explains; `cub` handles connected lifecycle.
> See [Interface Boundaries](../../docs/concepts/why-connected-mode.md#interface-boundaries-authoritative).

### Cross-environment queries

```bash
# All prod units
cub unit list --space "*" --label "TargetRole=prod"

# All apps owned by Product team
cub unit list --space "*" --label "AppOwner=Product"

# Version skew: compare dev vs prod for eshop
cub unit get eshop-api --space dev-eshop --json | jq '.image'
cub unit get eshop-api --space us-prod-eshop --json | jq '.image'
```

### Promotion handoff

```bash
# Push a version upgrade from dev to prod
cub unit push-upgrade eshop-api --from-space dev-eshop --to-space us-prod-eshop
```

### Label mapping: cluster vs ConfigHub

| Cluster label (`confighub.com/...`) | ConfigHub entity label | Where it lives |
|--------------------------------------|----------------------|----------------|
| `confighub.com/App` | `App` | Unit label in ConfigHub |
| `confighub.com/AppOwner` | `AppOwner` | Space or Unit label |
| `confighub.com/TargetRole` | `TargetRole` | Target label |
| `confighub.com/TargetRegion` | `TargetRegion` | Target label |

## Source

Full 6-app × 6-target dataset:
[confighubai/examples-internal/demo-data](https://github.com/confighubai/examples-internal/tree/main/demo-data)

Related issues:
- confighubai/confighub#3639 — App-Deployment-Target model
- confighubai/confighub#3666 — Opinionated Apps/Targets UI
