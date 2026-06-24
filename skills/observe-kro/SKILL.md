---
name: observe-kro
description: 'Use when the user wants to observe KRO-managed resources specifically — the controller-specific observer skill for kro (the Kubernetes Resource Orchestrator). Natural phrasing: "is this a kro instance?", "which ResourceGraphDefinition produced this?", "show me kro applyset writes", "what does `kro.run/applyset` mean?", "trace this resource back to the kro instance". kro is a Kubernetes-native resource orchestrator using applysets. Do NOT load for: generic ownership (use scout-observe), Crossplane (similar composition pattern but different — use observe-crossplane), authoring kro CRDs themselves.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout tree *) Bash(cub-scout tree *) Bash(cub scout tree *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *)
---

# observe-kro

The controller-specific observer skill for **kro** (Kubernetes Resource Orchestrator) — the project at `kro-run/kro`. cub-scout classifies kro-managed resources via labels, annotations, and owner-reference chains, and recognizes three distinct kro writer identities in `metadata.managedFields`.

Verified against `pkg/agent/ownership.go` `detectKroOwnership` and `pkg/agent/manager_strings.go` kro constants.

## When to use

- "Is this a kro instance?"
- "Which ResourceGraphDefinition produced deploy/api?"
- "Show me kro applyset writes"
- "What does the `kro.run/applyset` manager string mean?"
- "Trace this resource back to the kro instance"
- "Distinguish a kro Instance from a kro ResourceGraphDefinition"

## Do not load for

- Generic ownership detection — [`scout-observe`](../scout-observe/SKILL.md)
- Crossplane (similar composition concept, completely different writers) — [`observe-crossplane`](../observe-crossplane/SKILL.md)
- Argo / Flux / Helm — their specific skills
- Authoring kro CRDs (writing ResourceGraphDefinitions) — out of cub-scout scope

## Detection signals (in `detectKroOwnership`)

cub-scout walks three signal sources in order; the first hit wins:

| Pass | Signal | SubType | Confidence | Notes |
|------|--------|---------|-----------|-------|
| 1 | Any label key starting with `kro.run/` with a non-empty value | `instance` | high | Most common path — kro writes its instance identity into labels. The first matching label's value populates `Name`. |
| 2 | Any annotation key starting with `kro.run/` with a non-empty value | `instance` | high | Fallback for resources where the identity is in annotations rather than labels. |
| 3 | `ownerReferences[].apiVersion` matches a kro API group | `instance` or `definition` | high | The owner-reference fallback for kro-generated children. `definition` when the owner kind is `ResourceGraphDefinition`. |
| 4 | Resource's own `apiVersion` matches a kro API group | `instance` or `definition` | high | Catches the ResourceGraphDefinition / Instance CRDs themselves when they have no labels or owner-refs to walk. |

The kro API group recognition is in `isKroAPIVersion` (in `ownership.go`) — recognizes the `kro.run` group family.

## Manager strings (writers in `metadata.managedFields`)

Three distinct writers, all qualified with `kro.run/`:

| Manager | Source | Reconciles |
|---------|--------|-----------|
| `kro.run/applyset` | `kro-run/kro applyset.FieldManager` | Applyset-managed child resources — the bulk of kro writes |
| `kro.run/applyset-parent` | `kro-run/kro applyset.FieldManager + "-parent"` | Applyset metadata on the parent Instance |
| `kro.run/labeller` | `kro-run/kro FieldManagerForLabeler` | Finalizer/label SSA writes on Instances |

`controllerManagersForOwner(OwnerKro, ...)` returns all three matchers as exact strings.

## Applyset semantics

kro uses Kubernetes' [applyset](https://kubernetes.io/docs/reference/labels-annotations-taints/#applyset-kubernetes-io-id) feature to track which resources belong to a logical group. The `kro.run/applyset` manager identifies writes to the *managed children* of an applyset; `kro.run/applyset-parent` identifies writes to the *applyset parent metadata* on the Instance.

Operationally:
- kro reads a `ResourceGraphDefinition` (the recipe — kinds, dependencies, transforms)
- kro instantiates it as an `Instance` (or a CRD it generates from the RGD)
- kro applyset-controller produces and reconciles the child resources, writing `kro.run/applyset` to their `managedFields`
- kro labeller writes finalizers + applyset labels on the Instance via `kro.run/labeller`

