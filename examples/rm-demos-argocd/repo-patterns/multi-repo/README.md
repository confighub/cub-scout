# Repo Pattern: Multi-Repo per Team

## The Problem

Three teams, three repos, three ArgoCD namespaces. During an incident you need to answer:
*"Which teams are still running the old Alpine base image?"*

Nobody has cross-repo visibility. You'd need to `kubectl get` across namespaces and manually match repos.

**cub-scout gives you a single-cluster answer:**

```
$ ./cub-scout map list -q "image=*alpine:3.18*"

  STATUS  NAMESPACE   NAME               OWNER   MANAGED-BY         IMAGE
  ✓       payments    payment-api        ArgoCD  payment-api        api:2.4.1 (alpine:3.18)
  ✓       payments    payment-processor  ArgoCD  payment-processor  processor:1.2 (alpine:3.18)
  ✓       platform    redis              ArgoCD  redis              redis:7.2 (alpine:3.18)
  ✓       orders      order-api          ArgoCD  order-api          api:3.1 (alpine:3.19) ← already updated
```

## Structure

```
payments-team/configs/          # Payments team repo
├── payment-api/
├── payment-processor/
└── argocd/

orders-team/configs/            # Orders team repo
├── order-api/
├── order-processor/
└── argocd/

platform-team/configs/          # Platform team repo
├── redis/
├── postgres/
├── kafka/
└── argocd/
```

## How cub-scout Sees This

```bash
# All workloads across all teams, one view
$ ./cub-scout map list

  STATUS  NAMESPACE  NAME               OWNER   MANAGED-BY
  ✓       payments   payment-api        ArgoCD  payment-api
  ✓       payments   payment-processor  ArgoCD  payment-processor
  ✓       orders     order-api          ArgoCD  order-api
  ✓       orders     order-processor    ArgoCD  order-processor
  ✓       platform   redis              ArgoCD  redis
  ✓       platform   postgres           ArgoCD  postgres
  ✓       platform   kafka              ArgoCD  kafka

# Trace any workload back to its repo
$ ./cub-scout trace deployment/payment-api -n payments

  Deployment/payment-api (payments)
  ├── Owner: ArgoCD
  ├── Application: payment-api (argocd)
  └── Source: github.com/acme/payments-configs@main

# See ownership tree by tool
$ ./cub-scout tree ownership

  ArgoCD (7 resources)
  ├── Application/payment-api → payments/payment-api
  ├── Application/payment-processor → payments/payment-processor
  ├── Application/order-api → orders/order-api
  ├── Application/order-processor → orders/order-processor
  ├── Application/redis → platform/redis
  ├── Application/postgres → platform/postgres
  └── Application/kafka → platform/kafka
```

## Why Multi-Repo Benefits from cub-scout

| Without cub-scout | With cub-scout |
|-------------------|----------------|
| 3 separate ArgoCD views | 1 unified cluster view |
| Can't query across repos | Cluster-wide queries |
| No cross-team visibility | Full ownership visibility |
| Manual incident triage | `map list -q "image=*vulnerable*"` |

## Skeleton Classification

| Dimension | Value |
|-----------|-------|
| Tool | Argo CD (could be Flux) |
| Repo Count | Multi-repo (per team) |
| Env Strategy | Overlays or folders |
| Orchestration | Flat or App-of-Apps |

**Skeleton ID:** `argo-flat-multi` or `argo-aoa-multi`

## See Also

- [Apptique App-of-Apps](../../../apptique-examples/argo-app-of-apps/) — Working multi-app Argo hierarchy
- [Platform Example](../../../platform-example/) — Mixed ownership (Flux + orphans)
