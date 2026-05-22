---
name: observe-crossplane
description: 'Use when the user wants to observe CROSSPLANE-managed resources specifically — the controller-specific observer skill for Crossplane. Natural phrasing: "is this a Crossplane managed resource?", "which XR / Claim / Composition produced this?", "show me Crossplane composed children", "what does the `apiextensions.crossplane.io/composed-<hash>` manager mean?", "trace this resource back to the Claim", "is this from `*.crossplane.io` or `*.upbound.io`?", "show me ProviderConfig secrets". Load whenever intent involves Crossplane specifically — XRs (Composite Resources), Claims, Compositions, MRDs (managed-resource-definitions), provider managed resources, ProviderConfigs, or the crossplane-system namespace. Do NOT load for: generic ownership (use scout-observe), Helm/Argo/Flux (use their specific skills), authoring Crossplane CRDs themselves.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout tree *) Bash(cub-scout tree *) Bash(cub scout tree *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *) Bash(crossplane describe *) Bash(crossplane trace *) Bash(crossplane beta trace *)
---

# observe-crossplane

The controller-specific observer skill for **Crossplane**. cub-scout's coverage here is comprehensive — XRs, Claims, MRDs, composite/composed, the reference resolver, and the `crossplane-system` control-plane subset. This skill is the field guide.

Verified against `pkg/agent/ownership.go` (`detectCrossplaneOwnership`, `detectCrossplaneSystemOwnership`) and `pkg/agent/manager_strings.go` Crossplane constants.

Marked **experimental** in `CLAUDE.md` § "Ownership Detection" — the breadth of provider-specific writers Crossplane gets composed through means edge cases keep appearing.

## When to use

- "Is this a Crossplane managed resource?"
- "Which Claim / XR / Composition produced this?"
- "Show me the composed children of this composite resource"
- "What does the `apiextensions.crossplane.io/composed-<hash>` manager string mean?"
- "Trace this resource back to the Claim"
- "Is this in `*.crossplane.io` or `*.upbound.io`?"
- "Show me ProviderConfig secrets"

## Do not load for

- Generic ownership ("who owns this resource?") — [`scout-observe`](../scout-observe/SKILL.md)
- Helm / Argo / Flux specifically — their respective `observe-*` skills
- Authoring Compositions / XRDs / Claims — that's Crossplane YAML editing, not cub-scout
- Operating providers (provider install / upgrade) — out of cub-scout scope

## Detection signals (in `detectCrossplaneOwnership` + `detectCrossplaneSystemOwnership`)

Crossplane detection runs in **two passes**:

### Pass 1 — control-plane subset (`detectCrossplaneSystemOwnership`)

Catches the Crossplane control-plane / package-manager API groups even when they're not managed by GitOps. Narrow on purpose — broad capture would mislabel provider-managed resources like `*.aws.crossplane.io`.

| Trigger | SubType | Confidence | Notes |
|---------|---------|-----------|-------|
| `apiVersion` in `pkg.crossplane.io` or `apiextensions.crossplane.io` | `system` | high | XRDs, Compositions, Providers, ProviderRevisions, Functions, FunctionRevisions, Configurations |
| Resource in `crossplane-system` namespace AND apiGroup ends in `.crossplane.io` or `.upbound.io` AND kind is one of: `providerrevision`, `configurationrevision`, `functionrevision`, `provider`, `configuration`, `function`, `deploymentruntimeconfig` | `system` | medium | Heuristic — best-effort for the control-plane subset that lives in `crossplane-system` |

### Pass 2 — workload classification (`detectCrossplaneOwnership`)

Catches managed resources produced by Crossplane workflows.

| Signal | SubType | Confidence | Notes |
|--------|---------|-----------|-------|
| `crossplane.io/claim-name` label | `claim` | high | Resource was created from a Claim. `crossplane.io/claim-namespace` label populates `Namespace`. |
| `crossplane.io/composite` label | `composite` | high | Resource is a composed child of an XR. `Name` = composite name. |
| `crossplane.io/composition-resource-name` annotation | `managed-resource` | medium | Names the slot within a Composition that produced this resource. |
| `ownerReferences[].apiVersion` matches `*crossplane.io*` or `*upbound.io*` | `<lowercase owner kind>` | high | The owner-reference fallback when labels aren't present. |

