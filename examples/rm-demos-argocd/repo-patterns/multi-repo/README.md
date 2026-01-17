# Repo Pattern: Multi-Repo per Team

Each team has their own repository for configuration.

## Structure

```
payments-team/configs/          # Payments team repo
├── payment-api/
├── payment-processor/
└── argocd/

orders-team/configs/            # Orders team repo
├── order-api/
├── order-processor/
└── argocd/

platform-team/configs/          # Platform team repo
├── redis/
├── postgres/
├── kafka/
└── argocd/
```

## How ConfigHub Sees This

```yaml
Hub: acme-platform
  Sources:
    - https://github.com/acme/payments-configs
    - https://github.com/acme/orders-configs
    - https://github.com/acme/platform-configs

App Spaces:
  payments-team:
    Units: [payment-api, payment-processor]

  orders-team:
    Units: [order-api, order-processor]

  platform-team:
    Units: [redis, postgres, kafka]
```

## Key Commands

```bash
# See all units across all teams
cub unit list

# See just payments team
cub unit list --space payments-team

# Cross-team query: "What's running the old base image?"
cub unit list --where "image.base=alpine:3.18*"
# Returns units from ALL teams

# Bulk update across teams (each team approves their own)
cub unit update \
  --where "image.base=alpine:3.18*" \
  --set image.base=alpine:3.19.1
# Creates: CS-1 (payments), CS-2 (orders), CS-3 (platform)
```

## Benefits of Multi-Repo + ConfigHub

| Without ConfigHub | With ConfigHub |
|-------------------|----------------|
| 3 separate ArgoCD views | 1 unified fleet view |
| Can't query across repos | Fleet-wide queries |
| Bulk updates = 3 PRs | Bulk updates = 1 command |
| No cross-team visibility | Full visibility |

## Skeleton Classification

| Dimension | Value |
|-----------|-------|
| Tool | Argo CD (could be Flux) |
| Repo Count | Multi-repo (per team) |
| Env Strategy | Overlays or folders |
| Orchestration | Flat or App-of-Apps |

**Skeleton ID:** `argo-flat-multi` or `argo-aoa-multi`

## References

- [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](../../../../docs/IMPORT-GIT-REFERENCE-ARCHITECTURES.md) — Multi-repo patterns
- [REPO-SKELETON-TAXONOMY.md](../../../../docs/planning/REPO-SKELETON-TAXONOMY.md) — Full taxonomy
- [Kostis: App vs Config Repos](https://codefresh.io/blog/how-to-structure-your-argo-cd-repositories-using-application-sets/) — Why separate repos can be useful
