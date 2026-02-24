# Integration UI Mockups

**Status: Mockups Only** — UI designs for discussion, not working code.

The cub-scout exposes a standardized API that can be consumed by GUI tools. These mockups show how integrations could look.

---

## Integration 1: Argo CD Extension

> For working code, see [argocd-extension/](argocd-extension/)

The Argo CD extension adds a "risk issues" tab and status badge to application views.

### Status Panel (Badge in Application Header)

```
┌─ guestbook ─────────────────────────────────────────────────────────┐
│ [Synced] [Healthy]  ⚠️ 2 risk issues                         ⚙️ Settings  │
├─────────────────────────────────────────────────────────────────────┤
```

### Application Tab (Full risk issue View)

```
┌─ risk issues ─────────────────────────────────────────────────────────────┐
│                                                                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                          │
│  │    0     │  │    2     │  │    3     │                          │
│  │ Critical │  │ Warning  │  │   Info   │                          │
│  └──────────┘  └──────────┘  └──────────┘                          │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ ▶ RISK-2025-0023  WARNING  default/Deployment/guestbook-ui  │   │
│  └─────────────────────────────────────────────────────────────┘   │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ ▶ RISK-2025-0027  WARNING  monitoring/Deployment/grafana    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Expanded Finding

```
┌─────────────────────────────────────────────────────────────────────┐
│ ▼ RISK-2025-0023  WARNING  default/Deployment/guestbook-ui         │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Container lacks resource limits                                    │
│                                                                     │
│  Remediation:                                                       │
│  Add resources.limits to container spec:                            │
│    resources:                                                       │
│      limits:                                                        │
│        memory: "256Mi"                                              │
│        cpu: "500m"                                                  │
│                                                                     │
│  View full documentation →                                          │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Resource Tab (Per-Resource risk issues)

```
┌─ Deployment/guestbook-ui ───────────────────────────────────────────┐
│ Summary │ YAML │ Events │ Logs │ risk issues │                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Ownership                                                          │
│  ──────────────────────────────────────────────────────────────     │
│  Owner:      ConfigHub                                              │
│  Unit:       guestbook-ui                                           │
│  Space:      demo (App Space)                                       │
│  Revision:   15 (live: 15 ✓)                                        │
│  Deployer:   Argo CD (this application)                             │
│                                                                     │
│  Config CVEs                                                        │
│  ──────────────────────────────────────────────────────────────     │
│  ℹ️  RISK-2025-0023: Missing resource limits                        │
│      Remediation: Add resources.limits to container spec            │
│      [View Details]                                                 │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Integration 2: flux9s TUI Enhancement

> This is a proposal. See [flux9s/](flux9s/) for details.

flux9s is a K9s-inspired terminal UI for Flux. The integration adds ownership and risk issue columns.

### Resource List with Agent Data

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

### Agent Detail View (`:agent`)

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

### Relationship Graph (`:graph`)

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

### Fleet Summary (`:summary`)

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

---

## Integration 3: Flux Operator Web UI

> For working metrics exporter, see [flux-operator/](flux-operator/)

The Flux Operator provides a web UI with GitOps graph visualization. The integration adds ownership overlays.

### Enhanced GitOps Graph

```
┌─ GitOps Pipeline: payments ─────────────────────────────────────────────────┐
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                                                                     │   │
│  │   ┌────────────┐      ┌────────────────┐      ┌────────────────┐   │   │
│  │   │GitRepository│─────▶│ Kustomization │─────▶│  Deployment    │   │   │
│  │   │            │      │                │      │                │   │   │
│  │   │ payments   │      │ payments       │      │ backend        │   │   │
│  │   │ ✓ Ready    │      │ ✓ Applied      │      │ ✓ Ready 3/3   │   │   │
│  │   └────────────┘      │                │      │                │   │   │
│  │        Flux           │ ┌────────────┐ │      │ ┌────────────┐ │   │   │
│  │                       │ │ ConfigHub  │ │      │ │ ConfigHub  │ │   │   │
│  │                       │ │ payments/  │ │      │ │ payments/  │ │   │   │
│  │                       │ │ infra @42  │ │      │ │ backend@42 │ │   │   │
│  │                       │ └────────────┘ │      │ └────────────┘ │   │   │
│  │                       └────────────────┘      └────────────────┘   │   │
│  │                              │                       │             │   │
│  │                              │                       ▼             │   │
│  │                              │              ┌────────────────┐     │   │
│  │                              │              │   Service      │     │   │
│  │                              │              │   backend      │     │   │
│  │                              │              │   ✓ Ready      │     │   │
│  │                              │              │ ┌────────────┐ │     │   │
│  │                              │              │ │ ConfigHub  │ │     │   │
│  │                              ▼              │ │ backend@42 │ │     │   │
│  │                       ┌────────────────┐    │ └────────────┘ │     │   │
│  │                       │  HelmRelease   │    └────────────────┘     │   │
│  │                       │  redis         │                           │   │
│  │                       │  ✓ Deployed    │                           │   │
│  │                       │ ┌────────────┐ │                           │   │
│  │                       │ │   Helm     │ │                           │   │
│  │                       │ └────────────┘ │                           │   │
│  │                       └────────────────┘                           │   │
│  │                                                                     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─ Legend ─────────────────────────────────────────────────────────────┐  │
│  │ ConfigHub ████  Flux ████  Helm ████  Native ████   ⚠ Drift  🔴 risk issue │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Ownership Panel

