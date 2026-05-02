# ConfigHub Concepts Glossary

Quick reference for ConfigHub terminology.

The model is **Component → Variant → Target**, with **Connection** for
typed cross-Component dependencies. This glossary uses that vocabulary
throughout. The older Hub/AppSpace and App/Deployment terms are kept at
the bottom for historical reference.

---

## The Model

### Organization (Org)

Top-level container. Everything belongs to an Org.

```
Org: acme-corp
└── (Components, Variants, Users, Targets)
```

### Component

A logical piece of software — the thing your team thinks of as "a
service." Components are the family. They are the API-level VariantSet.

```
Component: payment-service
├── Base Variant: payment-service-base   (placeholders, non-deployable)
├── Deployable Variant: payment-service-prod  (bound to prod-east-cluster)
└── Deployable Variant: payment-service-staging (bound to staging-cluster)
```

A Component has one or more Variants and is identified by
`Labels.Component=<name>` on the Spaces that hold its Variants.

### Variant

A member of a Component family. A Variant is a Space with
`Labels.Variant=<value>`. Two flavours:

- **Base Variant** — non-deployable, placeholder-bearing. The canonical
  render that other Variants derive from.
- **Deployable Variant** — bound to a Target, placeholders resolved.
  This is what actually deploys.

```
Variant: payment-service-prod
├── Labels.Component: payment-service
├── Labels.Variant: prod
├── Target: prod-east-cluster
└── Units: payment-api, order-svc, redis
```

### Base Variant

Variant with no Target attached. Typically contains placeholders. Used
as the source for Deployable Variants. Promotion flows clone a Base
into a new Deployable Variant and adapt it for a specific environment.

> **Implementation note:** Today the system distinguishes Base vs
> Deployable by *presence of a Target*: no Target = Base. There has
> been discussion of making the distinction more explicit on the API.
> The model itself does not change either way.

### Deployable Variant

Variant with a Target attached. May still contain placeholders if it
was just cloned and not yet adapted — in that state, a
`vet-placeholders` trigger usually blocks apply. Once placeholders are
replaced, the apply gate clears.

### Adapting a Variant

The process of taking a freshly cloned Deployable Variant and
substituting environment-specific values for the placeholders inherited
from the Base. Adapting is currently a manual or scripted step
(`cub` from a shell, often AI-assisted). A first-class "vending machine"
flow for adapting is an open product question, not yet solved.

### AI Variant

A Deployable Variant whose delta or operation is AI-assisted. AI Variants
are downstream of onboarding and are not produced by the import flow.

### Target

A Kubernetes cluster managed by ConfigHub. Deployable Variants bind to
Targets. Targets connect through a Bridge Worker.

```
Component → Variant → Target ──▶ Bridge Worker ──▶ Cluster
```

### Connection

A typed contract describing what a Component needs and provides
(Secrets, ConfigMaps, ServiceAccounts, secret stores, image-pull
secrets, etc.).

In v1 of the import / onboard flow, Connections are produced as drafts
populated from cub-scout's discovery (GSF relations and trace results).
Each draft entry is marked `needsTyping: true` until the Connection v1
spec lands. Typed Connections are downstream of v1 onboarding.

### Unit

The Kubernetes-resource-level primitive. Each Unit is one deployable
manifest (Deployment, StatefulSet, Service, etc.). Units live inside a
Variant.

### Org / Space (API primitives)

Spaces are still the underlying API primitive that holds Units. After
the doctrine update, a Space carries `Labels.Component` and
`Labels.Variant` so that Component- and Variant-scoped queries work:

```bash
cub space list -l Component=payment-service
cub space list -l Component=payment-service,Variant=prod
```

---

## Sources of Truth

| Source | Role | Format |
|--------|------|--------|
| **Git** | WHAT you wrote | DRY (templates, overlays) |
| **ConfigHub** | HOW it should run | WET (rendered, resolved) |
| **Cluster** | NOW running | Live state |

### DRY vs WET

