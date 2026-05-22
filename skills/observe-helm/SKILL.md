---
name: observe-helm
description: 'Use when the user wants to observe HELM-managed resources specifically — the controller-specific observer skill for Helm. The key complication: there are three ways a resource can "be Helm" — direct `helm install/upgrade`, Flux helm-controller delivering a HelmRelease, and Argo CD rendering Helm charts as an Application source. This skill teaches the disambiguation matrix. Natural phrasing: "is this a Helm release?", "which Helm chart produced this deployment?", "is this Helm-via-Flux or direct Helm or Argo-as-Helm-renderer?", "what does `app.kubernetes.io/managed-by: Helm` mean here?", "trace this Helm release back to source". Do NOT load for: Flux-specifically (use observe-flux for the Kustomization side), Argo-specifically (use observe-argocd), generic ownership (use scout-observe), authoring Helm charts.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(helm list *) Bash(helm get *) Bash(helm status *) Bash(helm history *) Bash(flux get hr *) Bash(flux get hr *) Bash(argocd app get *)
---

# observe-helm

The controller-specific observer skill for **Helm**. The complication: "Helm-managed" can mean three different things depending on who wrote `app.kubernetes.io/managed-by: Helm`. This skill teaches the disambiguation matrix and the manager-string co-signal that resolves it.

Verified against `pkg/agent/ownership.go` `detectHelmOwnership` and `pkg/agent/manager_strings.go` `ManagerHelm`.

## When to use

- "Is this a Helm release?"
- "Which Helm chart produced deploy/api?"
- "Is this Helm-via-Flux, direct Helm, or Argo-as-Helm-renderer?"
- "What does the `app.kubernetes.io/managed-by: Helm` label actually mean here?"
- "Trace this Helm release back to its chart + source"

## Do not load for

- Flux specifically (Kustomization side, source-controller, the rest of Flux) — [`observe-flux`](../observe-flux/SKILL.md)
- Argo CD Applications (even when the source is a Helm chart, Argo writes are Argo writes) — [`observe-argocd`](../observe-argocd/SKILL.md)
- Generic ownership detection — [`scout-observe`](../scout-observe/SKILL.md)
- Authoring Helm charts / writing values.yaml — out of cub-scout scope

## Detection signals (in `detectHelmOwnership`)

cub-scout classifies a resource as Helm-owned when either of two label families is present:

| Signal | Source | SubType | Confidence | Resulting Name |
|--------|--------|---------|-----------|----------------|
| `app.kubernetes.io/managed-by: Helm` label | label | `release` | high | Value of `app.kubernetes.io/instance` |
| `helm.sh/chart` label | label | `release` | high | Value of `app.kubernetes.io/instance` (falls back to chart name) |

Both are written by Helm v3's default release labels. Pre-v3 Helm charts wrote `helm.sh/chart` only; v3+ writes both.

## The three-ways-to-be-Helm disambiguation matrix

This is the unique value of this skill. Same labels appear in all three cases; the **manager string** in `metadata.managedFields` distinguishes them.

| Scenario | `managed-by` label | Manager string in `managedFields` | cub-scout classification |
|----------|--------------------|-----------------------------------|--------------------------|
| Direct `helm install/upgrade` (a human or CI ran the CLI) | `Helm` | `helm` (bare; the default `kube.ManagedFieldsManager` from `helm/helm`) | `OwnerHelm` + controller manager is `helm` |
| Flux helm-controller (HelmRelease CRD) | `Helm` (Helm v3 sets it even when called via the library) | `helm-controller` | `OwnerFlux` `SubType=helmrelease` (Flux labels take detection priority over Helm); controller co-signal is `helm-controller` |
| Argo CD Application sourcing a Helm chart | `Helm` (Argo renders Helm and applies the result) | `argocd-controller` or `kubectl-client-side-apply` | `OwnerArgo` `SubType=application` (Argo labels take detection priority); controller co-signal is `argocd-controller` |

cub-scout's ownership detector runs in priority order — **Flux first, then Argo, then Helm**. So a HelmRelease-managed resource is `OwnerFlux`, not `OwnerHelm`. A pure direct-Helm resource is `OwnerHelm` because nothing else claimed it first.

The `ManagerHelm = "helm"` constant in `manager_strings.go` denotes the **bare** Helm CLI invocation, which is what you see in scenario 1 only. Third-party embedders (Flux helm-controller, terraform-provider-helm) override the field-manager identity with their own name.

## Manager strings

| Manager | Source | Meaning |
|---------|--------|---------|
| `helm` | upstream `helm/helm` `kube.ManagedFieldsManager` default (`filepath.Base(os.Args[0])`) | Direct `helm install` / `helm upgrade` invocation. |

