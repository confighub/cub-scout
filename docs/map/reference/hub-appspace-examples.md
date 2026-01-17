# Hub/AppSpace View Examples

Visual examples of the `B` key Hub/AppSpace grouping for common reference architectures.

---

## How It Works

Press `B` in Hub mode (`cub-agent map --hub`) to toggle between:
- **Flat view**: Org → Spaces (alphabetical)
- **Hub/AppSpace view**: Org → Hub (platform) → AppSpaces (teams)

**Categorization rules:**
- **Hub (Platform)**: `platform-*`, `infra-*`, `hub-*`, `shared-*`, `*-base`, `*-infra`
- **AppSpaces**: Everything else (team workspaces)

---

## Example 1: KubeCon Demo (Online Boutique)

Real spaces: `apptique-dev`, `apptique-prod`, `appchat-dev`, `appchat-prod`, `appvote-dev`, `appvote-prod`, `platform-dev`, `platform-prod`

### Flat View (default)

```
confighub ▾
├─ appchat-dev                    4 units
├─ appchat-prod                   4 units
├─ apptique-dev                  11 units
├─ apptique-prod                 11 units
├─ appvote-dev                    6 units
├─ appvote-prod                   6 units
├─ platform-dev                   7 targets
└─ platform-prod                  7 targets
```

### Hub/AppSpace View (press B)

```
confighub ▾
│
├─ Hub (Platform)                         2 spaces
│  ├─ platform-dev                        7 targets
│  │  └─ Workers: dev (Ready)
│  └─ platform-prod                       7 targets
│     └─ Workers: prod-worker (Ready)
│
└─ AppSpaces                              6 spaces
   ├─ appchat-dev                         4 units
   │  └─ chat-frontend, chat-backend, redis, postgres
   ├─ appchat-prod                        4 units
   ├─ apptique-dev                       11 units
   │  └─ frontend, cartservice, productcatalog, ...
   ├─ apptique-prod                      11 units
   ├─ appvote-dev                         6 units
   │  └─ voting-app, result-app, redis, worker, ...
   └─ appvote-prod                        6 units
```

**What this shows:**
- Platform team owns `platform-*` spaces with workers and targets
- App teams (apptique, appchat, appvote) have dev/prod spaces with units
- Clear separation: infrastructure vs application configs

---

## Example 2: TraderX (Multi-Region)

Real spaces: `fluffy-cub-traderx-base`, `fluffy-cub-traderx-infra`, `fluffy-cub-traderx-prod-asia`, `fluffy-cub-traderx-prod-eu`, `fluffy-cub-traderx-prod-us`

### Flat View

```
confighub ▾
├─ fluffy-cub-traderx-base
├─ fluffy-cub-traderx-infra
├─ fluffy-cub-traderx-prod-asia
├─ fluffy-cub-traderx-prod-eu
└─ fluffy-cub-traderx-prod-us
```

### Hub/AppSpace View (press B)

```
confighub ▾
│
├─ Hub (Platform)                         2 spaces
│  ├─ fluffy-cub-traderx-base            Base templates
│  │  └─ Units: traderx-base (upstream for all regions)
│  └─ fluffy-cub-traderx-infra           Shared infra
│     └─ Units: ingress-controller, cert-manager, monitoring
│
└─ AppSpaces                              3 spaces
   ├─ fluffy-cub-traderx-prod-asia       Asia production
   │  └─ Units: traderx (variant=prod, region=asia)
   ├─ fluffy-cub-traderx-prod-eu         EU production
   │  └─ Units: traderx (variant=prod, region=eu)
   └─ fluffy-cub-traderx-prod-us         US production
      └─ Units: traderx (variant=prod, region=us)
```

**What this shows:**
- Base and infra spaces are Hub (platform governance)
- Regional prod spaces are AppSpaces (team deployments)
- Each region clones from `traderx-base`

---

## Example 3: curious-cub (Full Pattern)

