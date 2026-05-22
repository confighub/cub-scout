---
name: observe-native
description: 'Use when the user wants to observe NATIVE Kubernetes resources — anything cub-scout''s ownership detector couldn''t classify as Argo / Flux / Helm / Crossplane / kro / ConfigHub / Terraform / Custom. Natural phrasing: "why is this resource unowned?", "show me orphan workloads", "this isn''t managed by anything — what does that mean?", "what is OwnerReferences-only ownership?", "is this a kubectl-applied resource without a controller?", "show me native K8s resources in this cluster". Load whenever intent is unowned / native / orphan / no-controller / OwnerReferences-only / direct-apply. Do NOT load for: any specific controller (use the matching observe-* skill), generic ownership when the controller IS known (use scout-observe), or apply/edit/delete operations (cub-scout never mutates).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout map orphans *) Bash(cub-scout map orphans *) Bash(cub scout map orphans *) Bash(./cub-scout scan *) Bash(cub-scout scan *) Bash(cub scout scan *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *)
---

# observe-native

The controller-specific observer skill for **native Kubernetes resources** — anything cub-scout's ownership detector couldn't classify under one of the controller-specific owners. Two distinct populations live here: K8s-native chains (a ReplicaSet under a Deployment, etc.) detected via OwnerReferences, and genuinely-unowned resources (a `kubectl apply -f` of a Deployment with no GitOps controller behind it).

Verified against `pkg/agent/ownership.go` `detectK8sOwnership` and the terminal `Ownership{Type: OwnerUnknown}` return.

## When to use

- "Why is this resource classified as unowned / native?"
- "Show me orphan workloads in this cluster"
- "What does `OwnerReferences-only ownership` mean?"
- "This Deployment isn't managed by anything — what's the implication?"
- "Is this a `kubectl apply -f` resource without any GitOps controller?"
- "Show me native K8s resources in this cluster"
- "Distinguish between OwnerType=k8s and OwnerType=unknown"

## Do not load for

- A specific controller you know is at play — load the matching `observe-*` skill
- Generic ownership detection ("who owns this?") — [`scout-observe`](../scout-observe/SKILL.md) walks all detectors
- Mutating operations (apply / edit / delete) — out of cub-scout scope

## Two distinct populations

cub-scout's ownership detector ends with two terminal classifications:

### 1. `OwnerType = k8s` — native K8s chain

When `detectK8sOwnership` finds a non-empty `metadata.ownerReferences` list, the resource is classified as `OwnerType=k8s` (`OwnerKubernetes` constant). The owner-reference parent's `Kind` and `Name` populate the `Ownership` fields. This is the **normal** state for child resources in a K8s-native chain:

- A `ReplicaSet` owned by a `Deployment`
- A `Pod` owned by a `ReplicaSet` (or `DaemonSet`, `StatefulSet`, `Job`)
- A `Job` owned by a `CronJob`

The owner is genuinely "Kubernetes" in the sense that the K8s built-in controllers (deployment-controller, replicaset-controller, etc.) are doing the reconciliation. cub-scout has no labels to dig into — the owner-reference IS the evidence.

### 2. `OwnerType = unknown` — no owner detected

When every detector returned empty AND there are no ownerReferences, the resource lands in `OwnerType=unknown` with no other fields populated. This is the "I genuinely don't know who owns this" classification — typically:

- A `kubectl apply -f deployment.yaml` to an empty namespace, no GitOps controller
- A `kubectl create` of an ad-hoc resource
- A leaked / orphaned resource whose original controller is gone (the ApplicationSet was deleted, the HelmRelease was uninstalled mid-flight, etc.)
- A resource whose labels were stripped by an admission webhook

cub-scout does NOT guess at this point. `parse, don't guess` is the v0.5 invariant — `unknown` is an honest answer, not a failure.

## Detection signals (in `detectK8sOwnership` + terminal fallthrough)

| Trigger | Resulting `Type` | Notes |
|---------|------------------|-------|
| `metadata.ownerReferences[]` non-empty | `OwnerKubernetes` (`"k8s"`) | The owner-reference parent's `Kind` and `Name` populate `Ownership.SubType` / `Ownership.Name`. |
| Every other detector returned empty AND no ownerReferences | `OwnerUnknown` (`"unknown"`) | The terminal fallthrough. |

`detectK8sOwnership` is the LAST controller-specific detector in `DetectOwnership`'s priority chain; the `unknown` fallthrough is the very last step.

## What attribution looks like for native resources

`controllerManagersForOwner(OwnerKubernetes, ...)` returns `nil` — there is no controller manager string that signals "this is K8s native." The classifier therefore cannot return `controller-drift` for these resources. The two possible attribution outcomes:

- **Interactive writer present** (`kubectl-edit`, `kubectl-patch`, `kubectl-apply`, etc.): `cause=manual-edit`
- **No interactive writer** (or `managedFields` empty / stripped): `cause=unknown`

This is intentional. A `kubectl apply -f` resource is genuinely written by the operator; calling that `controller-drift` would mask the human in the loop. And without any controller's signature in `managedFields`, cub-scout cannot honestly claim "the controller is reconciling" — there's no controller.

## Worked example

### Native K8s chain — `ReplicaSet` owned by `Deployment`

