# Repo Pattern: ArgoCD ApplicationSets

## The Problem

Your ApplicationSet generates Applications dynamically across clusters.
During an incident you need: *"Which clusters actually received the payment-api deployment?"*

ArgoCD's UI shows each Application individually. With 32 production clusters, scrolling through the UI is not an answer.

**cub-scout shows you what's actually running:**

```
$ ./cub-scout map list -q "owner=ArgoCD" -q "name=payment-api*"

  STATUS  NAMESPACE  NAME                          OWNER   MANAGED-BY
  ✓       payments   payment-api                   ArgoCD  payment-api-prod-us-east-1
  ✓       payments   payment-api                   ArgoCD  payment-api-prod-us-west-1
  ✓       payments   payment-api                   ArgoCD  payment-api-prod-eu-west-1
  ✗       payments   payment-api                   ArgoCD  payment-api-prod-ap-south-1  ← PROBLEM
```

## Structure

```yaml
# Single ApplicationSet generates Applications for all clusters
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: payment-api
  namespace: argocd
spec:
  generators:
    - clusters:
        selector:
          matchLabels:
            environment: production
  template:
    metadata:
      name: 'payment-api-{{name}}'
    spec:
      source:
        repoURL: https://github.com/acme/configs
        path: apps/payment-api
        targetRevision: HEAD
      destination:
        server: '{{server}}'
        namespace: payments
```

## How cub-scout Sees This

```bash
# See all ArgoCD-managed resources
$ ./cub-scout map list -q "owner=ArgoCD"

  STATUS  NAMESPACE  NAME         OWNER   MANAGED-BY
  ✓       payments   payment-api  ArgoCD  payment-api-prod-us-east-1
  ✓       payments   payment-api  ArgoCD  payment-api-prod-us-west-1
  ✓       payments   payment-api  ArgoCD  payment-api-prod-eu-west-1

# Trace any generated app back to its source
$ ./cub-scout trace deployment/payment-api -n payments

  Deployment/payment-api (payments)
  ├── Owner: ArgoCD
  ├── Application: payment-api-prod-us-east-1 (argocd)
  └── Source: github.com/acme/configs → apps/payment-api@HEAD

# Check GitOps health for all ArgoCD apps
$ ./cub-scout gitops status

  GitOps Pipeline Health
  ═══════════════════════
  ArgoCD Applications: 32 synced, 1 degraded
  ✗ payment-api-prod-ap-south-1: SyncFailed (ImagePullBackOff)

# Scan for configuration issues across generated apps
$ ./cub-scout scan -n payments

  RISK SCAN: payments namespace
  ─────────────────────────────
  HIGH (1)
  [RISK-2025-0001] payments/payment-api — missing resource limits
```

## Why ApplicationSets Benefit from cub-scout

| ApplicationSet Alone | + cub-scout |
|---------------------|-------------|
| See generated Apps in ArgoCD UI | Query all generated workloads in one command |
| Click through each app individually | `map list -q "owner=ArgoCD"` across all |
| Manual tracking of sync failures | `gitops status` shows all failures at once |
| No single-cluster config scanning | `scan` checks all generated resources |

## Skeleton Classification

| Dimension | Value |
|-----------|-------|
| Tool | Argo CD |
| Repo Count | Mono or Multi |
| Env Strategy | Cluster selector labels |
| Orchestration | ApplicationSet (generator) |

**Skeleton ID:** `argo-appset-mono` or `argo-appset-multi`

## See Also

- [Apptique ApplicationSet](../../../apptique-examples/argo-applicationset/) — Working ApplicationSet with directory generator
- [Apptique App-of-Apps](../../../apptique-examples/argo-app-of-apps/) — Alternative: parent manages children
