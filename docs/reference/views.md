# TUI Views Reference

Complete reference for all interactive TUI views.

## View Summary

| Key | View | Purpose |
|-----|------|---------|
| `s` | Status | Health dashboard |
| `w` | Workloads | Resources by owner |
| `p` | Pipelines | GitOps deployers |
| `d` | Drift | Out-of-sync resources |
| `o` | Orphans | Native resources |
| `c` | Crashes | Failing workloads |
| `i` | Issues | All problems |
| `u` | Suspended | Paused/stale resources |
| `a` | Apps | Group by app label |
| `D` | Dependencies | Upstream/downstream |
| `b` | Bypass | Factory bypass |
| `x` | Sprawl | Config distribution |
| `G` | Git Sources | Forward trace: Git → deployers → resources |
| `M` | Three Maps | All hierarchies |
| `4` | Cluster Data | All data sources TUI reads |
| `5` / `A` | App Hierarchy | Inferred ConfigHub model |

---

## Status View (`s`)

**Purpose:** Health summary dashboard

**Content:**
- Total resource count
- Healthy vs unhealthy
- Deployer summary (Flux, ArgoCD, Helm counts)
- Recent activity

**Layout:**
```
┌─────────────────────────────────────────────────────────────────┐
│                       CLUSTER STATUS                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Resources: 142    Healthy: 138 (97%)    Issues: 4              │
│                                                                 │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐            │
│  │ Flux      45 │ │ ArgoCD   32 │ │ Helm     28 │            │
│  └──────────────┘ └──────────────┘ └──────────────┘            │
│                                                                 │
│  Native: 3 (orphans)                                            │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Workloads View (`w`)

**Purpose:** All workloads grouped by owner

**Content:**
- Resources organized under owner sections
- Status indicators (✓ healthy, ⚠ warning, ✗ error)
- Namespace and name

**Layout:**
```
┌─────────────────────────────────────────────────────────────────┐
│                        WORKLOADS                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Flux (45 resources)                                            │
│    ✓ deploy/api-gateway           prod                          │
│    ✓ deploy/payment-service       prod                          │
│    ✓ svc/api-gateway              prod                          │
│    ...                                                          │
│                                                                 │
│  ArgoCD (32 resources)                                          │
│    ✓ deploy/frontend              web                           │
│    ⚠ deploy/backend               web         OutOfSync         │
│    ...                                                          │
│                                                                 │
│  Native (3 resources)                                           │
│    ⚠ deploy/debug-pod             prod        Orphan            │
│    ...                                                          │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Pipelines View (`p`)

**Purpose:** GitOps deployers (Flux + ArgoCD)

**Content:**
- Flux: Kustomizations, HelmReleases
- ArgoCD: Applications, ApplicationSets
- Status: Applied/Synced, Suspended, Failed

**Layout:**
```
┌─────────────────────────────────────────────────────────────────┐
│                        PIPELINES                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Flux Kustomizations                                            │
│    ✓ flux-system/apps            Applied     main@abc123        │
│    ✓ flux-system/infrastructure  Applied     main@abc123        │
│    ⚠ flux-system/monitoring      Suspended                      │
│                                                                 │
│  Flux HelmReleases                                              │
│    ✓ monitoring/prometheus       Applied     v2.45.0            │
│    ✓ monitoring/grafana          Applied     v9.5.0             │
│                                                                 │
│  ArgoCD Applications                                            │
│    ✓ argocd/frontend             Synced      HEAD               │
│    ⚠ argocd/backend              OutOfSync   HEAD               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Drift View (`d`)

**Purpose:** Resources diverged from desired state

**Content:**
- Resources where actual != desired
- Drift type: image, config, replica count
- Source reference

**Layout:**
```
┌─────────────────────────────────────────────────────────────────┐
│                         DRIFT                                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  RESOURCE                OWNER      DRIFT                       │
│  deploy/api-gateway      Flux       Image: v1.2.3 → v1.2.4      │
│  cm/app-config           ArgoCD     Key 'timeout' missing       │
│  deploy/frontend         ArgoCD     Replicas: 3 → 2             │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Orphans View (`o`)

