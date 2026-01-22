# New User Journey & Meaningful Examples Plan

**Status:** DRAFT
**Date:** 2026-01-22
**Goal:** Make cub-scout the BEST tool for learning GitOps through exploration

---

## The Problem

New users face multiple challenges:

1. **GitOps is confusing** - What's a Kustomization? How does Flux work? What's App-of-Apps?
2. **Existing tools assume knowledge** - kubectl shows resources, not relationships
3. **Examples are toys** - 5 resources in 1 namespace doesn't teach real patterns
4. **Import is scary** - "I don't understand what I'm importing"

## The Opportunity

**cub-scout can be the best GitOps learning tool** — not just a data viewer.

When a newcomer runs `cub-scout map`, they should LEARN what GitOps is, not just see a list. We **demystify what is going on in GitOps** for users who find many parts obscure or hard to see and understand.

GitOps is powerful but opaque:
- "Where did this Deployment come from?" → Hidden in Kustomization chains
- "Why isn't my change applying?" → Buried in reconciliation status
- "What depends on what?" → No visibility without digging
- "Is this managed by Git or was it kubectl'd?" → Labels you have to know to check

cub-scout makes the invisible visible by:
- Showing relationships, not just resources
- Explaining concepts in context ("This Deployment is managed by Flux via...")
- Providing meaningful examples with real complexity
- Guiding the journey from "I have a cluster" to "I understand and manage it"

---

## Part 1: Learning Mode for CLI

### Concept: `--explain` Flag

Add `--explain` flag to key commands that teaches concepts as it shows data.

#### `cub-scout map list --explain`

```
GITOPS OWNERSHIP EXPLAINED
════════════════════════════════════════════════════════════════════
cub-scout detects who manages each resource by reading labels.

FLUX resources have labels like:
  kustomize.toolkit.fluxcd.io/name: my-app
  kustomize.toolkit.fluxcd.io/namespace: flux-system

ARGOCD resources have labels like:
  app.kubernetes.io/instance: my-app
  argocd.argoproj.io/instance: my-app

HELM resources have:
  app.kubernetes.io/managed-by: Helm

NATIVE means no GitOps tool claims ownership (kubectl-applied).
════════════════════════════════════════════════════════════════════

NAMESPACE           KIND           NAME                    OWNER
boutique            Deployment     frontend                Flux
...

WHAT THIS MEANS:
• 28 resources are managed by Flux → Changes flow from Git automatically
• 12 resources are managed by Helm → Installed via helm install/upgrade
• 7 resources are Native → Applied manually, no Git source

NEXT STEPS:
→ See the Git→Deployment chain: cub-scout trace deploy/frontend -n boutique
→ See the full Flux pipeline: cub-scout map deployers
```

#### `cub-scout trace --explain`

```
OWNERSHIP CHAIN EXPLAINED
════════════════════════════════════════════════════════════════════
GitOps creates a chain from Git to running pods:

  Git Repository (source of truth)
       ↓ Flux watches for changes
  Kustomization (applies manifests)
       ↓ Creates/updates
  Deployment (desired state)
       ↓ K8s controller creates
  ReplicaSet → Pods (running containers)

When you change Git, Flux automatically propagates the change.
════════════════════════════════════════════════════════════════════

TRACE: Deployment/frontend in boutique

  ✓ GitRepository/boutique
    │ URL: https://github.com/stefanprodan/podinfo
    │
    │ ℹ️  This is where your code lives. Flux watches this repo.
    │
    └─▶ ✓ Kustomization/frontend
          │ Path: ./kustomize
          │
          │ ℹ️  This tells Flux which files to apply and how to customize them.
          │
          └─▶ ✓ Deployment/frontend
                │
                │ ℹ️  The Deployment manages your pods. It's what Kustomize created.
                │
                └─▶ ReplicaSet/frontend-7d4b8c → 3 Pods running

WHAT THIS MEANS:
• To change this app, edit files in the Git repo at ./kustomize
• Flux will detect the change and apply it automatically
• No need to run kubectl apply manually
```

### Concept: `cub-scout learn` Command

A dedicated command for learning GitOps concepts.

