---
name: observe-confighub-managed
description: 'Use when the user wants to observe CONFIGHUB-managed resources specifically — resources where the desired state lives in a ConfigHub Unit, delivered through Argo or Flux. Natural phrasing: "is this a ConfigHub unit?", "which ConfigHub Unit / Space produced this?", "show me ConfigHub-delivered-via-Argo resources", "what does `confighub.com/UnitSlug` mean?", "is this delivered via Argo OCI or Flux Kustomize?", "trace this back to the ConfigHub revision". Load when the user is specifically asking about ConfigHub-as-source-of-truth. Do NOT load for: vanilla Argo without ConfigHub (use observe-argocd), vanilla Flux (use observe-flux), generic ownership (use scout-observe), authoring ConfigHub units (use cub skills, not cub-scout).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout history *) Bash(cub-scout history *) Bash(cub scout history *) Bash(./cub-scout impact *) Bash(cub-scout impact *) Bash(cub scout impact *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(cub * get) Bash(cub * list) Bash(cub unit get *) Bash(cub link list *) Bash(cub auth status)
---

# observe-confighub-managed

The controller-specific observer skill for **ConfigHub-managed** resources — workloads whose desired state lives in a ConfigHub Unit and gets delivered through Argo (typically via OCI artifact) or Flux (Kustomization / OCIRepository). This skill is the bridge between cub-scout's "who owns the live resource?" question and ConfigHub's "where does the intent live?" answer.

Verified against `pkg/agent/ownership.go` `detectConfigHubOwnership` and `pkg/agent/manager_strings.go` `controllerManagersForOwner(OwnerConfigHub, ...)`.

## When to use

- "Is this a ConfigHub Unit?"
- "Which ConfigHub Unit / Space produced this?"
- "Show me ConfigHub-delivered-via-Argo resources"
- "What does the `confighub.com/UnitSlug` label mean?"
- "Is this resource delivered via ConfigHub-OCI-Argo or ConfigHub-OCI-Flux?"
- "Trace this back to the ConfigHub revision"
- "Why does cub-scout classify this as OwnerConfigHub when Argo labels are also present?"

## Do not load for

- Vanilla Argo without ConfigHub — [`observe-argocd`](../observe-argocd/SKILL.md)
- Vanilla Flux — [`observe-flux`](../observe-flux/SKILL.md)
- Generic ownership ("who owns this?") — [`scout-observe`](../scout-observe/SKILL.md)
- Authoring ConfigHub Units / Spaces / Targets — that's `cub`, not cub-scout. Route to [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills).
- Comparing ConfigHub intent to live state — [`scout-compare`](../scout-compare/SKILL.md) (`compare three-way`, `compare source-truth`)

## Detection signals (in `detectConfigHubOwnership`)

cub-scout classifies a resource as ConfigHub-owned when EITHER signal is present:

| Signal | Source | SubType | Confidence | Notes |
|--------|--------|---------|-----------|-------|
| `confighub.com/UnitSlug` label | label | `unit` | high | Primary signal. `Namespace` is populated from `confighub.com/SpaceName` (annotation preferred, label fallback). |
| `confighub.com/UnitSlug` annotation | annotation | `unit` | high | Fallback for resources where the unit slug lives in annotations. `Namespace` from `confighub.com/SpaceName` annotation. |

Resulting `Ownership`: `Type=confighub`, `SubType=unit`, `Name=<unit-slug>`, `Namespace=<space-name>`.

## Why ConfigHub priority is BEFORE Argo / Flux

The ownership detector runs **ConfigHub detection BEFORE Argo and Flux**. A resource delivered through `Argo CD reading a ConfigHub-rendered OCI bundle` has BOTH `confighub.com/UnitSlug` AND `argocd.argoproj.io/instance` labels. Without the priority order, the resource would classify as `OwnerArgo` and the "delivered by ConfigHub" fact would be lost.

The detector ordering reflects the architectural triad's authority model:
- ConfigHub is **authority** (where the intent lives — runs first)
- Argo / Flux are **delivery** (how the intent reaches the cluster — run second)
- The live resource is **runtime** (what actually got applied)

