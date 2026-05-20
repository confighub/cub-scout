# Mutation Cause Attribution

When `compare three-way` or `explain` detects that a live resource diverges from its desired state, the **mutation cause** classifies *why* — controller still reconciling vs someone bypassing the GitOps loop.

## The Problem

Two ways live state can diverge from desired:

```
desired (Git/ConfigHub)              live (cluster)
─────────────────────                ──────────────
replicas: 3              ──→  ✗      replicas: 1
```

- Maybe the **controller is mid-sync** — Argo CD just pushed the desired state and the replica count will converge in a few seconds.
- Maybe **someone ran `kubectl edit`** — bypassing GitOps. The controller will eventually overwrite, but in the meantime live state is wrong and the change isn't tracked anywhere.

Both look identical in a plain field diff. The difference matters because they need different responses: wait for one, investigate the other.

## How cub-scout Distinguishes Them

Kubernetes records *who last wrote each field* in `metadata.managedFields`. cub-scout reads this list and co-signals it against the resource owner detected from labels (see `pkg/agent/ownership.go`).

```
metadata:
  managedFields:
  - manager: argocd-application-controller   ← Argo CD writing
    operation: Apply
  - manager: kubectl-edit                    ← Someone editing manually
    operation: Update
```

- Manager string matches the **expected owner's** known controller → `controller-drift`
- Only `kubectl-*` managers (or bare `kubectl` for SSA) → `manual-edit`
- Both present → `manual-edit` (operator on top of controller — surface the manual involvement)
- Neither → `unknown` (parse-don't-guess; never misclassify)

The verified manager-string enumeration lives in `pkg/agent/manager_strings.go` with citations to upstream sources for Argo CD, Flux (kustomize/helm/source controllers), Helm direct, Crossplane (composite/composed/claim/MRD/refs), kro, and `kubectl-*`.

## Scenario A: Controller Reconciling

An Argo CD-managed Deployment has `replicas: 1` in the cluster but ConfigHub says `replicas: 3`. The controller will sync — wait.

See [`controller-drift.json`](controller-drift.json) for the annotated `compare three-way` output. The key field:

```json
"cause": "controller-drift",
"managerHint": "argocd-application-controller"
```

In `explain`:

```
  Drift          Detected by ConfigHub
  Mutation cause controller-drift (manager: argocd-application-controller)
```

**Next step:** wait for reconciliation; re-run `compare three-way` after a sync.

## Scenario B: Manual Edit on a GitOps Resource

Same resource, but this time a `kubectl edit` happened on top of Argo CD. The mismatch is *not* transient — Argo will fight back, but until then live state is wrong and the manual change is lost when Argo wins.

See [`manual-edit.json`](manual-edit.json). The key field:

```json
"cause": "manual-edit",
"managerHint": "kubectl-edit"
```

In `explain`:

```
  Drift          Detected by ConfigHub
  Mutation cause manual-edit (manager: kubectl-edit)
```

**Next step:** investigate who edited, port the change back to Git/ConfigHub if intentional, revert in the cluster.

## Co-signal: `kubectl-client-side-apply` is Ambiguous

Argo CD's CSA migration default and `kubectl apply --client-side` both write `kubectl-client-side-apply`. The string alone cannot disambiguate.

The classifier uses the resource owner (detected by labels and annotations like `argocd.argoproj.io/tracking-id`) as a co-signal:

- Argo-owned resource + `kubectl-client-side-apply` → `controller-drift` (Argo CSA migration)
- Native resource + `kubectl-client-side-apply` → `manual-edit` (`kubectl apply`)

## What This Doesn't Tell You (Yet)

At A1, the classification is **resource-level** — the same `cause` is reported for every field mismatch on one resource. If only one field was edited by hand and the rest are controller-managed, A1 still surfaces the resource-level conclusion (mixed → `manual-edit`).

Per-field-path resolution (decoding `FieldsV1` to attribute each field to its specific writer) is the next stage, A1.5.

## References

- Parent issue: [confighub/cub-scout#435](https://github.com/confighub/cub-scout/issues/435) — Attribution layer
- A1: [confighub/cub-scout#436](https://github.com/confighub/cub-scout/issues/436) — managedFields classification
- Implementation: `pkg/agent/manager_strings.go`, `pkg/agent/field_ownership.go`
- JSON contract: `docs/reference/json-contracts.md` § Field Mutation Attribution Contract