Real spaces: `curious-cub-base`, `curious-cub-infra`, `curious-cub-dev`, `curious-cub-staging`, `curious-cub-prod`

### Flat View

```
confighub ▾
├─ curious-cub
├─ curious-cub-base
├─ curious-cub-dev
├─ curious-cub-infra
├─ curious-cub-prod
└─ curious-cub-staging
```

### Hub/AppSpace View (press B)

```
confighub ▾
│
├─ Hub (Platform)                         2 spaces
│  ├─ curious-cub-base                   Base catalog
│  │  └─ Units: app-template, backend-base, frontend-base
│  └─ curious-cub-infra                  Shared infrastructure
│     └─ Units: external-secrets, cert-manager, ingress-nginx
│
└─ AppSpaces                              4 spaces
   ├─ curious-cub                        Default workspace
   ├─ curious-cub-dev                    Development
   │  └─ Units: curious-app (variant=dev)
   ├─ curious-cub-staging                Staging
   │  └─ Units: curious-app (variant=staging)
   └─ curious-cub-prod                   Production
      └─ Units: curious-app (variant=prod)
```

**What this shows:**
- `*-base` and `*-infra` → Hub (shared/governed)
- `*-dev`, `*-staging`, `*-prod` → AppSpaces (environments as team workspaces)
- Units have `variant` labels for environment

---

## Example 4: IITS Pattern (Jesper Examples)

Real spaces: `jesper-argocd`, `jesper-fluxcd`, `example-jesper-argocd-team`

### Flat View

```
confighub ▾
├─ example-jesper-argocd-team
├─ jesper-argocd
└─ jesper-fluxcd
```

### Hub/AppSpace View (press B)

```
confighub ▾
│
├─ Hub (Platform)                         0 spaces
│  └─ (none detected)
│
└─ AppSpaces                              3 spaces
   ├─ example-jesper-argocd-team         ArgoCD example
   │  └─ Deployer: ArgoCD
   │  └─ Units: podinfo, nginx, redis
   ├─ jesper-argocd                      ArgoCD workspace
   │  └─ Deployer: ArgoCD
   │  └─ Units: app-of-apps, applicationsets
   └─ jesper-fluxcd                      FluxCD workspace
      └─ Deployer: Flux
      └─ Units: kustomization-apps, helmreleases
```

**What this shows:**
- No platform spaces (small example, no shared infra)
- Each workspace uses ONE deployer (ArgoCD or Flux)
- App Space = team + deployer boundary

---

## Example 5: acorn-bear (Enterprise Multi-Region)

Real spaces: `acorn-bear-infra`, `acorn-bear-asia-prod`, `acorn-bear-eu-prod`, `acorn-bear-eu-staging`, `acorn-bear-us-staging`

### Hub/AppSpace View (press B)

```
confighub ▾
│
├─ Hub (Platform)                         1 space
│  └─ acorn-bear-infra                   Shared infrastructure
│     └─ Units: cluster-autoscaler, external-dns, vault
│
└─ AppSpaces                              4 spaces
   ├─ acorn-bear-asia-prod               Asia production
   │  └─ Units: (variant=prod, region=asia)
   ├─ acorn-bear-eu-prod                 EU production
   │  └─ Units: (variant=prod, region=eu)
   ├─ acorn-bear-eu-staging              EU staging
   │  └─ Units: (variant=staging, region=eu)
   └─ acorn-bear-us-staging              US staging
      └─ Units: (variant=staging, region=us)
```

**What this shows:**
- `*-infra` → Hub (shared across all regions)
- Regional spaces → AppSpaces (team workspaces per region/env)
- Labels encode both variant AND region

---

## Details Pane (Enter on Unit)

When you select a unit and press Enter, the details pane shows:

