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

> **Transitioning:** The Hub/AppSpace model is being replaced by App/Deployment/Target.
> See [ConfigHub Model](#confighub-model-app-centric) below for the new language.
> The current API still uses these concepts.

Governance layer that constrains what teams can do. Owns base templates.

```
Org: acme-corp
└── Platform Hub: platform-team
    ├── Base Catalog (templates)
    ├── Policies (what's allowed)
    └── Spaces (team workspaces)
```

### Space (formerly App Space)

Team workspace. One deployer (Argo OR Flux), one team. Contains Units.
In the new model, a Space maps to where an App's Deployments live.

```
Space: payments-team
├── Deployer: ArgoCD
└── Units: payment-api, order-svc, redis
```

**Space ≠ Environment.** Environments are labels (`variant=prod`), not separate spaces.

### Unit

Single deployable workload component with labels. In the new model, a Unit
maps to a component of an App.

```
Unit: payment-api
├── Labels: app=payment-api, variant=prod, region=us-east
├── Source: apps/payment-api/overlays/prod
└── Target: prod-east-cluster
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

**One Space = One Deployer.** Can't mix Flux and Argo in the same Space.

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
./cub-scout import -n payment-prod
# Scans cluster, detects ownership, suggests App structure
```

### GIT Import

Parse Git repo structure for base templates, overlays, variants. GUI capability.
Git import is the default source; live import is first-class when Git is unavailable.

### Base Template

Template in the platform catalog. Created from `base/` folders in Git. Never deployed directly.

```
apps/payment-api/base/  →  Base template in catalog
apps/payment-api/overlays/prod/  →  App component (references base)
```

---

## Scan Concepts

### Configuration Pattern

Cataloged risk issue. Configuration anti-pattern that causes problems.

Format: `RISK-2025-XXXX`

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

## ConfigHub Model (App-Centric)

ConfigHub is moving to app-centric language. The invariant is unchanged:
nothing implicit ever deploys, nothing observed silently overwrites intent.

### Operating Boundary

| System | Role |
|--------|------|
| **ConfigHub** | Stores/publishes explicit intended state + provenance |
| **Flux/Argo** | Reconcile runtime (ConfigHub does not replace your deployer) |
| **cub-scout** | Reports reality and drift (read-only observer) |

### App

A logical application — the thing your team thinks of as "a service." Contains
components (api, worker, database) owned by one team.

```
App: payment-service
├── Components: api, worker, redis
└── Deployments: dev, staging, prod
```

### Deployment

An App deployed to a specific Target (App × environment). What you get when you
deploy an App to production, staging, etc.

### Target

A Kubernetes cluster managed by ConfigHub. Connected via a Bridge Worker.

### OCI Transport

ConfigHub's default transport for rendered manifests. The pipeline is:

```
Git → ConfigHub renders → OCI artifact → Flux/Argo pulls from OCI → cluster
```

cub-scout does not interact with OCI directly. It discovers workloads from the
cluster end of this pipeline.

### Bridge Worker

Connector between ConfigHub and external systems (Kubernetes, ArgoCD, Flux).
Bridge workers handle the actual deployment — ConfigHub orchestrates, Flux/Argo
apply.

### Temporary API Mapping

While APIs evolve, the new concepts map to current CLI commands:

| New Concept | Current CLI | Notes |
|-------------|-------------|-------|
| App | Space | A Space holds one App's Deployments |
| App component | Unit | A Unit is a deployable component |
| Deployment | Space + Units + Target | Junction of App × environment |
| Target | Target | Unchanged |

The `cub` CLI still uses `cub unit list`, `cub space delete`, etc. Read the
output through the App/Deployment lens.

---

## See Also

- [Migration Playbook](../howto/migration-playbook.md) — Comprehensive migration guide
- [Import to ConfigHub](../howto/import-to-confighub.md) — Import architecture
- [Import from Live](../howto/import-from-live.md) — Cluster-only import (no Git required)
- [Hub/AppSpace Examples](hub-appspace-examples.md) — Mapping pattern examples
- [Scan for risk issues](../howto/scan-for-risks.md) — risk scanning guide
