# How To: Understand Ownership Detection

Map automatically detects who manages each Kubernetes resource. This guide explains how ownership detection works and how to interpret the results.

Owner detection identifies **who** manages a resource. `cub-scout trace` then uses that owner type to resolve the appropriate source chain. See [Trace Ownership](trace-ownership.md#owner-detection-vs-trace-chain).

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

| Owner | Detection | Labels/Annotations Checked |
|-------|-----------|----------------------------|
| **Flux** | Toolkit labels | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` |
| **ArgoCD** | Argo label or tracking-id | `argocd.argoproj.io/instance` label OR `argocd.argoproj.io/tracking-id` annotation |
| **Helm** | Managed-by label | `app.kubernetes.io/managed-by: Helm` |
| **Terraform** | Terraform metadata | `app.terraform.io/run-id` annotation OR `app.terraform.io/managed` label |
| **Crossplane** | Crossplane labels/ownerRefs | `crossplane.io/*` labels or Crossplane owner references |
| **kro** | kro metadata/ownerRefs | `kro.run/*` labels/annotations, kro owner references, or API group containing `kro.run` |
| **ConfigHub** | Unit slug | `confighub.com/UnitSlug` |
| **Custom** | Configured detector rules | `~/.cub-scout/detectors.yaml` / `$CUB_SCOUT_OWNERSHIP_DETECTORS` |
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

ArgoCD is detected by the presence of the Argo-specific label OR annotation:

```yaml
# Detection via label (most common)
metadata:
  labels:
    argocd.argoproj.io/instance: my-app

# Detection via tracking annotation (alternative)
metadata:
  annotations:
    argocd.argoproj.io/tracking-id: "my-app:apps/Deployment:default/nginx"
```

**Note:** If the `argocd.argoproj.io/instance` label exists but is empty, cub-scout falls back to `app.kubernetes.io/instance` for the owner name.

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

### Custom Detector Configuration

You can extend ownership detection without writing Go code.

Create `~/.cub-scout/detectors.yaml`:

```yaml
detectors:
  - name: internal-platform
    labels:
      - key: platform.company.com/managed-by
        value: platform-controller
    owner_name: Internal Platform
    owner_type: custom
```

Rules:

1. Built-in detectors run first.
2. Custom detectors run in file order.
3. First matching custom detector wins.

If config is invalid, cub-scout warns and falls back to built-ins.

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

Check for the Argo-specific label or annotation:
```bash
kubectl get deploy YOUR-DEPLOY -n YOUR-NS -o yaml | grep -E "argocd.argoproj.io/(instance|tracking-id)"
```

If neither `argocd.argoproj.io/instance` label nor `argocd.argoproj.io/tracking-id` annotation is present, the resource won't be detected as ArgoCD.

### Flux resource shows as Native

Check for toolkit labels:
```bash
kubectl get deploy YOUR-DEPLOY -n YOUR-NS -o yaml | grep toolkit.fluxcd.io
```

If no toolkit labels exist, Flux may not be adding them (check your Kustomization/HelmRelease).

## Next Steps

- [Find Orphans](find-orphans.md) - Identify unmanaged resources
- [Trace Ownership](trace-ownership.md) - Follow the chain to source