```
┌─────────────────────────────────────────────────────────────────────────┐
│ UNIT DETAILS                                                             │
│                                                                          │
│ Name:        traderx                                                     │
│ Space:       fluffy-cub-traderx-prod-us                                 │
│ Created:     2026-01-10T14:30:00Z                                       │
│                                                                          │
│ REVISIONS                                                                │
│ Head:        42                                                          │
│ Applied:     41                                                          │
│ Status:      BEHIND (1 revision pending)                                │
│                                                                          │
│ TARGET                                                                   │
│ Name:        us-prod-cluster                                            │
│ Toolchain:   Kubernetes/YAML                                            │
│                                                                          │
│ WORKER                                                                   │
│ Name:        us-prod-worker                                             │
│ Status:      Ready                                                       │
│                                                                          │
│ LABELS                                                                   │
│ app:         traderx                                                     │
│ variant:     prod                                                        │
│ region:      us                                                          │
│ team:        trading                                                     │
│ tier:        backend                                                     │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Query Examples by Architecture

### KubeCon Demo: "All prod units"
```bash
cub unit list --where "Labels.variant='prod'"
# Returns: apptique-prod units, appchat-prod units, appvote-prod units
```

### TraderX: "All Asia deployments"
```bash
cub unit list --where "Labels.region='asia'"
# Returns: fluffy-cub-traderx-prod-asia units
```

### curious-cub: "All variants of curious-app"
```bash
cub unit list --where "Labels.app='curious-app'"
# Returns: curious-app from dev, staging, prod spaces
```

---

---

## Example 6: Banko Pattern (Real-World Flux Production)

**Source:** Banko's production Flux repo structure (2026-01-14)

**Git structure:**
```
├── clusters/
│   ├── cluster-1.example.com/component-a/
│   ├── cluster-1.example.com/component-b/
│   ├── cluster-2.example.com/component-a/
│   └── cluster-3.example.com/component-a/
├── platform/                    # Versioned platform components
│   ├── cert-manager/v1.0.0/
│   ├── grafana/v2.1.0/
│   └── ingress/v3.0.0/
└── apps/                        # Internal applications
    ├── app-1/v1.0.0/
    └── app-2/v2.0.0/
```

### Hub/AppSpace View (press B)

```
confighub ▾
│
├─ Hub (Platform)                         1 space
│  └─ banko-platform                     Platform components
│     ├─ Units: cert-manager (version=v1.0.0)
│     ├─ Units: grafana (version=v2.1.0)
│     └─ Units: ingress (version=v3.0.0)
│
└─ AppSpaces                              4 spaces
   ├─ banko-apps                         Internal applications
   │  ├─ Units: app-1 (version=v1.0.0)
   │  └─ Units: app-2 (version=v2.0.0)
   ├─ cluster-1.example.com              Cluster 1
   │  └─ Units: component-a, component-b, component-c
   ├─ cluster-2.example.com              Cluster 2
   │  └─ Units: component-a, component-b
   └─ cluster-3.example.com              Cluster 3
      └─ Units: component-a
```

**What this shows:**
- `platform/` directory → Hub (versioned shared components)
- `apps/` directory → AppSpace (team-owned internal apps)
- Each `clusters/{name}` → AppSpace (per-cluster deployments)
- Version info preserved in labels: `version=v1.0.0`

**Key patterns:**
- Cluster-per-directory structure
- Explicit versioning (`v1.0.0/`) for platform components
- Clear separation: platform vs apps vs cluster-specific

---

## Example 7: Arnie Pattern (ArgoCD Folders-per-Environment)

**Source:** Arnie's certification course - "Use folders for environments"

**Git structure:**
```
├── base/                        # Common to all environments
├── envs/
│   ├── integration-gpu/
│   ├── integration-non-gpu/
│   ├── load-gpu/
│   ├── load-non-gpu/
│   ├── prod-asia/
│   ├── prod-eu/
│   ├── prod-us/
│   ├── qa/
│   ├── staging-asia/
│   ├── staging-eu/
│   └── staging-us/
└── variants/                    # Mixins/components
    ├── asia/
    ├── eu/
    ├── non-prod/
    ├── prod/
    └── us/
