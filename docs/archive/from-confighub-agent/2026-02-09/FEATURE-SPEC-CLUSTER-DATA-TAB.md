# Feature Spec: Cluster Data Tab

## Overview

**Tab Key:** `4` or `D`
**Purpose:** Show ALL information TUI is reading from the cluster, organized by source.

## Problem

Users don't know:
1. What data sources TUI is reading
2. Whether Flux/Argo CRDs are accessible
3. Whether Helm release details are available
4. What permissions are needed for full functionality

## Solution

A dedicated tab that transparently shows:
- Every data source being read
- Resource counts per source
- Permissions status
- Upgrade prompts for Connected mode

## Layout

```
┌─────────────────────────────────────────────────────────────────────┐
│  Standalone │ Cluster: prod-east │ Context: eks-prod-east          │
├─────────────────────────────────────────────────────────────────────┤
│  CLUSTER DATA                                                       │
│                                                                     │
│  FLUX (23 resources)                                     [Expand]  │
│  ├── Kustomizations (4)                                            │
│  │   ├── infrastructure ✓ Applied                                  │
│  │   └── apps ✓ Applied                                            │
│  ├── HelmReleases (2)                                              │
│  │   ├── datadog ✓ Applied                                         │
│  │   └── cert-manager ⚠ Outdated (1.14.4 → 1.14.5)                │
│  ├── GitRepositories (2)                                           │
│  └── HelmRepositories (3)                                          │
│                                                                     │
│  ARGOCD (8 resources)                                    [Expand]  │
│  ├── Applications (8)                                              │
│  │   ├── platform-apps (App of Apps)                              │
│  │   │   ├── redis ✓ Synced                                       │
│  │   │   └── rabbitmq ⚠ OutOfSync                                 │
│  └── ApplicationSets (1)                                           │
│                                                                     │
│  HELM (3 releases)                                       [Expand]  │
│  ├── aws-load-balancer-controller ✓ v2.5.0                        │
│  ├── external-dns ✓ v1.14.0                                        │
│  └── nginx ⚠ v14.0.0 (latest: v15.2.0)                            │
│                                                                     │
│  NATIVE (3 orphans)                                      [Expand]  │
│  ├── deploy/hotfix-payment-v2 (5 days old)                        │
│  ├── deploy/debug-shell (3 days old)                              │
│  └── configmap/temp-feature-flag (2 weeks old)                    │
│                                                                     │
│  PERMISSIONS                                                        │
│  ├── Core resources: ✓                                             │
│  ├── Flux CRDs: ✓                                                  │
│  ├── Argo CRDs: ✓                                                  │
│  ├── Helm secrets: ✓ (detailed release info available)            │
│  └── Argo API: ✗ (no ~/.argocd/config found)                      │
│                                                                     │
│  💡 Connect to ConfigHub for fleet-wide visibility                 │
└─────────────────────────────────────────────────────────────────────┘
```

## Data Sources

### Flux CRDs

**API Groups:**
- `kustomize.toolkit.fluxcd.io/v1`
- `helm.toolkit.fluxcd.io/v2`
- `source.toolkit.fluxcd.io/v1`

**Resources:**
| Resource | Purpose | Status Fields |
|----------|---------|---------------|
| Kustomization | Kustomize deployer | `.status.conditions`, `.status.lastAppliedRevision` |
| HelmRelease | Helm deployer | `.status.conditions`, `.spec.chart.spec.version` |
| GitRepository | Git source | `.status.artifact.url`, `.status.conditions` |
| HelmRepository | Helm source | `.status.conditions`, index contains available versions |
| OCIRepository | OCI source | `.status.artifact.url` |

**Outdated Detection:**
```go
// For HelmRelease, compare:
// 1. spec.chart.spec.version (current)
// 2. HelmRepository index (available versions)
// Flag if newer version exists
```

### ArgoCD CRDs

**API Group:** `argoproj.io/v1alpha1`

**Resources:**
| Resource | Purpose | Status Fields |
|----------|---------|---------------|
| Application | Single app deployer | `.status.sync.status`, `.status.health.status` |
| ApplicationSet | Multi-app generator | `.status.conditions` |

**App of Apps Detection:**
```go
// An Application is "App of Apps" if it:
// 1. Has sources pointing to a directory of Application manifests, OR
// 2. Creates other Application resources (check children)
```

### Helm Releases (Secrets)

**Detection:**
```go
// List secrets with label: owner=helm
// Each secret contains:
// - Chart name and version
// - Values used
// - Release status
```

**Permission Required:**
```yaml
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list"]
```

**If Permission Denied:**
```
HELM
├── Requires secrets access for detailed info
└── Grant 'secrets' read permission to enable
```

### Native/Orphan Resources

