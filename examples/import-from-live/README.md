# Import from Live Cluster

Discover workloads from a running Kubernetes cluster and propose a ConfigHub
App structure -- without touching Git, without modifying anything.

This example uses the "Arnie" pattern: ArgoCD with a folder-per-environment
(`envs/dev`, `envs/staging`, `envs/prod`) in a single deploy repo.

## The Problem

You have workloads running in a cluster. Some are managed by ArgoCD, some by
Helm, and a few are unmanaged leftovers. You want to understand what you have
and organize it into ConfigHub, but you don't want to start by hand-mapping
everything from Git repos.

`cub-scout import --dry-run` reads your live cluster and proposes an App
structure. No Git access required. No cluster modifications. Just read-only
discovery.

## What's Running

This example simulates a cluster with 13 resources across three namespaces:

| Resource | Namespace | Owner | How Detected |
|----------|-----------|-------|--------------|
| Application/myapp-dev | argocd | ArgoCD | ArgoCD Application CR |
| Application/myapp-staging | argocd | ArgoCD | ArgoCD Application CR |
| Application/myapp-prod | argocd | ArgoCD | ArgoCD Application CR |
| Deployment/api | myapp-dev | ArgoCD | `argocd.argoproj.io/instance` label |
| Deployment/api | myapp-staging | ArgoCD | `argocd.argoproj.io/instance` label |
| Deployment/api | myapp-prod | ArgoCD | `argocd.argoproj.io/instance` label |
| Deployment/worker | myapp-dev | ArgoCD | `argocd.argoproj.io/instance` label |
| Deployment/worker | myapp-staging | ArgoCD | `argocd.argoproj.io/instance` label |
| Deployment/worker | myapp-prod | ArgoCD | `argocd.argoproj.io/instance` label |
| StatefulSet/redis | myapp-dev | Helm | `app.kubernetes.io/managed-by: Helm` label |
| StatefulSet/redis | myapp-staging | Helm | `app.kubernetes.io/managed-by: Helm` label |
| StatefulSet/redis | myapp-prod | Helm | `app.kubernetes.io/managed-by: Helm` label |
| ConfigMap/debug-config | myapp-prod | Native | No GitOps labels |

The fixture files in `fixtures/` contain the full YAML manifests.

## Running the Discovery

Preview what cub-scout would propose without making any changes:

```bash
./cub-scout import --dry-run
```

For machine-readable output:

```bash
./cub-scout import --dry-run --json
```

To scope discovery to specific namespaces:

```bash
./cub-scout import --dry-run -n myapp-prod
```

### What the ASCII Output Looks Like

```
Import Preview (dry-run)
════════════════════════════════════════════════════════════════════

Discovered 9 workloads across 3 namespaces

  NAMESPACE       NAME     KIND          OWNER    READY
  myapp-dev       api      Deployment    ArgoCD   1/1
  myapp-dev       worker   Deployment    ArgoCD   1/1
  myapp-dev       redis    StatefulSet   Helm     1/1
  myapp-staging   api      Deployment    ArgoCD   2/2
  myapp-staging   worker   Deployment    ArgoCD   1/1
  myapp-staging   redis    StatefulSet   Helm     1/1
  myapp-prod      api      Deployment    ArgoCD   3/3
  myapp-prod      worker   Deployment    ArgoCD   2/2
  myapp-prod      redis    StatefulSet   Helm     3/3

Suggested App Structure
────────────────────────────────────────────────────────────────────

  App Space: myapp-team

  App: api
    api-dev       → Deployment/myapp-dev/api
    api-staging   → Deployment/myapp-staging/api
    api-prod      → Deployment/myapp-prod/api

  App: worker
    worker-dev    → Deployment/myapp-dev/worker
    worker-staging → Deployment/myapp-staging/worker
    worker-prod   → Deployment/myapp-prod/worker

  App: redis
    redis-dev     → StatefulSet/myapp-dev/redis
    redis-staging → StatefulSet/myapp-staging/redis
    redis-prod    → StatefulSet/myapp-prod/redis

Skipped:
  ConfigMap/myapp-prod/debug-config (Native — no ownership signal)

════════════════════════════════════════════════════════════════════
This is a dry run. No changes were made.
```

## What cub-scout Proposes

The suggestion engine groups workloads into an App structure using two signals:

1. **Component name** from `app.kubernetes.io/name` label (e.g., `api`, `worker`, `redis`)
2. **Environment variant** inferred from ArgoCD Application paths (`envs/dev` becomes `dev`)

Here is the JSON suggestion output (`--dry-run --json`):

