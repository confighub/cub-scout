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
cub-scout tree ownership

OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════

Flux (28 resources)
────────────────────────────────────────────────────────────────────
  ├── boutique/cart              Deployment   ✓ 2/2
  ├── boutique/checkout          Deployment   ✓ 1/1
  ├── boutique/frontend          Deployment   ✓ 3/3
  └── ... (24 more)

Helm (5 resources)
────────────────────────────────────────────────────────────────────
  ├── monitoring/prometheus      Deployment   ✓ 1/1
  └── ... (4 more)

Native (20 resources)  ⚠ ORPHANS — not managed by GitOps
────────────────────────────────────────────────────────────────────
  No GitOps labels detected — likely kubectl-applied

  NAMESPACE       KIND            NAME                     AGE
  ├── legacy-apps
  │   ├── Deployment     legacy-prometheus                 3d
  │   ├── Service        legacy-prometheus                 3d
  │   └── ConfigMap      legacy-prometheus-config          3d
  ├── temp-testing
  │   ├── Deployment     debug-nginx                      1d
  │   └── Deployment     debug-busybox                    1d
  └── default
      ├── Deployment     hotfix-worker                    12h
      ├── ConfigMap      old-feature-flags                 7d
      ├── ConfigMap      manual-override                   2d
      ├── Secret         manual-api-key                    5d
      └── CronJob        manual-cleanup                    4d

════════════════════════════════════════════════════════════════════
Ownership Distribution:

  Flux       ████████████████████████████░░░░░░░░░░░░  53%
  Helm       █████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   9%
  Native     ████████████████████░░░░░░░░░░░░░░░░░░░░  38%  ← orphans

→ To trace an orphan:  cub-scout trace deploy/debug-nginx -n temp-testing
→ To import orphans:   cub-scout import -n legacy-apps
```

## See Also

- [docs/getting-started/scale-demo.md](../../docs/getting-started/scale-demo.md) - Full scale demo guide
- [CLI-GUIDE.md](../../CLI-GUIDE.md) - Command reference