- **DRY** (Don't Repeat Yourself): Git stores templates, overlays, variables.
- **WET** (Write Everything Twice): ConfigHub stores rendered, resolved config.

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

Resource with no GitOps owner. Created via `kubectl apply` or direct
API call. Not tracked by Git.

---

## GitOps Concepts

### Source

Git repository (or OCI registry, post-onboarding) registered with
ConfigHub. Contains pattern metadata (app-of-apps, ApplicationSet,
mono-repo, etc.).

### Deployer

Tool that syncs source to cluster: Flux CD or Argo CD.

**One Variant = one Deployer.** Don't mix Flux and Argo controllers
for the same Variant.

### Worker / Bridge Worker

Connector between ConfigHub and external systems (Kubernetes, ArgoCD,
Flux). Bridge Workers handle the actual deployment — ConfigHub
orchestrates, the controller applies.

### OCI Transport

ConfigHub's default transport for rendered manifests:

```
ConfigHub renders → OCI artifact → Flux/Argo pulls from OCI → cluster
```

After [Pattern 1 takeover](../howto/onboard-existing.md), the
controller's source is ConfigHub OCI rather than Git directly.

cub-scout does not interact with OCI. It discovers workloads from the
cluster end of this pipeline.

---

## Import & Onboarding Concepts

### LIVE Import

Discover workloads from a running cluster. Read-only. Produces a
proposed Component / Variant / Target structure.

```bash
./cub-scout import -n payment-prod --dry-run
# Scans cluster, detects ownership, suggests Component structure
```

### GIT Import

Parse Git repo structure for base templates, overlays, variants.
GUI-driven. Git import is the default source; live import is
first-class when Git is unavailable.

### Onboard (Pattern 1 Takeover)

Discovers an existing Argo Application or Flux Kustomization, registers
it as a Component / Variant in ConfigHub, publishes OCI, and repoints
the controller's source from Git to ConfigHub OCI.

```bash
./cub-scout onboard --controller argo -n payment-prod --dry-run
```

See [Onboard Existing](../howto/onboard-existing.md). The command is
specified in [Pattern 1 Takeover Spec](../specs/pattern-1-takeover-v1.md);
final shipping name TBD.

### Render Diff

Comparison between the controller's current Git render and the
predicted ConfigHub OCI render. Used at takeover to verify
byte-equivalence (or to surface and accept a cosmetic diff).

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

## Operating Boundary

| System | Role |
|--------|------|
| **ConfigHub** | Stores/publishes explicit intended state + provenance. Source-of-record post-onboarding. |
| **Flux/Argo** | Reconcile runtime. ConfigHub does not replace your deployer. |
| **cub-scout** | Reports reality and drift. Read-only observer. Drives onboarding via `cub`, does not mutate the cluster directly. |

The invariant: nothing implicit ever deploys, nothing observed silently
overwrites intent.

---

## Historical / Replaced Terms

The terms below are kept for orientation — older docs and existing CLI
output may still use them. New work uses Component / Variant / Target.

### Hub / AppSpace (historical)

The original platform-hub-and-app-space model. Replaced by Org +
Component + Variant.

### App / Deployment (historical)

Intermediate vocabulary. App → Component, Deployment → Deployable
Variant. The CLI still has Space/Unit primitives underneath, but the
user-visible model is Component/Variant.

### App's "base template" (historical)

Replaced by Base Variant. Same concept, first-class name.

---

## See Also

- [Migration Playbook](../howto/migration-playbook.md) — Comprehensive migration guide
- [Import to ConfigHub](../howto/import-to-confighub.md) — Discovery and register
- [Onboard Existing](../howto/onboard-existing.md) — Pattern 1 takeover
- [Import from Live](../howto/import-from-live.md) — Cluster-only import
- [Vocabulary Alignment Spec](../specs/vocabulary-alignment-v1.md) — How cub-scout's output is moving to this vocabulary
- [Pattern 1 Takeover Spec](../specs/pattern-1-takeover-v1.md)
- [App Model Examples](app-model-examples.md) — Historical App/Deployment mapping pattern examples
- [Scan for risk issues](../howto/scan-for-risks.md)
