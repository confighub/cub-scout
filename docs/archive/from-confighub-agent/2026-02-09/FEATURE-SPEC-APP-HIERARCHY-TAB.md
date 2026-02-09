# Feature Spec: App Hierarchy Tab

## Overview

**Tab Key:** `5` or `A`
**Purpose:** Show TUI's best-effort interpretation of cluster resources in ConfigHub model.

## Problem

Users don't understand:
1. How their cluster resources map to ConfigHub's organizational model
2. What "Hub", "AppSpace", and "Unit" mean in practice
3. How to structure their resources for ConfigHub import
4. The upgrade path from Standalone to Connected

## Solution

A tab that infers and displays:
- Inferred Hub (platform infrastructure)
- Inferred AppSpaces (application environments)
- Inferred Units (deployable components)
- Inferred labels (grouping dimensions)

## Layout

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
│      ├── kube-prometheus   [group: core]                           │
│      ├── kyverno           [group: security]                       │
│      └── grafana           [group: observability]                  │
│                                                                     │
│  INFERRED APPSPACES:                                                │
│  ├── prod (namespace pattern)                                      │
│  │   ├── payment-api       [owner: Flux]                           │
│  │   ├── frontend          [owner: Flux]                           │
│  │   └── hotfix-payment-v2 [owner: Native] ⚠                      │
│  │                                                                  │
│  └── staging (namespace pattern)                                   │
│      └── payment-api       [owner: Flux]                           │
│                                                                     │
│  INFERRED LABELS:                                                   │
│  ├── group: core, security, observability, operations              │
│  ├── team: platform, payments, frontend (from labels)              │
│  └── tier: critical, standard (inferred from namespace)            │
│                                                                     │
│  💡 Import to ConfigHub to make this hierarchy official            │
└─────────────────────────────────────────────────────────────────────┘
```

## Inference Rules

### Hub Inference

A "Hub" is inferred from platform infrastructure patterns:

| Pattern | Example | Inference |
|---------|---------|-----------|
| Flux Kustomization named "infrastructure" | `flux-system/infrastructure` | Hub: infrastructure |
| Argo Application managing platform | `argocd/platform` | Hub: platform |
| Namespace prefix "platform-*" | `platform-monitoring` | Hub: platform |
| Label `confighub.com/hub` | Any resource | Direct mapping |

**Priority:**
1. Explicit `confighub.com/hub` label (highest)
2. Flux Kustomization named "infrastructure"
3. Argo Application with platform pattern
4. Namespace prefix heuristic

### AppSpace Inference

An "AppSpace" is inferred from environment patterns:

| Pattern | Example | Inference |
|---------|---------|-----------|
| Namespace suffix `-prod`, `-staging`, `-dev` | `payments-prod` | AppSpace: prod |
| Namespace prefix `prod-*`, `staging-*` | `prod-payments` | AppSpace: prod |
| Label `environment` or `env` | `env=production` | AppSpace: production |
| Flux Kustomization path | `clusters/prod/apps` | AppSpace: prod |
| Argo Application target cluster | `prod-cluster` | AppSpace: prod |

**Fallback:**
If no pattern matches, group by namespace directly.

### Unit Inference

A "Unit" is inferred from deployable components:

| Pattern | Example | Inference |
|---------|---------|-----------|
| Flux HelmRelease | `payments/api` | Unit: api |
| Argo Application | `argocd/payments-api` | Unit: payments-api |
| Deployment with app label | `app=payment-api` | Unit: payment-api |
| Helm release | `nginx` | Unit: nginx |

**Grouping:**
Resources with the same `app.kubernetes.io/instance` or `app` label are grouped into one Unit.

### Label Inference

Labels are inferred from:

| Source | Label Key | Example |
|--------|-----------|---------|
| K8s labels | `app.kubernetes.io/component` | group: backend |
| K8s labels | `team` | team: payments |
| Namespace patterns | `-prod`, `-critical` | tier: critical |
| Flux/Argo labels | Any | Preserved |

## Visual Indicators

| Indicator | Meaning |
|-----------|---------|
| ✓ | Healthy, synced |
| ⚠ | Warning (orphan, outdated, out of sync) |
| ✗ | Error (failed, crash) |
| `[owner: Flux]` | Managed by Flux |
| `[owner: Native]` | No GitOps owner (orphan) |
| `[group: X]` | Inferred grouping label |

## Confidence Levels

Display confidence for inferences:

```
INFERRED HUB: platform-infrastructure (high confidence)
└── Matches: Flux Kustomization "infrastructure" + namespace pattern