**Purpose:** Native (unmanaged) resources

**Content:**
- Resources with no GitOps owner
- Creation timestamp
- kubectl annotations

**Layout:**
```
┌─────────────────────────────────────────────────────────────────┐
│                        ORPHANS                                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ⚠ These resources have no GitOps owner                         │
│                                                                 │
│  RESOURCE            NAMESPACE    CREATED         SOURCE        │
│  deploy/debug-pod    prod         Jan 10 14:30    kubectl       │
│  cm/temp-config      staging      Jan 08 09:15    kubectl       │
│  secret/test-creds   dev          Jan 05 11:00    kubectl       │
│                                                                 │
│  Total: 3 orphan resources                                      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Crashes View (`c`)

**Purpose:** Crashing or failing workloads

**Content:**
- Pods in CrashLoopBackOff
- Deployments with ImagePullBackOff
- Failed jobs
- Restart counts

**Layout:**
```
┌─────────────────────────────────────────────────────────────────┐
│                        CRASHES                                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  RESOURCE                NAMESPACE    STATUS              RESTARTS │
│  ✗ pod/api-worker-xyz    prod         CrashLoopBackOff    5       │
│  ✗ deploy/payment-api    prod         ImagePullBackOff    0       │
│  ✗ job/migration-abc     prod         Failed              0       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Issues View (`i`)

**Purpose:** All resources with problems

**Content:**
- Superset of crashes, drift, orphans
- All unhealthy resources in one view
- Severity indicators

---

## Bypass View (`b`)

**Purpose:** Factory bypass detection

**Content:**
- Resources that bypassed normal deployment pipeline
- Direct kubectl applies to production
- Recommendations for remediation

---

## Sprawl View (`x`)

**Purpose:** Configuration sprawl analysis

**Content:**
- Config distribution by namespace
- Duplication detection
- Consolidation recommendations

---

## Git Sources View (`G`)

**Purpose:** Forward trace from Git to live resources

**Content:**
- GitRepositories, OCIRepositories, HelmRepositories
- Deployers that reference each source
- Resources deployed by each deployer

**Layout:**
```
┌────────────────────────────────────────────────────────────────────────────┐
│ GIT SOURCES → DEPLOYERS → RESOURCES                                        │
│ Forward trace: What does your Git define?                                  │
├────────────────────────────────────────────────────────────────────────────┤
│ Sources: 3 │ Deployers: 8 │ Workloads: 45                                  │
│                                                                            │
│ GIT REPOSITORIES                                                           │
│ ✓ platform-config                                                          │
│   github.com/myorg/platform-config @ main (abc1234)                       │
│   ├─▶ Kustomization/infrastructure → 12 resources                         │
│   └─▶ Kustomization/apps → 28 resources                                   │
│                                                                            │
│ ✓ app-manifests                                                            │
│   github.com/myorg/app-manifests @ main (def5678)                         │
│   └─▶ Kustomization/frontend → 5 resources                                │
│                                                                            │
│ HELM REPOSITORIES                                                          │
│ ✓ bitnami                                                                  │
│   https://charts.bitnami.com/bitnami                                      │
│   └─▶ HelmRelease/postgresql → 4 resources                                │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Suspended View (`u`)

**Purpose:** Find paused/stale GitOps resources

**Content:**
- Flux resources with `suspend: true`
- ArgoCD applications with paused sync
- Resources stale for >7 days

**Layout:**
```
┌────────────────────────────────────────────────────────────────────────────┐
│ SUSPENDED RESOURCES                                                 3 items │
├────────────────────────────────────────────────────────────────────────────┤
│ ⏸  Kustomization/monitoring    flux-system    Suspended 14d ago           │
│ ⏸  HelmRelease/grafana         monitoring     Suspended 3d ago            │
│ ⚠  Application/staging-app     argocd         Stale (no sync 8d)          │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Apps View (`a`)

