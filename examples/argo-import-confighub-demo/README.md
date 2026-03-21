# ArgoCD Import to ConfigHub Demo

Three import paths into ConfigHub. Same cluster. Different depths. This demo
runs `cub gitops import`, `cub-scout import-argocd`, and `cub-scout import`
against a kind cluster with real ArgoCD to show how they complement each other.
`cub gitops import` is the production path (rendered pipeline with auto-updates),
while the cub-scout tools provide quick discovery and catch resources outside
ArgoCD's scope.

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
evidence surfaces, but it does **not** yet prove post-import `cub-scout scan`
findings. That remains a follow-on slice.

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
../scripts/verify-connected-demo.sh --space argo-import-demo --renderer argocdrenderer
```

Optional (demo storytelling only): seed tagged synthetic ChangeSet history.

```bash
../scripts/seed-connected-demo-history.sh --space argo-import-demo --allow-synthetic --apply
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
| cub | `--live` mode only | [confighub.com/docs/cli](https://confighub.com/docs/cli) |
| curl | `--live` mode only | Usually pre-installed |
| python3 | `--live` mode only | Usually pre-installed |

For `--live` mode, authenticate first: `cub auth login`

You can also pass `--confighub-url=<url>` to override the ConfigHub server URL
(useful for local dev). By default, the demo reads the URL from `cub info`.

---

## What the Demo Builds

The demo creates a kind cluster with **real ArgoCD** and two distinct fixture
sets that create a deliberate contrast:

```
kind-argo-import-demo
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
This demo pins both the ArgoCD release and guestbook source revision for
deterministic behavior across reruns.

**What happens:** You'll see ArgoCD install progress, namespace creation,
fixture application, and guestbook sync status. If ArgoCD takes longer than
expected, the demo continues gracefully.

You should see output like:

```
=== Act 1: The Cluster ===

>> Creating kind cluster...
>> Cluster ready: kind-argo-import-demo

>> Installing ArgoCD v3.3.2 (this takes 2-4 minutes)...
>> Waiting for ArgoCD deployments...
>> ArgoCD installed and ready

>> Applying Arnie fixtures...
   3 ArgoCD Application CRs (myapp-dev/staging/prod) - CRs exist but not synced
   6 Deployments (api + worker x 3 envs) - ArgoCD labels, applied by kubectl
   3 StatefulSets (redis x 3 envs) - Helm-managed
   1 ConfigMap (debug-config) - Native/unmanaged

>> Applying guestbook Applications (real ArgoCD sync)...
   2 ArgoCD Applications: helm-guestbook + kustomize-guestbook
   These have syncPolicy.automated - ArgoCD will create real workloads

>> Waiting for guestbook apps to sync (up to 120s)...
>> Guestbook apps synced - ArgoCD created real workloads
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
helm-guestbook   0/1     1            0           1s
```

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
did. This gives broad coverage but only produces static snapshots -- it captures
what's on the cluster right now, not what the controller would render from
source. For ArgoCD-managed apps, `cub gitops import` (Act 4) goes deeper.

You should see `map list` output like:

```
NAMESPACE       KIND         NAME                   OWNER
guestbook       Deployment   helm-guestbook         ArgoCD    <-- real synced
guestbook       Service      helm-guestbook         ArgoCD
guestbook       Deployment   kustomize-guestbook-ui ArgoCD
guestbook       Service      kustomize-guestbook-ui ArgoCD
myapp-dev       Deployment   api                    ArgoCD    <-- Arnie (labels only)
myapp-dev       StatefulSet  redis                  Helm      <-- Helm-managed
myapp-dev       Deployment   worker                 ArgoCD
myapp-prod      Deployment   api                    ArgoCD
myapp-prod      ConfigMap    debug-config           Native    <-- no labels
myapp-prod      StatefulSet  redis                  Helm
myapp-prod      Deployment   worker                 ArgoCD
myapp-staging   Deployment   api                    ArgoCD
myapp-staging   StatefulSet  redis                  Helm
myapp-staging   Deployment   worker                 ArgoCD
...
Total: 70 resources
By Owner: ArgoCD(10) Helm(3) Native(57)
```

And `gitops status` output like:

```
GITOPS STATUS
================================================================

  Backend:   ARGOCD
  Transport: GIT

  ! 3/5 deployers failing

DEPLOYERS
----------------------------------------------------------------
  Y Application/helm-guestbook         Sync: Synced    Health: Healthy
  Y Application/kustomize-guestbook    Sync: Synced    Health: Healthy
  X Application/myapp-dev              Sync: Unknown   Health: Healthy
  X Application/myapp-prod             Sync: Unknown   Health: Healthy
  X Application/myapp-staging          Sync: Unknown   Health: Healthy
```

Notice: helm-guestbook and kustomize-guestbook are Synced/Healthy (real ArgoCD).
The myapp-* apps show Unknown sync because ArgoCD recognizes the Application
CRs but the source repo (`acme/myapp-deploy.git`) doesn't exist.

And `import --dry-run` output like:

```
DISCOVERED
  guestbook (2 workloads)
  myapp-dev (3 workloads)
  myapp-prod (3 workloads)
  myapp-staging (3 workloads)

WILL CREATE
  App: guestbook-team

  * api       workloads: 3    <-- api across 3 envs
  * guestbook workloads: 1    <-- kustomize-guestbook
  * helm-guestbook workloads: 1
  * redis     workloads: 3    <-- Helm redis (invisible to ArgoCD tools)
  * worker    workloads: 3    <-- worker across 3 envs

  Total: 5 deployments
```

Notice that `import --dry-run` groups resources across namespaces into logical
units. Redis appears here but won't appear in Act 3.

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

You should see `import-argocd --list` output like:

```
ArgoCD Applications in namespace 'argocd'
======================================

NAME                      SYNC         HEALTH       DESTINATION
----                      ----         ------       -----------
helm-guestbook            Synced       Healthy      local:guestbook
kustomize-guestbook       Synced       Healthy      local:guestbook
myapp-dev                 Unknown      Healthy      local:myapp-dev
myapp-prod                Unknown      Healthy      local:myapp-prod
myapp-staging             Unknown      Healthy      local:myapp-staging
```

And for a real synced app (`helm-guestbook --dry-run`):

```
Step 1: Reading ArgoCD Application 'helm-guestbook' from namespace 'argocd'...
  Y Found Application: helm-guestbook
    Source: https://github.com/argoproj/argocd-example-apps.git (path: helm-guestbook)
    Sync: Synced, Health: Healthy

Step 3: Finding resources managed by 'helm-guestbook' in namespace 'guestbook'...
  Y Found 2 managed resources:
    Y Deployment/helm-guestbook (Healthy)
    Y Service/helm-guestbook (Healthy)

Import Summary
  Space: helm-guestbook
  Unit: helm-guestbook
  Resources: 2 (1 Deployment, 1 Service)
  Labels: app=helm-guestbook
```

And for an Arnie app (`myapp-dev --dry-run`):

```
Step 1: Reading ArgoCD Application 'myapp-dev' from namespace 'argocd'...
  Y Found Application: myapp-dev
    Source: https://github.com/acme/myapp-deploy.git (path: envs/dev)
    Sync: Unknown, Health: Healthy

Step 3: Finding resources managed by 'myapp-dev' in namespace 'myapp-dev'...
  Y Found 2 managed resources:
    ! Deployment/api (Progressing)
    ! Deployment/worker (Progressing)

Step 4: Extracting labels from Git path...
  Y Extracted labels from path 'envs/dev':
    variant=dev

Import Summary
  Space: myapp-dev
  Unit: myapp-dev
  Resources: 2 (2 Deployment)
  Labels: variant=dev, app=myapp-dev
```

Notice the differences: helm-guestbook is Synced with Healthy resources;
myapp-dev is Unknown sync with Progressing resources. Also notice that
import-argocd extracted `variant=dev` from the Git path `envs/dev` --
a label that `cub-scout import` wouldn't know about.

Redis and debug-config don't appear anywhere in Act 3 output.

### Act 4: The Pipeline (cub gitops import)

This is the production-grade import path. While Acts 2-3 produce static
snapshots, `cub gitops import` builds a **live render pipeline** that
continuously produces the exact manifests your GitOps controller would apply.

> Run `./demo.sh --live` to see this act. Requires ConfigHub authentication
> (`cub auth login`) because it creates real ConfigHub units and workers.

The demo sets up the full pipeline:

1. Gets ArgoCD auth token from the cluster
2. Creates a ConfigHub space
3. Starts a discovery worker (local, reads K8s API)
4. Deploys an ArgoCD renderer worker in-cluster
5. Runs `cub gitops discover` to find Applications
6. Runs `cub gitops import` to create dry/wet unit pairs

**What to look for:** `cub gitops discover` finds the same 5 Applications.
But `cub gitops import` goes further: it renders each Application through
the actual ArgoCD renderer, producing the exact YAML that ArgoCD would apply.
For helm-guestbook, this means the Helm chart is fully template-expanded --
not the raw chart source that Acts 2-3 would capture, but the final rendered
Kubernetes manifests. The result is dry/wet unit pairs with MergeUnits links.

**Key insight:** This is the only tool that produces *rendered manifests* and
keeps them current. The dry/wet pairs are linked: when you push a change to
Git, ArgoCD re-renders, the renderer worker picks it up, and the wet unit in
ConfigHub auto-updates. No re-import needed. This makes it the right choice
for ongoing ArgoCD management, while Acts 2-3 are better suited for initial
discovery and one-time imports.

You should see output like (exact target names will vary):

```
>> Getting ArgoCD auth token...
>> ArgoCD auth token obtained
>> Creating ConfigHub space 'argo-import-demo'...
>> Starting discovery worker...
>> Deploying ArgoCD renderer worker in-cluster...
>> Waiting for targets to register...
>> Both targets registered

>> cub gitops discover  (finding ArgoCD Applications)
  Discovered 5 applications

>> cub gitops import  (creating dry/wet unit pairs)
  Imported helm-guestbook (dry + wet)
  Imported kustomize-guestbook (dry + wet)
  Imported myapp-dev (dry + wet)
  Imported myapp-staging (dry + wet)
  Imported myapp-prod (dry + wet)

>> Units created by cub gitops import:
  helm-guestbook            (dry)
  helm-guestbook-rendered   (wet)
  kustomize-guestbook       (dry)
  kustomize-guestbook-rendered (wet)
  ...
```

Each Application gets a **dry** unit (the Application spec as declared) and a
**wet** unit (the rendered output from the actual ArgoCD renderer). The wet
unit contains the fully-expanded Kubernetes manifests that ArgoCD would apply.

### Act 5: Management and Discovery with these tools

The demo prints a summary table. You should see:

```
=== Act 5: Management and Discovery ===

                     MANAGEMENT                    DISCOVERY
                     cub gitops     import-argocd  cub-scout
                     import         (per-app)      import
---------------------------------------------------------------
helm-guestbook          Y              Y              Y
kustomize-guestbook     Y              Y              Y
myapp-dev               Y              Y              Y
myapp-staging           Y              Y              Y
myapp-prod              Y              Y              Y
redis (Helm)            .              .              Y
debug-config (Native)   .              .              Y
---------------------------------------------------------------
Unit model           dry/wet pairs  per-app        flat groups
Rendering            controller     raw snapshot   raw snapshot
Pipeline             auto-updating  static         static

Y = found/imported    . = not visible to this tool
```

The key takeaways:

- **Management** (left side): `cub gitops import` manages ArgoCD Applications
  as a live pipeline -- rendered manifests, auto-updating units, dry/wet pairs.
  `import-argocd` is the lightweight alternative when you don't need a pipeline.
- **Discovery** (right side): `cub-scout import` finds everything on the cluster
  regardless of ownership type. It's the only tool that sees Helm and Native
  resources.
- **Use together:** Run `cub gitops import` for your ArgoCD apps, then
  `cub-scout import` to catch what's left (Helm, Native, unlabeled).

---

## When to Use Which

### Management (cub gitops)

| Scenario | Command |
|----------|---------|
| "Set up continuous pipeline for ArgoCD apps" | `cub gitops import` |
| "See what ArgoCD would render from source" | `cub gitops import` (dry/wet pairs) |
| "Keep ConfigHub units in sync as Git changes" | `cub gitops import` (auto-updating) |
| "Import ArgoCD + Flux apps with rendered manifests" | `cub gitops import` |

### Discovery (cub-scout)

| Scenario | Command |
|----------|---------|
| "Import a specific ArgoCD Application (quick)" | `cub-scout import-argocd <name>` |
| "What's running on my cluster?" | `cub-scout map list` |
| "Import everything, including Helm and Native" | `cub-scout import` (review + confirm once) |
| "Non-interactive broad import + immediate connect" | `cub-scout import --yes --connect` |
| "Find resources ArgoCD doesn't manage" | `cub-scout import` |
| "What changed since last sync?" | `cub-scout gitops status` |

### Decision Tree

```
Do you have ArgoCD or Flux Applications?
  YES -->
    cub gitops import   (rendered pipeline, auto-updating dry/wet pairs)
    Then also run:
    cub-scout import    (catches Helm/Native resources outside ArgoCD/Flux)
                       (single confirm: import + worker/target connect)
  NO, or just exploring -->
    cub-scout import    (broad discovery, all workload types)
    or:
    cub-scout import-argocd <name>  (quick per-app import, no pipeline needed)
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
         v               v               v
   +----------+    +----------+    +----------+
   | cub      |    | scout    |    | scout    |
   | gitops   |    | import-  |    | import   |
   | import   |    | argocd   |    |          |
   +----------+    +----------+    +----------+
   MANAGEMENT      quick import    DISCOVERY
   rendered +      per-app         all workloads
   auto-updating   static          broad coverage
```

### How Each Tool Works

**cub gitops import** (pipeline-level) -- the production path
- Discovers: ArgoCD Applications (or Flux HelmReleases/Kustomizations)
- Renders: through actual ArgoCD/Flux renderer (in-cluster worker)
- Creates: dry/wet unit pairs with MergeUnits links that auto-update
- Requires: ConfigHub space + discovery worker + in-cluster renderer worker
- Strength: the only tool that produces controller-rendered manifests and
  keeps them current as Git changes

**cub-scout import-argocd** (application-level) -- per-app detail
- Reads: ArgoCD Application CRs + their managed resources
- Extracts: Git source path, sync/health status, path-derived labels
- Creates: per-Application ConfigHub units (static snapshot)
- Requires: kubectl access + ArgoCD CRDs
- Strength: quick per-Application import without ConfigHub infrastructure

**cub-scout import** (workload-level) -- broad discovery
- Reads: Deployments, StatefulSets, DaemonSets
- Detects ownership: labels (`argocd.argoproj.io/instance`, `app.kubernetes.io/managed-by: Helm`, etc.)
- Creates: flat ConfigHub units with raw manifest snapshots
- Requires: kubectl access only
- Strength: the only tool that sees Helm and Native resources

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

## Feedback

Please share quick feedback on these:

1. Was the flow from ArgoCD Application -> rendered manifests -> ConfigHub dry/wet units clear?
2. Which output was most useful: `map`, `gitops status`, `import-argocd`, or `cub gitops import` results?
3. Did any app/team/variant label mapping look wrong or confusing?
4. Where did setup friction appear most: auth, workers, targets, or import?
5. After this demo, does ConfigHub operational control-plane path make sense? If not, what feels unclear or missing?

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

## Repeatable Import Check

Validate connected import delegation behavior locally:

```bash
make test-import-delegation
```

## Connected Mode Delegation (Current)

`cub-scout import` now tries connected-mode delegation automatically:

```
cub-scout import
  |
  |-- ArgoCD/Flux workloads + required targets present
  |   --> delegate to: cub gitops import (rendered pipeline)
  |
  |-- Helm/Native workloads
  |   --> import directly via cub-scout snapshot path
  |
  `-- Missing gitops prerequisites
      --> fallback to cub-scout snapshot for those workloads
```

This gives a single-command path with mixed ownership support:
- GitOps-managed workloads use `cub gitops import` when possible.
- Helm/Native (and any undelegated GitOps workloads) stay on the scout import path.

Delegation requires targets in the same App Space:
- a Kubernetes discovery target
- and an Argo or Flux renderer target (`argocdrenderer` / `fluxrenderer`)

## Related

- [`examples/import-from-live/`](../import-from-live/) - Simpler demo with just `cub-scout import`
- [CLI-GUIDE.md](../../CLI-GUIDE.md) - Complete CLI reference
- [#201](https://github.com/confighub/cub-scout/issues/201) - Design: connected-mode upgrade path