```bash
cub-scout learn gitops     # What is GitOps? Interactive explanation
cub-scout learn flux       # How Flux works with live cluster examples
cub-scout learn argocd     # How ArgoCD works with live cluster examples
cub-scout learn kustomize  # What is Kustomize? Base + overlays explained
cub-scout learn helm       # Helm releases, charts, values
cub-scout learn ownership  # How cub-scout detects ownership
```

Each lesson:
1. Explains the concept
2. Shows examples from YOUR cluster (if available)
3. Suggests commands to try
4. Links to documentation

---

## Part 2: Meaningful Examples

### Current State

| Example | Services | Layers | Helm | Kustomize | Realistic? |
|---------|----------|--------|------|-----------|------------|
| flux-boutique | 5 | 1 | No | No | ❌ Toy |
| apptique-examples/flux-monorepo | 1 | 2 (base+overlay) | No | Yes | ⚠️ Simple |
| apptique-examples/argo-appset | 1 | 1 | No | No | ⚠️ Simple |
| Google Online Boutique | 11 | 1 | No | No | ⚠️ Flat |

### What "Meaningful" Looks Like

A realistic GitOps repo has:

```
my-platform/
├── infrastructure/           # Cluster-wide resources
│   ├── sources/              # GitRepositories, HelmRepositories
│   ├── rbac/                 # ClusterRoles, ServiceAccounts
│   └── monitoring/           # Prometheus, Grafana (Helm)
│
├── apps/                     # Application workloads
│   ├── frontend/
│   │   ├── base/             # Common configuration
│   │   └── overlays/
│   │       ├── dev/          # Dev-specific patches
│   │       ├── staging/      # Staging config
│   │       └── prod/         # Prod config (more replicas, resources)
│   │
│   ├── backend/
│   │   ├── base/
│   │   └── overlays/...
│   │
│   └── database/             # PostgreSQL via Helm
│       ├── base/
│       │   └── helmrelease.yaml
│       └── overlays/
│           ├── dev/          # Small instance
│           └── prod/         # HA configuration
│
├── clusters/
│   ├── dev/
│   │   └── kustomization.yaml  # Points to apps/*/overlays/dev
│   ├── staging/
│   │   └── kustomization.yaml
│   └── prod/
│       └── kustomization.yaml
```

### Proposed: `platform-example`

A complete, realistic platform example:

| Component | Implementation | Teaches |
|-----------|----------------|---------|
| **Frontend** | Kustomize base + 3 overlays | Multi-environment deployment |
| **Backend API** | Kustomize base + overlays | Service dependencies |
| **PostgreSQL** | Flux HelmRelease | Helm charts via GitOps |
| **Redis** | Flux HelmRelease | Caching layer |
| **Prometheus** | Kustomize + upstream | Monitoring stack |
| **Ingress** | NGINX Helm chart | External access |

**Resource count:** ~50 resources across 5 namespaces

**What it demonstrates:**
1. Multi-layer Kustomize (base → overlay → cluster)
2. Helm charts managed by Flux HelmRelease
3. Dependencies between apps (frontend → backend → database)
4. Environment promotion (dev → staging → prod)
5. Infrastructure vs applications separation
6. **The Clobbering Problem** - Real-world GitOps pitfall

### Clobbering Scenario (Teaching Moment)

The platform-example includes a deliberate "clobbering" scenario:

```yaml
# PostgreSQL deployed via HelmRelease
# values.yaml sets: maxConnections: 100

# But someone "broke glass" and ran:
kubectl patch configmap postgres-config -n prod \
  --patch '{"data":{"max_connections":"500"}}'

# cub-scout shows the danger:
$ cub-scout map orphans
⚠️  ConfigMap/postgres-config has live drift
    Git: max_connections=100
    Live: max_connections=500
    Next Flux reconciliation will RESET to 100!

$ cub-scout trace deploy/postgres -n prod --explain
# Shows the full layer stack and where clobbering can occur
```

**Learning outcome:** Users understand why direct `kubectl` changes are risky in GitOps, and how cub-scout makes this visible BEFORE the clobbering happens.

See: `docs/diagrams/clobbering-problem.d2` for visual explanation

### Upgrade Tracing Scenario (Teaching Moment)

When helm charts upgrade, users struggle to trace what changed through the layers:

