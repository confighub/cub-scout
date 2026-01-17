# Repo Pattern: Monorepo with Folders

A single repository with folders per application/environment.

## Structure

```
platform-configs/
├── apps/
│   ├── payment-api/
│   │   ├── base/
│   │   │   ├── deployment.yaml
│   │   │   ├── service.yaml
│   │   │   └── kustomization.yaml
│   │   └── overlays/
│   │       ├── dev/
│   │       │   └── kustomization.yaml
│   │       ├── staging/
│   │       │   └── kustomization.yaml
│   │       └── prod/
│   │           └── kustomization.yaml
│   ├── order-api/
│   │   └── ... (same structure)
│   └── inventory-api/
│       └── ... (same structure)
│
└── argocd/
    └── apps.yaml  # App-of-apps or ApplicationSet
```

## How ConfigHub Sees This

```yaml
Hub: acme-platform
  Source: https://github.com/acme/platform-configs

App Space: apps
  Units:
    - payment-api (3 variants: dev, staging, prod)
    - order-api (3 variants: dev, staging, prod)
    - inventory-api (3 variants: dev, staging, prod)
```

## Key Commands

```bash
# See all apps
cub unit list --space apps

# See all prod variants
cub unit list --where "variant=prod"

# Update one app across all environments
cub unit update payment-api --all-variants --set image.tag=v2.1.0

# Promote from staging to prod
cub promote payment-api --from-variant staging --to-variant prod
```

## ArgoCD Integration

ArgoCD pulls rendered manifests from OCI:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: all-apps
spec:
  generators:
    - git:
        repoURL: https://github.com/acme/platform-configs
        directories:
          - path: apps/*/overlays/*
  template:
    spec:
      source:
        repoURL: oci://ghcr.io/acme/configs
        # ConfigHub pushes rendered manifests here
```

## Skeleton Classification

| Dimension | Value |
|-----------|-------|
| Tool | Argo CD / Flux |
| Repo Count | Monorepo |
| Env Strategy | Overlays (Kustomize) |
| Orchestration | ApplicationSet |

**Skeleton ID:** `argo-appset-mono` or `flux-kust-mono`

## References

- [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](../../../../docs/IMPORT-GIT-REFERENCE-ARCHITECTURES.md) — Pattern 4: Flux Mono-Repo
- [REPO-SKELETON-TAXONOMY.md](../../../../docs/planning/REPO-SKELETON-TAXONOMY.md) — Full taxonomy
- [Kostis: GitOps Folder Structures](https://codefresh.io/blog/the-three-best-folder-structures-for-gitops-and-two-worst-ones/) — Why folders beat branches
