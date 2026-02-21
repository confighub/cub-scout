# demo-data: App-Deployment-Target Example

**Status:** Working (standalone) | Future (connected mode fleet queries)

## The Problem

> "I have 6 apps deployed across multiple targets. Which versions are running where?
> Are any targets behind? Which team owns what?"

## cub-scout answers this

```bash
# Scan all manifests for risk issues (no cluster required)
cub-scout scan --file examples/demo-data/manifests.yaml

# Apply to a cluster and explore
kubectl apply -f examples/demo-data/manifests.yaml
cub-scout map workloads
cub-scout map deployers
cub-scout patterns detect
```

## What This Shows

The **App-Deployment-Target** (ADT) model: 6 applications deployed across 2 targets,
managed through Flux with ConfigHub labels for cross-cutting queries.

### Apps (6)

| App | Owner | Image |
|-----|-------|-------|
| aichat | AI Team | aichat-api |
| website | Product Team | website-frontend |
| docs | Product Team | docs-server |
| eshop | Commerce Team | eshop-api |
| portal | Platform Team | portal-dashboard |
| platform | Platform Team | platform-controller |

### Targets (2)

| Target | Region | Role |
|--------|--------|------|
| us-prod | us-east-1 | production |
| eu-prod | eu-west-1 | production |

### Intentional Version Skew

`eshop-api` runs `:4.2.1` in us-prod but `:4.2.0` in eu-prod.
This is the kind of drift that fleet queries surface instantly.

### Labels for Cross-Cutting Queries

Every resource has:
- `confighub.com/UnitSlug` — the ConfigHub unit identity
- `confighub.com/SpaceName` — the ConfigHub space (app-target combination)
- `confighub.com/App` — application name
- `confighub.com/AppOwner` — team that owns the app
- `confighub.com/TargetRole` — deployment target role (production, staging, etc.)
- `confighub.com/TargetRegion` — deployment target region

## Directory Structure

```
demo-data/
  manifests.yaml        # All resources in one file (kubectl apply -f)
  README.md             # This file
```

## Bridge Patterns Detected

When applied to a cluster or analyzed with `patterns detect`:

| Pattern | What It Finds |
|---------|---------------|
| `delivery.bridge.git_flux` | Git -> Flux pipeline (GitRepository + Kustomizations + workloads) |
| `delivery.bridge.confighub_oci` | ConfigHub -> OCI pipeline (OCIRepository with ConfigHub labels) |

## Delivery Chain

```
GitRepository (platform-apps)
  -> Kustomization (per app-target)
    -> Deployment + Service (workloads with ConfigHub + Flux labels)

OCIRepository (confighub-bundles, ConfigHub origin)
  -> Kustomization (confighub-deploy)
    -> Deployment (dual-managed: Flux + ConfigHub labels)
```

## Future: Connected Mode

When connected mode ships (#158), this dataset enables:

1. **Fleet queries** — "all prod deployments in US" via ConfigHub labels
2. **Version skew detection** — diff eshop across targets
3. **App-Target navigation** — drill from app to its deployments across targets
4. **Cross-cutting ownership** — "all apps owned by Platform Team"

## See Also

- [Bridge patterns](../../docs/reference/patterns-contract.md) — delivery pipeline detection
- [GSF schema](../../docs/reference/gsf-schema.md) — delivery chain metadata
- Source dataset: [confighubai/examples-internal/demo-data](https://github.com/confighubai/examples-internal/tree/main/demo-data)
