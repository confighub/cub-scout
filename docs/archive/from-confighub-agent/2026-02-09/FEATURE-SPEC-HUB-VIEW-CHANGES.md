# Feature Spec: Hub View Changes

## Overview

**Tab Key:** `H` (existing)
**Change:** Filter to current cluster by default

## Problem

Current Hub view shows ALL Units across the entire organization, which:
1. Overwhelms users with irrelevant information
2. Makes it hard to focus on current cluster
3. Doesn't leverage the "I'm already pointed at this cluster" context

## Solution

Hub view defaults to showing only Units deployed to the current cluster. Toggle with 'a' to see all Units.

## Behavior Change

### Current Behavior
- Shows all Units across all Spaces in org
- No filtering by cluster

### New Behavior
- **DEFAULT:** Show only Units with targets on current cluster
- **TOGGLE (key 'a'):** Show all Units in org
- Header shows filter state

## Layout

### Default: This Cluster Only

```
┌─────────────────────────────────────────────────────────────────────┐
│  Connected │ Cluster: prod-east │ Context: eks-prod-east           │
├─────────────────────────────────────────────────────────────────────┤
│  HUB │ Showing: This cluster only │ Press 'a' for all              │
│                                                                     │
│  Space: apptique-prod                                               │
│  ├── payment-api         ✓ Synced    rev.42   → prod-east          │
│  ├── frontend            ✓ Synced    rev.38   → prod-east          │
│  ├── order-service       ✓ Synced    rev.15   → prod-east          │
│  └── inventory           ⚠ Outdated  rev.12   → prod-east          │
│                                                                     │
│  Space: platform-prod                                               │
│  ├── cert-manager        ✓ Synced    rev.8    → prod-east          │
│  └── ingress-nginx       ✓ Synced    rev.5    → prod-east          │
│                                                                     │
│  6 Units on this cluster │ 24 total in org                         │
└─────────────────────────────────────────────────────────────────────┘
```

### Toggled: All Units

```
┌─────────────────────────────────────────────────────────────────────┐
│  Connected │ Cluster: prod-east │ Context: eks-prod-east           │
├─────────────────────────────────────────────────────────────────────┤
│  HUB │ Showing: All Units │ Press 'a' for this cluster             │
│                                                                     │
│  Space: apptique-prod                                               │
│  ├── payment-api         ✓ Synced    rev.42   → prod-east          │
│  ├── frontend            ✓ Synced    rev.38   → prod-east          │
│  ├── order-service       ✓ Synced    rev.15   → prod-east          │
│  ├── inventory           ⚠ Outdated  rev.12   → prod-east          │
│  ├── analytics           ✓ Synced    rev.20   → prod-west          │
│  └── reporting           ✓ Synced    rev.18   → prod-west          │
│                                                                     │
│  Space: apptique-staging                                            │
│  ├── payment-api         ✓ Synced    rev.44   → staging-cluster    │
│  └── frontend            ✓ Synced    rev.39   → staging-cluster    │
│                                                                     │
│  ...                                                                │
│                                                                     │
│  24 Units total │ 6 on this cluster                                 │
└─────────────────────────────────────────────────────────────────────┘
```

## Implementation

### Cluster Detection

```go
func getCurrentCluster() (string, error) {
    // Get current kubectl context
    ctx, err := getKubectlContext()
    if err != nil {
        return "", err
    }

    // Extract cluster name from context
    // Context format varies: "arn:aws:eks:...", "gke_project_zone_cluster", "kind-name"
    return extractClusterName(ctx)
}
```

### Target Matching

```go
func filterUnitsToCluster(units []Unit, clusterName string) []Unit {
    var filtered []Unit
    for _, unit := range units {
        for _, target := range unit.Targets {
            if matchesCluster(target, clusterName) {
                filtered = append(filtered, unit)
                break
            }
        }
    }
    return filtered
}

func matchesCluster(target Target, clusterName string) bool {
    // Exact match
    if target.ClusterName == clusterName {
        return true
    }
    // Partial match (for different naming conventions)
    if strings.Contains(target.ClusterName, clusterName) {
        return true
    }
    if strings.Contains(clusterName, target.ClusterName) {
        return true
    }
    return false
}
```

### State Management

```go
type HierarchyModel struct {
    // ... existing fields

    showAllUnits bool  // false = this cluster only, true = all units
}

func (m *HierarchyModel) handleKeypress(msg tea.KeyMsg) {
    switch msg.String() {
    case "a":
        m.showAllUnits = !m.showAllUnits
        m.reloadUnits()
    }
}

func (m *HierarchyModel) reloadUnits() {
    if m.showAllUnits {
        m.units = m.allUnits
    } else {
        m.units = filterUnitsToCluster(m.allUnits, m.currentCluster)
    }
}
```

### Header Rendering

```go
func (m HierarchyModel) renderHeader() string {
    mode := "Connected"
    filter := "This cluster only"
    toggle := "Press 'a' for all"

    if m.showAllUnits {
        filter = "All Units"
        toggle = "Press 'a' for this cluster"
    }

    return fmt.Sprintf("%s │ Cluster: %s │ Context: %s\n"+
        "HUB │ Showing: %s │ %s",
        mode, m.clusterName, m.contextName,
        filter, toggle)
}
```

## Edge Cases

### No Matching Cluster

If no targets match current cluster:

```
┌─────────────────────────────────────────────────────────────────────┐
│  HUB │ Showing: This cluster only │ Press 'a' for all              │
│                                                                     │
│  ⚠ No Units deployed to this cluster (prod-east)                   │
│                                                                     │
│  Possible reasons:                                                  │
│  • Cluster name mismatch (check Target configurations)              │
│  • Units deployed to different clusters                             │
│  • No targets configured yet                                        │
│                                                                     │
│  Press 'a' to see all 24 Units in org                              │
└─────────────────────────────────────────────────────────────────────┘
```

### Cluster Name Variations

Handle common naming patterns:

| Context | Extracted Cluster |
|---------|-------------------|
| `arn:aws:eks:us-east-1:123:cluster/prod-east` | `prod-east` |
| `gke_myproject_us-central1-a_prod-east` | `prod-east` |
| `kind-prod-east` | `prod-east` |
| `docker-desktop` | `docker-desktop` |
| `minikube` | `minikube` |

## Tests

### Unit Tests

```go
func TestHubView_FilterToCluster(t *testing.T)
func TestHubView_ShowAllToggle(t *testing.T)
func TestHubView_ClusterNameExtraction(t *testing.T)
func TestHubView_TargetMatching(t *testing.T)
func TestHubView_NoMatchingUnits(t *testing.T)
func TestHubView_HeaderDisplay(t *testing.T)
```

### Golden File Tests

```
testdata/hub_view_filtered.golden
testdata/hub_view_all.golden
testdata/hub_view_no_matches.golden
```

## Acceptance Criteria

- [ ] Hub view defaults to showing current cluster only
- [ ] 'a' key toggles between "this cluster" and "all"
- [ ] Header shows current filter state
- [ ] Header shows toggle hint
- [ ] Cluster name extracted correctly from various context formats
- [ ] Handles no matching units gracefully
- [ ] Count shows "X on this cluster | Y total"
- [ ] Toggle state persists during session

## Keymap Addition

| Key | Action | Context |
|-----|--------|---------|
| `a` | Toggle cluster filter | Hub view only |

## Related Documents

- [TUI-PRD.md](TUI-PRD.md) - Overall PRD
- [keybindings.md](../map/reference/keybindings.md) - Full keymap
