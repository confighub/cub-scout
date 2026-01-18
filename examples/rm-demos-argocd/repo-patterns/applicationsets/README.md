# Repo Pattern: ArgoCD ApplicationSets

Using ApplicationSets to generate Applications dynamically.

## Structure

```yaml
# Single ApplicationSet generates Applications for all clusters
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
            environment: production
  template:
    metadata:
      name: 'payment-api-{{name}}'
    spec:
      source:
        repoURL: https://github.com/acme/configs
        path: apps/payment-api
        targetRevision: HEAD
      destination:
        server: '{{server}}'
        namespace: payments
```

## How ConfigHub Sees This

```yaml
Hub: acme-platform

App Space: payments-team
  Units:
    # The generator
    - payment-api-appset (type: generator)

    # Generated instances (automatically tracked)
    - payment-api-prod-us-east-1 (instance_of: payment-api-appset)
    - payment-api-prod-us-west-1 (instance_of: payment-api-appset)
    - payment-api-prod-eu-west-1 (instance_of: payment-api-appset)
    # ... (all 32 production clusters)
```

## Key Commands

```bash
# See all generators
cub unit list --where "type=generator"

# See what a generator creates
cub unit list --where "instance_of=payment-api-appset"

# See orphaned instances (generator deleted but instances remain)
cub unit list --where "instance_of!=null AND instance_of.exists=false"

# Update the generator (propagates to all instances)
cub unit update payment-api-appset --set image.tag=v2.1.0
# This updates the source, all generated Applications re-sync
```

## The ConfigHub Advantage

| ApplicationSet Alone | + ConfigHub |
|---------------------|-------------|
| See generated Apps in ArgoCD UI | Query generated instances fleet-wide |
| No visibility into what's generated where | `cub unit list --where "instance_of=X"` |
| Manual tracking of generators | Automatic generator → instance tracking |
| No fleet-wide updates | Update generator, all instances follow |

## Skeleton Classification

| Dimension | Value |
|-----------|-------|
| Tool | Argo CD |
| Repo Count | Mono or Multi |
| Env Strategy | Cluster selector labels |
| Orchestration | ApplicationSet (generator) |

**Skeleton ID:** `argo-appset-mono` or `argo-appset-multi`

## References

- [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](../../../../docs/IMPORT-GIT-REFERENCE-ARCHITECTURES.md) — Pattern 2: ApplicationSet
- [REPO-SKELETON-TAXONOMY.md](../../../../docs/planning/REPO-SKELETON-TAXONOMY.md) — Full taxonomy
- [Kostis: ApplicationSet Best Practices](https://codefresh.io/blog/how-to-structure-your-argo-cd-repositories-using-application-sets/) — Official guide
- [CCVE-2025-3724](https://github.com/confighubai/confighub-ccve/blob/main/scanner/CCVE-2025-3724.yaml) — Complex ApplicationSet anti-pattern
