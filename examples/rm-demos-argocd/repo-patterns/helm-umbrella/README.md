# Repo Pattern: Helm Umbrella Charts

## The Problem

Your platform team manages an umbrella chart with 4 sub-charts. After a Redis upgrade,
you need to know: *"Is the new Redis version actually running, or did Helm say it deployed
but the pods are still on the old image?"*

`helm list` says the release is deployed. But "deployed" and "running" are different things.

**cub-scout shows what's actually live:**

```
$ ./cub-scout map list -q "owner=Helm" -q "name=redis*"

  STATUS  NAMESPACE  NAME   OWNER  MANAGED-BY     IMAGE
  ✓       platform   redis  Helm   platform-stack  redis:7.2.4  ← confirmed live
```

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

## How cub-scout Sees This

```bash
# See all Helm-managed resources
$ ./cub-scout map list -q "owner=Helm"

  STATUS  NAMESPACE  NAME        OWNER  MANAGED-BY      IMAGE
  ✓       platform   redis       Helm   platform-stack  redis:7.2.4
  ✓       platform   postgres    Helm   platform-stack  postgres:16.1
  ✓       platform   kafka       Helm   platform-stack  kafka:3.7.0
  ✓       monitoring prometheus  Helm   platform-stack  prometheus:2.51
  ✓       monitoring grafana     Helm   platform-stack  grafana:10.4

# Trace a sub-chart resource back to the umbrella release
$ ./cub-scout trace deployment/redis -n platform

  Deployment/redis (platform)
  ├── Owner: Helm
  └── Release: platform-stack (platform)

# Check for drift between Helm release and live state
$ ./cub-scout drift --helm-release platform-stack -n platform

  Drift Report
  ════════════
  No drift detected.

# Scan for misconfigurations across all platform components
$ ./cub-scout scan -n platform -n monitoring

  RISK SCAN
  ─────────
  HIGH (1)
  [RISK-2025-0027] monitoring/grafana — known dashboard auth bypass

  WARNING (1)
  [RISK-2025-0001] platform/kafka — missing resource limits
```

## Ownership Tree

```
./cub-scout tree ownership

OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════

Helm (5 resources)
────────────────────────────────────────────────────────────────────
  Release: platform-stack (platform)
  │   Chart: platform-charts (umbrella)
  │   Status: deployed (revision 14)
  │
  ├── platform/redis          Deployment   ✓ 3/3   redis:7.2.4
  ├── platform/postgres       Deployment   ✓ 1/1   postgres:16.1
  ├── platform/kafka          Deployment   ✓ 3/3   kafka:3.7.0
  ├── monitoring/prometheus   Deployment   ✓ 1/1   prometheus:2.51
  └── monitoring/grafana      Deployment   ✓ 1/1   grafana:10.4

════════════════════════════════════════════════════════════════════

  Umbrella Chart        Sub-charts              Live Pods
  ──────────────        ──────────              ─────────
  platform-stack   →    redis (sub-chart)  →    3 pods, redis:7.2.4 ✓
                   →    postgres           →    1 pod,  postgres:16.1 ✓
                   →    kafka              →    3 pods, kafka:3.7.0 ✓
                   →    monitoring         →    2 pods (prometheus + grafana)

→ Verify live images: cub-scout map list -q "owner=Helm"
→ Check for drift:    cub-scout drift --helm-release platform-stack -n platform
```

## Why Helm Umbrella Benefits from cub-scout

| Helm Umbrella Alone | + cub-scout |
|--------------------|-------------|
| `helm list` shows release status | `map list` shows live pod state |
| `helm get values` per release | Query across all releases |
| No visibility into what's running | Full image + owner visibility |
| No config scanning | `scan` catches known misconfigurations |

## Skeleton Classification

| Dimension | Value |
|-----------|-------|
| Tool | Helm (via Argo CD or Flux) |
| Repo Count | Monorepo |
| Env Strategy | Values files |
| Orchestration | Umbrella chart |

**Skeleton ID:** `helm-umbrella`

## See Also

- [Flux Boutique](../../../flux-boutique/) — Flux managing Kustomize (compare with Helm)
- [Platform Example](../../../platform-example/) — Flux HelmRelease managing Prometheus stack
