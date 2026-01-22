# GitOps Repository Structures

Reference architectures for common GitOps repository layouts. Use these patterns when importing existing GitOps setups into ConfigHub.

**Named patterns:** These patterns have nicknames from real-world examples:
- **Arnie pattern** = Pattern 1 & 2 (Environment-per-Folder)
- **Banko pattern** = Pattern 3 (Cluster-per-Directory)
- **Fluxy pattern** = Pattern 4 (Multi-Repo Fleet)

See [Hub/AppSpace Examples](hub-appspace-examples.md) for how these render in the TUI (`B` key).

---

## Pattern 1: Environment-per-Folder (Kustomize)

Best for: Argo CD or Flux with Kustomize overlays.

### Structure

```
gitops-repo/
├── base/                    # Common to all environments
│   └── deployment.yaml
├── envs/
│   ├── dev/
│   │   └── kustomization.yaml
│   ├── staging/
│   │   └── kustomization.yaml
│   ├── prod-us/
│   │   └── kustomization.yaml
│   ├── prod-eu/
│   │   └── kustomization.yaml
│   └── prod-asia/
│       └── kustomization.yaml
└── variants/                # Reusable mixins
    ├── prod/
    │   └── values-prod.yaml
    ├── non-prod/
    │   └── values-non-prod.yaml
    └── gpu/
        └── values-gpu.yaml
```

### How It Works

Each environment folder contains a `kustomization.yaml` that:
1. References `../../base`
2. Includes relevant variants as components
3. Applies environment-specific patches

**Example: `envs/staging/kustomization.yaml`**
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: staging
resources:
  - ../../base
components:
  - ../../variants/non-prod
patchesStrategicMerge:
  - version.yaml      # Image tag (promotable)
  - replicas.yaml     # Replica count
  - settings.yaml     # Business settings
```

### Promotion

All promotions are file copy operations:
```bash
# Promote version from dev to staging
cp envs/dev/version.yaml envs/staging/version.yaml
git commit -m "Promote v1.2.3 to staging"
```

### ConfigHub Mapping

| GitOps | ConfigHub |
|--------|-----------|
| `envs/{env}` | Space or Unit variant |
| `variants/{name}` | Shared configuration (Hub) |
| `base/` | Base Unit template |

---

## Pattern 2: Environment-per-Folder (Helm)

Best for: Argo CD or Flux with Helm charts.

### Structure

```
helm-repo/
├── chart/
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
├── common/
│   └── values-common.yaml
├── variants/
│   ├── prod/
│   │   └── values-prod.yaml
│   └── non-prod/
│       └── values-non-prod.yaml
└── envs/
    ├── dev/
    │   ├── values-version.yaml    # Promotable
    │   ├── values-replicas.yaml
    │   └── values-settings.yaml   # Promotable
    ├── staging/
    │   └── ...
    └── prod/
        └── ...
```

### How It Works

Render with multiple values files:
```bash
helm template chart/ \
  --values common/values-common.yaml \
  --values variants/prod/values-prod.yaml \
  --values envs/prod/values-version.yaml \
  --values envs/prod/values-settings.yaml
```

### Promotion

Same as Kustomize — copy files between environment folders:
```bash
cp envs/staging/values-version.yaml envs/prod/values-version.yaml
```

---

## Pattern 3: Cluster-per-Directory (Flux)

Best for: Multi-cluster Flux deployments with shared platform components.

### Structure

```
flux-repo/
├── clusters/
│   ├── cluster-1/
│   │   ├── platform-a/
│   │   │   └── kustomization.yaml
│   │   ├── platform-b/
│   │   │   └── kustomization.yaml
│   │   └── app-x/
│   │       └── kustomization.yaml
│   ├── cluster-2/
│   │   ├── platform-a/
│   │   │   └── kustomization.yaml
│   │   └── platform-b/
│   │       └── kustomization.yaml
│   └── cluster-3/
│       └── platform-a/
│           └── kustomization.yaml
│
├── platform/                      # Shared infrastructure (versioned)
│   ├── cert-manager/
│   │   └── v1.14.0/
│   │       ├── sync.yaml
│   │       └── namespace.yaml
│   ├── monitoring/
│   │   └── v2.5.0/
│   │       ├── sync.yaml
│   │       └── values.yaml
│   └── ingress/
│       └── v1.9.0/
│           └── sync.yaml
│
└── apps/                          # Team applications
    ├── app-x/
    │   ├── base/
    │   ├── dev/
    │   └── prod/
    └── app-y/
        └── v1.0.0/
            └── manifests.yaml