A ConfigHub-managed resource still gets full Argo/Flux *attribution* even though it classifies as `OwnerConfigHub`. The attribution layer's controller co-signal accepts both `argocd-controller` and `kustomize-controller` / `helm-controller` as valid writers for ConfigHub-owned resources — see "Controller co-signals" below.

## Manager strings (writers in `metadata.managedFields`)

`controllerManagersForOwner(OwnerConfigHub, ...)` returns FOUR matchers — the union of Argo and Flux core writers, because both delivery surfaces are valid for ConfigHub:

| Manager | Source | Delivery path |
|---------|--------|--------------|
| `argocd-controller` | Argo CD application-controller SSA | ConfigHub → OCI → Argo CD → cluster |
| `kubectl-client-side-apply` | Argo CD CSA migration default | Same path; Argo writes via CSA fallback |
| `kustomize-controller` | Flux kustomize-controller | ConfigHub → OCI/Git → Flux Kustomization → cluster |
| `helm-controller` | Flux helm-controller | ConfigHub → OCI → Flux HelmRelease → cluster |

Any of these as a writer is `controller-drift` (a controller reconciled the ConfigHub intent). A `kubectl-*` writer on a ConfigHub-owned resource is `manual-edit` — someone bypassed both ConfigHub AND the delivery controller.

## Delivery path disambiguation

cub-scout doesn't classify the *delivery* path in ownership detection — it just records the unit slug. To find out *how* a ConfigHub Unit got to the cluster, walk the labels:

| If you also see... | The delivery path is... |
|-------------------|------------------------|
| `argocd.argoproj.io/instance` | ConfigHub-via-Argo (typically OCI) |
| `kustomize.toolkit.fluxcd.io/name` | ConfigHub-via-Flux Kustomization |
| `helm.toolkit.fluxcd.io/name` | ConfigHub-via-Flux HelmRelease |
| Neither | Either a direct `cub` apply or an unusual delivery path; check `cub unit get <slug>` for the target |

