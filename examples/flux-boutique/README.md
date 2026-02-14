# Flux Boutique Demo

## The Problem

You have 5 microservices deployed by Flux from the same Git repo.
A team member asks: *"Which Kustomization deploys the payment service?"*
With `kubectl` alone you'd need to cross-reference GitRepositories, Kustomizations, and Deployments manually.

**cub-scout answers this in one command:**

```
$ ./cub-scout trace deployment/payment -n boutique

  Deployment/payment (boutique)
  ├── Owner: Flux
  ├── Kustomization: payment (boutique)
  └── Source: GitRepository/boutique → stefanprodan/podinfo@master
```

## What It Demonstrates

| What you'll see | Why it matters |
|-----------------|----------------|
| 5 services, all Flux-managed | Ownership detection works across multiple Kustomizations |
| Single GitRepository → 5 Kustomizations | Source fan-out visibility |
| Full trace from Deployment → Kustomization → GitRepository | Answers "where does this come from?" instantly |
| No orphans or unmanaged resources | Everything has a clear owner |

## Architecture

```
GitRepository/boutique (stefanprodan/podinfo)
├── Kustomization/frontend → Deployment/frontend
├── Kustomization/cart → Deployment/cart
├── Kustomization/checkout → Deployment/checkout
├── Kustomization/payment → Deployment/payment
└── Kustomization/shipping → Deployment/shipping
```

Pattern: **Flux monorepo** — one GitRepository, multiple Kustomizations with patches.
This is the most common Flux deployment pattern for microservices.

## Quick Start

```bash
# Requires: Flux installed on your cluster
kubectl apply -f boutique.yaml

# Wait for deployments
kubectl wait --for=condition=available deployment --all -n boutique --timeout=120s

# See ownership at a glance
./cub-scout map

# Trace any service to its source
./cub-scout trace deployment/frontend -n boutique
```

## Resources Created

| Resource | Count | Description |
|----------|-------|-------------|
| Namespace | 1 | `boutique` |
| GitRepository | 1 | Points to stefanprodan/podinfo |
| Kustomization | 5 | One per microservice |
| Deployment | 5 | frontend, cart, checkout, payment, shipping |
| Service | 5 | One per deployment |
| HPA | 5 | Auto-scaling for each service |

## Offline Use

```bash
# Scan the fixture without a cluster
./cub-scout scan --file boutique.yaml
```

## Cleanup

```bash
kubectl delete ns boutique
```

## See Also

- [Apptique Flux Monorepo](../apptique-examples/flux-monorepo/) — multi-environment version (dev/prod overlays)
- [Platform Example](../platform-example/) — full env-per-folder (Arnie) pattern with orphans
