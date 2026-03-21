# Flux Import to ConfigHub Demo (D2 Pattern)

Management and discovery for Flux-managed clusters. This demo runs
`cub gitops import` and `cub-scout import` against a kind cluster with real
Flux, using the Control Plane "D2" reference architecture as the brownfield
pattern. `cub gitops import` is the production path (rendered pipeline with
auto-updates), while cub-scout provides broad discovery including resources
outside Flux's scope.

## AI-First Files

This example now has a local AI-first path alongside the narrated `demo.sh` path:

- [`AI_START_HERE.md`](./AI_START_HERE.md)
- [`prompts.md`](./prompts.md)
- [`contracts.md`](./contracts.md)
- `./setup.sh`
- `./verify.sh`
- `./cleanup.sh`

Use those files if you want the incubator-style structure without rewriting the
existing five-act demo.

## Read-Only First

Preview the AI-first path before you mutate anything:

```bash
./setup.sh --explain
./setup.sh --explain-json
```

These commands do not create a cluster and do not create ConfigHub state.

## AI-First Quick Start

Cluster plus walkthrough, while keeping the cluster running for verification:

```bash
./setup.sh
./verify.sh
```

Cluster plus connected import path:

```bash
cub auth login
./setup.sh --with-worker
./verify.sh
```

Clean up explicitly when you are done:

```bash
./cleanup.sh
```

Important boundary: `./verify.sh` checks cluster, ConfigHub, and `cub-scout`
evidence surfaces and now includes a `cub-scout scan` summary. That scan
evidence stays separate from proving ConfigHub import/render success, and the
script may report either concrete findings or an explicit no-findings contract.

## Human Demo Quick Start

```bash
# Observe only (no ConfigHub changes, ~5 min)
./demo.sh

# Full pipeline including ConfigHub import (~6 min, requires cub auth)
./demo.sh --live

# Full pipeline + seeded synthetic history (demo storytelling)
./demo.sh --live --seed-history

# Keep cluster running for interactive exploration
./demo.sh --keep

# Local dev: explicit ConfigHub URL
./demo.sh --live --confighub-url=http://localhost:9090
```

## Connected Readiness Check

After a `--live` run, verify the demo has active workers/targets and connected workloads:

```bash
../scripts/verify-connected-demo.sh --space flux-import-demo --renderer fluxrenderer
```

Optional (demo storytelling only): seed tagged synthetic ChangeSet history.

```bash
../scripts/seed-connected-demo-history.sh --space flux-import-demo --allow-synthetic --apply
```

When `--keep` is used with `--live`, the demo leaves the discovery worker
running and prints a `demo-worker-lifecycle.sh stop --pid-file ...` command so
connected state is preserved for extended sessions.

## Prerequisites