```json
{
  "appSpace": "myapp-team",
  "units": [
    {
      "slug": "api-dev",
      "app": "api",
      "variant": "dev",
      "workloads": [
        "Deployment/myapp-dev/api"
      ]
    },
    {
      "slug": "api-prod",
      "app": "api",
      "variant": "prod",
      "workloads": [
        "Deployment/myapp-prod/api"
      ]
    },
    {
      "slug": "api-staging",
      "app": "api",
      "variant": "staging",
      "workloads": [
        "Deployment/myapp-staging/api"
      ]
    },
    {
      "slug": "redis-dev",
      "app": "redis",
      "variant": "dev",
      "workloads": [
        "StatefulSet/myapp-dev/redis"
      ]
    },
    {
      "slug": "redis-prod",
      "app": "redis",
      "variant": "prod",
      "workloads": [
        "StatefulSet/myapp-prod/redis"
      ]
    },
    {
      "slug": "redis-staging",
      "app": "redis",
      "variant": "staging",
      "workloads": [
        "StatefulSet/myapp-staging/redis"
      ]
    },
    {
      "slug": "worker-dev",
      "app": "worker",
      "variant": "dev",
      "workloads": [
        "Deployment/myapp-dev/worker"
      ]
    },
    {
      "slug": "worker-prod",
      "app": "worker",
      "variant": "prod",
      "workloads": [
        "Deployment/myapp-prod/worker"
      ]
    },
    {
      "slug": "worker-staging",
      "app": "worker",
      "variant": "staging",
      "workloads": [
        "Deployment/myapp-staging/worker"
      ]
    }
  ]
}
```

This output matches the golden test at
`cmd/cub-scout/testdata/import-suggest/arnie-pattern.golden.json`.

### Reading the Proposal

The output uses **App language** (what users see) which maps to **ConfigHub API
language** (what the system uses) like this:

| App Language | ConfigHub API | This Example |
|--------------|---------------|--------------|
| App Space | Space | `myapp-team` |
| App (component) | app field on Unit | `api`, `worker`, `redis` |
| Environment | variant field on Unit | `dev`, `staging`, `prod` |
| Unit | Unit | `api-dev`, `api-prod`, etc. |

So when the proposal says "App: api, Environments: dev/staging/prod", it means
three Units (`api-dev`, `api-staging`, `api-prod`) inside the Space `myapp-team`.

### Variant Inference

cub-scout infers variants (environments) from the ArgoCD Application's
`spec.source.path`:

| Application Path | Inferred Variant |
|------------------|-----------------|
| `envs/dev` | `dev` |
| `envs/staging` | `staging` |
| `envs/prod` | `prod` |

The last path segment is used as the variant. For Flux workloads, the same logic
applies to the Kustomization's `spec.path` (e.g., `./apps/api/overlays/dev`
yields `dev`). For Helm-only workloads without path context, cub-scout falls
back to namespace-based inference (e.g., `myapp-dev` yields `dev`).

## Ownership Detection

cub-scout detects ownership by parsing actual labels and annotations on each
resource. It never guesses.

| Owner | Detection Rule | Example Labels |
|-------|---------------|----------------|
| ArgoCD | `argocd.argoproj.io/instance` label present | `argocd.argoproj.io/instance: myapp-dev` |
| Helm | `app.kubernetes.io/managed-by` equals `Helm` | `app.kubernetes.io/managed-by: Helm` |
| Native | None of the known ownership labels found | (no GitOps labels) |

In this example:
- The 6 Deployments (api + worker across 3 envs) are ArgoCD-managed
- The 3 StatefulSets (redis across 3 envs) are Helm-managed
- The debug-config ConfigMap is Native (unmanaged)

Native resources are detected and reported but excluded from the App structure
proposal. They have no ownership signal, so cub-scout cannot safely assign them
to an App.

## What Happens Next

cub-scout's job stops at the proposal. It reads the cluster, detects ownership,
and suggests a structure. It does not write anything to the cluster or to
ConfigHub.

The next step -- actually creating the App Space and Units in ConfigHub -- is
handled by ConfigHub's bridge import. That is ConfigHub's responsibility, not
cub-scout's.

The typical flow:

1. **cub-scout discovers** (this example) -- read-only scan, propose structure
2. **You review** -- adjust naming, merge or split Apps as needed
3. **ConfigHub imports** -- bridge import creates the Space, Units, and OCI pipeline
4. **Flux/Argo deploys** -- your existing deployers continue to run, now tracked by ConfigHub

For details on the ConfigHub side, see the
[ConfigHub bridge import documentation](https://docs.confighub.com/guides/bridge-import).

## Fixtures

The `fixtures/` directory contains Kubernetes manifests representing the cluster
state. These are standard YAML you could apply with `kubectl apply -f`:

| File | Contents |
|------|----------|
| `fixtures/applications.yaml` | 3 ArgoCD Application CRs (dev, staging, prod) |
| `fixtures/deployments.yaml` | 6 Deployments with ArgoCD labels (api + worker x 3 envs) |
| `fixtures/statefulsets.yaml` | 3 StatefulSets with Helm labels (redis x 3 envs) |
| `fixtures/native.yaml` | 1 unmanaged ConfigMap (debug-config) |

The expected JSON output is at `expected-output/suggestion.json`.

## Related Docs

| Doc | What It Covers |
|-----|----------------|
| [CLI-GUIDE.md](../../CLI-GUIDE.md) | Full `import` command reference |
| [docs/howto/import-to-confighub.md](../../docs/howto/import-to-confighub.md) | Canonical import path (Argo/Helm to ConfigHub) |
| [Argo App-of-Apps example](../apptique-examples/argo-app-of-apps/) | ArgoCD hierarchy detection |
| [Flux Monorepo example](../apptique-examples/flux-monorepo/) | Flux ownership detection |
