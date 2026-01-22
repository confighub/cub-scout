# GitOps Overview

## What is GitOps?

GitOps uses Git repositories as the source of truth for your infrastructure. Changes flow through Git, and automation ensures your cluster matches what's in Git.

```
Git Repository  →  GitOps Controller  →  Kubernetes Cluster
(desired state)     (watches & applies)    (actual state)
```

## The GitOps Loop

1. **You push to Git** - Change a YAML file
2. **Controller detects** - Flux/ArgoCD sees the change
3. **Reconciliation** - Controller applies changes to cluster
4. **Drift correction** - If someone modifies the cluster directly, it's reset to match Git

## GitOps Tools

### Flux

Flux uses **Kustomization** resources to apply manifests:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: my-app
  namespace: flux-system
spec:
  sourceRef:
    kind: GitRepository
    name: platform
  path: ./apps/my-app
```

**Detection labels:**
- `kustomize.toolkit.fluxcd.io/name`
- `kustomize.toolkit.fluxcd.io/namespace`

### ArgoCD

ArgoCD uses **Application** resources:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  namespace: argocd
spec:
  source:
    repoURL: https://github.com/example/platform
    path: apps/my-app
  destination:
    server: https://kubernetes.default.svc
```

**Detection labels:**
- `app.kubernetes.io/instance`
- `argocd.argoproj.io/instance`

### Helm (via GitOps)

Helm can be managed by GitOps using **HelmRelease** (Flux) or as an Application source (ArgoCD):

```yaml
# Flux HelmRelease
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: redis
spec:
  chart:
    spec:
      chart: redis
      sourceRef:
        kind: HelmRepository
        name: bitnami
```

**Detection labels:**
- `app.kubernetes.io/managed-by: Helm`

## The Chain

GitOps creates a chain from source to deployed resources:

```
GitRepository/HelmRepository
        ↓
Kustomization/HelmRelease/Application
        ↓
Deployment/Service/ConfigMap
        ↓
ReplicaSet → Pods
```

Use `cub-scout trace` to visualize this chain:

```bash
cub-scout trace deployment/my-app -n production
```

## Native (Non-GitOps) Resources

Resources without GitOps ownership are called **Native**. They were likely applied with:
- `kubectl apply`
- `kubectl create`
- Helm install (without GitOps wrapper)

**Risks of Native resources:**
- No audit trail in Git
- Lost if cluster is rebuilt
- May conflict with GitOps reconciliation

Use `cub-scout map orphans` to find them.

## The Clobbering Problem

When someone modifies a GitOps-managed resource directly:

```bash
kubectl scale deployment/my-app --replicas=5  # Manual change
```

The GitOps controller will **reset it** to match Git. This is by design but can be surprising.

See: [The Clobbering Problem](clobbering-problem.md)

## Multi-Environment Patterns

GitOps supports multiple environments through:

1. **Kustomize overlays** - Base + environment-specific patches
2. **Separate branches** - main, staging, prod branches
3. **Path-based** - Different paths for different environments

```
apps/my-app/
├── base/           # Common config
└── overlays/
    ├── dev/        # replicas: 1
    ├── staging/    # replicas: 2
    └── prod/       # replicas: 3, HPA, TLS
```

## Further Reading

- [Flux Documentation](https://fluxcd.io/docs/)
- [ArgoCD Documentation](https://argo-cd.readthedocs.io/)
- [The Clobbering Problem](clobbering-problem.md)
- [docs/diagrams/flux-architecture.svg](../diagrams/flux-architecture.svg)
