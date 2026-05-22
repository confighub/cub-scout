---
name: observe-flux
description: 'Use when the user wants to observe FLUX-managed resources specifically — the controller-specific observer skill for Flux. Natural phrasing: "is this resource managed by Flux?", "which Kustomization owns this?", "show me the HelmRelease for this deployment", "what GitRepository / OCIRepository / Bucket is the source?", "did Flux reconcile the latest revision?", "show me Flux source-controller writes". Load whenever intent involves Flux specifically — Kustomizations, HelmReleases, source-controller, kustomize-controller, helm-controller, or source artifacts (GitRepository / HelmRepository / OCIRepository / Bucket). Do NOT load for: Argo (use observe-argocd), direct Helm without Flux (use observe-helm), generic ownership (use scout-observe), authoring Flux CRDs (Flux is its own tool — this skill is about observing Flux-managed resources, not configuring Flux).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout gitops status *) Bash(cub-scout gitops status *) Bash(cub scout gitops status *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(flux get *) Bash(flux trace *) Bash(flux events *)
---

# observe-flux

The controller-specific observer skill for **Flux**. Documents the four Flux controllers (kustomize, helm, source, image) cub-scout knows about, the labels/annotations each writes, the manager-string enumeration in `managedFields`, and the source-artifact hierarchy.

Verified against `pkg/agent/ownership.go` `detectFluxOwnership` and `pkg/agent/manager_strings.go` Flux constants (`ManagerFluxKustomize`, `ManagerFluxHelm`, `ManagerFluxSource`).

## When to use

- "Is this resource managed by Flux?"
- "Which Kustomization owns deploy/api?"
- "Show me the HelmRelease producing this deployment"
- "What GitRepository / OCIRepository / Bucket is feeding this?"
- "Did Flux reconcile the latest revision?"
- "Show me what the Flux source-controller wrote"
- "Trace this resource back to the Flux source artifact"

## Do not load for

- Argo-managed resources — [`observe-argocd`](../observe-argocd/SKILL.md)
- Direct `helm install` (not via Flux helm-controller) — [`observe-helm`](../observe-helm/SKILL.md)
- Generic ownership ("who owns this?" without a specific controller) — [`scout-observe`](../scout-observe/SKILL.md)
- Authoring Kustomization / HelmRelease / source CRDs themselves — that's the `flux` CLI or YAML editing, not cub-scout

## Detection signals (in `detectFluxOwnership`)

cub-scout classifies a resource as Flux-owned when EITHER of two label families is present, in priority order:

| Signal | Source | SubType | Confidence | Resulting Name |
|--------|--------|---------|-----------|----------------|
| `kustomize.toolkit.fluxcd.io/name` label | label | `kustomization` | high | Kustomization name |
| `helm.toolkit.fluxcd.io/name` label | label | `helmrelease` | high | HelmRelease name |

The matching `*-namespace` labels (`kustomize.toolkit.fluxcd.io/namespace`, `helm.toolkit.fluxcd.io/namespace`) populate the `Namespace` field on the resulting Ownership.

## Manager strings (writers in `metadata.managedFields`)

Flux writes through Server-Side Apply with three controllers, each identified by an exact manager string:

| Controller | Manager | Reconciles |
|-----------|---------|-----------|
| kustomize-controller | `kustomize-controller` | Resources produced by a `Kustomization` |
| helm-controller | `helm-controller` | Resources produced by a `HelmRelease` |
| source-controller | `source-controller` | The source artifacts themselves (GitRepository / HelmRepository / OCIRepository / Bucket / HelmChart) |

cub-scout's `controllerManagersForOwner(OwnerFlux, ownerSubType)` returns the narrow matcher set when `ownerSubType` is known (`kustomization` → only `kustomize-controller`; `helmrelease` → only `helm-controller`; `source` / `gitrepository` / `ocirepository` / `helmrepository` / `bucket` → only `source-controller`). When subtype is absent, all three are accepted as controller co-signals.

## Source artifact hierarchy

Flux delivery is a two-stage chain:

```
GitRepository / OCIRepository / HelmRepository / Bucket   (source-controller)
              ↓ (fetched, revision pinned)
       Kustomization or HelmRelease                       (kustomize/helm-controller)
              ↓ (applied)
        Deployment / Service / ConfigMap …               (manager: kustomize-controller / helm-controller)
```

cub-scout's tracer (`pkg/agent/flux_trace.go`) walks this chain backwards from a workload:

1. Read the workload's labels → find the owning Kustomization or HelmRelease
2. Read the Kustomization/HelmRelease spec → find the source (GitRepository/Bucket/etc.) via `spec.sourceRef`
3. Read the source spec → resolve to the git URL + revision (`status.artifact.revision`)

This is what powers `cub-scout trace` and the `gitSource{repoUrl, revision, path}` evidence on `compare` JSON for Flux-owned resources.

### Multi-source-type coverage

