# How-To Guides

Task-based guides for using cub-agent map.

## App Hierarchy: The Big Picture

Before diving into specific tasks, understand how map shows the complete app hierarchy:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         THE APP HIERARCHY                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. REPOS (DRY + Code)                                                      │
│     ├── git@github.com:org/platform-config.git                              │
│     ├── git@github.com:org/app-manifests.git                                │
│     └── oci://registry.example.com/charts                                   │
│         │                                                                   │
│         ▼                                                                   │
│  2. DRY TEMPLATES (GitOps Patterns)                                         │
│     ├── App of Apps (ArgoCD)                                                │
│     │     └── Parent Application → Child Applications                       │
│     ├── ApplicationSets (ArgoCD)                                            │
│     │     └── Generator → Multiple Applications                             │
│     ├── Kustomizations (Flux)                                               │
│     │     └── Base + Overlays per environment                               │
│     └── HelmReleases (Flux/Helm)                                            │
│           └── Chart + Values per environment                                │
│         │                                                                   │
│         ▼                                                                   │
│  3. WET CONFIG (Rendered Data)                                              │
│     ├── ConfigHub Units (source of truth)                                   │
│     │     └── Rendered manifests stored                                     │
│     ├── OCI Artifacts (transport)                                           │
│     │     └── Immutable config packages                                     │
│     └── Git branches/tags (traditional)                                     │
│           └── Rendered output committed                                     │
│         │                                                                   │
│         ▼                                                                   │
│  4. LIVE APPS & RESOURCES                                                   │
│     ├── Namespaces                                                          │
│     ├── Deployments, Services, ConfigMaps                                   │
│     ├── Custom Resources (CRDs)                                             │
│     └── Actual running state in cluster                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## What Map Shows at Each Level

| Level | Map Command | What You See |
|-------|-------------|--------------|
| **1. Repos** | `map trace` → source | Git URL, OCI registry |
| **2. DRY Templates** | `map deployers` | Kustomizations, HelmReleases, Applications |
| **3. WET Config** | `map --hub` | ConfigHub Units, revisions |
| **4. Live Resources** | `map list` | Deployments, Services, actual state |
| **All at once** | `map deep-dive` | Every data source with full details |
| **Inferred model** | `map app-hierarchy` | Units tree with workloads |

## Rich Detail Commands

For maximum cluster insight, use these commands:

```bash
# All cluster data sources with LiveTree (Deployment → ReplicaSet → Pod)
cub-agent map deep-dive

# Inferred ConfigHub model with Units, namespaces, ownership
cub-agent map app-hierarchy

# With ConfigHub context (shows Unit/Space/dependency info)
cub-agent map deep-dive --connected
```

## The Three Maps View

Press `M` in the TUI to see all hierarchies at once:

```
┌─────────────────────┐ ┌─────────────────────┐ ┌─────────────────────┐
│ MAP 1: GitOps Trees │ │ MAP 2: ConfigHub    │ │ MAP 3: Repos        │
├─────────────────────┤ ├─────────────────────┤ ├─────────────────────┤
│ Flux                │ │ Org: mycompany      │ │ platform-config     │
│ ├─ Kustomization    │ │ └─ Space: prod      │ │ ├─ clusters/        │
│ │  └─ Deployments   │ │    ├─ Unit: api     │ │ │  └─ prod/         │
│ │                   │ │    ├─ Unit: web     │ │ └─ apps/            │
│ ArgoCD              │ │    └─ Unit: db      │ │                     │
│ ├─ Application      │ │                     │ │ app-manifests       │
│ │  └─ Deployments   │ │ Space: staging      │ │ └─ services/        │
│ └─ ApplicationSet   │ │ └─ Unit: api        │ │                     │
│    └─ Applications  │ │                     │ │                     │
└─────────────────────┘ └─────────────────────┘ └─────────────────────┘
```

## Guides

| Guide | Purpose |
|-------|---------|
| [ownership-detection.md](ownership-detection.md) | Understand who owns each resource (Level 4) |
| [find-orphans.md](find-orphans.md) | Find resources outside GitOps (Level 4) |
| [trace-ownership.md](trace-ownership.md) | Trace from Live → WET → DRY → Repo (All levels) |
| [scan-for-ccves.md](scan-for-ccves.md) | Find configuration issues (Level 3-4) |
| [query-resources.md](query-resources.md) | Filter across any level |
| [import-to-confighub.md](import-to-confighub.md) | Bring Live into WET (ConfigHub) |

## Hierarchy Examples

### Example 1: Flux Monorepo Pattern

```
REPO: git@github.com:org/platform-config
└─ DRY: clusters/prod/kustomization.yaml
   └─ WET: (rendered at apply time)
      └─ LIVE: Deployment/nginx in prod namespace
```

**Map trace shows:**
```bash
cub-agent map trace deploy/nginx -n prod
# Deployment → Kustomization → GitRepository → git URL
```

### Example 2: ArgoCD App of Apps

```
REPO: git@github.com:org/argocd-apps
└─ DRY: apps/parent-app.yaml (App of Apps)
   └─ DRY: apps/frontend/application.yaml (child)
      └─ REPO: git@github.com:org/frontend-config
         └─ DRY: manifests/
            └─ LIVE: Deployment/frontend
```

**Map shows parent and children separately:**
```bash
cub-agent map list -q "owner=ArgoCD"
# Shows both parent Application and child Applications
```

### Example 3: ConfigHub with OCI Transport

```
REPO: git@github.com:org/app-source
└─ DRY: helm/values-prod.yaml
   └─ WET: ConfigHub Unit "frontend" (store)
      └─ WET: OCI artifact (transport)
         └─ DRY: Flux OCIRepository
            └─ LIVE: Deployment/frontend
```

**Map --hub shows the ConfigHub hierarchy:**
```bash
cub-agent map --hub
# Shows: Org → Space → Unit → Revision → Live resources
```

## Navigating Hierarchies

### From Live to Source (Trace Up)
```bash
cub-agent map trace deploy/myapp -n prod
```

### From Source to Live (Trace Down)
```bash
cub-agent map deployers   # See all GitOps controllers
cub-agent map list        # See what they deployed
```

### Across All Levels
```bash
cub-agent map            # Interactive TUI
# Press M for Three Maps view
```
