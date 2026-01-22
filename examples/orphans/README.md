# Orphan Resources for Demo

Kubernetes resources that simulate real-world "orphans" - resources deployed via kubectl that GitOps doesn't know about.

## What's an Orphan?

An orphan resource has no GitOps owner:
- No Flux labels (`kustomize.toolkit.fluxcd.io/*`)
- No ArgoCD labels (`argocd.argoproj.io/instance`)
- No Helm labels (`app.kubernetes.io/managed-by: Helm`)
- No ConfigHub labels (`confighub.com/UnitSlug`)

cub-scout detects these as "Native" ownership and highlights them as potential orphans.

## Why Orphans Matter

Every cluster accumulates orphans:
- **Legacy systems** that predate GitOps adoption
- **Temporary resources** from debugging sessions (that nobody deleted)
- **Manual hotfixes** applied during incidents
- **ConfigMaps and Secrets** created via kubectl

These orphans cause problems:
- **Drift** - Live state doesn't match Git
- **Security** - Untracked resources may have vulnerabilities
- **Cost** - Forgotten resources consume capacity
- **Compliance** - No audit trail for manual changes

## Usage

```bash
# Deploy orphan resources
kubectl apply -f realistic-orphans.yaml

# Find them with cub-scout
cub-scout map orphans

# Or in the TUI
cub-scout map
# Press 'o' for orphans view

# Cleanup
kubectl delete -f realistic-orphans.yaml
```

## What's Included

| Namespace | Resources | Simulates |
|-----------|-----------|-----------|
| `legacy-apps` | Prometheus deployment, configs, secrets | Pre-GitOps monitoring |
| `temp-testing` | nginx, busybox deployments | Debug resources |
| `default` | ConfigMaps, Secrets, CronJobs | Manual operations |

Total: ~20 orphan resources across 3 namespaces

## Expected Output

```
cub-scout map orphans

ORPHAN RESOURCES (20)
────────────────────────────────────────
These resources have no GitOps owner.

NAMESPACE       KIND            NAME                    AGE
legacy-apps     Deployment      legacy-prometheus       3d
legacy-apps     Service         legacy-prometheus       3d
legacy-apps     ConfigMap       legacy-prometheus-config 3d
temp-testing    Deployment      debug-nginx             1d
temp-testing    Deployment      debug-busybox           1d
default         Deployment      hotfix-worker           12h
default         ConfigMap       old-feature-flags       7d
default         ConfigMap       manual-override         2d
default         Secret          manual-api-key          5d
default         CronJob         manual-cleanup          4d
...
```

## See Also

- [docs/SCALE-DEMO.md](../../docs/SCALE-DEMO.md) - Full scale demo guide
- [CLI-GUIDE.md](../../CLI-GUIDE.md) - Command reference