```bash
$ kubectl get rs api-7b8d9c5f -n prod -o yaml | yq '.metadata.ownerReferences, .metadata.managedFields[].manager'
ownerReferences:
  - apiVersion: apps/v1
    kind: Deployment
    name: api
    uid: ...
managers:
  - kube-controller-manager
```

cub-scout reads this as:

```
$ cub-scout explain rs/api-7b8d9c5f -n prod
Resource: ReplicaSet/api-7b8d9c5f in prod
  Owner:    Kubernetes Deployment "api" (ownerRef)
  Drift:    None expected (K8s native control)
```

### Genuinely unowned — direct `kubectl apply`

```bash
$ kubectl get deploy adhoc -n default -o yaml | yq '.metadata.labels, .metadata.annotations, .metadata.ownerReferences, .metadata.managedFields[].manager'
labels: {}
annotations: {}
ownerReferences: []
managers:
  - kubectl-client-side-apply
```

cub-scout reads this as:

```
$ cub-scout explain deploy/adhoc -n default
Resource: Deployment/adhoc in default
  Owner:    Unknown (no GitOps labels, no ownerReferences, no managed-by signal)
  Drift:    N/A (no controller; cannot reconcile)
  Last writer: kubectl (manager: kubectl-client-side-apply)
```

## Orphan detection (`map orphans`)

`cub-scout map orphans` surfaces resources that had a controller and lost it — the GitOps-equivalent of `OwnerType=unknown` for resources that *used* to be classifiable. The verb walks for stale labels:

- `argocd.argoproj.io/instance` but no matching Application in the cluster
- `kustomize.toolkit.fluxcd.io/name` but no matching Kustomization
- `crossplane.io/composite` but no matching XR

These are different from `OwnerType=unknown` (no signal at all) — they're `OwnerType=<controller>` with a broken upstream. The orphan verb's output is the field guide for cleanup; see [`scout-observe`](../scout-observe/SKILL.md) for the map-orphans flow.

## Why this matters

The native population is the **floor** of the ownership distribution. A healthy GitOps cluster has very few `OwnerType=k8s` workloads (they're typically just child ReplicaSets and Pods, which is fine) and even fewer `OwnerType=unknown` workloads (which are usually a smell — adhoc applies that nobody reconciles).

When `cub-scout doctor` reports a high `unknown` count, that's a signal:

- The cluster has accumulated adhoc resources nobody owns
- Or an admission webhook is stripping labels
- Or a controller migration left labels stale

Either way, the user needs to investigate, not paper over it with a different classification.

## Edge cases

- **`OwnerType=k8s` with weird OwnerKind**: A custom CRD's ownerReference can land here (if no other detector matched it first). E.g., `kind=MyCustomCRD apiVersion=mycompany.com/v1`. cub-scout records `SubType=mycustomcrd` and surfaces it; the user has to know what `MyCustomCRD` means.
- **Stripped `managedFields`**: Some admission webhooks strip `metadata.managedFields` after applying their own changes. A native resource with stripped managedFields can't be attributed — cause is `unknown`, with `OmissionManagedFields` recorded.
- **`OwnerType=unknown` with `kubectl-client-side-apply`**: Same string Argo's CSA-migration default uses, but without an Argo signal. cub-scout treats this as `manual-edit` — a human ran `kubectl apply`. See the disambiguation rule in [`observe-argocd`](../observe-argocd/SKILL.md).
- **Resources with ONLY `app.kubernetes.io/managed-by: Helm` and no `instance` label**: Detected as `OwnerHelm` `release=""` (empty name). NOT native. The Helm label is enough to trigger the Helm detector before fallthrough.
- **Custom detectors** (`detectCustomOwnership`): If the user has configured a custom ownership detector (see `examples/custom-ownership-detectors/`), it runs AFTER all built-ins but BEFORE `detectK8sOwnership` and the unknown fallthrough. A custom detector can claim resources that would otherwise land here.

## References

- Code: `pkg/agent/ownership.go` `detectK8sOwnership` + the `Ownership{Type: OwnerUnknown}` terminal, `pkg/agent/manager_strings.go` `interactiveManagers` enumeration
- Related skills:
  - [`scout-observe`](../scout-observe/SKILL.md) — `map orphans` and the umbrella ownership detector
  - [`scout-attribute`](../scout-attribute/SKILL.md) — `cause=unknown` semantics
  - [`observe-argocd`](../observe-argocd/SKILL.md), [`observe-flux`](../observe-flux/SKILL.md), [`observe-helm`](../observe-helm/SKILL.md), [`observe-crossplane`](../observe-crossplane/SKILL.md), [`observe-kro`](../observe-kro/SKILL.md), [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md) — the seven controllers that classify ABOVE native
- Custom detectors: [`examples/custom-ownership-detectors/`](../../examples/custom-ownership-detectors/)

## Constraints

- This skill is read-only. Adopting an orphaned resource into a controller (e.g., labelling it with `argocd.argoproj.io/instance`) is a mutation and out of allowed-tools — user-driven.
- `OwnerType=unknown` is **not** a failure mode. It's an honest answer. Don't pressure-classify it into a known owner without evidence.
- For genuinely-unowned resources, the operator response is policy: adopt into GitOps, delete, or accept-as-canonical. cub-scout produces the evidence; the decision is the operator's.