```
Monday:    Everything works
Tuesday:   Helm chart upgraded (14.0 → 15.0)
Wednesday: Production OOMing. What changed?

$ cub-scout trace deploy/postgresql -n prod --diff

CHANGE DETECTED: HelmRelease/postgresql
├── Chart: 14.0.0 → 15.0.0
├── Upstream changes:
│   - maxConnections: 100 → 150
│   - resources.memory: 256Mi → 512Mi
│
└── Your values didn't override these.
    Consider adding to values-prod.yaml:
      maxConnections: 100
```

**Without cub-scout:** Git archaeology through repo tree, overlay, and chart mix. 30-60 minutes.
**With cub-scout:** Layer-by-layer diff showing exactly what changed. 5 seconds.

See: `docs/diagrams/upgrade-tracing.d2` for visual explanation

---

## Part 3: Guided Import Journey

### Current Import Experience

```bash
cub-scout import -n production
# Shows: list of resources
# User: "I don't know what I'm importing or why"
```

### Proposed Import Experience

```bash
cub-scout import --wizard
```

```
IMPORT WIZARD
════════════════════════════════════════════════════════════════════

STEP 1: Discover Your Cluster
────────────────────────────────────────────────────────────────────

Scanning cluster for GitOps patterns...

Found:
  • 3 Flux Kustomizations managing 15 Deployments
  • 2 Helm Releases (PostgreSQL, Redis)
  • 5 Native resources (no GitOps owner)

STEP 2: Understand the Structure
────────────────────────────────────────────────────────────────────

Detected patterns:

  [App: frontend]
  ├── Flux Kustomization: frontend (flux-system)
  ├── Deployment: frontend (production)
  ├── Service: frontend (production)
  └── Ingress: frontend-ingress (production)

  [App: backend]
  ├── Flux Kustomization: backend (flux-system)
  ├── Deployment: backend-api (production)
  ├── Deployment: backend-worker (production)
  └── Service: backend-api (production)

  [Database: PostgreSQL]
  ├── Helm Release: postgresql (production)
  ├── StatefulSet: postgresql (production)
  └── Service: postgresql (production)

STEP 3: Map to ConfigHub
────────────────────────────────────────────────────────────────────

Suggested ConfigHub structure:

  Space: production
  ├── Unit: frontend          (from Kustomization/frontend)
  ├── Unit: backend           (from Kustomization/backend)
  └── Unit: postgresql        (from HelmRelease/postgresql)

  Dependencies detected:
  • frontend → backend (service reference)
  • backend → postgresql (DATABASE_URL env var)

Do you want to:
  [1] Import all as suggested
  [2] Customize the structure
  [3] Import one app at a time
  [4] Cancel and explore more first
```

### Import from Multi-Layer Repo

When importing from a real repo structure:

```bash
cub-scout import --from-repo https://github.com/myorg/platform
```

```
REPOSITORY ANALYSIS
════════════════════════════════════════════════════════════════════

Detected structure: Flux Monorepo with Kustomize Overlays

  infrastructure/
    └── monitoring/ (HelmRelease: prometheus, grafana)

  apps/
    ├── frontend/
    │   ├── base/           → shared configuration
    │   └── overlays/
    │       ├── dev/        → ConfigHub Space: frontend-dev
    │       ├── staging/    → ConfigHub Space: frontend-staging
    │       └── prod/       → ConfigHub Space: frontend-prod
    │
    └── backend/
        ├── base/
        └── overlays/...

PROPOSED CONFIGHUB STRUCTURE:

  Hub: platform
  ├── Space: infrastructure
  │   └── Unit: monitoring
  │
  ├── Space: frontend-dev
  │   └── Unit: frontend (variant: dev)
  ├── Space: frontend-staging
  │   └── Unit: frontend (variant: staging)
  └── Space: frontend-prod
      └── Unit: frontend (variant: prod)

This maps your Git structure to ConfigHub's Hub/Space/Unit model.
```

---

## Part 4: In-TUI Learning

### Concept Tooltips in TUI

When hovering over or selecting items in the TUI:

