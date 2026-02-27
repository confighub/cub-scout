# Combined Demo: Three Tools, Same Cluster, Different Lenses

Three import paths. Same cluster. Different lenses. This demo runs
`cub-scout import`, `cub-scout import-argocd`, and `cub gitops import`
against a kind cluster with real ArgoCD to show what each tool sees,
what it misses, and why you might need all three.

## Quick Start

```bash
# Observe only (no ConfigHub changes, ~5 min)
./demo.sh

# Full pipeline including ConfigHub import (~6 min, requires cub auth)
./demo.sh --live

# Keep cluster running for interactive exploration
./demo.sh --keep
```

## Prerequisites

| Tool | Required for | Install |
|------|-------------|---------|
| docker | All modes | [docker.com](https://docker.com) |
| kind | All modes | `brew install kind` |
| kubectl | All modes | `brew install kubectl` |
| go | All modes (builds cub-scout) | [go.dev](https://go.dev) |
| cub | `--live` mode only | [confighub.com/docs/cli](https://confighub.com/docs/cli) |

For `--live` mode, authenticate first: `cub auth login`

---

## What the Demo Builds

The demo creates a kind cluster with **real ArgoCD** and two distinct fixture
sets that create a deliberate contrast:

```
kind-combined-demo
|
|-- argocd/                              ArgoCD server (real, running)
|   |-- Application: helm-guestbook     <-- real, synced by ArgoCD
|   |-- Application: kustomize-guestbook <-- real, synced by ArgoCD
|   |-- Application: myapp-dev          <-- CR exists, workloads kubectl-applied
|   |-- Application: myapp-staging      <-- CR exists, workloads kubectl-applied
|   `-- Application: myapp-prod         <-- CR exists, workloads kubectl-applied
|
|-- guestbook/                           Created BY ArgoCD (synced)
|   |-- Deployment: helm-guestbook-ui
|   `-- Deployment: kustomize-guestbook-ui
|
|-- myapp-dev/
|   |-- Deployment: api                 ArgoCD labels, kubectl-applied
|   |-- Deployment: worker              ArgoCD labels, kubectl-applied
|   `-- StatefulSet: redis              Helm-managed
|
|-- myapp-staging/                       (same pattern as dev)
|
`-- myapp-prod/
    |-- Deployment: api                 ArgoCD labels, kubectl-applied
    |-- Deployment: worker              ArgoCD labels, kubectl-applied
    |-- StatefulSet: redis              Helm-managed
    `-- ConfigMap: debug-config         Native (no management labels)
```

### Why Two Fixture Sets?

- **Guestbook apps** have `syncPolicy.automated` so ArgoCD actually syncs
  them from Git, creating real Deployments and Services. This is what a
  real ArgoCD-managed workload looks like.

- **Arnie fixtures** (api, worker, redis) are kubectl-applied with ArgoCD
  labels. The Application CRs exist but ArgoCD didn't create the workloads.
  This is common in brownfield clusters where teams add ArgoCD labels to
  existing resources before fully migrating.

- **Redis** uses `app.kubernetes.io/managed-by: Helm` labels only. No ArgoCD
  relationship. This is the Helm-only case.

- **debug-config** has no management labels at all. This is a "Native" resource
  that only `cub-scout import` can see.

---

## The Five Acts

### Act 1: The Cluster (~4 min)

Creates the kind cluster, installs real ArgoCD (~2-4 min for all 6
deployments), and applies both fixture sets. The guestbook Applications have
`syncPolicy.automated`, so ArgoCD fetches the source repos and creates real
Deployments in the `guestbook` namespace.

**What happens:** You'll see ArgoCD install progress, namespace creation,
fixture application, and guestbook sync status. If ArgoCD takes longer than
expected, the demo continues gracefully.

### Act 2: The Observer (cub-scout)

Runs three cub-scout commands:

| Command | What it shows |
|---------|--------------|
| `cub-scout map list` | Full resource inventory with ownership classification |
| `cub-scout gitops status` | ArgoCD Application health and sync status |
| `cub-scout import --dry-run` | Proposed ConfigHub unit groupings |

**What to look for:** cub-scout sees *everything* -- 15+ workloads across all
namespaces, all ownership types. It groups them into logical units (api, worker,
redis, guestbook). Notice it sees redis (Helm) and debug-config (Native)
alongside the ArgoCD workloads.

**Key insight:** cub-scout reports what the labels say, not what the controller
did. The Arnie deployments appear as "ArgoCD" because they have the
`argocd.argoproj.io/instance` label -- even though ArgoCD didn't create them.
This is correct behavior: ownership is determined by labels, not runtime state.

### Act 3: The Application Importer (import-argocd)

Runs the per-Application importer:

| Command | What it shows |
|---------|--------------|
| `import-argocd --list` | All 5 ArgoCD Applications |
| `import-argocd helm-guestbook --dry-run` | Real synced app with managed resources |
| `import-argocd myapp-dev --dry-run` | Arnie app (labels only, not synced) |

**What to look for:** import-argocd sees all 5 Applications. Watch what
happens with helm-guestbook vs myapp-dev: helm-guestbook has real managed
resources that ArgoCD synced; myapp-dev has resources with ArgoCD labels
but ArgoCD shows them as OutOfSync because it didn't actually sync them.

**Key insight:** import-argocd extracts per-Application metadata -- Git
source path, sync status, health, and path-derived labels. This is richer
than the flat workload view from `cub-scout import`, but it *only sees
ArgoCD Applications*. Redis and debug-config are invisible.

### Act 4: The Pipeline (cub gitops import)

> Requires `--live` flag and ConfigHub authentication.

Sets up the full ConfigHub render pipeline:

1. Gets ArgoCD auth token from the cluster
2. Creates a ConfigHub space
3. Starts a discovery worker (local, reads K8s API)
4. Deploys an ArgoCD renderer worker in-cluster
5. Runs `cub gitops discover` to find Applications
6. Runs `cub gitops import` to create dry/wet unit pairs

**What to look for:** `cub gitops discover` finds the same 5 Applications.
But `cub gitops import` goes further: it renders them through the actual
ArgoCD renderer, producing the exact YAML that ArgoCD would apply. For
helm-guestbook, this means the Helm chart is fully expanded. The result is
dry/wet unit pairs with MergeUnits links.

**Key insight:** This is the only tool that produces *rendered manifests* --
what ArgoCD would actually apply, not raw snapshots. And the dry/wet pairs
are linked: when the renderer produces new output, the wet unit auto-updates.
This is a continuous pipeline, not a one-time import.

### Act 5: The Comparison

The summary table shows what each tool found:

```
                          cub-scout    import-argocd    cub gitops
                          import       (per-app)        import
---------------------------------------------------------------
helm-guestbook (ArgoCD)      Y              Y              Y*
kustomize-guestbook          Y              Y              Y*
myapp-dev api+worker         Y              Y              Y*
myapp-staging api+worker     Y              Y              Y*
myapp-prod api+worker        Y              Y              Y*
redis (Helm x 3 envs)       Y              .              .
debug-config (Native)        Y              .              .
---------------------------------------------------------------
Unit model                flat groups    per-app        dry/wet pairs
Rendering                 raw snapshot   raw snapshot   controller-rendered
Pipeline                  static         static         linked (auto-update)
```

---

## When to Use Which

| Scenario | Tool |
|----------|------|
| "What's running on my cluster?" | `cub-scout map list` |
| "How would I organize this into ConfigHub?" | `cub-scout import --dry-run` |
| "Import everything, including Helm and Native" | `cub-scout import --yes` |
| "Import this specific ArgoCD Application" | `cub-scout import-argocd <name>` |
| "Set up continuous render pipeline for ArgoCD apps" | `cub gitops import` |
| "Find Helm/Native resources ArgoCD doesn't manage" | `cub-scout import` |
| "What changed since last sync?" | `cub-scout gitops status` |

### Decision Tree

```
Do you need to see ALL resources (including Helm, Native)?
  YES --> cub-scout import
  NO, just ArgoCD Applications -->
    Do you need rendered manifests and auto-updating pipelines?
      YES --> cub gitops import
      NO, one-time import is fine --> cub-scout import-argocd
```

---

## Architecture

```
                    Kubernetes Cluster
 +---------------------------------------------------------+
 |                                                         |
 |  +----------+    +----------+    +----------+           |
 |  | ArgoCD   |    | Helm     |    | Native   |           |
 |  | managed  |    | managed  |    |          |           |
 |  +----+-----+    +----+-----+    +----+-----+           |
 |       |               |               |                 |
 +-------+---------------+---------------+-----------------+
         |               |               |
    +----+----+     +----+----+     +----+----+
    |         |     |         |     |         |
    v         v     v         v     v         v
 +------+  +------+  +------+
 |scout |  |argocd|  |gitops|   cub-scout import: sees ALL (Y Y Y)
 |import|  |import|  |import|   import-argocd:    ArgoCD only (Y . .)
 +------+  +------+  +------+   cub gitops:       ArgoCD rendered (Y . .)
```

### How Each Tool Works

**cub-scout import** (workload-level)
- Reads: Deployments, StatefulSets, DaemonSets
- Detects ownership: labels (`argocd.argoproj.io/instance`, `app.kubernetes.io/managed-by: Helm`, etc.)
- Creates: flat ConfigHub units with raw manifest snapshots
- Requires: kubectl access only

**cub-scout import-argocd** (application-level)
- Reads: ArgoCD Application CRs + their managed resources
- Extracts: Git source path, sync/health status, path-derived labels
- Creates: per-Application ConfigHub units
- Requires: kubectl access + ArgoCD CRDs

**cub gitops import** (pipeline-level)
- Discovers: ArgoCD Applications (or Flux HelmReleases/Kustomizations)
- Renders: through actual ArgoCD renderer (in-cluster worker)
- Creates: dry/wet unit pairs with MergeUnits links
- Requires: ConfigHub space + discovery worker + in-cluster renderer worker

---

## Exploring After the Demo

With `--keep`, the cluster stays running for interactive exploration:

```bash
# TUI map with ownership colors
./cub-scout map

# Security scan
./cub-scout scan

# Trace ownership chain for a specific resource
./cub-scout trace deploy/api -n myapp-prod

# Show YAML for an ArgoCD Application
./cub-scout import-argocd helm-guestbook --show-yaml

# GitOps health dashboard
./cub-scout gitops status
```

## Troubleshooting

**ArgoCD takes too long to install**
The full ArgoCD install pulls several images. On a slow connection this can
take 5+ minutes. The demo will warn you and continue. Guestbook apps may not
sync, but Acts 2-3 still work with the Arnie fixtures.

**Guestbook apps don't sync**
ArgoCD needs to pull from GitHub. If your network blocks this, the guestbook
Deployments won't appear in the `guestbook` namespace. The comparison will
still show the contrast -- just the guestbook row will be missing from Act 2.

**--live mode: "ConfigHub auth required"**
Run `cub auth login` first. The demo needs a valid ConfigHub session for
space creation, worker management, and gitops import.

**--live mode: targets don't register**
The discovery worker runs locally and connects to ConfigHub. The renderer
worker runs in-cluster and also connects to ConfigHub. Both need network
access to your ConfigHub instance. Check firewall rules and
`CONFIGHUB_URL` reachability from inside the kind cluster.

## Related

- [`examples/import-from-live/`](../import-from-live/) - Simpler demo with just `cub-scout import`
- [CLI-GUIDE.md](../../CLI-GUIDE.md) - Complete CLI reference
- [Issue #201](https://github.com/confighubai/cub-scout/issues/201) - Design: connected-mode upgrade path