INFERRED APPSPACE: prod (medium confidence)
└── Based on: namespace suffix pattern only
```

| Confidence | Criteria |
|------------|----------|
| High | Multiple patterns match, explicit labels |
| Medium | Single pattern match |
| Low | Heuristic only, no explicit signals |

## Actions

| Key | Action |
|-----|--------|
| `i` | Import selected item to ConfigHub |
| `Enter` | Expand/collapse section |
| `c` | Copy inferred structure as YAML |
| `?` | Show inference rules help |

## Import Flow

When user presses `i`:

```
┌─────────────────────────────────────────────────────────────────────┐
│  IMPORT TO CONFIGHUB                                                │
│                                                                     │
│  Selected: payment-api (inferred Unit)                              │
│                                                                     │
│  Inferred structure:                                                │
│    Hub: platform-infrastructure                                     │
│    AppSpace: prod                                                   │
│    Unit: payment-api                                                │
│    Labels: team=payments, tier=critical                             │
│                                                                     │
│  [Accept] [Modify] [Cancel]                                         │
│                                                                     │
│  ⚠ This will create the Unit in ConfigHub. You can modify the      │
│     structure after import.                                         │
└─────────────────────────────────────────────────────────────────────┘
```

## Implementation

### File: `cmd/cub-agent/app_hierarchy_tab.go`

```go
type AppHierarchyModel struct {
    hub        InferredHub
    appSpaces  []InferredAppSpace
    labels     InferredLabels

    expanded   map[string]bool
    cursor     int
    selected   string
}

type InferredHub struct {
    Name       string
    Source     string  // "Flux Kustomization", "Argo Application", etc.
    Confidence string  // "high", "medium", "low"
    Units      []InferredUnit
}

type InferredAppSpace struct {
    Name       string
    Pattern    string  // "namespace suffix", "label", etc.
    Confidence string
    Units      []InferredUnit
}

type InferredUnit struct {
    Name      string
    Owner     string  // "Flux", "ArgoCD", "Helm", "Native"
    Labels    map[string]string
    Resources []Resource
    IsOrphan  bool
}

type InferredLabels struct {
    Groups []string  // ["core", "security", "observability"]
    Teams  []string  // ["platform", "payments"]
    Tiers  []string  // ["critical", "standard"]
}
```

### Inference Logic

```go
func inferHierarchy(resources []Resource) AppHierarchyModel {
    m := AppHierarchyModel{}

    // 1. Find platform infrastructure (Hub)
    m.hub = inferHub(resources)

    // 2. Group remaining by environment (AppSpaces)
    m.appSpaces = inferAppSpaces(resources)

    // 3. Within each AppSpace, identify Units
    for _, space := range m.appSpaces {
        space.Units = inferUnits(resources, space.Name)
    }

    // 4. Collect label dimensions
    m.labels = inferLabels(resources)

    return m
}

func inferHub(resources []Resource) InferredHub {
    // Check for infrastructure Kustomization
    for _, r := range resources {
        if r.Kind == "Kustomization" && r.Name == "infrastructure" {
            return InferredHub{
                Name:       "platform-infrastructure",
                Source:     "Flux Kustomization",
                Confidence: "high",
            }
        }
    }
    // ... other patterns
}
```

## Tests

### Unit Tests

```go
func TestAppHierarchy_InferHub_FluxKustomization(t *testing.T)
func TestAppHierarchy_InferHub_ArgoApplication(t *testing.T)
func TestAppHierarchy_InferHub_ExplicitLabel(t *testing.T)
func TestAppHierarchy_InferAppSpaces_NamespaceSuffix(t *testing.T)
func TestAppHierarchy_InferAppSpaces_Label(t *testing.T)
func TestAppHierarchy_InferUnits_HelmRelease(t *testing.T)
func TestAppHierarchy_InferUnits_Deployment(t *testing.T)
func TestAppHierarchy_InferLabels(t *testing.T)
func TestAppHierarchy_Confidence(t *testing.T)
```

### Golden File Tests

```
testdata/app_hierarchy_flux_only.golden
testdata/app_hierarchy_argo_only.golden
testdata/app_hierarchy_mixed.golden
testdata/app_hierarchy_with_orphans.golden
```

## Acceptance Criteria

- [x] Tab accessible via `5` or `A` key
- [x] Infers Units from GitOps deployers (Flux, ArgoCD, Helm)
- [x] Infers AppSpaces from namespace/label patterns
- [x] Shows environment inference (production, staging, development)
- [x] Highlights orphan resources (Native workloads)
- [x] Shows inferred label dimensions (component, team, app, tier)
- [x] Shows ownership graph (deployer → workloads tree)
- [x] Disclaimer shown that this uses deterministic rule-based logic (no AI)
- [x] LiveTree shows Deployment → ReplicaSet → Pod hierarchy
- [x] Service dependency inference (shows "calls: X" from env vars)
- [x] CLI equivalent: `cub-agent map app-hierarchy`

## Related Documents

- [TUI-PRD.md](TUI-PRD.md) - Overall PRD
- [FEATURE-SPEC-CLUSTER-DATA-TAB.md](FEATURE-SPEC-CLUSTER-DATA-TAB.md) - Cluster Data tab
- ~/Desktop/RM-TO-CONFIGHUB-MAPPING.md - Repo structure → ConfigHub mapping
