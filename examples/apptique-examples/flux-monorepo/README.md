# Flux Monorepo Pattern

## The Problem

You have a single repo with Kustomize base + overlays for dev and prod.
During a deployment issue, you need to know:
*"Is the dev Kustomization reconciling from the right path? What's the Git source?"*

With kubectl you'd need to check the Flux Kustomization CR, cross-reference the GitRepository,
and verify the path. Three separate commands.

**cub-scout shows the full chain:**

```
$ ./cub-scout trace deployment/frontend -n apptique-dev

  Deployment/frontend (apptique-dev)
  ├── Owner: Flux
  ├── Kustomization: apptique-dev (flux-system)
  │   └── Path: ./apps/apptique/overlays/dev
  └── Source: GitRepository/apptique-examples → confighubai/confighub-agent@main
```

## Architecture

```
GitRepository/apptique-examples
├── Kustomization/apptique-dev  → overlays/dev  → Deployment/frontend (apptique-dev)
└── Kustomization/apptique-prod → overlays/prod → Deployment/frontend (apptique-prod)
```

```
flux-monorepo/
├── clusters/
│   ├── dev/kustomization.yaml      # Flux Kustomization CR
│   └── prod/kustomization.yaml
├── apps/apptique/
│   ├── base/                       # Shared manifests
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── kustomization.yaml
│   └── overlays/
│       ├── dev/                    # Dev patches
│       │   ├── kustomization.yaml
│       │   └── namespace.yaml
│       └── prod/                   # Prod patches
│           ├── kustomization.yaml
│           └── namespace.yaml
└── infrastructure/
    └── gitrepository.yaml          # Flux GitRepository CR
```

**How it works:**
1. GitRepository points to the repo
2. Each cluster has a Flux Kustomization CR pointing to an overlay path
3. Kustomize builds base + overlay → produces Deployments in the target namespace

## What It Demonstrates

| What you'll see | Why it matters |
|-----------------|----------------|
| Flux ownership detected via labels | `kustomize.toolkit.fluxcd.io/*` labels work |
| Trace from Deployment → Kustomization → GitRepository | Full provenance chain |
| Dev and prod as separate Kustomizations | Environment isolation visible |
| Base/overlay structure detected by `parse-repo` | Repo skeleton recognition |

## Quick Start

```bash
# Create the GitRepository source
kubectl apply -f examples/apptique-examples/flux-monorepo/infrastructure/gitrepository.yaml

# Deploy dev environment
kubectl apply -f examples/apptique-examples/flux-monorepo/clusters/dev/kustomization.yaml

# (Optional) Deploy prod environment
kubectl apply -f examples/apptique-examples/flux-monorepo/clusters/prod/kustomization.yaml

# Watch Flux reconcile
flux get kustomizations --watch

# See ownership
./cub-scout map list -q "owner=Flux"

# Trace to source
./cub-scout trace deployment/frontend -n apptique-dev

# Check GitOps pipeline health
./cub-scout gitops status
```

## Ownership Labels

Flux adds these labels to managed resources:

```yaml
labels:
  kustomize.toolkit.fluxcd.io/name: apptique-dev
  kustomize.toolkit.fluxcd.io/namespace: flux-system
```

cub-scout uses these to detect Flux ownership and trace back to the Kustomization and GitRepository CRs.

## Offline Repo Parsing

```bash
# Parse without a cluster
./cub-scout parse-repo --path examples/apptique-examples/flux-monorepo

# Expected: detects flux-tenant-mono skeleton, base/overlay structure
```

## When to Use Flux Monorepo

- Teams that prefer Kustomize over Helm
- Base + overlay pattern for environment promotion
- Single repo for all environments (simpler Git workflow)
- Flux-native clusters

## Cleanup

```bash
kubectl delete kustomization apptique-dev apptique-prod -n flux-system
kubectl delete gitrepository apptique-examples -n flux-system
kubectl delete ns apptique-dev apptique-prod
```

## See Also

- [Flux Boutique](../../flux-boutique/) — Simpler single-app Flux example
- [Platform Example](../../platform-example/) — Full Arnie (env-per-folder) pattern
- [Argo ApplicationSet](../argo-applicationset/) — Same app, ArgoCD instead of Flux
- [Monorepo Pattern](../../rm-demos-argocd/repo-patterns/monorepo/) — Pattern comparison