## Worked example

```bash
$ kubectl get deploy api -n prod -o yaml | yq '.metadata.labels, .metadata.ownerReferences, .metadata.managedFields[].manager'
labels:
  kro.run/instance: payments-platform
  kro.run/applyset-id: applyset-7m4k9
ownerReferences:
  - apiVersion: kro.run/v1alpha1
    kind: PaymentsPlatform                                # generated by an RGD
    name: payments-platform
managers:
  - kro.run/applyset
```

cub-scout reads this as:

```
$ cub-scout explain deploy/api -n prod
Resource: Deployment/api in prod
  Owner:    kro instance "payments-platform" (label:kro.run/instance)
  Drift:    None detected
  Mutation cause: controller-drift (manager: kro.run/applyset)
```

### When the labels are missing

If the labels were stripped (older kro, custom admission webhook, etc.), cub-scout falls back to ownerReferences:

```bash
$ kubectl get deploy api -n prod -o yaml | yq '.metadata.ownerReferences, .metadata.managedFields[].manager'
ownerReferences:
  - apiVersion: kro.run/v1alpha1
    kind: PaymentsPlatform
    name: payments-platform
managers:
  - kro.run/applyset
```

Same classification: `OwnerKro` `instance=payments-platform`. Pass 3 (ownerReferences) catches it.

## Edge cases

- **`ResourceGraphDefinition` itself**: When the *RGD* is the resource being observed (not an instance), Pass 4 catches it via `apiVersion` matching the kro group, with `SubType=definition`. The RGD is `OwnerKro` even though it has no kro labels of its own.
- **Multiple kro labels**: A resource can have several `kro.run/*` labels (instance ID, applyset ID, etc.). Pass 1 picks the FIRST matching label by Go map iteration order, which is non-deterministic. The `Name` populated may differ across cluster reads, but the `Type=kro` classification is stable. This is a known limitation; see #427-adjacent discussion.
- **Empty-value labels**: A `kro.run/foo: ""` label is skipped (Pass 1 explicitly requires `value != ""`); the next signal source is tried. Prevents cub-scout from labelling resources with stray empty kro annotations.
- **Cross-controller composition**: kro + Crossplane / kro + Flux is possible. Ownership detection precedence is Flux -> Argo -> Sveltos -> Modelplane -> Helm -> Terraform -> ConfigHub -> Crossplane -> kro -> Custom -> K8s native. kro runs LATE in the chain, so if Flux labels are also present, the resource classifies as `OwnerFlux` and kro is invisible to the ownership detector.
- **Generated CRDs**: kro can generate new CRDs from an RGD (e.g., the `PaymentsPlatform` kind in the example). Resources of those generated kinds are still classifiable — the generated owner-reference apiVersion matches the kro group.

## References

- Code: `pkg/agent/ownership.go` `detectKroOwnership` + `isKroAPIVersion`, `pkg/agent/manager_strings.go` `ManagerKroApplyset` / `ManagerKroApplysetParent` / `ManagerKroLabeller`
- Upstream:
  - Applyset field manager: [`kro-run/kro applyset.FieldManager`](https://github.com/kro-run/kro)
  - Labeller field manager: [`kro-run/kro FieldManagerForLabeler`](https://github.com/kro-run/kro)
  - Applyset spec: [Kubernetes applyset KEP](https://github.com/kubernetes/enhancements/tree/master/keps/sig-cli/3659-kubectl-apply-prune)
- Related skills: [`scout-observe`](../scout-observe/SKILL.md), [`scout-attribute`](../scout-attribute/SKILL.md), [`observe-crossplane`](../observe-crossplane/SKILL.md) (similar composition pattern, different writers)
- Examples: kro composition layout in [`examples/kro-composition/`](../../examples/kro-composition/)

## Constraints

- This skill is read-only. kro Instance creation / RGD mutation are out of allowed-tools.
- kro ownership precedence runs AFTER Flux / Argo / Helm / Terraform / ConfigHub / Crossplane. Resources with any of those labels classify under those owners, not kro, even if kro is also writing.
- Pass-1 label iteration is non-deterministic when multiple kro labels are present. The `Type` classification is stable; the `Name` field may vary.
