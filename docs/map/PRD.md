# Map TUI: Product Requirements Document

## Overview

**Product:** cub-scout map
**Version:** 2.0
**Updated:** 2026-01-18

Map is a read-only Kubernetes resource observer that provides ownership visibility, configuration scanning, and GitOps tracing for platform engineers and SREs.

## Problem Statement

Modern Kubernetes clusters have resources deployed by multiple tools:
- Flux CD (Kustomizations, HelmReleases)
- Argo CD (Applications, ApplicationSets)
- Helm (direct installs)
- kubectl (manual deployments)
- ConfigHub (managed units)

**Pain points:**
1. No unified view of "who owns what"
2. Native/manual deployments ("shadow IT") go undetected
3. Configuration issues hide until production incidents
4. Tracing from resource to source requires manual investigation
5. Fleet-wide visibility requires checking each tool's UI

## User Personas

### Platform Engineer (Primary)
- Manages Kubernetes infrastructure
- Deploys common services (ingress, cert-manager, monitoring)
- Needs to know what's running across clusters
- Uses Flux, ArgoCD, or Helm daily

### SRE / Operations
- Responds to incidents
- Needs to trace problems to source quickly
- Wants to find configuration drift
- Needs to identify orphan resources

### DevOps Lead / Decision Maker
- Evaluates tools for team
- Needs to see value quickly
- Wants zero-friction adoption
- Considers upgrade to paid tier

## User Stories

### OSS User (Standalone Mode)
1. As a platform engineer, I want to see all resources and their owners so I know what's deployed
2. As an SRE, I want to find orphan resources so I can identify shadow IT
3. As a platform engineer, I want to trace a resource to its source so I can fix configuration issues
4. As an SRE, I want to scan for configuration anti-patterns before they cause incidents

### Connected User
5. As a platform engineer, I want to see my entire fleet so I can manage multiple clusters
6. As a platform engineer, I want to import existing workloads to ConfigHub so I can manage them centrally
7. As a DevOps lead, I want to see the Hub/AppSpace model so I understand how platform and app teams collaborate

## Feature Specification

### Top-Level Commands (17)

| Command | Purpose | Mode |
|---------|---------|------|
| `map` | Interactive TUI explorer | Standalone/Connected |
| `tree` | Hierarchical views (runtime, ownership, git, etc.) | Standalone/Connected |
| `trace` | Show GitOps ownership chain | Standalone |
| `scan` | Scan for CCVEs | Standalone |
| `discover` | Find workloads by owner (alias for `map workloads`) | Standalone |
| `health` | Check cluster health (alias for `map issues`) | Standalone |
| `snapshot` | Dump cluster state as JSON | Standalone |
| `import` | Import workloads into ConfigHub | Connected |
| `import-argocd` | Import ArgoCD Application | Connected |
| `app-space` | Manage App Spaces | Connected |
| `remedy` | Execute CCVE remediation | Standalone |
| `combined` | Git repo + cluster alignment | Standalone/Connected |
| `parse-repo` | Parse GitOps repo structure | Standalone |
| `demo` | Run interactive demos | Standalone |
| `version` | Print version | Standalone |
| `completion` | Generate shell completions | Standalone |
| `setup` | Set up shell config | Standalone |

### Tree Subcommands (7)

| Command | Purpose | Mode |
|---------|---------|------|
| `tree` / `tree runtime` | Deployment → ReplicaSet → Pod hierarchy | Standalone |
| `tree ownership` | Resources grouped by GitOps owner | Standalone |
| `tree git` | Git source structure from repositories | Standalone |
| `tree patterns` | Detected GitOps patterns (D2, Arnie, etc.) | Standalone |
| `tree config` | ConfigHub Unit relationships (wraps `cub unit tree`) | Connected |
| `tree suggest` | Suggested Hub/AppSpace organization | Standalone |
| `tree workloads` | Workloads by owner (alias for map workloads) | Standalone |

### Map Subcommands (17)

| Command | Purpose | Mode |
|---------|---------|------|
| `map` | Launch interactive TUI | Standalone |
| `map list` | Scriptable resource listing | Standalone |
| `map status` | One-line cluster health | Standalone |
| `map workloads` | Workloads grouped by owner | Standalone |
| `map deployers` | GitOps deployers (Flux + ArgoCD) | Standalone |
| `map orphans` | Unmanaged (Native) resources | Standalone |
| `map crashes` | Crashing pods/deployments | Standalone |
| `map issues` | Resources with problems | Standalone |
| `map drift` | Resources diverged from desired | Standalone |
| `map bypass` | Factory bypass detection | Standalone |
| `map sprawl` | Configuration sprawl analysis | Standalone |
| `map dashboard` | Unified health + ownership | Standalone |
| `map deep-dive` | All cluster data with LiveTree | Standalone |
| `map app-hierarchy` | Inferred ConfigHub model | Standalone |
| `map queries` | Saved queries management | Standalone |
| `map fleet` | Hub/AppSpace model view | Connected |
| `map hub` | ConfigHub hierarchy explorer | Connected |

### TUI Views (17)

| Key | View | Content |
|-----|------|---------|
| `s` | Status/Dashboard | Health summary, deployer counts |
| `w` | Workloads | All workloads grouped by owner |
| `a` | Apps | Grouped by app label + variant |
| `p` | Pipelines | GitOps deployers (Flux/ArgoCD) |
| `d` | Drift | Out-of-sync resources |
| `o` | Orphans | Native (unmanaged) resources |
| `c` | Crashes | Failing pods/deployments |
| `i` | Issues | All unhealthy resources |
| `u` | sUspended | Paused/forgotten resources |
| `b` | Bypass | Factory bypass detection |
| `x` | Sprawl | Configuration sprawl |
| `D` | Dependencies | Upstream/downstream relationships |
| `G` | Git sources | Forward trace from Git |
| `4` | Cluster Data | All data sources TUI reads |
| `5`/`A` | App Hierarchy | Inferred ConfigHub model |
| `M` | Maps | Three Maps view (GitOps + ConfigHub + repos) |
| `H` | Hub | ConfigHub hierarchy (connected mode) |

