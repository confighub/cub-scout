# Contracts: what each artifact does and does not prove

This example is precise about claim strength, in the spirit of helm-expt's
"narrowest true claim" discipline. Do not collapse these into "verified".

| Artifact | Proves | Does NOT prove |
|---|---|---|
| `object-set-matches` receipt | Every desired object identity in `fixtures/release-objects.yaml` is present live, and every **authored field** matches. | That the workload is **running** (status is stripped); that **prerequisites** (the Secret) exist; that **no extra** live objects exist; freshness. |
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

## What would make the claim complete

`workloads-converged` (#476, readiness) and `prerequisites-met` (#477, target
facts) now ship and `verify.sh` runs them — `object-set-matches` PASS plus those
two BLOCKs is the honest picture of this install. Still open in
[`docs/proposals/helm-expt-driven-gaps.md`](../../docs/proposals/helm-expt-driven-gaps.md):
observation freshness (`observedAt` + `expiresAt`, #478) and an optional
closed-world check.