```

### Hub/AppSpace View (press B)

```
confighub ▾
│
├─ Hub (Platform)                         2 spaces
│  ├─ arnie-base                         Base templates
│  │  └─ Units: app-base (upstream for all envs)
│  └─ arnie-variants                     Shared variants/mixins
│     ├─ Units: variant-asia
│     ├─ Units: variant-eu
│     ├─ Units: variant-us
│     ├─ Units: variant-prod
│     └─ Units: variant-non-prod
│
└─ AppSpaces                             11 spaces
   ├─ integration-gpu                    Integration (GPU)
   │  └─ Units: app (variant=integration, gpu=true)
   ├─ integration-non-gpu                Integration (non-GPU)
   │  └─ Units: app (variant=integration, gpu=false)
   ├─ load-gpu                           Load testing (GPU)
   ├─ load-non-gpu                       Load testing (non-GPU)
   ├─ qa                                 QA environment
   │  └─ Units: app (variant=qa)
   ├─ staging-asia                       Staging Asia
   │  └─ Units: app (variant=staging, region=asia)
   ├─ staging-eu                         Staging EU
   │  └─ Units: app (variant=staging, region=eu)
   ├─ staging-us                         Staging US
   │  └─ Units: app (variant=staging, region=us)
   ├─ prod-asia                          Production Asia
   │  └─ Units: app (variant=prod, region=asia)
   ├─ prod-eu                            Production EU
   │  └─ Units: app (variant=prod, region=eu)
   └─ prod-us                            Production US
      └─ Units: app (variant=prod, region=us)
```

**What this shows:**
- `base/` → Hub (template for all envs)
- `variants/` → Hub (shared mixins referenced by envs)
- Each `envs/{name}` → AppSpace (one per environment)
- Labels encode: `variant`, `region`, `gpu` dimensions

**Key patterns:**
- Folder-per-environment (not branches!)
- Promotion = file copy: `cp envs/qa/version.yml envs/staging-us/`
- Everything on single branch
- Variants are composable mixins

### Promotion Flow (from Arnie)

```
┌──────────────────────────────────────────────────────────────────────┐
│  PROMOTION: Just copy files!                                          │
├──────────────────────────────────────────────────────────────────────┤
│                                                                       │
│  QA → staging-us:                                                     │
│    cp envs/qa/version.yml envs/staging-us/version.yml                │
│                                                                       │
│  staging-us → prod-us (with settings):                               │
│    cp envs/staging-us/version.yml envs/prod-us/                      │
│    cp envs/staging-us/settings.yml envs/prod-us/                     │
│                                                                       │
│  Global change to all prod:                                           │
│    Edit variants/prod/prod.yml                                        │
│                                                                       │
└──────────────────────────────────────────────────────────────────────┘
```

---

## Summary: Pattern → Hub/AppSpace Mapping

| Pattern | Hub Contains | AppSpaces Contain |
|---------|--------------|-------------------|
| **KubeCon Demo** | `platform-*` (workers, targets) | `app*-dev`, `app*-prod` (units) |
| **TraderX** | `*-base`, `*-infra` | `*-prod-{region}` |
| **curious-cub** | `*-base`, `*-infra` | `*-dev`, `*-staging`, `*-prod` |
| **IITS/Jesper** | (none) | Per-deployer workspaces |
| **Banko (Flux)** | `platform/` (versioned) | `clusters/*`, `apps/` |
| **Arnie (ArgoCD)** | `base/`, `variants/` | `envs/*` (one per environment) |

---

## See Also

- [Hub/AppSpace Model](../../../internal/planning/map/02-HUB-APPSPACE-MODEL.md) - Conceptual model
- [Adoption Patterns](../../../internal/planning/map/USE-CASE-ADOPTION-PATTERNS.md) - Banko/Arnie source details
- [Keybindings Reference](keybindings.md) - All keyboard shortcuts
- [Views Reference](views.md) - What each view shows