### Ownership Detection

| Owner | Detection Method | Labels |
|-------|------------------|--------|
| Flux | Toolkit labels | `kustomize.toolkit.fluxcd.io/*`, `helm.toolkit.fluxcd.io/*` |
| ArgoCD | Both required | `app.kubernetes.io/instance` AND `argocd.argoproj.io/instance` |
| Helm | Managed-by | `app.kubernetes.io/managed-by: Helm` |
| ConfigHub | Unit slug | `confighub.com/UnitSlug` |
| Native | None detected | No GitOps ownership labels |

### Query Language

```
field=value           # Exact match
field!=value          # Not equal
field~=pattern        # Regex match
field=val1,val2       # IN list
field=prefix*         # Wildcard
AND / OR              # Logical operators

Fields: kind, namespace, name, owner, status, cluster, labels[key]
```

### Saved Queries

| Query | Filter |
|-------|--------|
| `all` | All resources |
| `orphans` | `owner=Native` |
| `gitops` | `owner!=Native` |
| `flux` | `owner=Flux` |
| `argo` | `owner=ArgoCD` |
| `helm` | `owner=Helm` |
| `prod` | `namespace=*-prod,prod-*,production` |
| `dev` | `namespace=*-dev,dev-*,development` |

### Wizards

| Wizard | Trigger | Purpose |
|--------|---------|---------|
| Import | `i` key | Bring Kubernetes workloads into ConfigHub |
| Create | `c` key | Create new space/unit/target |
| Delete | `d`/`x` key | Delete resources with confirmation |

### Command Palette

| Key | Feature |
|-----|---------|
| `:` | Open command palette (run shell commands) |
| `?` | Show help overlay |

Command palette supports:
- Shell commands (`kubectl`, `flux`, `argocd`, etc.)
- Command history (↑/↓ to navigate, last 20 commands)
- Inline output display (max 8 lines)

## Success Metrics

### Adoption
- Time to first value: < 5 minutes (build + run + see results)
- Zero-friction: no config, no account required for standalone mode

### User Value
- Ownership detection accuracy: 100% for labeled resources
- Query response time: < 1 second for 1000 resources
- CCVE scan coverage: 46 active patterns

### Upgrade Path
- Connect rate: OSS users who connect to ConfigHub
- Import rate: Connected users who import workloads
- Paid conversion: Connected users who upgrade

## Technical Constraints

### Read-Only by Default
Core operations (`map`, `list`, `trace`, `scan`) are read-only:
- Only use `get`, `list`, `watch` Kubernetes verbs
- No modifications without explicit user action
- Exception: `import` wizard can modify when requested

### Dependencies
- Go 1.24+
- kubectl access to cluster
- cub CLI (for connected mode)
- Optional: Flux, ArgoCD, Helm (for ownership detection)

### Performance
- Resource limit: 10,000 resources per cluster
- Startup time: < 2 seconds
- Memory: < 100MB for typical clusters

## Completed Features

### Phase 1 (Complete)
- Ownership detection (Flux, ArgoCD, Helm, ConfigHub, Native)
- TUI views (status, workloads, pipelines, orphans, crashes, issues)
- CCVE scanning (46 active patterns)
- Query language with saved queries
- Trace command (forward and reverse)

### Phase 2 (Complete)
- **Cluster Data tab** (`4`) — All data sources with LiveTree (Deployment → ReplicaSet → Pod)
- **App Hierarchy tab** (`5`/`A`) — Inferred ConfigHub model
- **Mode indicator** — Header shows Standalone vs Connected
- **Hub view filter** — Default to current cluster, toggle for all
- **Additional views** — Apps, sUspended, Bypass, Sprawl, Dependencies, Git sources, Maps
- **Command palette** (`:`) — Run shell commands inline with history
- **Help overlay** (`?`) — Full keybinding reference

### Phase 3 (In Progress)
- Remedy command — Execute auto-remediation for CCVEs
- Combined command — Git + cluster alignment
- Parse-repo command — Analyze GitOps repo structure

## Future Roadmap

### Phase 4 (Planned)
- AI-powered trace ("why did this fail?")
- OCI source detection (Rendered Manifest pattern)
- Bridge pattern detection

### Phase 5 (Future)
- Apps/Actions integration
- Custom remediation
- Enterprise RBAC

## Reference Architectures

Map is tested against these patterns:

| Pattern | Example | Status |
|---------|---------|--------|
| Monorepo + Kustomize | apptique/flux-monorepo | Tested |
| Multi-repo + ApplicationSets | IITS examples | Tested |
| App of Apps | apptique/argo-app-of-apps | Tested |
| Helm umbrella | confighub/examples | Tested |
| Mixed Flux + ArgoCD | — | Tested |

## Appendix: DRY → WET → Live Model

```
   DRY (Source)              WET (Rendered)            LIVE (Cluster)
┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐
│ Helm charts     │      │ ConfigHub       │      │ Kubernetes      │
│ Kustomizations  │ ──▶  │ (store: Units)  │ ──▶  │ (actual state)  │
│ Terraform       │      │ OCI transport   │      │ Flux/Argo apply │
└─────────────────┘      └─────────────────┘      └─────────────────┘
```

**OSS TUI:** Shows LIVE only (single cluster)
**Connected:** Shows full DRY → WET → LIVE chain