That's it — the bare `helm` string is the only one cub-scout treats as a "Helm controller" writer. Anything else writing Helm-labelled resources is some OTHER controller using the Helm library, and its identity is in *its* manager string (`helm-controller`, `argocd-controller`, etc.).

`controllerManagersForOwner(OwnerHelm, ...)` returns just `{ManagerHelm}` — the narrow match.

## Worked example

```bash
# Direct helm install
$ kubectl get deploy api -n prod -o yaml | yq '.metadata.labels, .metadata.managedFields[].manager'
labels:
  app.kubernetes.io/managed-by: Helm
  app.kubernetes.io/instance: payments-api
  helm.sh/chart: payments-api-1.2.3
managers:
  - helm
```

cub-scout reads this as `OwnerHelm` `release=payments-api`. A subsequent `kubectl edit` adds a `kubectl-edit` manager → drift cause is `manual-edit`.

```bash
# Flux helm-controller
$ kubectl get deploy api -n prod -o yaml | yq '.metadata.labels, .metadata.managedFields[].manager'
labels:
  app.kubernetes.io/managed-by: Helm
  app.kubernetes.io/instance: payments-api
  helm.sh/chart: payments-api-1.2.3
  helm.toolkit.fluxcd.io/name: payments-api          # ← Flux label wins
  helm.toolkit.fluxcd.io/namespace: flux-system
managers:
  - helm-controller
```

cub-scout reads this as `OwnerFlux` `helmrelease=payments-api in flux-system`. The Helm labels are still there but the Flux label takes priority. The controller co-signal is `helm-controller`, not bare `helm`.

```bash
# Argo CD as Helm renderer
$ kubectl get deploy api -n prod -o yaml | yq '.metadata.labels, .metadata.managedFields[].manager'
labels:
  app.kubernetes.io/managed-by: Helm
  app.kubernetes.io/instance: payments-api          # Helm chart's instance label
  argocd.argoproj.io/instance: payments-app          # ← Argo label
  helm.sh/chart: payments-api-1.2.3
managers:
  - argocd-controller
```

cub-scout reads this as `OwnerArgo` `application=payments-app`. Argo rendered the Helm chart and applied the result; the `managed-by: Helm` label is an artifact of the chart, not evidence of who's reconciling.

## Edge cases

- **No instance label**: `app.kubernetes.io/managed-by: Helm` without `app.kubernetes.io/instance` produces `OwnerHelm` with an empty `Name`. cub-scout falls back to the `helm.sh/chart` label value when available.
- **Legacy charts**: Pre-v3 Helm wrote only `helm.sh/chart`. cub-scout's second detector branch handles this — produces `OwnerHelm` with `Name` from `app.kubernetes.io/instance` or, if that's also missing, from the chart name itself.
- **Argo as Helm renderer for ConfigHub**: When ConfigHub renders a unit through Argo with a Helm source, the resource has `confighub.com/UnitSlug` + Argo labels + Helm labels. ConfigHub detection runs first → `OwnerConfigHub`. See [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md).
- **`terraform-provider-helm`**: Writes its own manager identity (`terraform-provider-helm`), so resources installed through it look like Helm-labelled with a TF-attributable manager. cub-scout's verified manager enumeration does NOT include this string — falls through to `cause=unknown`, parse-don't-guess.
- **`source-truth-pass` predicate caveat**: The `helm-flux` and `helm-argo` strategies emit a `runtime.helm_chart_anchor` proof gap because the runtime chart-version extractor isn't fully wired (Phase 2 of #409). The strategy still produces a verdict; the gap is recorded in `omissions[]`.

## References

- Code: `pkg/agent/ownership.go` `detectHelmOwnership`, `pkg/agent/manager_strings.go` `ManagerHelm`
- Upstream:
  - Helm field-manager default: [`helm/helm` `kube.ManagedFieldsManager`](https://github.com/helm/helm) — `filepath.Base(os.Args[0])`
  - Helm v3 labels: [Helm chart best practices — Labels and Annotations](https://helm.sh/docs/chart_best_practices/labels/)
- Related skills: [`observe-flux`](../observe-flux/SKILL.md), [`observe-argocd`](../observe-argocd/SKILL.md), [`scout-attribute`](../scout-attribute/SKILL.md), [`scout-compare`](../scout-compare/SKILL.md) (the `helm-*` strategies)

## Constraints

- This skill is read-only. `helm install`, `helm upgrade`, `helm rollback`, `helm uninstall` are mutations and NOT in allowed-tools; route to the user driving the `helm` CLI.
- The disambiguation matrix is **the** value here. Don't conflate Helm-via-Flux with direct Helm — the operator response is completely different (revert via Flux reconcile vs. helm rollback).
- `app.kubernetes.io/managed-by: Helm` is necessary but not sufficient to call something "Helm-managed" — always check the manager string and the Flux/Argo label co-signals.
