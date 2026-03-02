# Combined Git + Live Cluster Alignment

This example shows `cub-scout combined --suggest --json` aligning a Git repo
structure with live cluster state. It uses the "Banko" pattern -- a Flux monorepo
with Kustomize base/overlay per app.

## The Scenario

You have a payments team ("Banko") that runs two services via Flux.
You want to understand: does the cluster match what's in Git?

## 1. Here's the Git Repo

```
git-repo/
├── apps/
│   ├── payment-api/
│   │   ├── base/
│   │   │   ├── deployment.yaml
│   │   │   ├── service.yaml
│   │   │   └── kustomization.yaml
│   │   └── overlays/
│   │       ├── dev/kustomization.yaml
│   │       └── prod/kustomization.yaml
│   ├── payment-worker/
│   │   ├── base/
│   │   │   ├── deployment.yaml
│   │   │   └── kustomization.yaml
│   │   └── overlays/
│   │       ├── dev/kustomization.yaml
│   │       └── prod/kustomization.yaml
│   └── notifications-service/          <-- defined in Git, not deployed yet
│       └── base/
│           ├── deployment.yaml
│           └── kustomization.yaml
└── platform/
    └── monitoring/
        └── kustomization.yaml
```

Standard Flux monorepo. Each app has a `base/` and environment-specific
`overlays/`. Kustomizations patch replicas and resource limits per environment.

## 2. Here's What's Running

The cluster has five Deployments across two namespaces:

| Namespace      | Deployment       | Owner  | Replicas | Notes                 |
|----------------|------------------|--------|----------|-----------------------|
| payment-dev    | payment-api      | Flux   | 1        | Matches Git overlay   |
| payment-dev    | payment-worker   | Flux   | 1        | Matches Git overlay   |
| payment-prod   | payment-api      | Flux   | 3        | Matches Git overlay   |
| payment-prod   | payment-worker   | Flux   | 2        | Matches Git overlay   |
| payment-prod   | cache-warmer     | Native | 1        | Not in Git            |

The Flux-managed Deployments carry `kustomize.toolkit.fluxcd.io/*` labels.
The `cache-warmer` was applied manually -- no GitOps labels at all.

## 3. Here's What cub-scout Finds

```bash
./cub-scout combined \
  --git-path examples/combined-git-live/git-repo \
  --namespace payment-dev,payment-prod \
  --suggest --json
```

The alignment result sorts every app into one of three buckets:

| App                   | Status       | What It Means                               |
|-----------------------|--------------|---------------------------------------------|
| payment-api (dev)     | aligned      | In Git, in cluster, Flux labels match       |
| payment-api (prod)    | aligned      | In Git, in cluster, Flux labels match       |
| payment-worker (dev)  | aligned      | In Git, in cluster, Flux labels match       |
| payment-worker (prod) | aligned      | In Git, in cluster, Flux labels match       |
| notifications-service | git-only     | Defined in Git, not deployed anywhere       |
| cache-warmer          | cluster-only | Running in cluster, no matching Git source  |

cub-scout doesn't guess whether `git-only` or `cluster-only` is a problem.
It reports the facts. You decide what to do about it.

Sample summary from this scenario:

```text
SUMMARY
  aligned: 4
  git-only: 1
  cluster-only: 1

NON-ALIGNED
  notifications-service  git-only
  cache-warmer          cluster-only
```

## 4. Here's the Suggested App Model

When you add `--suggest`, cub-scout generates a suggested ConfigHub app model:

```
PROPOSED APP MODEL (SUGGESTION ONLY)

  Catalog Bases
    payment-api            (apps/payment-api/base)
    payment-worker         (apps/payment-worker/base)
    notifications-service  (apps/notifications-service/base)

  Team Scope: banko-team
    Deployer: Flux
    Reconciliation Rules (proposed policy, not applied by combined):
      variant=prod  -> drift:revert, approval:required
      variant=dev   -> drift:accept, approval:none

    payment-api
      payment-api-dev    (variant=dev)   aligned
      payment-api-prod   (variant=prod)  aligned

    payment-worker
      payment-worker-dev  (variant=dev)   aligned
      payment-worker-prod (variant=prod)  aligned

    notifications-service
      notifications-service (variant=default)  NOT DEPLOYED

    cache-warmer
      cache-warmer (variant=default)  ORPHAN (not in Git)

  Summary: 4 aligned, 1 git-only, 1 cluster-only
```

Note: current JSON output still uses legacy keys like `model: "hub-appspace"` and
`appSpace` for compatibility, even when we describe the result as an app model.

Variant inference comes from the Kustomization overlay paths:
`overlays/dev` becomes variant `dev`, `overlays/prod` becomes variant `prod`.

## Key Points

- **Combined mode merges two information sources.** Git tells you what should
  exist. The cluster tells you what actually exists. cub-scout shows the gap.

- **Three states, no ambiguity.** `aligned` means both sources agree. `git-only`
  means intent without deployment. `cluster-only` means deployment without intent.

- **Read-only.** cub-scout never modifies the cluster or the Git repo. The
  `--suggest` flag produces a proposal, not an action.

- **Policy is suggested, not enforced here.** Reconciliation/approval rules in
  `--suggest` output are planning defaults for ConfigHub, not an applied control loop.

- **Variant inference is deterministic.** It comes from Kustomization paths, not
  guessing. If the path contains `overlays/prod`, the variant is `prod`.

## Files in This Example

| File | Purpose |
|------|---------|
| `git-repo/` | Simulated Flux monorepo with base/overlay Kustomize structure |
| `cluster-fixtures/deployments.yaml` | Cluster Deployments with Flux labels + one orphan |
| `expected-output/alignment.json` | Full JSON output from `combined --suggest --json` |

## Try It

If you have a cluster with these resources deployed:

```bash
# Apply the fixtures
kubectl create namespace payment-dev
kubectl create namespace payment-prod
kubectl apply -f examples/combined-git-live/cluster-fixtures/deployments.yaml

# Run combined alignment
./cub-scout combined \
  --git-path examples/combined-git-live/git-repo \
  --namespace payment-dev,payment-prod \
  --suggest

# JSON output for scripting
./cub-scout combined \
  --git-path examples/combined-git-live/git-repo \
  --namespace payment-dev,payment-prod \
  --suggest --json | jq '.alignment[] | select(.status != "aligned")'

# Optional: quick check against expected output fixture
./cub-scout combined \
  --git-path examples/combined-git-live/git-repo \
  --namespace payment-dev,payment-prod \
  --suggest --json > /tmp/combined-live.json

jq -r '.alignment | group_by(.status) | map({status: .[0].status, count: length})' /tmp/combined-live.json
jq -r '.alignment | group_by(.status) | map({status: .[0].status, count: length})' examples/combined-git-live/expected-output/alignment.json
```

## See Also

- [Flux Monorepo Pattern](../apptique-examples/flux-monorepo/) -- Single-app Flux example
- [Fleet Import](../fleet-import/) -- Aggregating imports from multiple clusters
- [CLI-GUIDE.md](../../CLI-GUIDE.md) -- Full CLI reference