**Purpose:** Group workloads by application

**Content:**
- Resources grouped by `app` label value
- Variants shown per app (prod, staging, dev)
- Status per variant

**Layout:**
```
┌────────────────────────────────────────────────────────────────────────────┐
│ APPS                                                              12 apps │
├────────────────────────────────────────────────────────────────────────────┤
│ payment-api                                                                │
│   ├─ [prod]    → prod-east (healthy)                                      │
│   ├─ [staging] → staging-cluster (healthy)                                │
│   └─ [dev]     → dev-cluster (syncing)                                    │
│                                                                            │
│ order-service                                                              │
│   ├─ [prod]    → prod-east (healthy), prod-west (healthy)                │
│   └─ [staging] → staging-cluster (degraded)                              │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Dependencies View (`D`)

**Purpose:** Show upstream/downstream dependencies

**Content:**
- Resources this item depends on (upstream)
- Resources depending on this item (downstream)
- Missing dependencies highlighted

**Layout:**
```
┌────────────────────────────────────────────────────────────────────────────┐
│ DEPENDENCIES: flux-system/apps                                             │
├────────────────────────────────────────────────────────────────────────────┤
│ UPSTREAM (depends on):                                                     │
│   ✓ flux-system/infrastructure                                             │
│   ✓ flux-system/cert-manager                                               │
│                                                                            │
│ DOWNSTREAM (depended on by):                                               │
│   → flux-system/monitoring                                                 │
│   → flux-system/ingress                                                    │
│   ⚠ flux-system/broken-app (missing dependency)                           │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Three Maps View (`M`)

**Purpose:** All hierarchies in one view

**Content:**
Three side-by-side panels showing:
1. **GitOps Trees:** Flux + ArgoCD hierarchies
2. **ConfigHub:** Org → Space → Unit hierarchy
3. **Repositories:** Git/OCI sources

**Layout:**
```
┌─────────────────────┐ ┌─────────────────────┐ ┌─────────────────────┐
│ MAP 1: GitOps Trees │ │ MAP 2: ConfigHub    │ │ MAP 3: Repos        │
├─────────────────────┤ ├─────────────────────┤ ├─────────────────────┤
│ Flux                │ │ Org: mycompany      │ │ platform-config     │
│ ├─ Kustomization    │ │ └─ Space: prod      │ │ ├─ clusters/        │
│ │  └─ Deployments   │ │    ├─ Unit: api     │ │ └─ apps/            │
│ │                   │ │    └─ Unit: web     │ │                     │
│ ArgoCD              │ │                     │ │ app-manifests       │
│ └─ Application      │ │ Space: staging      │ │ └─ services/        │
│    └─ Deployments   │ │ └─ Unit: api        │ │                     │
└─────────────────────┘ └─────────────────────┘ └─────────────────────┘
```

---

## Cluster Data View (`4`)

**Purpose:** Show ALL information TUI is reading from the cluster

**Content:**
- Flux CRDs (Kustomizations, HelmReleases, GitRepositories)
- ArgoCD CRDs (Applications, ApplicationSets)
- Helm releases (if secrets accessible)
- Native/orphan resources
- Permissions status

