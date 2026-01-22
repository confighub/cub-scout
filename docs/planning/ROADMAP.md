# cub-scout Roadmap

**Last Updated:** 2026-01-22

This document consolidates the product roadmap for cub-scout, organized as linear implementation phases.

---

## Completed Work

### Documentation & Diagrams (Done)

| Item | Description | Status |
|------|-------------|--------|
| README positioning | Navigation-first "Demystify GitOps" tagline | ✅ Done |
| Problem framing | What's obscure about GitOps | ✅ Done |
| SCALE-DEMO | Navigation focus | ✅ Done |
| Product plan | `planning/PRODUCT-PLAN-LAUNCH.md` | ✅ Done |
| D2: Flux architecture | `docs/diagrams/flux-architecture.d2` | ✅ Done |
| D2: Ownership trace | `docs/diagrams/ownership-trace.d2` | ✅ Done |
| D2: Kustomize overlays | `docs/diagrams/kustomize-overlays.d2` | ✅ Done |
| D2: Ownership detection | `docs/diagrams/ownership-detection.d2` | ✅ Done |
| D2: Clobbering problem | `docs/diagrams/clobbering-problem.d2` | ✅ Done |
| D2: Upgrade tracing | `docs/diagrams/upgrade-tracing.d2` | ✅ Done |
| SVG renders | All D2 diagrams rendered to SVG | ✅ Done |

---

## Phase 1: CLI UX Polish (Priority: P1)

Make existing commands more helpful with headers, summaries, and next steps.

### 1.1 `map orphans` — Add Context Header

**Problem:** Users see raw data without understanding why orphans matter.

**Before:**
```
NAMESPACE           KIND           NAME                    OWNER
argocd              Application    api-gateway             Native
argocd              StatefulSet    argocd-application-controller   Native
...
```

**After:**
```
ORPHAN RESOURCES
════════════════════════════════════════════════════════════════════
Resources not managed by GitOps (Flux, ArgoCD, Helm, ConfigHub).
These may be: legacy systems, manual hotfixes, debug pods, or shadow IT.

NAMESPACE           KIND           NAME                    OWNER
argocd              Application    api-gateway             Native
...

Total: 45 orphan resources across 8 namespaces

→ To import into ConfigHub: cub-scout import --wizard
→ To trace ownership: cub-scout trace <kind>/<name> -n <namespace>
```

**Implementation:**
```go
// In runMapOrphans, before calling runMapList:
if !mapJSON && !mapCount && !mapNamesOnly {
    fmt.Println(orphanHeaderStyle.Render("ORPHAN RESOURCES"))
    fmt.Println(strings.Repeat("═", 68))
    fmt.Println(dimStyle.Render("Resources not managed by GitOps (Flux, ArgoCD, Helm, ConfigHub)."))
    fmt.Println(dimStyle.Render("These may be: legacy systems, manual hotfixes, debug pods, or shadow IT."))
    fmt.Println()
}
```

**Files:** `cmd/cub-scout/map.go`

---

### 1.2 `map issues` — Add Next Steps

**After:**
```
✗ Kustomization/payment-api in break-glass-demo: ArtifactFailed
✗ HelmRelease/payment-api in flux-system: SourceNotReady
...

31 issues found

→ For remediation commands: cub-scout scan
→ To trace a failing resource: cub-scout trace <kind>/<name> -n <namespace>
→ To see full details: cub-scout map deep-dive
```

---

### 1.3 Differentiate `map crashes` from `map issues`

**Problem:** Both commands show nearly identical output.

| Command | Focus | Shows |
|---------|-------|-------|
| `map crashes` | Pod health only | CrashLoopBackOff, ImagePullBackOff, OOMKilled, Error |
| `map issues` | GitOps health | All: deployers + workloads |

**`map crashes` proposed:**
```
CRASHING PODS
═══════════════════════════════════════════════════════════════════
Pods in CrashLoopBackOff, Error, OOMKilled, or ImagePullBackOff.

NAMESPACE      POD                           STATUS           RESTARTS   AGE
demo-prod      postgresql-abc123-xyz         CrashLoopBackOff 47         2d
monitoring     prometheus-def456-uvw         OOMKilled        12         6h
...

5 crashing pods

→ View logs: kubectl logs -n <namespace> <pod> --previous
→ Describe: kubectl describe pod -n <namespace> <pod>
```

**`map issues` proposed:**
```
RESOURCES WITH ISSUES
═══════════════════════════════════════════════════════════════════
Deployers and workloads with conditions != Ready.

DEPLOYERS (7 issues)
✗ Kustomization/payment-api in break-glass-demo: ArtifactFailed
✗ HelmRelease/payment-api in flux-system: SourceNotReady
...

WORKLOADS (24 issues)
✗ Deployment/postgresql in demo-prod: 0/1 ready
✗ Deployment/prometheus in monitoring: 1/2 ready
...

31 total issues (7 deployers, 24 workloads)

→ For remediation: cub-scout scan
→ To trace: cub-scout trace <kind>/<name> -n <namespace>
```

