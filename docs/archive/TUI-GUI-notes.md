# TUI vs GUI: What Each Can Show

Notes on capabilities of the TUI (cub-agent CLI) vs GUI (confighub.com) for LIVE cluster data and GIT source data.

---

## Key Insight: LIVE vs GIT

**You don't need Git to infer variant** — the Kustomization object stores `spec.path` in the cluster. This is **certain** data, not inference.

| From LIVE (Certain) | From GIT (Additional) |
|---------------------|----------------------|
| What resources exist | Other variants not deployed here |
| Who owns them | Base definitions (`apps/base/`) |
| Kustomization `spec.path` | What SHOULD exist (drift) |
| Applied revision/SHA | History, PRs, pending commits |

The `cub-agent import` command reads `Kustomization.spec.path` directly from the cluster to infer variant (e.g., `./staging` → `variant=staging`).

---

## The Mapping Rule

| What in Git/Cluster | Maps To in ConfigHub |
|---------------------|----------------------|
| Git repo URL | **Source** (with pattern metadata) |
| `base/` folders | **Base Unit** in Hub Catalog |
| Each deployed Application/Kustomization | **Unit** in App Space |
| Overlays/variants | **Labels** on Unit (`variant=prod`) |
| Tenant folders | **App Space** per tenant |
| Orchestration parents (App-of-Apps root) | **Nothing** — deployer mechanism, not config |

**Key insight:** Deployer orchestration (App-of-Apps parent, ApplicationSet generator) is NOT modeled as Units. ConfigHub has its own orchestration via Workers → Targets.

---

## Scope Rule: TUI vs GUI

```
┌─────────────────────────────────────────────────────────────────┐
│  TUI (cub-agent)         │  GUI (confighub.com)                 │
├──────────────────────────┼──────────────────────────────────────┤
│  LIVE only               │  LIVE + GIT + Other sources          │
│  1 Cluster               │  N Clusters (Fleet)                  │
└──────────────────────────┴──────────────────────────────────────┘
```

**This is a design rule, not a limitation.**

- **TUI** = Fast, local, single-cluster. Derives everything from LIVE cluster data.
- **GUI** = Fleet-wide, multi-source. Aggregates LIVE (via Workers) + GIT (via provider integration) + other sources.

### Architecture: Hub Owns Workers

```
HUB (owns Worker lifecycle)
├── Workers
│   ├── worker-east ──────────────────▶ prod-east (Target)
│   └── worker-west ──────────────────▶ prod-west (Target)
│
└── APP SPACES (select worker for deploy)
    └── payments-team
        ├── Unit: payment-api → deploys via worker-east
        └── Unit: payment-api → deploys via worker-west
```

- **Hub** owns Workers and their lifecycle
- **App Spaces** select which Worker to use for deploying Units
- **Workers** connect Hub to Targets and enable `refresh` / `import` operations