**Detection:**
Resources with NO ownership labels:
- No `kustomize.toolkit.fluxcd.io/*`
- No `helm.toolkit.fluxcd.io/*`
- No `argocd.argoproj.io/instance`
- No `app.kubernetes.io/managed-by: Helm`
- No `confighub.com/UnitSlug`

**Display:**
- Resource kind and name
- Age (time since creation)
- Last modifier (if available from annotations)

## Permissions Section

Shows what TUI can and cannot access:

| Source | Check Method | Status Display |
|--------|--------------|----------------|
| Core resources | List deployments | ✓ or ✗ |
| Flux CRDs | List Kustomizations | ✓ or ✗ (CRDs not installed) |
| Argo CRDs | List Applications | ✓ or ✗ (CRDs not installed) |
| Helm secrets | List secrets with owner=helm | ✓ or ✗ (permission denied) |
| Argo API | Check ~/.argocd/config | ✓ or ✗ (config not found) |

## Expand/Collapse Behavior

| Key | Action |
|-----|--------|
| `Enter` | Expand/collapse section |
| `→` or `l` | Expand section |
| `←` or `h` | Collapse section |
| `e` | Expand all |
| `E` | Collapse all |

## Implementation

### File: `cmd/cub-agent/cluster_data_tab.go`

```go
type ClusterDataModel struct {
    fluxResources    []FluxResource
    argoResources    []ArgoResource
    helmReleases     []HelmRelease
    nativeResources  []NativeResource
    permissions      PermissionsStatus

    expanded map[string]bool  // section expansion state
    cursor   int
}

type PermissionsStatus struct {
    CoreResources bool
    FluxCRDs      bool
    ArgoCRDs      bool
    HelmSecrets   bool
    ArgoAPI       bool

    FluxError  string  // "CRDs not installed"
    ArgoError  string  // "Permission denied"
    HelmError  string  // "Secrets access denied"
}

func (m ClusterDataModel) View() string {
    // Render header
    // Render each section (Flux, Argo, Helm, Native)
    // Render permissions
    // Render upgrade prompt
}
```

### Startup Sequence

```go
func loadClusterData() ClusterDataModel {
    m := ClusterDataModel{}

    // 1. Try Flux CRDs
    if fluxInstalled() {
        m.fluxResources, err = listFluxResources()
        m.permissions.FluxCRDs = err == nil
        if err != nil {
            m.permissions.FluxError = err.Error()
        }
    }

    // 2. Try Argo CRDs
    if argoInstalled() {
        m.argoResources, err = listArgoResources()
        m.permissions.ArgoCRDs = err == nil
    }

    // 3. Try Helm secrets
    m.helmReleases, err = listHelmSecrets()
    m.permissions.HelmSecrets = err == nil

    // 4. List Native resources (always works)
    m.nativeResources = listNativeResources()

    // 5. Check Argo API (optional enhancement)
    m.permissions.ArgoAPI = checkArgoConfig()

    return m
}
```

## Tests

### Unit Tests

```go
func TestClusterDataTab_FluxResources(t *testing.T)
func TestClusterDataTab_ArgoResources(t *testing.T)
func TestClusterDataTab_HelmReleases(t *testing.T)
func TestClusterDataTab_NativeOrphans(t *testing.T)
func TestClusterDataTab_PermissionsDisplay(t *testing.T)
func TestClusterDataTab_ExpandCollapse(t *testing.T)
func TestClusterDataTab_OutdatedDetection(t *testing.T)
```

### Golden File Tests

```
testdata/cluster_data_tab_flux.golden
testdata/cluster_data_tab_argo.golden
testdata/cluster_data_tab_mixed.golden
testdata/cluster_data_tab_permissions_denied.golden
```

## Acceptance Criteria

- [x] Tab accessible via `4` or `D` key
- [x] Shows Flux resources with status (Kustomizations, HelmReleases, GitRepositories, etc.)
- [x] Shows Argo resources with sync status (Applications, ApplicationSets, AppProjects)
- [x] Shows Helm releases with full details (chart, values, history, hooks)
- [x] Shows Native resources with age
- [x] Detects outdated Helm charts
- [x] Shows permissions status
- [x] Gracefully handles missing permissions
- [x] Expand/collapse works correctly
- [x] Upgrade prompt shown in Standalone mode
- [x] LiveTree shows Deployment → ReplicaSet → Pod hierarchy
- [x] CLI equivalent: `cub-agent map deep-dive`

## Related Documents

- [TUI-PRD.md](TUI-PRD.md) - Overall PRD
- [FEATURE-SPEC-APP-HIERARCHY-TAB.md](FEATURE-SPEC-APP-HIERARCHY-TAB.md) - App Hierarchy tab
- ~/Desktop/TUI-ARCHITECTURE-EXPLAINED.md - How TUI reads cluster data
- ~/Desktop/HELM-UPGRADE.md - Helm upgrade detection
