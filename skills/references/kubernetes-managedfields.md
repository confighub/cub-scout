# Reference: Kubernetes managedFields

The data substrate cub-scout's [scout-attribute](../scout-attribute/SKILL.md) skill reads to classify field mutations. This reference covers what's in `metadata.managedFields`, how cub-scout consumes it, and the edge cases that produce `omissions[]` entries.

## What managedFields is

Every K8s resource (since v1.18 GA) carries a `metadata.managedFields` array. Each entry records who last wrote which fields:

```yaml
metadata:
  managedFields:
  - manager: argocd-controller
    operation: Apply
    apiVersion: apps/v1
    time: "2026-05-21T13:30:00Z"
    fieldsType: FieldsV1
    fieldsV1:
      f:spec:
        f:replicas: {}
        f:template:
          f:spec:
            f:containers:
              k:{"name":"api"}:
                f:image: {}
  - manager: kubectl-edit
    operation: Update
    apiVersion: apps/v1
    time: "2026-05-21T13:42:00Z"
    fieldsType: FieldsV1
    fieldsV1:
      f:spec:
        f:replicas: {}
```

The API server maintains this list automatically during server-side apply (SSA) and tracks field ownership per-path. Two writers can co-own the same resource — they can't co-own the same field.

## Anatomy of an entry

| Field | Meaning |
|---|---|
| `manager` | Free-form string identifying the writer. Argo, Flux, Helm, kubectl, custom controllers — each picks its own. See [`verified-manager-strings.md`](verified-manager-strings.md) for the enumeration cub-scout recognizes. |
| `operation` | `Apply` (server-side apply) or `Update` (legacy update / patch / direct write). |
| `apiVersion` | Which API version the writer was operating against — matters for field-existence over time. |
| `time` | When the writer last touched any of its fields. **Not** per-field — the latest write to *any* owned field. |
| `fieldsType` | Always `FieldsV1` in practice. Reserved for future encodings. |
| `fieldsV1` | The Trie-encoded set of fields this manager owns. JSON object where `f:foo` means "owns field foo" and `k:{"name":"api"}` is a list-key selector. |
| `subresource` | Optional — present when the write was to a subresource (`status`, `scale`, etc.). |

## What cub-scout reads from it

