# Flux Boutique Demo

A multi-service Flux demo showcasing cub-scout TUI views with 5 microservices.

## What It Shows

| TUI View | What You'll See |
|----------|-----------------|
| `s` Status | 7 deployers, 10 workloads, 90% GitOps managed |
| `w` Workloads | All services grouped under Flux |
| `p` Pipelines | 5 Kustomizations + podinfo + ArgoCD app |
| `T` Trace | Full chain: GitRepository → Kustomization → Deployment |
| `G` Git Sources | Single repo deploying multiple services |

## Quick Setup

```bash
# Apply to any cluster with Flux installed
kubectl apply -f boutique.yaml

# Wait for deployments
kubectl wait --for=condition=available deployment --all -n boutique --timeout=120s

# Explore with TUI
cub-scout map
```

## Architecture

```
GitRepository/boutique (stefanprodan/podinfo)
├── Kustomization/frontend → Deployment/frontend
├── Kustomization/cart → Deployment/cart
├── Kustomization/checkout → Deployment/checkout
├── Kustomization/payment → Deployment/payment
└── Kustomization/shipping → Deployment/shipping
```

All 5 services use the same source (podinfo) with different names via Kustomize patches.
This simulates a real microservices architecture where multiple services share infrastructure patterns.

## Resources Created

| Resource | Count | Description |
|----------|-------|-------------|
| Namespace | 1 | `boutique` |
| GitRepository | 1 | Points to stefanprodan/podinfo |
| Kustomization | 5 | One per microservice |
| Deployment | 5 | frontend, cart, checkout, payment, shipping |
| Service | 5 | One per deployment |
| HPA | 5 | Auto-scaling for each service |

## Cleanup

```bash
kubectl delete ns boutique
```

## See Also

- [FluxCD Community Microservices Demo](https://github.com/fluxcd-community/microservices-demo) - Full 20-service demo (requires newer K8s)
- [Google Online Boutique](https://github.com/GoogleCloudPlatform/microservices-demo) - 11-service e-commerce demo
- [flux2-kustomize-helm-example](https://github.com/fluxcd/flux2-kustomize-helm-example) - Multi-env reference architecture
