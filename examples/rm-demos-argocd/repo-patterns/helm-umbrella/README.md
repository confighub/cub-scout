# Repo Pattern: Helm Umbrella Charts

Using umbrella charts to manage multiple sub-charts with shared values.

## Structure

```
platform-charts/
├── Chart.yaml              # Umbrella chart
├── charts/
│   ├── redis/              # Sub-chart
│   ├── postgres/           # Sub-chart
│   ├── kafka/              # Sub-chart
│   └── monitoring/         # Sub-chart (prometheus, grafana, etc.)
├── values.yaml             # Base values
├── values-dev.yaml         # Dev overrides
├── values-staging.yaml     # Staging overrides
└── values-prod.yaml        # Prod overrides
```

## How ConfigHub Sees This

```yaml
Hub: acme-platform
  Source: https://github.com/acme/platform-charts (Helm)

App Space: platform-team
  Units:
    # Each sub-chart becomes a Unit
    - redis
        values_files: [values.yaml, values-prod.yaml]
    - postgres
        values_files: [values.yaml, values-prod.yaml]
    - kafka
        values_files: [values.yaml, values-prod.yaml]
    - monitoring
        values_files: [values.yaml, values-prod.yaml]
```

## Key Commands

```bash
# See all platform units
cub unit list --space platform-team

# Update a specific value across all environments
cub unit update redis --set redis.replicas=5

# Update the chart version for all units
cub unit update --space platform-team --set chart.version=2.0.0

# See which clusters have which chart version
cub unit list --where "chart.version!=2.0.0"
```

## The ConfigHub Advantage with Helm

| Helm Umbrella Alone | + ConfigHub |
|--------------------|-------------|
| One values file per env | Query across all envs |
| No visibility across clusters | Fleet-wide Helm values visibility |
| `helm upgrade` per cluster | One command, all clusters |
| No drift detection | Detect when live != desired |

## Example: Update Redis Across Fleet

```bash
# Traditional way
for cluster in $(kubectl config get-contexts -o name); do
  kubectl config use-context $cluster
  helm upgrade platform ./platform-charts -f values-prod.yaml --set redis.replicas=5
done
# (Repeat for 47 clusters, hope you don't miss one)

# ConfigHub way
cub unit update redis --where "environment=production" --set replicas=5
# Done. All 47 clusters. With approval workflow.
```

## Skeleton Classification

| Dimension | Value |
|-----------|-------|
| Tool | Helm (via Argo CD or Flux) |
| Repo Count | Monorepo |
| Env Strategy | Values files |
| Orchestration | Umbrella chart |

**Skeleton ID:** `helm-umbrella`

## References

- [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](../../../../docs/IMPORT-GIT-REFERENCE-ARCHITECTURES.md) — Helm patterns
- [REPO-SKELETON-TAXONOMY.md](../../../../docs/planning/REPO-SKELETON-TAXONOMY.md) — Full taxonomy
- [Kostis: Helm Anti-patterns](https://codefresh.io/blog/argo-cd-application-anti-patterns/) — Avoid hardcoded values (CCVE-2025-3722)
