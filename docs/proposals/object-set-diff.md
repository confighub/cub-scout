# Proposal: object-set-diff (field-level set diff + value provenance + fleet)

**Status:** proposal. Driven by the helm-expt user story (helm-expt#991, schema
in helm-expt#992). Self-contained per the `examples/helm-expt` constraint.

## Honest starting point: most of this already ships

cub-scout v2.5.0 already delivers the hard part of "a GitOps-tool-agnostic diff
that beats Argo's and Flux's." `compare three-way --dry-from` (#479):

- diffs **DRY** (a local rendered file/dir — held render-once data) vs **LIVE**
  (the cluster), **field-level** (`compareResourceResult.Mismatches`), with no
  connected mode required;
- attributes drift to the writing **field manager** (`Live.Attribution.Cause` +
  `ManagerHint`) — i.e. *which controller/tool* changed a field;
- back-resolves source provenance with `--source-path` (gitSource file:line per
  mismatch);
- is **tool-agnostic by construction** — it reads *the cluster + the DRY source*,
  never the reconciler, so Argo-, Flux-, and cub-direct-delivered objects diff
  identically;
- scopes to resource / namespace / cluster / view, with CI exit codes.

`object-set-matches` (`receipt_object_set.go`) already proves a whole rendered
set is present and field-matched. So we should **not** claim a from-scratch diff
engine. The deltas below are what is genuinely missing.

## The genuine gaps

### 1. `object-set-diff` — the set-level receipt (the new primitive)
`object-set-matches` answers a **boolean** ("does the set match?"); `compare
three-way` answers **per resource**. Neither emits a single **set-level delta
receipt**: the whole rendered object set vs live, as `changedObjects` /
`addedObjects` / `removedObjects` with per-object field deltas, signed and
chain-walkable like the other receipts. That receipt is what a dry-run ("what
will this change touch across the set?") and drift ("what differs across the
set?") both need. Schema: `ObjectSetDiffReceipt` (helm-expt#992) — extend, don't
reinvent, by aggregating the existing per-resource `Mismatches` into a set-level
receipt.

### 2. Value provenance (folds into the #481 epic)
`--source-path` resolves a mismatch to a gitSource *file:line*. The user story
wants the mismatch resolved to the **input value** that caused it (e.g.
"`image.digest` changed these two StatefulSets"). helm-expt already proves this
mapping: `value-source-map.yaml` (scored 13/13 in `blast-radius-accuracy`).
This is exactly the #481 Helm/Kustomize provenance epic (Phase 2 values-key
resolution). The object-set-diff should carry a `provenance.valuePath` per
changed object, sourced from an ingested value-source-map.

### 3. Fleet view
`compare three-way` is single-scope (one cluster). The user story needs the diff
**across N environments** with override-protection ("this change reaches dev /
staging / prod-us-east; prod-eu-west pins it, so it is shielded"). helm-expt
ships the input (`fleet.yaml` + the fleet blast-radius surface); cub-scout would
aggregate per-environment object-set-diffs into a fleet roll-up.

## Unification (free, once #1 lands)
The same compare engine serves both day-1 and day-2:
- **dry-run** (pre-apply): `--dry-from <changed render>` vs LIVE → what a proposed
  change would touch.
- **drift** (post-apply): `--dry-from <current render>` vs LIVE → what differs now.

One receipt shape (`ObjectSetDiffReceipt`) for both.

## Better than Argo / Flux — the honest version
Argo and Flux each give an accurate live-side diff (we already use the same
server-side technique) but re-render the desired side (lookup + generated-value
nondeterminism → OutOfSync), are per-Application/HelmRelease, carry no
value-provenance, no cross-fleet view. cub-scout's compare is **already
tool-agnostic and deterministic-desired** (via `--dry-from`); the differentiators
this proposal adds are the **set-level receipt**, **value-provenance**, and
**fleet**. (This refines helm-expt#992, which over-stated the diff itself as the
frontier — the frontier is narrower.)

## Plan
- Issue A: `object-set-diff` receipt + `compare object-set --dry-from <set> [--diff]`
  (aggregate existing per-resource `Mismatches`; emit `ObjectSetDiffReceipt`).
- Issue B: value-provenance — ingest a value-source-map and stamp
  `provenance.valuePath` per changed object. Track under the #481 epic (Phase 2).
- Issue C: fleet roll-up over per-environment object-set-diffs.
- Runnable proof: extend `examples/helm-expt/` (a redis `image.digest` dry-run +
  a drift case) — self-contained; reproduces the helm-expt#992 worked example.

## Non-goals
- Replacing Argo/Flux delivery (cub-scout stays a read-only observer/differ).
- A local re-render engine (that is #481 Phase 4; here the DRY side is the
  held render-once data, supplied via `--dry-from`).

## Acceptance
- `ObjectSetDiffReceipt` emitted by a `compare object-set` path, signed +
  chain-walkable like existing receipts.
- The redis worked example runs offline in `examples/helm-expt` for both a
  dry-run and a drift case, tool-agnostic.
- Value-provenance stamped per changed object where a value-source-map is
  supplied; honest marker when it is not.