---

### 1.4 Summary Lines for All Commands

| Command | Current | Proposed Summary |
|---------|---------|------------------|
| `map list` | ✓ Has summary | Keep as-is |
| `map orphans` | ✗ None | "45 orphan resources across 8 namespaces" |
| `map crashes` | ✗ None | "5 crashing pods" |
| `map issues` | ✗ None | "31 issues (7 deployers, 24 workloads)" |
| `map workloads` | ✗ None | "48 workloads: 28 Flux, 12 Helm, 8 Native" |
| `map deployers` | ✗ None | "13 deployers: 8 Kustomizations, 3 HelmReleases, 2 Applications" |

---

### 1.5 Link D2 Diagrams from Output

When showing explanatory content, link to relevant D2 diagrams:

```
→ Visual guide: docs/diagrams/ownership-detection.svg
```

---

## Phase 2: Learning Mode (Priority: P1)

### 2.1 `--explain` Flag for Key Commands

Add `--explain` flag that teaches concepts as it shows data.

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

**Files to modify:**
- `cmd/cub-scout/map.go` — `--explain` for map list
- `cmd/cub-scout/trace.go` — `--explain` for trace
- `cmd/cub-scout/scan.go` — `--explain` for scan

---

## Phase 3: Meaningful Example (Priority: P1)

### 3.1 Create `platform-example`

A complete, realistic platform example with ~50 resources.

**Structure:**
```
examples/platform-example/
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
│   └── prod/
│
└── README.md                 # Full documentation with learning journey
```

**Components:**

| Component | Implementation | Teaches |
|-----------|----------------|---------|
| **Frontend** | Kustomize base + 3 overlays | Multi-environment deployment |
| **Backend API** | Kustomize base + overlays | Service dependencies |
| **PostgreSQL** | Flux HelmRelease | Helm charts via GitOps |
| **Redis** | Flux HelmRelease | Caching layer |
| **Prometheus** | Kustomize + upstream | Monitoring stack |
| **Ingress** | NGINX Helm chart | External access |

**Resource count:** ~50 resources across 5 namespaces

### 3.2 Clobbering Scenario (Teaching Moment)

Include a deliberate "clobbering" scenario:

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
```

**Learning outcome:** Users understand why direct `kubectl` changes are risky in GitOps.

See: `docs/diagrams/clobbering-problem.svg`

### 3.3 Upgrade Tracing Scenario

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

See: `docs/diagrams/upgrade-tracing.svg`

---

## Phase 4: Documentation Restructure (Priority: P2)

Restructure docs using the [Diataxis framework](https://diataxis.fr/).

### 4.1 Proposed Structure

```
docs/
├── README.md                # Docs index (single page, links to sections)
│
├── getting-started/         # TUTORIALS (learning-oriented)
│   ├── install.md           # All install methods
│   ├── first-map.md         # Your first 5 minutes
│   └── understand-gitops.md # For GitOps newcomers (uses D2 diagrams)
│
├── howto/                   # HOW-TO GUIDES (task-oriented)
│   ├── find-orphans.md
│   ├── trace-ownership.md
│   ├── scan-for-risks.md
│   ├── query-resources.md
│   └── import-to-confighub.md
│
├── reference/               # REFERENCE (information-oriented)
│   ├── commands.md          # All CLI commands (merge CLI-GUIDE.md)
│   ├── keybindings.md       # TUI shortcuts
│   ├── query-syntax.md      # Query language
│   ├── gsf-schema.md        # JSON schema
│   └── ownership-labels.md  # How detection works
│
├── concepts/                # EXPLANATION (understanding-oriented)
│   ├── gitops-overview.md   # What is GitOps?
│   ├── ownership-detection.md
│   ├── clobbering-problem.md  # The PDF content!
│   └── flux-vs-argo.md
│
└── diagrams/                # D2 source files + SVG renders (keep)
```

### 4.2 Archive Gold to Extract

These archive docs have excellent content to migrate:

| Archive File | Gold Content | Migrate To |
|--------------|--------------|------------|
| `JOURNEY-MAP.md` | TUI screenshots, health bars, trace boxes | `getting-started/first-map.md` |
| `JOURNEY-QUERY.md` | Query syntax, examples, cheat sheet | `reference/query-syntax.md` |
| `EXAMPLES-TUI-MAP-FLEET-IITS-STUDIES.md` | IITS pain points, before/after | `concepts/why-cub-scout.md` |
| `IMPORT-GIT-REFERENCE-ARCHITECTURES.md` | GitOps patterns, repo structures | `concepts/gitops-patterns.md` |

### 4.3 ASCII Art to Preserve

**Health dashboard:**
```
┌─ CLUSTER HEALTH ─────────────────────────────────────────┐
│  ████████████████████░░░░  85%  (17/20 ready)           │
└──────────────────────────────────────────────────────────┘
```

**Trace visualization:**
```
┌─ TRACE: payment-api ─────────────────────────────────────┐
│  ┌─────────────────────────┐                            │
│  │ GitRepository           │                            │
│  │ flux-system/platform    │                            │
│  └───────────┬─────────────┘                            │
│              ▼                                           │
│  ┌─────────────────────────┐                            │
│  │ Kustomization           │                            │
│  └───────────┬─────────────┘                            │
│              ▼                                           │
│  ┌─────────────────────────┐                            │
│  │ Deployment              │                            │
│  └─────────────────────────┘                            │
└──────────────────────────────────────────────────────────┘
```

**Fleet hierarchy:**
```
  payment-api
  |-- variant: prod
  |   |-- cluster-east @ rev 89
  |   |-- cluster-west @ rev 89
  |   |-- cluster-eu @ rev 87    <- behind!
  |-- variant: staging
      |-- cluster-staging @ rev 92
