# TUI Tiered Features: Product Requirements Document

## Overview

**Product:** cub-agent map (TUI)
**Version:** 2.0
**Updated:** 2026-01-16

Extends the TUI with tiered features that clearly communicate:
1. **What mode we're in** (Standalone vs Connected)
2. **What we're reading** from the cluster (all sources)
3. **Interpreted hierarchy** (app hierarchy in ConfigHub model)

## Problem Statement

Current TUI lacks clarity around:
1. Whether connected to ConfigHub or running standalone
2. What data sources are being read (Flux CRDs? Argo? Helm secrets?)
3. How cluster resources map to ConfigHub's organizational model
4. Which features require Connected mode vs work Standalone

## Three-Tier Model

| Tier | Mode | Capabilities |
|------|------|--------------|
| **Standalone** | OSS, single cluster | Read labels, detect ownership, Risk scan |
| **Connected** | + ConfigHub.com | Fleet queries, workers read from Flux/Argo |
| **Full Product** | + GUI | Edit DRY source, preview diffs, rollback |

## Feature Specifications

### 1. Header Mode Indicator

**Requirement:** Every tab MUST display:
- Mode: "Standalone" or "Connected to ConfigHub.com"
- Cluster name (from kubectl context)
- Current kubectl context

**Implementation:**
```
┌─────────────────────────────────────────────────────────────────────┐
│  Standalone │ Cluster: prod-east │ Context: eks-prod-east          │
├─────────────────────────────────────────────────────────────────────┤
```

**Acceptance Criteria:**
- [x] Header visible on all tabs (implemented in `renderModeHeader()`)
- [x] Mode updates when connecting/disconnecting
- [x] Cluster name extracted from kubeconfig

### 2. Cluster Data Tab (Key: 4 or 'D')

**Requirement:** New tab showing ALL information TUI reads from cluster.

**Content:**
- Flux CRDs (Kustomizations, HelmReleases, GitRepositories, etc.)
- ArgoCD CRDs (Applications, ApplicationSets)
- Helm releases (from secrets if accessible)
- Native/orphan resources
- Permissions status

**See:** [FEATURE-SPEC-CLUSTER-DATA-TAB.md](FEATURE-SPEC-CLUSTER-DATA-TAB.md)

### 3. App Hierarchy Tab (Key: 5 or 'A')

**Requirement:** New tab showing TUI's interpretation of cluster in ConfigHub model.

**Content:**
- Inferred Hub (from infrastructure Kustomization/Applications)
- Inferred AppSpaces (from namespace patterns)
- Inferred labels (from resource labels)
- Mapping to ConfigHub org-hub-appspace-units model

**See:** [FEATURE-SPEC-APP-HIERARCHY-TAB.md](FEATURE-SPEC-APP-HIERARCHY-TAB.md)

### 4. Hub View Filter

**Requirement:** Hub view ('H') filters to current cluster by default.

**Behavior:**
- Default: Show only Units deployed to current cluster
- Toggle ('a'): Show all Units in org
- Header shows: "Showing: This cluster only │ Press 'a' for all"

**See:** [FEATURE-SPEC-HUB-VIEW-CHANGES.md](FEATURE-SPEC-HUB-VIEW-CHANGES.md)

## User Stories

### Standalone User
1. As a platform engineer, I want to see what data sources TUI is reading so I understand what permissions it needs
2. As an SRE, I want to see how my cluster maps to the ConfigHub model so I understand the upgrade path
3. As a platform engineer, I want to know if Helm secrets are accessible so I can see detailed release info

### Connected User
4. As a platform engineer, I want Hub view to focus on my current cluster so I see relevant units
5. As an SRE, I want to toggle between "this cluster" and "all units" so I can compare fleet-wide

## Success Criteria

| Criterion | Measurement |
|-----------|-------------|
| Mode clarity | User knows within 1 second if connected |
| Data source visibility | All sources listed with counts |
| Permissions visibility | Missing permissions clearly shown |
| Hierarchy inference | 80%+ of resources correctly inferred |
| Hub filter | Default shows current cluster only |

## Technical Constraints

### Auth Requirements

| Feature | Auth Required |
|---------|---------------|
| Core resources | kubeconfig (standard kubectl access) |
| Flux CRDs | kubeconfig + Flux CRD read |
| Argo CRDs | kubeconfig + Argo CRD read |
| Helm secrets | kubeconfig + secrets read (optional) |
| Connected mode | kubeconfig + `cub auth login` |

### Graceful Degradation

TUI MUST work with minimal permissions:
1. Try to read each data source
2. If permission denied, note it in Permissions section
3. Continue with available data
4. Never fail startup due to missing optional permissions

### RBAC Requirements

```yaml
# Minimal (always needed)
- apiGroups: ["", "apps"]
  resources: ["deployments", "statefulsets", "services", "configmaps"]
  verbs: ["get", "list", "watch"]

# Flux CRDs (optional, enhances detection)
- apiGroups: ["kustomize.toolkit.fluxcd.io", "helm.toolkit.fluxcd.io", "source.toolkit.fluxcd.io"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]

# Argo CRDs (optional, enhances detection)
- apiGroups: ["argoproj.io"]
  resources: ["*"]
  verbs: ["get", "list", "watch"]

# Helm secrets (optional, enables release details)
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list"]
```

## Keymap Changes

| Key | Current | New |
|-----|---------|-----|
| `4` or `D` | — | Cluster Data tab |
| `5` or `A` | Apps view | App Hierarchy tab (rename) |
| `a` (in Hub) | — | Toggle "this cluster" / "all units" |

## Related Documents

- [FEATURE-SPEC-CLUSTER-DATA-TAB.md](FEATURE-SPEC-CLUSTER-DATA-TAB.md)
- [FEATURE-SPEC-APP-HIERARCHY-TAB.md](FEATURE-SPEC-APP-HIERARCHY-TAB.md)
- [FEATURE-SPEC-HUB-VIEW-CHANGES.md](FEATURE-SPEC-HUB-VIEW-CHANGES.md)
- [UXBOW-TESTING-STRATEGY.md](UXBOW-TESTING-STRATEGY.md)
- [TUI-GUI-UNIFIED-PRODUCT.md](TUI-GUI-UNIFIED-PRODUCT.md)