| Tool | Required for | Install |
|------|-------------|---------|
| docker | All modes | [docker.com](https://docker.com) |
| kind | All modes | `brew install kind` |
| kubectl | All modes | `brew install kubectl` |
| go | All modes (builds cub-scout) | [go.dev](https://go.dev) |
| flux | All modes (installs Flux) | `brew install fluxcd/tap/flux` |
| cub | `--live` mode only | [confighub.com/docs/cli](https://confighub.com/docs/cli) |

For `--live` mode, authenticate first: `cub auth login`

You can also pass `--confighub-url=<url>` to override the ConfigHub server URL
(useful for local dev). By default, the demo reads the URL from `cub info`.

---

## What the Demo Builds

The demo creates a kind cluster with **real Flux** and two distinct fixture
sets based on the Control Plane D2 reference architecture:

```
kind-flux-import-demo
|
|-- flux-system/                           Flux controllers (real, running)
|   |-- GitRepository: podinfo            <-- real, Flux pulls from GitHub
|   |-- GitRepository: platform-config    <-- fictional (acme/platform-config)
|   |-- Kustomization: podinfo            <-- real, Flux reconciles this
|   |-- Kustomization: infrastructure     <-- CR exists, source fails
|   |-- Kustomization: apps               <-- CR exists, source fails
|   |-- Kustomization: payment-api        <-- CR exists, source fails
|   `-- Kustomization: frontend           <-- CR exists, source fails
|
|-- podinfo/                               Created BY Flux (reconciled)
|   |-- Deployment: podinfo
|   `-- Service: podinfo
|
|-- cert-manager/                          Helm-via-Flux labels, kubectl-applied
|   |-- HelmRelease: cert-manager
|   `-- Deployment: cert-manager
|
|-- monitoring/                            Helm-via-Flux labels, kubectl-applied
|   |-- HelmRelease: monitoring
|   |-- Deployment: prometheus
|   `-- Deployment: grafana
|
|-- payments/                              Kustomization labels, kubectl-applied
|   |-- Deployment: payment-api
|   `-- Service: payment-api
|
|-- store/
|   |-- Deployment: frontend              Kustomization labels, kubectl-applied
|   |-- Service: frontend
|   `-- ConfigMap: debug-config           Native (no management labels)
|
`-- cache/
    `-- StatefulSet: redis                Helm-managed (no Flux labels)
```

### Why Two Fixture Sets?

- **Podinfo** has a real GitRepository + Kustomization with `spec.interval`.
  Flux actually pulls from GitHub and creates real Deployments. This is what
  a real Flux-managed workload looks like.

- **D2 brownfield fixtures** (payment-api, frontend, cert-manager, monitoring)
  are kubectl-applied with Flux ownership labels. The Kustomization/HelmRelease
  CRs exist but the source repo (`acme/platform-config`) doesn't, so Flux
  can't reconcile them. This is common in brownfield clusters where teams add
  Flux labels before fully migrating.

- **Redis** uses `app.kubernetes.io/managed-by: Helm` labels only. No Flux
  relationship. This is the Helm-only case.

- **debug-config** has no management labels. This is a "Native" resource
  that only `cub-scout import` can see.

### The D2 Pattern

The brownfield fixtures follow the [Control Plane Flux reference architecture](https://github.com/controlplaneio-fluxcd)
(internally called "D2"):

| Layer | Owner | Contains |
|-------|-------|----------|
| `infrastructure/` | Platform team | cert-manager, monitoring (via HelmReleases) |
| `apps/` | Dev teams | payment-api, frontend (via Kustomizations) |

See [`examples/d2-control-plane/`](../d2-control-plane/) for the full D2
reference with documentation.

---

## The Five Acts

### Act 1: The Cluster (~4 min)

Creates the kind cluster, installs real Flux (~1-2 min), and applies both
fixture sets. The podinfo Kustomization has a real source, so Flux fetches
the repo and creates real Deployments in the `podinfo` namespace.

You should see output like:

```
=== Act 1: The Cluster ===

>> Creating kind cluster...
>> Cluster ready: kind-flux-import-demo

>> Installing Flux (this takes 1-2 minutes)...
>> Flux installed

>> Applying D2 brownfield fixtures (Control Plane pattern)...
   Flux CRs: GitRepository, 2 Kustomizations (infrastructure + apps)
             2 per-app Kustomizations (payment-api, frontend)
             2 HelmReleases (cert-manager, monitoring)
   Infrastructure: cert-manager, prometheus, grafana (Helm-via-Flux labels)
   Applications:   payment-api, frontend (Kustomization labels)
   Helm-only:      redis (no Flux labels)
   Native:         debug-config (no labels)

>> Applying podinfo Kustomization (real Flux reconciliation)...
>> Waiting for podinfo to sync (up to 120s)...
>> Podinfo synced - Flux created real workloads
NAME      READY   UP-TO-DATE   AVAILABLE   AGE
podinfo   0/1     1            0           2s
```

### Act 2: The Observer (cub-scout)

Runs three cub-scout commands:

| Command | What it shows |
|---------|--------------|
| `cub-scout map list` | Full resource inventory with ownership classification |
| `cub-scout gitops status` | Flux Kustomization/HelmRelease health and sync status |
| `cub-scout import --dry-run` | Proposed ConfigHub unit groupings |

**What to look for:** cub-scout sees *everything* -- workloads across all
namespaces, all ownership types. It classifies by labels: Flux(13), Helm(1),
Native(40).

You should see `gitops status` output like:

```
GITOPS STATUS
================================================================

  Backend:   FLUX
  Transport: GIT

  ! 6/7 deployers failing

SOURCES
----------------------------------------------------------------
  X GitRepository/platform-config         <-- fictional source, expected failure
  Y GitRepository/podinfo                 <-- real, healthy

DEPLOYERS
----------------------------------------------------------------
  X Kustomization/apps                    Source artifact not found
  X Kustomization/frontend               Source artifact not found
  X Kustomization/infrastructure          Source artifact not found
  X Kustomization/payment-api             Source artifact not found
  Y Kustomization/podinfo                 <-- real, reconciled
  X HelmRelease/cert-manager              SourceNotReady
  X HelmRelease/monitoring                SourceNotReady
```

Notice: podinfo is healthy (real Flux). The D2 brownfield Kustomizations show
`ArtifactFailed` because the source repo doesn't exist — this is the expected
contrast between real-reconciled and label-only fixtures.

### Act 3: The Flux View (trace + patterns)

There's no `import-flux` command (unlike ArgoCD which has `import-argocd`).
Instead, Act 3 shows Flux-specific cub-scout capabilities:

| Command | What it shows |
|---------|--------------|
| `cub-scout tree ownership` | Flux ownership chains (infra vs apps layers) |
| `cub-scout tree patterns` | D2/Control Plane pattern auto-detection |
| `cub-scout trace deploy/payment-api -n payments` | Full Kustomization chain |
| `cub-scout trace deploy/cert-manager -n cert-manager` | Helm-via-Flux chain |

You should see `tree ownership` output like:

```
Ownership Hierarchy
------------------------------------------------------------
Flux (6)
  |-- cert-manager/cert-manager -> kustomization/cert-manager
  |-- monitoring/grafana -> kustomization/monitoring
  |-- monitoring/prometheus -> kustomization/monitoring
  |-- payments/payment-api -> kustomization/payment-api
  |-- podinfo/podinfo -> kustomization/podinfo
  `-- store/frontend -> kustomization/frontend

Helm (1)
  `-- cache/redis
```

And `trace deploy/payment-api` output like:

```
TRACE: Deployment/payment-api in payments

  Object:          Deployment/payment-api
  Namespace:       payments
  Status:          Managed by Flux
  ---
  Kustomization:   payment-api
  Namespace:       flux-system
  Target:          payments
  Path:            ./apps/payment-api/overlays/prod
  ---
  GitRepository:   platform-config
  Namespace:       flux-system
  URL:             https://github.com/acme/platform-config
  Branch:          main
  Status:          Last reconciliation failed (source not found)
```

Notice the full chain: Deployment -> Kustomization -> GitRepository. The trace
shows the D2 path (`./apps/payment-api/overlays/prod`) and the failure reason
(fictional source repo). This is Flux-specific depth that `cub-scout import`
alone wouldn't provide.

### Act 4: The Pipeline (cub gitops import)

This is the production-grade import path. While Acts 2-3 produce static
snapshots, `cub gitops import` builds a **live render pipeline** that
continuously produces the exact manifests Flux would apply.

> Run `./demo.sh --live` to see this act. Requires ConfigHub authentication
> (`cub auth login`) because it creates real ConfigHub units and workers.

The demo sets up the full pipeline:

1. Creates a ConfigHub space
2. Recreates and starts a discovery BridgeWorker (`cub worker delete/create ... discovery-worker`, then `cub worker run -t Kubernetes`)
3. Recreates a renderer BridgeWorker slug (`cub worker delete/create ... flux-renderer-worker`)
4. Deploys that Flux renderer worker in-cluster (`cub worker install -t fluxrenderer ... flux-renderer-worker`)
5. Runs `cub gitops discover` to find Flux deployers
6. Runs `cub gitops import` to create dry/wet unit pairs for deployers that can render

**Key insight:** This is the only tool that produces *rendered manifests* and
keeps them current. The dry/wet pairs are linked: when you push a change to
Git, Flux re-renders, the renderer worker picks it up, and the wet unit in
ConfigHub auto-updates. No re-import needed.

### Act 5: Management and Discovery

The demo prints a summary table. You should see:

```
=== Act 5: Management and Discovery ===

                     MANAGEMENT                    DISCOVERY
                     cub gitops     tree/trace     cub-scout
                     import         (patterns)     import
---------------------------------------------------------------
podinfo (Flux)          Y              Y              Y
payment-api (Flux)      Y              Y              Y
frontend (Flux)         Y              Y              Y
cert-manager (Helm)     .              Y              Y
monitoring (Helm)       .              Y              Y
redis (Helm-only)       .              .              Y
debug-config (Native)   .              .              Y
---------------------------------------------------------------
Unit model           dry/wet pairs  trace chains   flat groups
Rendering            controller     label-based    raw snapshot
Pipeline             auto-updating  read-only      static

Y = found/imported    . = not visible to this tool

Management: cub gitops import
  Rendered pipeline with auto-updating dry/wet unit pairs.
  Use for renderable Flux deployers you want to manage continuously.

Discovery: cub-scout import + tree/trace
  Broad cluster inventory (import) or Flux-specific structure (tree/trace).
  Use to find everything, including Helm/Native resources outside Flux.

Together: cub gitops import for Flux apps, then cub-scout import for the rest.
```

In this fixture, `cert-manager` and `monitoring` HelmReleases point to
intentionally missing chart sources, so they remain discovery-only.

The key takeaways:

- **Management** (left side): `cub gitops import` manages renderable Flux
  deployers as a live pipeline -- rendered manifests, auto-updating units,
  dry/wet pairs.
- **Discovery** (middle + right): `tree/trace` reveals Flux-specific structure
  (ownership chains, D2 pattern detection). `cub-scout import` finds everything
  regardless of ownership type, including Helm and Native resources.
- **Use together:** Run `cub gitops import` for your Flux apps, then
  `cub-scout import` to catch what's left (Helm, Native, unlabeled).

---

## When to Use Which

### Management (cub gitops)

| Scenario | Command |
|----------|---------|
| "Set up continuous pipeline for Flux apps" | `cub gitops import` |
| "See what Flux would render from source" | `cub gitops import` (dry/wet pairs) |
| "Keep ConfigHub units in sync as Git changes" | `cub gitops import` (auto-updating) |
| "Import renderable Flux deployers with rendered manifests" | `cub gitops import` |

### Discovery (cub-scout)

| Scenario | Command |
|----------|---------|
| "What's running on my cluster?" | `cub-scout map list` |
| "Import everything, including Helm and Native" | `cub-scout import` (review + confirm once) |
| "Non-interactive broad import + immediate connect" | `cub-scout import --yes --connect` |
| "See Flux ownership chains and D2 layers" | `cub-scout tree ownership` |
| "Trace a resource back to its Git source" | `cub-scout trace deploy/<name> -n <ns>` |
| "Find resources Flux doesn't manage" | `cub-scout import` |
| "What changed since last sync?" | `cub-scout gitops status` |

### Decision Tree

```
Do you have Flux Kustomizations or HelmReleases?
  YES -->
    cub gitops import   (rendered pipeline, auto-updating dry/wet pairs)
    Then also run:
    cub-scout import    (catches Helm/Native resources outside Flux)
                       (single confirm: import + worker/target connect)
  NO, or just exploring -->
    cub-scout import    (broad discovery, all workload types)
    cub-scout tree ownership  (see Flux chains if Flux is present)
```

---

## Architecture

```
                    Kubernetes Cluster
 +---------------------------------------------------------+
 |                                                         |
 |  +----------+    +----------+    +----------+           |
 |  | Flux     |    | Helm     |    | Native   |           |
 |  | managed  |    | managed  |    |          |           |
 |  +----+-----+    +----+-----+    +----+-----+           |
 |       |               |               |                 |
 +-------+---------------+---------------+-----------------+
         |               |               |
         v               v               v
   +----------+    +----------+    +----------+
   | cub      |    | scout    |    | scout    |
   | gitops   |    | tree/    |    | import   |
   | import   |    | trace    |    |          |
   +----------+    +----------+    +----------+
   MANAGEMENT      Flux chains     DISCOVERY
   rendered +      ownership +     all workloads
   auto-updating   pattern detect  broad coverage
```

### How Each Tool Works

**cub gitops import** (pipeline-level) -- the production path
- Discovers: Flux Kustomizations and HelmReleases
- Renders: through actual Flux renderer (in-cluster worker, `-t fluxrenderer`)
- Creates: dry/wet unit pairs with MergeUnits links that auto-update
- Note: only deployers with resolvable sources are imported in this fixture
- Requires: ConfigHub space + discovery BridgeWorker slug + discovery worker run
  (`cub worker create` + `cub worker run`) + renderer BridgeWorker slug
  (`cub worker create`) + in-cluster renderer install (`cub worker install`)
- Strength: the only tool that produces controller-rendered manifests and
  keeps them current as Git changes

**cub-scout tree/trace** (Flux-specific discovery)
- Reads: Kustomization CRs, HelmRelease CRs, GitRepository CRs
- Shows: ownership chains (Deployment -> Kustomization -> GitRepository)
- Detects: D2/Control Plane pattern (infrastructure vs apps layers)
- Requires: kubectl access + Flux CRDs
- Strength: reveals Flux-specific structure and multi-layer chains

**cub-scout import** (workload-level) -- broad discovery
- Reads: Deployments, StatefulSets, DaemonSets
- Detects ownership: labels (`kustomize.toolkit.fluxcd.io/name`,
  `helm.toolkit.fluxcd.io/name`, `app.kubernetes.io/managed-by: Helm`, etc.)
- Creates: flat ConfigHub units with raw manifest snapshots
- Requires: kubectl access only
- Strength: the only tool that sees Helm-only and Native resources

---

## Exploring After the Demo

With `--keep`, the cluster stays running for interactive exploration:

```bash
# TUI map with ownership colors
./cub-scout map

# Flux health dashboard
./cub-scout gitops status

# Trace ownership chain for a specific resource
./cub-scout trace deploy/podinfo -n podinfo

# See the D2 layer structure
./cub-scout tree ownership

# Security scan
./cub-scout scan
```

## Feedback

Please share quick feedback on these:

1. Was the dry/wet model clear after Act 4?
2. Which output was most useful: `map`, `gitops status`, `tree/trace`, or `cub gitops import` results?
3. Where did setup friction appear most: auth, workers, targets, or import?
4. Did the app/team/variant label mapping match how your teams think about ownership?
5. After this demo, does ConfigHub operational control-plane path make sense? If not, what feels unclear or missing?

## Troubleshooting

**Flux takes too long to install**
The `flux install` command pulls several images. On a slow connection this can
take 3+ minutes. The demo waits up to 5 minutes.

**Podinfo doesn't sync**
Flux needs to pull from GitHub. If your network blocks this, the podinfo
Deployment won't appear in the `podinfo` namespace. The demo still works --
Acts 2-3 show the brownfield fixtures.

**--live mode: "ConfigHub auth required"**
Run `cub auth login` first. The demo needs a valid ConfigHub session for
space creation, worker management, and gitops import.

**cert-manager trace fails**
The brownfield HelmRelease references a HelmRepository (`jetstack`) that
doesn't exist in the cluster. The trace shows this failure, which is
expected behavior -- it demonstrates graceful degradation on missing chain
links.

**`./verify.sh` shows `scan_contract=no-findings-observed`**
That is an explicit scan contract, not a failure. It means `cub-scout scan`
returned summary data for the kept-alive cluster but no current findings.

Inspect the raw scan output with:

```bash
./cub-scout scan --state --json
```

If the demo is expected to show a deterministic finding and does not, treat
that as fixture drift before changing the docs to overclaim a finding path.

## Repeatable Import Check

Validate connected import delegation behavior locally:

```bash
make test-import-delegation
```

## Related

- [`examples/d2-control-plane/`](../d2-control-plane/) - D2 pattern fixture with full documentation
- [`examples/argo-import-confighub-demo/`](../argo-import-confighub-demo/) - Equivalent demo for ArgoCD
- [CLI-GUIDE.md](../../CLI-GUIDE.md) - Complete CLI reference