**Layout:**
```
┌─────────────────────────────────────────────────────────────────────┐
│  Standalone │ Cluster: prod-east │ Context: eks-prod-east          │
├─────────────────────────────────────────────────────────────────────┤
│  CLUSTER DATA                                                       │
│                                                                     │
│  FLUX (23 resources)                                     [Expand]  │
│  ├── Kustomizations (4)                                            │
│  ├── HelmReleases (2)                                              │
│  │   └── cert-manager ⚠ Outdated (1.14.4 → 1.14.5)                │
│  ├── GitRepositories (2)                                           │
│  └── HelmRepositories (3)                                          │
│                                                                     │
│  ARGOCD (8 resources)                                    [Expand]  │
│  ├── Applications (8)                                              │
│  └── ApplicationSets (1)                                           │
│                                                                     │
│  HELM (3 releases)                                       [Expand]  │
│  └── nginx ⚠ v14.0.0 (latest: v15.2.0)                            │
│                                                                     │
│  NATIVE (3 orphans)                                      [Expand]  │
│  └── deploy/hotfix-payment-v2 (5 days old)                        │
│                                                                     │
│  PERMISSIONS                                                        │
│  ├── Core resources: ✓                                             │
│  ├── Flux CRDs: ✓                                                  │
│  ├── Argo CRDs: ✓                                                  │
│  └── Helm secrets: ✗ (permission denied)                           │
│                                                                     │
│  💡 Connect to ConfigHub for fleet-wide visibility                 │
└─────────────────────────────────────────────────────────────────────┘
```

**Keybindings:**
| Key | Action |
|-----|--------|
| `Enter` | Expand/collapse section |
| `→` / `l` | Expand section |
| `←` / `h` | Collapse section |

---

## App Hierarchy View (`5` or `A`)

**Purpose:** Show TUI's best-effort interpretation of cluster in ConfigHub model

**Content:**
- Inferred Hub (from infrastructure patterns)
- Inferred AppSpaces (from namespace patterns)
- Inferred Units (from HelmReleases, Applications, Deployments)
- Inferred labels (grouping dimensions)

**Layout:**
```
┌─────────────────────────────────────────────────────────────────────┐
│  Standalone │ Cluster: prod-east │ Context: eks-prod-east          │
├─────────────────────────────────────────────────────────────────────┤
│  APP HIERARCHY (Inferred)                                           │
│                                                                     │
│  ⚠ This is TUI's interpretation. Connect to ConfigHub for actual   │
│     hierarchy.                                                      │
│                                                                     │
│  INFERRED HUB: platform-infrastructure                              │
│  └── Based on: Flux Kustomization "infrastructure"                 │
│      ├── cert-manager      [group: core]                           │
│      ├── ingress-nginx     [group: core]                           │
│      └── kyverno           [group: security]                       │
│                                                                     │
│  INFERRED APPSPACES:                                                │
│  ├── prod (namespace pattern)                                      │
│  │   ├── payment-api       [owner: Flux]                           │
│  │   └── hotfix-payment-v2 [owner: Native] ⚠                      │
│  │                                                                  │
│  └── staging (namespace pattern)                                   │
│      └── payment-api       [owner: Flux]                           │
│                                                                     │
│  INFERRED LABELS:                                                   │
│  ├── group: core, security, observability                          │
│  ├── team: platform, payments (from labels)                        │
│  └── tier: critical, standard (inferred)                           │
│                                                                     │
│  💡 Import to ConfigHub to make this hierarchy official            │
└─────────────────────────────────────────────────────────────────────┘
```

**Keybindings:**
| Key | Action |
|-----|--------|
| `i` | Import selected item to ConfigHub |
| `Enter` | Expand/collapse section |
| `c` | Copy inferred structure as YAML |

---

## ConfigHub Views (--hub mode)

### Hierarchy Navigator

Main view showing ConfigHub structure:
- Organization
  - Spaces
    - Units
      - Revisions
    - Targets
    - Workers

### Activity View (`a`)

Recent activity on ConfigHub resources.

### Details Pane

Right panel showing selected resource details.

---

## View Navigation

| Action | Key |
|--------|-----|
| Switch to view | View letter (`s`, `w`, `p`, etc.) |
| Cycle views | `Tab` |
| Focus details | `Tab` (when in list) |
| Back to list | `Escape` |

## See Also

- [Keybindings](keybindings.md) - All keyboard shortcuts
- [Commands](commands.md) - CLI commands