The attribution layer (#435) consumes two slices of managedFields:

### 1. Resource-level cause classification (A1)

For each `compareFieldMismatch`, cub-scout asks: *do any managedFields entries belong to a recognized GitOps controller?*

- **Any entry** matches the expected owner's manager string (per [verified-manager-strings.md](verified-manager-strings.md)) → `cause: controller-drift`
- **No controller entry** present, only `kubectl-*` (or bare `kubectl` SSA) → `cause: manual-edit`
- **Both** present → `cause: manual-edit` (the operator edited *on top of* controller reconciliation; surface manual involvement)
- **Neither** → `cause: unknown` (parse, don't guess)

The expected owner is determined by `pkg/agent/ownership.go` (labels + annotations). cub-scout co-signals the manager string against the label-based owner to disambiguate cases where the same manager string is used by different writers — most notably `kubectl-client-side-apply`, which Argo CD uses for CSA migration *and* `kubectl apply` writes.

### 2. Per-field-path resolution (A1.5)

When `FieldsV1` is decodable, cub-scout walks the Trie and produces per-canonical-path attribution. Field paths like `.spec.replicas` and `.spec.template.spec.containers` get their own `cause` + `managerHint`.

This matters when, say, Argo owns `.spec.template.spec.containers` but someone edited `.spec.replicas`. Without per-path resolution, the resource-level rollup loses the distinction (mixed → `manual-edit`).

List-key selectors (`k:{"name":"api"}`) are partially supported — cub-scout reads them but does not yet expose them in the canonical-path map. Fields like `containers[name="api"].image` fall back to the resource-level rollup for now.

## What happens when evidence is missing

Two horizons here — be precise about which you're reading.

**Today (in the attribution layer JSON contract on `compareFieldMismatch` and `explain`):**

| Condition | Today's output |
|---|---|
| `metadata.managedFields` array is empty or missing | `cause: "unknown"`; `managerHint` omitted |
| `fieldsV1` is present but undecodable | per-field attribution falls back to resource-level rollup |
| Field path doesn't have a canonical-path mapping (e.g., container images spanning list items) | per-field result inherits resource-level rollup |
| The expected owner is `OwnerUnknown` (no recognizable labels) | `cause: "unknown"`; the cause classifier refuses to guess |
| No connected-mode evidence available (standalone mode) | `bindingSource` field is simply omitted from the JSON (zero value via `omitempty`) |

The principle is the same as the receipt layer: **never guess**. The current JSON contract uses `cause: "unknown"` plus field omission to express "we don't know"; the contract is documented in [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § "Field Mutation Attribution Contract".

**Future (when receipts ship in #446 batch 1):** `cub-scout receipt verify` will surface these gaps as structured `omissions[]` array entries on the receipt envelope, with shape `{missing: <field/source>, reason: <human-readable>, severity?: info|warning}`. The schema there forces the distinction between *"all checks passed"* and *"the checks we ran passed, but we didn't run all of them"* — see [`docs/proposals/receipts-way-forward.md`](../../docs/proposals/receipts-way-forward.md) § "Omissions". Until receipts ship, the attribution layer expresses gaps via `cause: unknown` + field omission.

## Edge cases and caveats

### managedFields is field-manager evidence, not complete mutation-history proof

A field with `manager: argocd-controller` was *most recently* written by Argo CD. It may have been edited by `kubectl-edit` an hour earlier and overwritten by Argo since — that history is *not* in managedFields. For full mutation history, cross-reference the cluster audit log (out of cub-scout's scope).

The `no-manual-edits-since <timestamp>` predicate planned for `scout-verify` carries this caveat in its description; receipts produced by that predicate include the limitation in their `omissions[]` entries.

### Some controllers strip managedFields

`internal/scan/confighub_provider.go` and `pkg/query/drift.go` in cub-scout itself strip managedFields *for diff purposes* (it's noise in field comparisons). The attribution layer reads managedFields *before* the strip; the existing strip paths are unchanged.

Some external controllers and admission webhooks also strip managedFields — most commonly old controllers that pre-date GA. cub-scout records the strip as an `omissions[]` entry.

### Server-side apply vs Update operations

`operation: Apply` (SSA) is the cleaner signal — the API server tracks ownership at field-path granularity, and conflicting Applies fail by default.

`operation: Update` (legacy patch / put / direct edit) records ownership but conflicts silently overwrite. `kubectl edit` writes Update entries; so does any non-SSA controller.

cub-scout treats both as ownership evidence — the `operation` field is preserved in the JSON contract but doesn't change the classification.

### kubectl-client-side-apply ambiguity

Argo CD's CSA migration default and `kubectl apply --client-side` both write the manager string `kubectl-client-side-apply`. The classifier disambiguates via the `argocd.argoproj.io/tracking-id` annotation (label co-signal). See [`verified-manager-strings.md`](verified-manager-strings.md) for the full disambiguation table.

### Crossplane composed-resource hash suffix

Crossplane composed resources have manager strings like `apiextensions.crossplane.io/composed-<hash>` where the hash is per-XR. The classifier matches by **prefix**, not exact string, so `apiextensions.crossplane.io/composed-9f8e7d6c5b4a` matches the Crossplane composed-children rule.

### Subresources

Writes to `/status` carry `subresource: status` and a separate manager (typically the same controller, but conceptually distinct). cub-scout's compare and attribution code focuses on the main resource — subresource managedFields entries are preserved in the raw evidence but don't drive the cause classification.

## What's in the cluster vs what's in the receipt

When a receipt is emitted (forthcoming, #446), the `evidence.attribution` block carries:

- The full deduplicated list of manager strings seen on the resource (`managers: ["argocd-controller", "kubectl-edit"]`)
- The classified `cause` + representative `managerHint`
- Per-canonical-path attribution where available

The receipt does *not* embed the raw `fieldsV1` Trie — that's too noisy and would inflate receipt size. The classification result is the contract; the raw evidence is reproducible by running `kubectl get -o yaml --show-managed-fields` against the same resource at the same revision.

## References

- Kubernetes official docs: [Server-side apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/)
- `apimachinery` `ManagedFieldsEntry` type — `k8s.io/apimachinery/pkg/apis/meta/v1` (see `types.go`)
- `sigs.k8s.io/structured-merge-diff/v4/fieldpath` — the `FieldsV1` decoding library cub-scout uses
- cub-scout code: `pkg/agent/field_ownership.go` (A1 classifier), `pkg/agent/field_ownership_paths.go` (A1.5 per-path resolution), `pkg/agent/manager_strings.go` (verified enumeration)
- Attribution layer parent issue: #435
- JSON contract: `docs/reference/json-contracts.md` § "Field Mutation Attribution Contract"

## See also

- [`verified-manager-strings.md`](verified-manager-strings.md) — the enumeration of manager strings cub-scout recognizes, with upstream citations
- [`scout-attribute/SKILL.md`](../scout-attribute/SKILL.md) — the skill that uses this data