See: [CLI-REFERENCE.md — Connected Mode](CLI-REFERENCE.md#connected-mode-cub-agent-vs-cub-cli)

---

## LIVE Data Capabilities

### What We Know From LIVE (regardless of repo structure)

| Deployer | Path Source | Example | TUI Support |
|----------|-------------|---------|-------------|
| Flux Kustomization | `spec.path` | `./apps/staging/payment` | ✅ Implemented |
| Flux HelmRelease | `spec.chart.spec.sourceRef` | chart name + values | ⚠️ Partial |
| Argo Application | `spec.source.path` | `apps/payment/overlays/prod` | ✅ Implemented |
| Argo ApplicationSet | generator patterns | cluster/git/list generators | ❌ TODO |

**Key insight:** We don't need to parse Git — the deployer objects already tell us the path.

The `cub-agent import` command reads these paths directly from the cluster and uses them to infer variant (e.g., `./staging` → `variant=staging`).

### TUI vs GUI Capabilities

| Capability | TUI (cub-agent) | GUI (confighub.com) |
|------------|-----------------|---------------------|
| **Single cluster** | ✅ Direct kubectl access | ✅ Via Worker |
| **Multiple clusters** | ⚠️ Switch contexts manually | ✅ All Targets aggregated |
| **Ownership detection** | ✅ Labels/annotations | ✅ Same, fleet-wide |
| **Trace** | ✅ flux trace / argocd get | ✅ Same + visual chain |
| **Scan (Kyverno)** | ✅ PolicyReports | ✅ Same + history |
| **Kustomization path** | ✅ Can read spec.path | ✅ Same, all clusters |
| **Infer variant** | ✅ From path | ✅ From path, all clusters |
| **Real-time** | ✅ Live query | ⚠️ Worker poll interval |

### What We Can Know From LIVE (Certain)

- What resources exist
- Who owns them (Flux, Argo, Helm, ConfigHub, Native)
- Current state (replicas, conditions, health)
- Owning deployer objects (Kustomization, HelmRelease, Application)
- Source references (`spec.sourceRef` → GitRepository name)
- **Path in repo** (`spec.path` → `apps/staging/podinfo`)
- Applied revision (`status.lastAppliedRevision` → commit SHA)
- What was applied (`last-applied-configuration` annotation)

### Inferring App/Variant From Kustomization Path

```
Kustomization.spec.path = "./staging"     → variant=staging
Kustomization.spec.path = "./production"  → variant=prod
Kustomization.spec.path = "./base/..."    → Base Unit (no variant)
```

This works because Flux standard repos use path structure:
```
apps/
├── base/podinfo/      → Base definition
├── staging/           → Overlay for staging
└── production/        → Overlay for production
```

### Key Insight: Path Is Available From LIVE

**You don't need to parse Git to infer variant** — the Kustomization object stores `spec.path` in the cluster. The `cub-agent import` command reads this directly:

1. Workload has label `kustomize.toolkit.fluxcd.io/name: my-ks`
2. Fetch `Kustomization/my-ks` from cluster
3. Read `spec.path: ./staging`
4. Infer `variant=staging`

This is **certain** data (not inference) because Flux explicitly stores the path.

### Inference Priority Order

When inferring app/variant for import, we check in this order:

| Priority | Source | Example | Confidence |
|----------|--------|---------|------------|
| 0 | Flux Kustomization `spec.path` | `./staging` → staging | High (Flux stores it) |
| 0 | Argo Application `spec.source.path` | `apps/prod` → prod | High (Argo stores it) |
| 1 | K8s label `app.kubernetes.io/name` | `payment-api` | High (standard) |
| 2 | K8s label `environment` or `env` | `production` | High (explicit) |
| 3 | Namespace pattern | `myapp-prod` → prod | Medium (convention) |
| 4 | Workload name | `payment-worker` | Low (fallback) |

GitOps deployer paths take priority because they're the most reliable signal — the deployer explicitly stores the path.

---

## GIT Data Capabilities

| Capability | TUI (cub-agent) | GUI (confighub.com) |
|------------|-----------------|---------------------|
| **Read repo structure** | ⚠️ Needs --from-git impl | ✅ GitHub/GitLab integration |
| **See all overlays** | ⚠️ Clone + parse | ✅ API access to repo |
| **Base definitions** | ⚠️ Parse kustomization.yaml | ✅ Visual tree view |
| **Pending commits** | ❌ Not deployed = not visible | ✅ Compare Git vs Live |
| **PR/commit history** | ❌ | ✅ Git provider integration |
| **Drift (Git vs Live)** | ⚠️ Limited to last-applied | ✅ Full diff |

### What Git Adds Over LIVE

| Git Adds | Why It Matters |
|----------|----------------|
| **Other variants** | Cluster only shows what's deployed HERE |
| **Base definitions** | `apps/base/` may not be deployed anywhere |
| **What SHOULD exist** | Drift by deletion: removed from cluster but still in Git |
| **History** | Who changed it, when, why, PR approval |
| **DRY templates** | Kustomize overlays, Helm values - intent before rendering |
| **Pending changes** | Commits not yet reconciled |

---

## Import Wizard Comparison

| What | TUI | GUI |
|------|-----|-----|
| **Single cluster import** | ✅ `--from-live` | ✅ Point-and-click |
| **Multi-cluster import** | ⚠️ `--context a --context b` | ✅ "Select targets" checkbox |
| **Infer app/variant** | ✅ From Kustomization path | ✅ Same + visual preview |
| **See full variant matrix** | ❌ Only what's deployed | ✅ Git + Live combined |
| **Suggest Base Units** | ❌ Can't see undeployed bases | ✅ From Git structure |
| **Preview before import** | ✅ `--dry-run` | ✅ Visual wizard |

---

## Import Approaches

### 1. From LIVE (Single Cluster)

```bash
cub-agent import --from-live --namespace my-app
```

- ✅ Works standalone
- ✅ Infers variant from Kustomization path
- ❌ Can't see other clusters
- ❌ Can't see undeployed variants

### 2. From LIVE (Multiple Clusters)

```bash
cub-agent import --from-live \
  --context k8s-staging \
  --context k8s-prod
```

- ✅ Aggregates across clusters
- ✅ Sees full deployed variant matrix
- ❌ Needs kubeconfig for all clusters
- ❌ Can't see undeployed variants

### 3. From Fleet (ConfigHub Connected)

```bash
cub-agent import --from-fleet
```

- ✅ Queries all Targets via ConfigHub API
- ✅ Workers already reporting data
- ✅ No extra kubeconfig needed
- ❌ Can't see undeployed variants (Git-only)

### 4. From Git

```bash
cub-agent import --from-git https://github.com/org/flux-repo
```

- ✅ Sees complete structure including bases
- ✅ Knows all possible variants
- ✅ Can suggest Base Units for Hub catalog
- ❌ Complex parsing (Kustomize, Helm)
- ❌ DRY vs WET gap

### 5. GUI (Best of Both)

ConfigHub GUI can correlate:
- **LIVE**: What's deployed where (from Workers)
- **GIT**: What variants exist (from repo integration)

Shows unified view:
```
Found across your fleet:
  k8s-staging:  apps (path: ./staging)   → variant=staging
  k8s-prod:     apps (path: ./production) → variant=prod

Git also contains:
  apps/base/podinfo  → Base Unit (not deployed, template only)
```

---

## Summary

```
┌─────────────────────────────────────────────────────────────────────┐
│                           TUI (cub-agent)                            │
├─────────────────────────────────────────────────────────────────────┤
│  LIVE: ✅ Full access, one cluster at a time                        │
│  GIT:  ⚠️ Could implement, but complex parsing                      │
│  Best for: Quick checks, single cluster, CI/CD pipelines            │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                        GUI (confighub.com)                           │
├─────────────────────────────────────────────────────────────────────┤
│  LIVE: ✅ Fleet-wide, all targets aggregated                        │
│  GIT:  ✅ Provider integration, full structure                      │
│  Best for: Fleet view, import wizard, Git+Live correlation          │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Flux Standard Repo Structure

Reference: https://github.com/fluxcd/flux2-kustomize-helm-example

```
clusters/
├── staging/
│   └── apps.yaml         # Kustomization: path: ./staging
└── production/
    └── apps.yaml         # Kustomization: path: ./production

apps/
├── base/podinfo/         # Base HelmRelease (Hub catalog candidate)
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── repository.yaml
│   └── release.yaml
├── staging/              # Overlay (variant=staging)
│   ├── kustomization.yaml  # resources: ../base/podinfo
│   └── podinfo-values.yaml
└── production/           # Overlay (variant=prod)
    ├── kustomization.yaml
    └── podinfo-values.yaml
```

From LIVE we see: `Kustomization.spec.path = ./staging` → infer `variant=staging`

From GIT we additionally see: `apps/base/podinfo` exists as upstream for all variants

---

## See Also

- [JOURNEY-IMPORT.md](JOURNEY-IMPORT.md) — Step-by-step import walkthrough
- [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) — Full import reference
- [IMPORT-GIT-REFERENCE-ARCHITECTURES.md](IMPORT-GIT-REFERENCE-ARCHITECTURES.md) — GitOps patterns → ConfigHub mapping
- [IMPORT-FROM-SOURCES.md](IMPORT-FROM-SOURCES.md) — Flow from TUI → GUI
- [02-HUB-APPSPACE-MODEL.md](planning/map/02-HUB-APPSPACE-MODEL.md) — Hub/App Space model
- [TUI-TRACE.md](TUI-TRACE.md) — Trace documentation
