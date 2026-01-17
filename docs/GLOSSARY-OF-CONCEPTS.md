# ConfigHub Concepts Glossary

Quick reference for ConfigHub terminology.

---

## The Model

### Organization (Org)

Top-level container. Everything belongs to an Org.

```
Org: acme-corp
└── (Hubs, Spaces, Users)
```

### Platform Hub

Governance layer that constrains what teams can do. Owns base templates.

```
Org: acme-corp
└── Platform Hub: platform-team
    ├── Base Catalog (templates)
    ├── Policies (what's allowed)
    └── App Spaces (team workspaces)
```

**Hub owns the skeletons.** Base templates, shared configs, and reusable patterns live in the Hub's Base Catalog.

### App Space

Team workspace. One deployer (Argo OR Flux), one team. Contains Units.

```
App Space: payments-team
├── Deployer: ArgoCD
└── Units: payment-api, order-svc, redis
```

**App Space ≠ Environment.** Environments are labels (`variant=prod`), not separate spaces.

### Unit

Single deployable workload with labels. The atomic element of config.

```
Unit: payment-api
├── Labels: app=payment-api, variant=prod, region=us-east
├── Source: apps/payment-api/overlays/prod
└── Target: prod-east-cluster
```

### App (Application)

**App = a name.** Just a label value like `app=payment-api`.

The "application" emerges from querying Units with that label:
```bash
cub query "Labels['app'] = 'payment-api'"
# Returns all Units for payment-api across all variants/regions
```

### Variant

Label indicating environment or configuration flavor: `variant=prod`, `variant=staging`, `variant=canary`.

**Not a folder.** Git paths like `overlays/prod` map to `variant=prod` label on the Unit.

---

## Sources of Truth

| Source | Role | Format |
|--------|------|--------|
| **Git** | WHAT you wrote | DRY (templates, overlays) |
| **ConfigHub** | HOW it should run | WET (rendered, resolved) |
| **Cluster** | NOW running | Live state |

### DRY vs WET

- **DRY** (Don't Repeat Yourself): Git stores templates, overlays, variables
- **WET** (Write Everything Twice): ConfigHub stores rendered, resolved config

**WET is operational truth** — what you see in ConfigHub is what deploys.

---

## Ownership & Detection

### Owner

Who manages a Kubernetes resource:

| Owner | Detection |
|-------|-----------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| **Argo CD** | `argocd.argoproj.io/instance` label |
| **Helm** | `app.kubernetes.io/managed-by: Helm` |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | No GitOps labels (kubectl apply, direct API) |

### Orphan

Resource with no GitOps owner. Created via `kubectl apply` or direct API call. Not tracked by Git.

---

## GitOps Concepts

### Source

Git repository registered with ConfigHub. Contains pattern metadata (app-of-apps, applicationset, mono-repo, etc.).

### Deployer

Tool that syncs Git to cluster: Flux CD or Argo CD.

**One App Space = One Deployer.** Can't mix Flux and Argo in same App Space.

### Target

Kubernetes cluster managed by ConfigHub. Connected via Worker.

### Worker

Bridge between ConfigHub and cluster. Runs locally, connects outbound to ConfigHub API.

```
Hub ──▶ Worker ──▶ Target (cluster)
```

---

## Import Concepts

### LIVE Import

Discover workloads from running cluster. TUI capability.

```bash
./cub-agent import -n payment-prod
# Scans cluster, detects ownership, suggests App Space structure
```

### GIT Import

Parse Git repo structure for base templates, overlays, variants. GUI capability.

### Base Unit

Template in Hub's Base Catalog. Created from `base/` folders in Git. Never deployed directly.

```
apps/payment-api/base/  →  Base Unit in Hub Catalog
apps/payment-api/overlays/prod/  →  Unit (references base)
```

---

## CCVE Concepts

### CCVE

Cloud Configuration Vulnerability and Exposure. Configuration anti-pattern that causes problems.

Format: `CCVE-2025-XXXX`

### Categories

| Category | What It Detects |
|----------|-----------------|
| **SOURCE** | Git/repo issues |
| **RENDER** | Template/overlay issues |
| **APPLY** | Deployment failures |
| **DRIFT** | Live ≠ Git |
| **DEPEND** | Missing dependencies |
| **STATE** | Controller stuck/failed |
| **ORPHAN** | No GitOps owner |
| **CONFIG** | Misconfiguration |

### Severity

- **Critical**: Outage imminent or data loss risk
- **High**: Service degradation likely
- **Medium**: Best practice violation
- **Low**: Suboptimal but functional

---

## See Also

- [02-HUB-APPSPACE-MODEL.md](planning/map/02-HUB-APPSPACE-MODEL.md) — Full model specification
- [IMPORT-JESPER-OVERVIEW.md](planning/historical/IMPORT-JESPER-OVERVIEW.md) — Import architecture
- [CCVE-GUIDE.md](CCVE-GUIDE.md) — CCVE user guide