```

**Side-by-side panels:**
```
┌─ RESOURCES ────────────────┬─ PIPELINES ────────────┐
│  Flux        8  ████████   │  ✓ GitRepo → Kust → D  │
│  ArgoCD      5  █████      │  ✓ GitRepo → App → D   │
│  Helm        4  ████       │  ⚠ HelmRelease pending │
│  Native      3  ███        │                        │
└────────────────────────────┴────────────────────────┘
```

**Trace with problem marker:**
```
┌─────────────────────────────────────────────────────────────────────┐
│ TRACE: Deployment/broken-app                                        │
├─────────────────────────────────────────────────────────────────────┤
│   🟢 ✓ 🟣 GitRepository/infra-repo                                  │
│       └─▶ 🔴 ✗ 🔵 Kustomization/apps        ◀── PROBLEM HERE        │
│               │ 🔴 Error: path './clusters/prod/apps' not found     │
│               └─▶ Deployment/broken-app (stale)                     │
├─────────────────────────────────────────────────────────────────────┤
│ 🟡 ⚠ Chain broken at Kustomization/apps                            │
└─────────────────────────────────────────────────────────────────────┘
```

### 4.4 Writing Style

1. **Concise** - Say it in 10 words, not 50
2. **Task-focused** - "To find orphans, run..." not "Orphans are resources that..."
3. **Code first** - Show the command, then explain
4. **No fluff** - No "In this guide, we will..." — just do it

**BAD:**
> In this section, we will explore how to use the cub-scout map command to discover resources in your Kubernetes cluster that are not currently being managed by any GitOps tooling.

**GOOD:**
> ```bash
> cub-scout map orphans
> ```
> Shows all resources not managed by Flux, ArgoCD, or Helm.

---

## Phase 5: Advanced Features (Priority: P2-P3)

### 5.1 `cub-scout learn` Command (P3)

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

### 5.2 Enhanced Import Wizard (P2)

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

### 5.3 In-TUI Learning (P3)

Contextual tooltips when hovering/selecting items:

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

### 5.4 JSON Output Consistency (P2)

Ensure all commands support `--json`:
- `map orphans --json`
- `map crashes --json`
- `map issues --json`
- `map workloads --json`
- `map deployers --json`

### 5.5 Exit Codes for Scripting (P3)

- `0` - Success
- `1` - Error (command failed)
- `2` - Issues found (e.g., `map issues` found problems)

```bash
cub-scout map issues || echo "Issues found!"
cub-scout scan --severity critical && echo "No critical issues"
```

### 5.6 Diff & Upgrade Tracing (P1-P2)

| Feature | Description | Priority |
|---------|-------------|----------|
| `trace --diff` | Show live vs git differences | P1 |
| Chart version diff | Show what changed between helm chart versions | P2 |
| Layer-by-layer trace | Show which layer caused a change | P2 |
| Upgrade impact preview | Before upgrading, show what will change | P3 |

---

## Priority Summary

| Priority | Count | Focus |
|----------|-------|-------|
| **P1** | 12 | Core demystification: orphans UX, --explain, platform-example, trace --diff |
| **P2** | 9 | Polish: crashes/issues differentiation, import wizard, docs restructure |
| **P3** | 7 | Nice-to-have: learn command, exit codes, in-TUI learning |

---

## Validation Criteria

For each change, verify:
- [ ] Solves a real user problem
- [ ] Teaches, not just shows
- [ ] Works with realistic scale (50+ resources)
- [ ] No breaking changes to existing behavior
- [ ] Can be tested/demoed

---

## Files Reference

| Phase | Files to Modify |
|-------|-----------------|
| Phase 1 | `cmd/cub-scout/map.go` |
| Phase 2 | `cmd/cub-scout/map.go`, `trace.go`, `scan.go` |
| Phase 3 | `examples/platform-example/` (new) |
| Phase 4 | `docs/` restructure |
| Phase 5 | Multiple files, new `learn.go` |
