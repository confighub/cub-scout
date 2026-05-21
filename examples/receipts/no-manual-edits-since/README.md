# Receipt Example: `no-manual-edits-since`

`no-manual-edits-since` verifies that no interactive (`kubectl-*`)
writer touched `metadata.managedFields` after a given cutoff
timestamp. It's the receipt-shaped complement to the attribution
layer's `manual-edit` cause classification — useful for release-gate
checks like "no operator emergency-edited this Deployment after the
freeze started."

cub-scout **never invents a cutoff** — you must pass `--since` in
RFC 3339 form.

## Quick Start

```bash
# "No manual edits after midnight UTC today."
./cub-scout receipt verify deploy/api -n prod --since 2026-05-22T00:00:00Z

# Write the canonical JSON form to disk for audit attachment.
./cub-scout receipt verify deploy/api -n prod --since 2026-05-22T00:00:00Z --format json --out api.no-edits.receipt.json
```

## Verdict Logic

The predicate walks `live.metadata.managedFields` and classifies each
entry by manager:

- **Controller writer** (e.g. `argocd-controller`, `kustomize-controller`,
  `helm-controller`) → ignored. We don't claim anything about
  controller writes; only about interactive ones.
- **Interactive writer** (any `kubectl-*` manager, plus the verified
  enumeration in `pkg/agent/manager_strings.go`) → checked against
  the cutoff:
  - `e.Time > since` → **BLOCK**. A human bypassed the GitOps loop
    after the freeze.
  - `e.Time <= since` → ignored. Old edit, already accounted for.
  - `e.Time == nil` → **INCONCLUSIVE** + `OmissionManagedFieldsTime`.
    We cannot place the entry on the timeline and refuse to claim
    "no manual edits since T" without proof.
- **Unknown writer** (not in the verified manager-string enumeration)
  → ignored. Conservative — same `parse, don't guess` rule the
  attribution layer uses.

If no managedFields exist at all → **INCONCLUSIVE** +
`OmissionManagedFields`. Old K8s versions, admission-webhook-stripped
resources, or replayed objects all degrade gracefully rather than
producing false PASS.

## Caveat: managedFields Is Best-Effort

K8s `managedFields` is the only field-level evidence cub-scout has in
standalone mode. The predicate inherits its limitations:

- An admission webhook can strip the entries.
- A `kubectl apply --server-side` with a custom field manager can mask
  the writer identity.
- A namespace migration can rewrite the entries with a fresh Time.

In all these cases the predicate emits INCONCLUSIVE rather than
fabricating a PASS. See `docs/proposals/receipts-way-forward.md` § "field-manager evidence caveat" for the full discussion.

## Example Files

### `pass-controller-only.json` — PASS

`argocd-controller` is the only writer; its Time predates the cutoff.
No interactive writer present, so verdict: PASS.

### `block-kubectl-edit.json` — BLOCK

Same Deployment, but a `kubectl-edit` entry now appears with Time
after the cutoff. Verdict: BLOCK. The `nextStep` reason names the
violating manager and the offending timestamp so the operator can
investigate in `kubectl get --show-managed-fields`.

## References

- Parent issue: [`confighub/cub-scout#446`](https://github.com/confighub/cub-scout/issues/446) — Receipt capability
- Manager-string enumeration: `pkg/agent/manager_strings.go`
- Attribution layer (companion evidence): [`examples/drift/mutation-cause-attribution/`](../../drift/mutation-cause-attribution/)
- Implementation: `pkg/agent/receipt_predicates.go` `EvaluateNoManualEditsSince`
