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

All resources carry ConfigHub labels for multi-dimensional querying:

| Label | Values | Use |
|-------|--------|-----|
| `App` | eshop, website | Group by application |
| `AppOwner` | Product, Marketing | Group by team |
| `TargetRole` | dev, prod | Filter by environment |
| `TargetRegion` | us-east | Filter by region |

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

## Future: Fleet Queries (Connected Mode)

When connected mode fleet queries are implemented, this example will
demonstrate cross-environment queries:

```bash
# All prod deployments (future)
./cub-scout fleet query "Labels.TargetRole=prod"

# Version skew detection (future)
./cub-scout fleet diff --app eshop --field image

# All apps owned by Product team (future)
./cub-scout fleet query "Labels.AppOwner=Product"
```

## Source

Full 6-app × 6-target dataset:
[confighubai/examples-internal/demo-data](https://github.com/confighubai/examples-internal/tree/main/demo-data)

Related issues:
- confighubai/confighub#3639 — App-Deployment-Target model
- confighubai/confighub#3666 — Opinionated Apps/Targets UI
