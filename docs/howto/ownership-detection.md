# How To: Understand Ownership Detection

Map automatically detects who manages each Kubernetes resource. This guide explains how ownership detection works and how to interpret the results.

## The Problem

Your cluster has resources from multiple sources:
- Flux deployed some via Kustomizations
- ArgoCD deployed others via Applications
- Helm installed some charts
- Someone used `kubectl apply` directly

**Question:** Who owns this deployment?

## The Solution

Run map to see ownership automatically:

```bash
cub-scout map list
```

Output:
```
NAME            NAMESPACE    OWNER      STATUS
payment-api     prod         Flux       ✓ Synced
frontend        prod         ArgoCD     ✓ Synced
redis           prod         Helm       ✓ Deployed
debug-pod       prod         Native     ⚠ Orphan
```

## How Detection Works

Map checks labels on each resource to determine ownership:

| Owner | Detection | Labels Checked |
|-------|-----------|----------------|
| **Flux** | Toolkit labels | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` |
| **ArgoCD** | Both required | `app.kubernetes.io/instance` AND `argocd.argoproj.io/instance` |
| **Helm** | Managed-by label | `app.kubernetes.io/managed-by: Helm` |
| **ConfigHub** | Unit slug | `confighub.com/UnitSlug` |
| **Native** | Nothing detected | No GitOps ownership labels |

### Flux Detection

Flux adds toolkit labels when it deploys resources:

```yaml
# Resource deployed by Flux Kustomization
metadata:
  labels:
    kustomize.toolkit.fluxcd.io/name: my-app
    kustomize.toolkit.fluxcd.io/namespace: flux-system
```

```yaml
# Resource deployed by Flux HelmRelease
metadata:
  labels:
    helm.toolkit.fluxcd.io/name: my-release
    helm.toolkit.fluxcd.io/namespace: flux-system
```

### ArgoCD Detection

ArgoCD requires BOTH labels (this prevents false positives):

```yaml
metadata:
  labels:
    app.kubernetes.io/instance: my-app        # Required
    argocd.argoproj.io/instance: my-app       # Also required
```

**Why both?** Some resources have only `app.kubernetes.io/instance` from other tools. Requiring both ensures accurate detection.

### Helm Detection

Helm sets the managed-by label:

```yaml
metadata:
  labels:
    app.kubernetes.io/managed-by: Helm
```

### Native Detection

If no GitOps labels are found, the resource is marked as **Native**. This usually means:
- Someone ran `kubectl apply` directly
- A controller created the resource
- Labels were removed

## Filter by Owner

Show only specific owners:

```bash
# Only Flux resources
cub-scout map list -q "owner=Flux"

# Only ArgoCD resources
cub-scout map list -q "owner=ArgoCD"

# Only Native (unmanaged) resources
cub-scout map list -q "owner=Native"

# All GitOps-managed resources
cub-scout map list -q "owner!=Native"
```

## TUI View

In the interactive TUI, resources are color-coded by owner:

| Owner | Color |
|-------|-------|
| Flux | Cyan |
| ArgoCD | Purple |
| Helm | Orange |
| ConfigHub | Blue |
| Native | Gray |

Press `w` (Workloads) to see resources grouped by owner.

## Troubleshooting

### Resource shows wrong owner

Check the resource's labels:
```bash
kubectl get deploy YOUR-DEPLOY -n YOUR-NS -o jsonpath='{.metadata.labels}' | jq
```

Map uses labels for detection. If labels are missing or incorrect, ownership will be wrong.

### ArgoCD resource shows as Native

Ensure BOTH labels are present:
- `app.kubernetes.io/instance`
- `argocd.argoproj.io/instance`

If only one is present, the resource won't be detected as ArgoCD.

### Flux resource shows as Native

Check for toolkit labels:
```bash
kubectl get deploy YOUR-DEPLOY -n YOUR-NS -o yaml | grep toolkit.fluxcd.io
```

If no toolkit labels exist, Flux may not be adding them (check your Kustomization/HelmRelease).

## Next Steps

- [Find Orphans](find-orphans.md) - Identify unmanaged resources
- [Trace Ownership](trace-ownership.md) - Follow the chain to source