| Source kind | Provides | Notes |
|-------------|----------|-------|
| `GitRepository` | Plain Git tree | The common case. Revision is the Git SHA. |
| `OCIRepository` | OCI artifact (e.g., a ConfigHub-rendered OCI bundle) | Revision is the OCI digest. ConfigHub-via-OCI-via-Flux strategy in source-truth. |
| `HelmRepository` | Helm chart repo | Fed to a HelmRelease. Chart version is the anchor. |
| `Bucket` | S3 / GCS / generic blob store | Less common; supported in the tracer. |
| `HelmChart` | A specific chart from a HelmRepository | Inlined into HelmRelease usually; cub-scout walks through. |

The Flux interop slice (#349 / v0.20.0) explicitly covers all five source types.

## Worked example

```bash
$ kubectl get deploy api -n prod -o yaml | yq '.metadata.labels, .metadata.managedFields[].manager'
labels:
  kustomize.toolkit.fluxcd.io/name: platform-prod
  kustomize.toolkit.fluxcd.io/namespace: flux-system
managers:
  - kustomize-controller
  - kubectl-patch                                # uh oh
```

cub-scout reads this as:

```
$ cub-scout explain deploy/api -n prod
Resource: Deployment/api in prod
  Owner:    Flux Kustomization "platform-prod" in flux-system (label:kustomize.toolkit.fluxcd.io/name)
  Drift:    Detected
  Mutation cause: manual-edit (manager: kubectl-patch; controller co-signal: kustomize-controller)
  Git source: https://github.com/org/platform-config @abc123 path=clusters/prod/apps
```

The owner co-signal (`kustomize-controller`) plus the interactive writer (`kubectl-patch`) is unambiguous: someone bypassed Flux. Route the user to the original-author path: commit the change back to git, let Flux reconcile.

## Edge cases

- **Multi-source Argo equivalent**: Flux doesn't have an analogue to Argo's `spec.sources[]` multi-source; each Kustomization/HelmRelease has exactly one `spec.sourceRef`. No multi-source proof gap to worry about.
- **`source-controller` writing to source artifacts**: When the resource being observed IS a `GitRepository` / `OCIRepository` / etc., the writer is `source-controller`. cub-scout classifies these as `OwnerFlux` with `SubType=source-controller`-family (or specific subtype if known).
- **Helm-via-Flux vs direct Helm**: A HelmRelease produces Deployments whose `metadata.managedFields` shows `helm-controller`, NOT `helm`. The bare `helm` manager string means a direct `helm install` invocation (no Flux). See [`observe-helm`](../observe-helm/SKILL.md) for the disambiguation matrix.
- **ConfigHub units delivered through Flux**: Resource has both `confighub.com/UnitSlug` and `kustomize.toolkit.fluxcd.io/name`. cub-scout classifies as `OwnerConfigHub` (ConfigHub detection priority), and accepts `kustomize-controller` / `helm-controller` as controller co-signals. See [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md).
- **kstatus migration risk**: `cub-scout watch` v2.1.0 may flip `Ready=true → false` for stalled Flux workloads — tracked in `#427`. Not a Flux-source issue, but worth knowing about when reading watch output.

## References

- Code: `pkg/agent/ownership.go` `detectFluxOwnership`, `pkg/agent/manager_strings.go` `ManagerFluxKustomize` / `ManagerFluxHelm` / `ManagerFluxSource`, `pkg/agent/flux_trace.go`
- Upstream:
  - kustomize-controller name: [`fluxcd/kustomize-controller`](https://github.com/fluxcd/kustomize-controller) `controllerName`
  - helm-controller name: [`fluxcd/helm-controller`](https://github.com/fluxcd/helm-controller) `controllerName`
  - source-controller name: [`fluxcd/source-controller`](https://github.com/fluxcd/source-controller) `controllerName`
- Related skills: [`scout-observe`](../scout-observe/SKILL.md), [`scout-attribute`](../scout-attribute/SKILL.md), [`observe-argocd`](../observe-argocd/SKILL.md), [`observe-helm`](../observe-helm/SKILL.md), [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md)
- Examples: [`examples/flux-import-confighub-demo/`](../../examples/flux-import-confighub-demo/), [`examples/flux-boutique/`](../../examples/flux-boutique/)
- Interop slice: `#349` (Flux Operator Interop v0.20.0)

## Constraints

- This skill is read-only. `flux reconcile`, `flux suspend`, `flux resume`, `flux trace --apply` are mutations and NOT in allowed-tools; route to the `flux` CLI with the user driving.
- ApplicationSet-equivalent in Flux is `Kustomization` + `OCIRepository` fan-out; cub-scout supports parsing this layout but the configuration itself lives in Flux YAML, not in cub-scout.
- For Helm-as-source-of-truth verdicts via `compare source-truth`, the `helm-flux` strategy emits a `runtime.helm_chart_anchor` proof gap until the runtime chart-version extractor is plumbed in (Phase 2, partial). See `scout-compare`.