The `compare source-truth --strategy <name>` verbs cover the four ConfigHub-aware strategies explicitly:
- `confighub-oci-argo` — ConfigHub authors a Unit, renders to OCI, Argo CD reads OCI, applies to cluster
- `confighub-oci-flux` — Same authoring chain, Flux is the deliverer
- (helm / git strategies aren't ConfigHub-authoritative; they describe vanilla Argo / Flux)

## Connected-mode evidence enrichment

When cub-scout is run **connected** (`cub auth login`), additional ConfigHub-side evidence is layered onto the resource:

- `confighubUrl` — the ConfigHub GUI deep-link to the unit's revision
- `incomingBindings[]` — the upstream units feeding this unit via ConfigHub Links (#438)
- per-field `bindingSource` — which Link supplies a specific field's value (#439)
- `compare three-way` shows the full DRY (ConfigHub) / WET (rendered) / LIVE (cluster) trio
- `compare source-truth --strategy confighub-oci-{argo,flux}` runs the strategy-typed verdict over all three surfaces

Standalone-mode cub-scout still classifies as `OwnerConfigHub` (the label is on the live resource), but the connected-mode evidence isn't available. Receipts record this as `OmissionConfigHubUnitSubject`.

## Worked example

```bash
$ kubectl get deploy api -n prod -o yaml | yq '.metadata.labels, .metadata.annotations, .metadata.managedFields[].manager'
labels:
  confighub.com/UnitSlug: payments-api
  argocd.argoproj.io/instance: payments-app
  app: api
annotations:
  confighub.com/SpaceName: payments
  argocd.argoproj.io/tracking-id: "payments-app:apps/Deployment:prod/api"
managers:
  - argocd-controller
  - kubectl-edit                                          # uh oh
```

cub-scout reads this as:

```
$ cub-scout explain deploy/api -n prod
Resource: Deployment/api in prod
  Owner:    ConfigHub unit "payments-api" in space "payments" (label:confighub.com/UnitSlug)
  Delivered via: Argo CD application "payments-app" (label:argocd.argoproj.io/instance)
  Drift:    Detected
  Mutation cause: manual-edit (manager: kubectl-edit; controller co-signal: argocd-controller)
  ConfigHub URL: https://hub.confighub.com/p/payments/u/payments-api    (connected mode)
```

The classifier correctly identifies BOTH the source of truth (ConfigHub Unit) and the delivery path (Argo). The mutation cause is `manual-edit` despite the controller co-signal — that's the attribution layer's job.

## Edge cases

- **`confighub.com/UnitSlug` annotation without the label**: Some resources only get the annotation. cub-scout's second branch in `detectConfigHubOwnership` catches this. `SpaceName` is still read from annotations (not labels) for consistency.
- **ConfigHub-managed without a delivery controller**: A `cub apply` directly to the cluster (no Argo / Flux in the middle) leaves only `confighub.com/UnitSlug` on the resource. Ownership classifies as `OwnerConfigHub`; the controller writer is... whatever wrote it (often a `kubectl-*` form), which means `cause=manual-edit` even though the user intended a controlled apply. This is a known artifact of the `cub apply` path; #410/#428 triad work is the long-term fix.
- **ConfigHub via Helm (not Flux helm-controller)**: When ConfigHub renders to a Helm chart that's then handed to Argo's Helm renderer, the resource has `confighub.com/UnitSlug` + `argocd.argoproj.io/instance` + `app.kubernetes.io/managed-by: Helm`. Detection priority: ConfigHub wins; controller co-signal recognizes `argocd-controller`. The Helm labels are an artifact of the chart, not evidence of a Helm-specific delivery.
- **`OwnerConfigHub` with `cause=unknown`**: Older clusters that stripped `managedFields` produce this — the unit slug is there but cub-scout can't tell *who* wrote the live state. The attribution layer emits `cause=unknown` with an `OmissionManagedFields` entry rather than guessing.

## References

- Code: `pkg/agent/ownership.go` `detectConfigHubOwnership`, `pkg/agent/manager_strings.go` `controllerManagersForOwner(OwnerConfigHub, ...)`
- Upstream: ConfigHub label spec — `confighub.com/UnitSlug`, `confighub.com/SpaceName`, `confighub.com/SpaceID`, `confighub.com/RevisionNum`
- Related skills:
  - [`scout-observe`](../scout-observe/SKILL.md) — the umbrella ownership detector
  - [`scout-attribute`](../scout-attribute/SKILL.md) — per-field attribution; recognizes ConfigHub-via-Argo and ConfigHub-via-Flux controller co-signals
  - [`scout-compare`](../scout-compare/SKILL.md) — `compare three-way` and `compare source-truth` use ConfigHub as the DRY layer
  - [`scout-govern`](../scout-govern/SKILL.md) — `history`, `impact`, `views resolve` are ConfigHub-aware
  - [`scout-verify`](../scout-verify/SKILL.md) — receipts add a `confighub-unit://` subject in connected mode
  - [`observe-argocd`](../observe-argocd/SKILL.md), [`observe-flux`](../observe-flux/SKILL.md) — the delivery paths
- Examples: [`examples/argo-import-confighub-demo/`](../../examples/argo-import-confighub-demo/), [`examples/flux-import-confighub-demo/`](../../examples/flux-import-confighub-demo/), [`examples/connect-and-compare/`](../../examples/connect-and-compare/)

## Constraints

- This skill is read-only. ConfigHub Unit mutation, space creation, target binding, etc. are all `cub` operations and out of allowed-tools.
- ConfigHub detection priority is INTENTIONAL — a ConfigHub-managed resource classifies as `OwnerConfigHub` even when Argo / Flux labels are also present. Don't reverse the order.
- Connected-mode evidence enrichment requires `cub auth login`. Standalone-mode users get correct ownership classification but no `confighubUrl`, no `incomingBindings`, no DRY/WET/LIVE compare. Surface this as an omission, not silent missing data.
