# flux9s cub-scout Integration

[flux9s](https://github.com/dgunzy/flux9s) is a K9s-inspired terminal UI for Flux. This example shows how to integrate cub-scout capabilities.

## What the Integration Adds

| Feature | Description |
|---------|-------------|
| **Owner Column** | Shows ConfigHub/Flux/Argo/Helm/Native for each resource |
| **ConfigHub Context** | Unit, Space, Revision for ConfigHub-managed resources |
| **Relationship View** | Tree showing GitRepo → Kustomization → Deployment |
| **Status Aggregation** | Health summary across all deployers |
| **risk issue Indicators** | Warning badges for detected issues |
| **Drift Detection** | Shows when live state differs from desired |

## Mockup: Resource List with Agent Data

```
┌─ flux9s ────────────────────────────────────────────────────────────────────┐
│ Kustomizations                                                flux-system   │
├─────────────────────────────────────────────────────────────────────────────┤
│ NAME           READY  STATUS      OWNER      UNIT        SPACE    ISSUES   │
│ apps           True   Applied     ConfigHub  apps        prod     -        │
│ infrastructure True   Applied     ConfigHub  infra       prod     ⚠ 1      │
│ monitoring     False  BuildFailed Flux       -           -        🔴 2     │
│ tenant-a       True   Suspended   ConfigHub  tenant-a    prod     ℹ 1      │
│ tenant-b       True   Applied     ConfigHub  tenant-b    prod     -        │
├─────────────────────────────────────────────────────────────────────────────┤
│ Summary: 5 resources │ ConfigHub: 4 │ Flux: 1 │ Health: 4/5 │ risk issues: 4     │
├─────────────────────────────────────────────────────────────────────────────┤
│ :agent  Agent View  :graph  Relationships  :issues  Show Issues            │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Mockup: Agent Detail View (`:agent`)

```
┌─ Agent: Kustomization/apps ─────────────────────────────────────────────────┐
│                                                                             │
│  Ownership                                                                  │
│  ─────────────────────────────────────────────────────────────────────────  │
│  Owner:      ConfigHub                                                      │
│  Unit:       apps                                                           │
│  Space:      prod (payments-team)                                           │
│  Revision:   42 (live: 42 ✓)                                                │
│  Deployer:   Flux (this Kustomization)                                      │
│                                                                             │
│  Managed Resources                                                          │
│  ─────────────────────────────────────────────────────────────────────────  │
│  KIND         NAME           NAMESPACE   STATUS   DRIFT   risk issue              │
│  Deployment   backend        prod        Ready    -       -                 │
│  Deployment   frontend       prod        Ready    -       -                 │
│  Service      backend        prod        Ready    -       -                 │
│  Service      frontend       prod        Ready    -       -                 │
│  ConfigMap    app-config     prod        Ready    ⚠       -                 │
│                                                                             │
│  Issues (1)                                                                 │
│  ─────────────────────────────────────────────────────────────────────────  │
│  ⚠️  DRIFT: ConfigMap/app-config                                            │
│      Field: data.LOG_LEVEL                                                  │
│      Desired: "info"  Live: "debug"                                         │
│      [a]ccept drift  [r]estore desired                                      │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ <y>:yaml  <enter>:select resource  <esc>:back                               │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Mockup: Relationship Graph (`:graph`)

```
┌─ Relationships ─────────────────────────────────────────────────────────────┐
│                                                                             │
│  GitRepository/app-repo (Flux)                                              │
│  │                                                                          │
│  └─▶ Kustomization/apps (ConfigHub: apps @ prod)                           │
│      │                                                                      │
│      ├─▶ Deployment/backend (ConfigHub: apps @ prod)                       │
│      │   └─▶ ReplicaSet/backend-abc123                                     │
│      │       └─▶ Pod/backend-abc123-xyz (3 replicas)                       │
│      │                                                                      │
│      ├─▶ Deployment/frontend (ConfigHub: apps @ prod)                      │
│      │   └─▶ ReplicaSet/frontend-def456                                    │
│      │       └─▶ Pod/frontend-def456-uvw (2 replicas)                      │
│      │                                                                      │
│      ├─▶ Service/backend ──▶ Deployment/backend                            │
│      │                                                                      │
│      └─▶ Service/frontend ──▶ Deployment/frontend                          │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ <j/k>:navigate  <enter>:select  <esc>:back                                  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Mockup: Fleet Summary (`:summary`)

```
┌─ Fleet Summary ─────────────────────────────────────────────────────────────┐
│                                                                             │
│  ┌─ Ownership ──────────┐  ┌─ Health ──────────────┐  ┌─ Issues ─────────┐ │
│  │ ConfigHub     45     │  │ Ready        52       │  │ Critical   0     │ │
│  │ Flux          12     │  │ Progressing   3       │  │ Warning    3     │ │
│  │ ArgoCD         8     │  │ Degraded      2       │  │ Info       7     │ │
│  │ Helm           3     │  │ Failed        1       │  │ Drift      2     │ │
│  │ Native         5     │  │ Unknown       2       │  │            ────  │ │
│  │              ────    │  │              ────     │  │ Total     12     │ │
│  │ Total        73     │  │ Total        60       │  │                  │ │
│  └─────────────────────┘  └──────────────────────┘  └──────────────────┘ │
│                                                                             │
│  Recent Activity                                                            │
│  ─────────────────────────────────────────────────────────────────────────  │
│  10:30  Kustomization/apps reconciled (rev 42 → 43)                        │
│  10:28  Drift detected: ConfigMap/app-config                               │
│  10:15  HelmRelease/monitoring upgraded (v1.2.0 → v1.2.1)                  │
│  10:02  RISK-2025-0008 resolved: Kustomization/infra                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Implementation Approach

Since flux9s is written in Rust and doesn't have a plugin system, options are:

### Option A: Fork and Extend

Add Agent integration directly to flux9s source:

```rust
// src/agent/client.rs
use reqwest::Client;

pub struct AgentClient {
    base_url: String,
    client: Client,
}

impl AgentClient {
    pub async fn get_map(&self, namespace: Option<&str>) -> Result<ResourceMap> {
        let url = match namespace {
            Some(ns) => format!("{}/api/map?namespace={}", self.base_url, ns),
            None => format!("{}/api/map", self.base_url),
        };
        let resp = self.client.get(&url).send().await?;
        resp.json().await
    }

    pub async fn get_resource_info(&self, kind: &str, ns: &str, name: &str) -> Result<ResourceInfo> {
        let url = format!("{}/api/map/resource?kind={}&namespace={}&name={}",
            self.base_url, kind, ns, name);
        let resp = self.client.get(&url).send().await?;
        resp.json().await
    }
}
```

### Option B: Contribute Upstream

1. Open issue at https://github.com/dgunzy/flux9s/issues proposing the feature
2. Reference this design document
3. Offer to implement the PR

### Option C: Companion Mode

Run flux9s alongside a dedicated agent TUI:

```bash
# Terminal 1
flux9s

# Terminal 2
cub-scout tui --watch
```

## Agent API Requirements (Proposed)

> **Not Yet Implemented:** These API endpoints are proposed for a future HTTP API mode. Currently, integrations should use CLI commands like `cub-scout snapshot` and `cub-scout scan --json`.

The integration would require these Agent API endpoints:

| Endpoint | Purpose |
|----------|---------|
| `GET /api/map` | Full resource map with ownership |
| `GET /api/map?namespace=X` | Filtered by namespace |
| `GET /api/map/resource?kind=X&ns=Y&name=Z` | Single resource details |
| `GET /api/map/graph?root=Kind/name` | Relationship graph |
| `GET /api/summary` | Fleet-wide aggregation |
| `GET /api/risk/findings` | risk issue scan results |
| `GET /api/drift` | Drift detection results |
| `WS /ws/watch` | Real-time updates |

## References

- [flux9s GitHub](https://github.com/dgunzy/flux9s)
- [flux9s Documentation](https://flux9s.ca/)
- [cub-scout GSF Schema](../../docs/GSF-SCHEMA.md)
