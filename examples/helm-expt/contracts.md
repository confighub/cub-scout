# Contracts: what each artifact does and does not prove

This example is precise about claim strength, in the spirit of helm-expt's
"narrowest true claim" discipline. Do not collapse these into "verified".

| Artifact | Proves | Does NOT prove |
|---|---|---|
| `object-set-matches` receipt | Every desired object identity in `fixtures/release-objects.yaml` is present live, and every **authored field** matches. | That the workload is **running** (status is stripped); that **prerequisites** (the Secret) exist; that **no extra** live objects exist; freshness. |
| `object-set-diff` receipt (`compare object-set --dry-from`) | The **set-level delta** between a rendered (desired) object set and live: `changedObjects` with per-object **authored-field deltas**, plus `removedObjects` (desired absent live) and — with `--diff` — `addedObjects` (live extras). One shape for dry-run (changed render vs live) and drift (current render vs live). | That the workload is **running** (status is stripped); value-provenance (which input value caused a delta — Issue B); a cross-fleet roll-up (Issue C); that no live extras exist unless `--diff` was passed. |
| `cub-scout doctor` | A human-readable health summary; surfaces the broken pod as a **warning**. | A gating verdict — `error: 0` here despite a dead pod. |
| `cub-scout compare drift --file` | Whether supported workload fields (e.g. replicas, images) drifted. | Full authored-field equivalence; readiness; prerequisites. |
| The fixture's missing Secret | (by design) reproduces F3 — the unmet prerequisite. | n/a |

## The deliberate scope line

`object-set-matches` is honest about its own boundary. The receipt carries this
omission verbatim:

> object-set-matches verifies desired object identities and authored fields from
> the rendered manifests; it does not prove that no extra live objects exist
> outside that desired set.

And it strips `status`, so readiness is out of scope **by construction**, not by
oversight. The PASS verdict is therefore *correct for what it claims*. The
example exists to document the distance between "what it claims" and "the install
actually worked".

## Subject digest caveat

Both receipt subjects (`rendered-object-set://…` and `k8s-live-object-set://…`)
carry the same SHA-256 because `matchMode: authored-fields` hashes the projected
authored fields, not an independent snapshot of live state. Read the "live"
subject as "the authored-field projection of live", not "everything that is live".

## object-set-diff verdict rule

`compare object-set --dry-from` emits an `object-set-diff` receipt whose verdict
is, by construction:

- **BLOCK** — one or more objects present on both sides have **authored-field
  deltas** (`changedObjects` non-empty). This is real drift / "this change would
  touch these fields".
- **WATCH** — only **closure deltas** (`addedObjects` / `removedObjects`) and no
  changed objects. The set's membership differs but every shared object's
  authored fields match.
- **PASS** — no changed, added, or removed objects.

It is **tool-agnostic** by construction (reads the cluster + the `--dry-from`
source, never the reconciler) and **deterministic-desired** (the desired side is
the supplied render, not a re-render). The receipt is signed (fingerprint) and
chain-walkable (`inputAttestations[]`), like the other receipts. Per-field
`cause` / `managerHint` attribution is carried for classified fields when live
`managedFields` resolve a writer; absent that signal the field is left
unattributed (it is **not** stamped a bare `unknown`).

## What would make the claim complete

`workloads-converged` (#476, readiness) and `prerequisites-met` (#477, target
facts) now ship and `verify.sh` runs them — `object-set-matches` PASS plus those
two BLOCKs is the honest picture of this install. Observation freshness
(`observedAt` + `expiresAt`, #478) now ships too — pass `--ttl` to stamp it, as
`verify.sh` does. Still open in
[`docs/proposals/helm-expt-driven-gaps.md`](../../docs/proposals/helm-expt-driven-gaps.md):
the optional closed-world check.