```

### How It Works

**Flux Kustomization** points at `./clusters/{cluster-name}` to bootstrap everything.

**Example: `clusters/cluster-1/platform-a/kustomization.yaml`**
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../../platform/cert-manager/v1.14.0/sync.yaml
  - ../../../platform/cert-manager/v1.14.0/namespace.yaml
  - cluster-overrides.yaml
```

### Key Patterns

| Directory | Purpose |
|-----------|---------|
| `clusters/` | One directory per cluster |
| `platform/` | Shared infra, versioned (`v1.0.0/`) |
| `apps/` | Team applications |

### ConfigHub Mapping

| GitOps | ConfigHub |
|--------|-----------|
| `clusters/{name}` | Target (cluster) |
| `platform/{component}` | Hub (shared platform config) |
| `apps/{app}` | AppSpace (team-owned) |

---

---

## Pattern 4: Multi-Repo Fleet (Flux with OCI)

Best for: Enterprise Flux with separate repos per layer and OCI artifacts.

### Structure

Three separate repositories:

**fleet-repo/** (Platform team - orchestration)
```
fleet-repo/
├── clusters/
│   ├── staging/
│   │   └── flux-system/
│   │       └── flux-instance.yaml
│   └── production/
│       └── flux-system/
│           └── flux-instance.yaml
├── tenants/
│   └── team-a.yaml              # ResourceSet per tenant
└── terraform/
    └── bootstrap.tf             # Cluster bootstrap
```

**infra-repo/** (Platform team - add-ons)
```
infra-repo/
├── components/
│   ├── cert-manager/
│   │   ├── controllers/
│   │   │   ├── base/
│   │   │   ├── production/
│   │   │   └── staging/
│   │   └── configs/
│   │       ├── base/
│   │       ├── production/
│   │       └── staging/
│   ├── monitoring/
│   └── ingress/
└── update-policies/
    └── cert-manager.yaml        # Automate version updates
```

**apps-repo/** (Dev teams - applications)
```
apps-repo/
├── components/
│   └── {namespace}/
│       ├── base/
│       │   └── release.yaml
│       ├── production/
│       │   └── values.yaml
│       └── staging/
│           └── values.yaml
└── update-policies/
    └── app-updates.yaml
```

### Key Patterns

| Repo | Layer | Owner | Contains |
|------|-------|-------|----------|
| fleet | Orchestration | Platform | Cluster configs, tenants |
| infra | Infrastructure | Platform | Add-ons (cert-manager, monitoring) |
| apps | Applications | Dev teams | HelmReleases by namespace |

### OCI Artifacts ("Gitless GitOps")

Instead of Flux pointing directly at Git:
1. CI builds manifests from Git
2. Pushes OCI artifact to registry
3. Flux pulls from OCI registry

This decouples Git structure from deployment.

### ConfigHub Mapping

| GitOps | ConfigHub |
|--------|-----------|
| `fleet-repo` | Organization config |
| `infra-repo` | Hub (shared platform) |
| `apps-repo` | AppSpaces (per team/namespace) |
| `clusters/{name}` | Target |

---

## Detecting Patterns with Map

`cub-scout map` auto-detects these patterns via:

| Pattern | Detection |
|---------|-----------|
| Flux Kustomization | `kustomize.toolkit.fluxcd.io/*` labels |
| Flux HelmRelease | `helm.toolkit.fluxcd.io/*` labels |
| Argo CD Application | `argocd.argoproj.io/instance` label |
| Helm | `app.kubernetes.io/managed-by: Helm` |

Run `cub-scout trace` to see the full chain from Git → Deployer → Resource.

---

## Importing to ConfigHub

The `cub-scout import` wizard respects existing structures:

```bash
# Preview what would be created
cub-scout import --dry-run

# Interactive wizard
cub-scout import --wizard
```

The wizard will:
1. Detect your GitOps tool (Flux, Argo, Helm)
2. Discover environment/cluster structure
3. Suggest ConfigHub Units that mirror your layout
4. Create Spaces matching your environments

---

## See Also

- [hub-appspace-examples.md](hub-appspace-examples.md) — How patterns render in Hub/AppSpace TUI view
- [import-to-confighub.md](../howto/import-to-confighub.md) — Import guide
- [ownership-detection.md](../howto/ownership-detection.md) — How ownership is detected
- [trace-ownership.md](../howto/trace-ownership.md) — Trace resource chains