The two passes are intentional: control-plane resources (XRDs, Compositions, etc.) classify differently from workload resources (a Composite Resource's Deployment, etc.). The control-plane pass runs FIRST so a Composition itself never gets misclassified as managed-resource.

## Manager strings (writers in `metadata.managedFields`)

Five distinct writers, four of which appear on managed children, plus the reference resolver:

| Manager | Source | Reconciles |
|---------|--------|-----------|
| `apiextensions.crossplane.io/composite` | `crossplane/crossplane FieldOwnerXR` | Fields on the XR itself (composite-controller writes) |
| `apiextensions.crossplane.io/composed-<hash>` | `crossplane/crossplane FieldOwnerComposedPrefix + ComposedFieldOwnerName` | Fields on composed children (one suffix per XR). **Match by prefix only.** |
| `apiextensions.crossplane.io/claim` | `crossplane/crossplane` claim package | Fields synced from a Claim back to its XR |
| `apiextensions.crossplane.io/managed` | `crossplane/crossplane FieldOwnerMRD` | Fields written by the managed-resource-definition reconciler |
| `managed.crossplane.io/api-simple-reference-resolver` | `crossplane/crossplane-runtime fieldOwnerAPISimpleRefResolver` | Cross-resource reference resolution (every provider that links via crossplane-runtime) |

`controllerManagersForOwner(OwnerCrossplane, ...)` returns all five matchers, with the composed-prefix one matching by `strings.HasPrefix`.

### The composed-prefix gotcha

`apiextensions.crossplane.io/composed-<hash>` is the only manager string in cub-scout's enumeration that does NOT match by exact equality. The `<hash>` is per-XR (Crossplane computes it from the XR identity), so trying to match on the exact string would miss every other XR. The matcher uses `strings.HasPrefix`, captured in the `managerMatcher{pattern, isPrefix}` shape in `manager_strings.go`.

## Worked example

```bash
$ kubectl get bucket my-bucket -n crossplane-system -o yaml | yq '.metadata.labels, .metadata.ownerReferences, .metadata.managedFields[].manager'
labels:
  crossplane.io/composite: storage-prod-7m4k9
  crossplane.io/claim-name: storage-prod
  crossplane.io/claim-namespace: platform
ownerReferences:
  - apiVersion: storage.acme.io/v1alpha1
    kind: XStorage
    name: storage-prod-7m4k9
managers:
  - apiextensions.crossplane.io/composite
  - apiextensions.crossplane.io/composed-9f3a2b
  - managed.crossplane.io/api-simple-reference-resolver
```

cub-scout reads this as:

```
$ cub-scout explain bucket/my-bucket -n crossplane-system
Resource: Bucket/my-bucket in crossplane-system
  Owner:    Crossplane composite "storage-prod-7m4k9" (label:crossplane.io/composite)
  Drift:    None detected
  Mutation cause: controller-drift (manager: apiextensions.crossplane.io/composed-9f3a2b)
```

The matcher correctly identifies `apiextensions.crossplane.io/composed-9f3a2b` via the prefix match.

## Edge cases

- **Pass-1 vs pass-2 ordering**: A `Composition` resource lives in `crossplane-system` AND its apiGroup is `apiextensions.crossplane.io`. Pass 1 catches it → `SubType=system`. Pass 2 would otherwise misclassify it. Don't reorder.
- **Claim vs composite labels both present**: `crossplane.io/claim-name` wins (Pass 2 priority order). The composite name is recoverable from the ownerReference chain.
- **Provider-managed resources without labels**: If a provider creates a resource without writing the Crossplane labels, cub-scout falls back to ownerReference inspection (`*crossplane.io*` or `*upbound.io*` in `apiVersion`). If neither matches, the resource classifies as `OwnerUnknown` (parse-don't-guess).
- **`apiextensions.crossplane.io/managed` (MRD)**: A WRITER, not a detector signal. cub-scout's ownership detector doesn't look for this manager string to identify ownership — it identifies XR/Claim/composite via labels first. MRD is only relevant as a *controller co-signal* once ownership is already established.
- **Reference resolver writes**: Every Crossplane provider that links resources via `crossplane-runtime` will produce a `managed.crossplane.io/api-simple-reference-resolver` entry in `managedFields`. This is normal and counts as `controller-drift` (the controller resolved a reference), not `manual-edit`.

## ProviderConfig secrets

Crossplane providers reference auth secrets via `ProviderConfig.spec.credentials.secretRef`. cub-scout's secret evidence layer (#328) detects these and surfaces them in `trace` output. Cross-namespace resolution is handled (a ProviderConfig in `crossplane-system` can reference a secret in any namespace). The secret status surfaces as `present` / `missing` / `unreadable` / `unresolved` — see [`scout-observe`](../scout-observe/SKILL.md) for the secret evidence surface.

## References

- Code: `pkg/agent/ownership.go` `detectCrossplaneOwnership` + `detectCrossplaneSystemOwnership`, `pkg/agent/manager_strings.go` `ManagerCrossplaneComposite` / `ManagerCrossplaneComposedPrefix` / `ManagerCrossplaneClaim` / `ManagerCrossplaneMRD` / `ManagerCrossplaneRefResolver`
- Upstream:
  - Field owners: [`crossplane/crossplane FieldOwnerXR / FieldOwnerComposedPrefix / FieldOwnerMRD`](https://github.com/crossplane/crossplane)
  - Reference resolver: [`crossplane/crossplane-runtime fieldOwnerAPISimpleRefResolver`](https://github.com/crossplane/crossplane-runtime)
- Related skills: [`scout-observe`](../scout-observe/SKILL.md), [`scout-attribute`](../scout-attribute/SKILL.md), [`observe-native`](../observe-native/SKILL.md) (for OwnerReference fallback)
- Examples: [`examples/crossplane-system/`](../../examples/crossplane-system/)

## Constraints

- Crossplane ownership is marked **experimental** in CLAUDE.md — edge cases keep appearing as new providers emit new manager strings. If a manager string isn't in the verified enumeration, the classifier falls through to `unknown`. Parse, don't guess.
- This skill is read-only. `crossplane install`, `crossplane update`, claim creation / mutation are out of allowed-tools.
- For ProviderConfig secret evidence, the per-field cause classification still works — manual edits to a managed resource produce `manual-edit` regardless of how exotic the controller chain is.