```
┌─ Fleet Overview ────────────────────────────────────────────────────────────┐
│                                                                             │
│  ┌─ Ownership ─────────────┐  ┌─ Health ───────────────┐                   │
│  │                         │  │                        │                   │
│  │  ConfigHub  ████████ 45 │  │  Ready      ████████ 52│                   │
│  │  Flux       ████     12 │  │  Progressing ██      3 │                   │
│  │  ArgoCD     ███       8 │  │  Degraded   ██       2 │                   │
│  │  Helm       █         3 │  │  Failed     █        1 │                   │
│  │  Native     ██        5 │  │  Unknown    █        2 │                   │
│  │                         │  │                        │                   │
│  └─────────────────────────┘  └────────────────────────┘                   │
│                                                                             │
│  ┌─ ConfigHub Spaces ───────────────────────────────────────────────────┐  │
│  │ Space              Units  Healthy  Drifted  risk issues                    │  │
│  │ payments-prod        12      12        0       0                     │  │
│  │ payments-staging      8       7        1       1                     │  │
│  │ monitoring            5       4        0       2                     │  │
│  │ platform-infra       20      19        1       0                     │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌─ Issues ─────────────────────────────────────────────────────────────┐  │
│  │ 🔴 RISK-2025-0004  Kustomization/monitoring  BuildFailed             │  │
│  │ ⚠️  RISK-2025-0023  Deployment/api           Missing limits          │  │
│  │ ⚠️  DRIFT          ConfigMap/app-config     LOG_LEVEL changed        │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Agent API Endpoints (Proposed)

> **Not Yet Implemented:** These API endpoints are proposed for a future HTTP API mode. Currently, use CLI commands like `cub-scout snapshot -o -` and `cub-scout scan --json`.

All integrations would use these standardized endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /api/map` | Full resource map with ownership |
| `GET /api/map?namespace=X` | Filter by namespace |
| `GET /api/map/resource?kind=X&ns=Y&name=Z` | Single resource details |
| `GET /api/map/graph?root=Kind/name` | Relationship graph |
| `GET /api/summary` | Fleet-wide aggregation |
| `GET /api/risk/findings` | risk issue scan results |
| `GET /api/drift` | Drift detection results |
| `WS /ws/watch` | Real-time updates |

---

## JSON Output Example

```bash
cub-scout map --json
```

```json
{
  "cluster": "kind-atk",
  "scannedAt": "2025-12-31T09:10:00Z",
  "gitops": [
    {
      "kind": "Kustomization",
      "name": "apps",
      "namespace": "flux-system",
      "owner": "Flux",
      "ready": true,
      "suspended": false,
      "revision": "main@sha1:abc1234",
      "inventoryCount": 5
    }
  ],
  "workloads": [
    {
      "name": "backend",
      "namespace": "prod",
      "owner": "ConfigHub",
      "ownerRef": "backend-prod",
      "confighub": {
        "unit": "backend",
        "space": "payments-prod",
        "spaceId": "sp-payments-001",
        "revision": "42"
      },
      "ready": true,
      "desired": 3,
      "available": 3,
      "image": "backend:2.4.1"
    }
  ]
}
```

---

## See Also

- [README.md](README.md) — Integration overview
- [argocd-extension/](argocd-extension/) — Working Argo CD extension
- [flux-operator/](flux-operator/) — Working metrics exporter
- [flux9s/](flux9s/) — TUI proposal