```
┌─ cub-scout map ───────────────────────────────────────────────────┐
│ WORKLOADS BY OWNER                                                 │
│                                                                    │
│ Flux (28)                                                          │
│ > ▶ frontend          production    Deployment  ✓                  │
│     backend-api       production    Deployment  ✓                  │
│                                                                    │
│ ┌─ INFO ─────────────────────────────────────────────────────────┐ │
│ │ FLUX OWNERSHIP                                                 │ │
│ │                                                                │ │
│ │ This Deployment is managed by Flux via:                        │ │
│ │   Kustomization: frontend (flux-system)                        │ │
│ │   GitRepository: platform (flux-system)                        │ │
│ │                                                                │ │
│ │ Changes to this resource should be made in Git, not kubectl.   │ │
│ │                                                                │ │
│ │ Press T to trace the full ownership chain                      │ │
│ │ Press ? for more help                                          │ │
│ └────────────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────────┘
```

### Contextual Help

Press `?` on any resource type for contextual help:

```
WHAT IS A KUSTOMIZATION?
════════════════════════════════════════════════════════════════════

A Kustomization is a Flux resource that tells Flux which files to
apply from a Git repository.

  apiVersion: kustomize.toolkit.fluxcd.io/v1
  kind: Kustomization
  metadata:
    name: frontend
  spec:
    path: ./apps/frontend/overlays/prod  ← Directory in Git
    sourceRef:
      kind: GitRepository
      name: platform                      ← Which Git repo

HOW IT WORKS:
1. Flux watches the GitRepository for changes
2. When Git changes, Flux reads the Kustomization
3. Flux applies all YAML files from the specified path
4. Your cluster stays in sync with Git automatically

THIS KUSTOMIZATION:
• Source: platform (https://github.com/myorg/platform)
• Path: ./apps/frontend/overlays/prod
• Manages: 4 resources (Deployment, Service, ConfigMap, Ingress)

Press Enter to dismiss, T to trace, or ? for more help
```

---

## Implementation Phases

### Phase 1: Meaningful Example (Week 1)

Create `examples/platform-example/`:
- 5 apps with Kustomize base + overlays
- 2 Helm releases (PostgreSQL, Redis)
- Infrastructure layer (monitoring, ingress)
- 50+ resources across 5 namespaces
- Full documentation with learning journey

### Phase 2: `--explain` Flag (Week 2)

Add to key commands:
- `map list --explain`
- `map workloads --explain`
- `trace --explain`
- `scan --explain`

### Phase 3: `cub-scout learn` Command (Week 3)

Interactive learning:
- `learn gitops`
- `learn flux`
- `learn argocd`
- `learn kustomize`
- `learn helm`

### Phase 4: Enhanced Import Wizard (Week 4)

- Repo structure detection
- Multi-layer understanding
- Dependency inference
- Hub/Space/Unit mapping

---

## Success Criteria

1. **New user can understand GitOps in 30 minutes** using cub-scout + examples
2. **Every command teaches** through `--explain` mode
3. **Examples are realistic** (50+ resources, multi-layer, Helm + Kustomize)
4. **Import understands structure** not just individual resources
5. **No assumed knowledge** - explanations available everywhere

---

## Appendix: Learning Journey Outline

### Journey 1: "I have a cluster, what's in it?"

```bash
# Step 1: Quick overview
cub-scout map status
# Output: "45 resources | Flux: 28 | Helm: 12 | Native: 5"

# Step 2: See the structure
cub-scout map workloads
# Shows deployments grouped by owner

# Step 3: Understand one app
cub-scout trace deploy/frontend -n production
# Shows Git → Kustomization → Deployment → Pods

# Step 4: Deep dive
cub-scout map deep-dive
# Shows everything with full detail
```

### Journey 2: "I want to learn GitOps"

```bash
# Step 1: Deploy the platform example
kubectl apply -k examples/platform-example/clusters/dev

# Step 2: Learn what was created
cub-scout learn flux

# Step 3: Explore interactively
cub-scout map
# Press ? for help, explore tabs

# Step 4: Make a change in Git, watch it propagate
# (guided in documentation)
```

### Journey 3: "I want to manage this in ConfigHub"

```bash
# Step 1: Understand current state
cub-scout map app-hierarchy

# Step 2: Import with wizard
cub-scout import --wizard

# Step 3: Verify in ConfigHub
cub-scout map --hub
```
