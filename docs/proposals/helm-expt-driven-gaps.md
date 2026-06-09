# cub-scout gaps surfaced by the helm-expt use case

**Status:** Proposal / issue drafts (from a cross-repo synthesis)
**Driving use case:** `confighub/helm-expt` as cub-scout's first real "install verification" consumer
**Runnable reproduction:** [`examples/helm-expt/verify.sh`](../../examples/helm-expt/verify.sh)
**Last updated:** 2026-06-09

## Why this exists

helm-expt renders Helm charts to an explicit object set and wants cub-scout to be
the **live witness** that closes the loop: "the objects we approved are present,
matching, and actually working in the cluster." helm-expt's own docs already
assign cub-scout a day-2 charter — *"post-install readiness, Job result, webhook
health, PVC binding, drift"* (`helm-expt/docs/user/introduction-to-the-harness.md:125`)
— and its pain-point CSV has a dedicated `cub_scout_answer` column.

But when helm-expt actually needed to verify an install, it **did not call
cub-scout** — it shipped its own kubectl verifier
(`helm-expt/scripts/verify-install.mjs`) and its own receipt format. The reason is
a **scope gap**: cub-scout's keystone install predicate `object-set-matches`
proves *presence + authored-field match* and deliberately strips `status`, so it
cannot prove the very things helm-expt's charter assigns to cub-scout.

This document records that gap as prioritized, issue-ready write-ups. The
`examples/helm-expt/` demo reproduces it against a real kind cluster so each gap
is observable, not theoretical.

## The reproduction (real output, 2026-06-09)

The fixture applies a Deployment that references a Secret (`app-db-secret`) which
is **not** part of the shipped object set — helm-expt finding **F3** (the
"separated secret" / unmet prerequisite, `helm-expt/tests/findings.md:132`).

```text
receipt object-set-matches : PASS   (exit 0 under --fail-on any-non-pass)
  evidence.summary         : { desired:3, matched:3, missing:0, mismatched:0, inconclusive:0 }
  Deployment/web           : status "matched"  (authored fields equal live)
  receipt.nextSteps        : "every desired rendered object was present and all authored fields matched live"

cub-scout doctor           : { healthy:4, warning:1, error:0 }

ACTUAL cluster reality     : pod web-… CreateContainerConfigError, Ready=False
                             app-db-secret absent
```

So the governance-grade artifact (the receipt, the thing `--fail-on` gates CI on)
says **PASS / exit 0**, and `doctor` reports **error: 0** — while the workload
cannot start. This is exactly the helm-expt finding: *"Argo and cub-scout report
`Synced` while the workload sits in `CreateContainerConfigError` … the failure is
silent at the governance/diagnosis layer"* (`helm-expt/tests/findings.md:134,159`).

Note `doctor` does emit one **warning** — cub-scout is not blind — but a warning
is not a BLOCK and the receipt gate is still green. The lesson is about the
**verdict that becomes a durable artifact**, not about whether a human reading
`doctor` could eventually notice.

Two structural observations from the receipt JSON, relevant below:

1. The receipt carries only `verifiedAt` — **no freshness/TTL/expiry field**
   (`grep -iE 'ttl|fresh|expire'` → nothing).
2. Both subjects (`rendered-object-set://…` and `k8s-live-object-set://…`) carry
   the **same digest**, because `matchMode: authored-fields` hashes the *projected
   authored fields*, not an independent hash of live state. The "live" subject is
   therefore not evidence about everything actually live (status, extra objects,
   workload health) — only about the authored-field projection.

---

## Issue drafts

Each block below is copy-paste ready for `gh issue create`. Priority is my
recommendation; effort is rough.

### P0 — `workloads-converged` predicate (status-aware readiness)

**Suggested labels:** `enhancement`, `receipts`, `predicate`, `helm-expt`

**Problem.** `object-set-matches` strips `status` by design, so a receipt can be
PASS while Deployments are not Ready, Jobs have not Succeeded, and PVCs are not
Bound. helm-expt's charter assigns "post-install readiness, Job result, webhook
health, PVC binding" to cub-scout, and its own verifier re-implements PVC/PING
checks precisely because the receipt can't. The result is a silent false green
(see reproduction above and `helm-expt/tests/findings.md` F3).

**Proposal.** Add a `workloads-converged` predicate (companion to, not a change
of, `object-set-matches`) that reads `status` for the desired object set and
asserts readiness: Deployment/Statefulset/DaemonSet rollout complete, Job
Succeeded, PVC Bound, pods not in `CreateContainerConfigError` / `CrashLoopBackOff`
/ `ImagePullBackOff` / `CreateContainerError`. Verdict BLOCK on any not-converged;
WATCH while still progressing within a grace window; PASS when all converged.
Pair it with `object-set-matches` via `--input-attestation` for a "present AND
working" chain.

**Secondary fix (same theme).** cub-scout's false-green detection should treat
`CreateContainerConfigError` as an error-class pod reason, not a warning — helm-expt
found the equivalent one-line gap in its own contradiction checker
(`helm-expt/tests/findings.md` notes the scanner watched for `ImagePullBackOff` /
`CrashLoopBackOff` / `Error` but **not** `CreateContainerConfigError`). In the
reproduction, `doctor` rated this pod `warning`, not `error`.

**Acceptance.** Re-run `examples/helm-expt/verify.sh`: `workloads-converged`
returns BLOCK (exit 2 under `--fail-on any-non-pass`) for the F3 fixture; PASS
once `app-db-secret` is created and the pod becomes Ready.

**Non-goals.** Application-level health probes beyond Kubernetes readiness; SLO
evaluation.

**Effort.** Medium. Reuses kstatus-style readiness cub-scout already has for
`map status` / `watch`; the new surface is the predicate envelope + per-object
status evidence.

---

### P0 — `prerequisites-met` predicate (target facts)

**Suggested labels:** `enhancement`, `receipts`, `predicate`, `helm-expt`

**Problem.** helm-expt's load-bearing abstraction is **target facts**: cluster-state
dependencies (required CRDs, Secrets/keys, StorageClass, IngressClass, namespace,
kube version) lifted out of hidden Helm `lookup` into explicit, declared,
*live-checkable* inputs (`helm-expt/packages/*/collector/target-facts.sh` already
`kubectl get`s them). cub-scout knows CRDs/webhooks only as static-scan risk
types, not as a verifiable predicate. The unmet prerequisite in the reproduction
(absent `app-db-secret`) is invisible to every current predicate.

**Proposal.** Add a `prerequisites-met` predicate that takes a declared set of
required cluster facts and verifies them live: CRDs established (and serving),
named Secrets/keys present, StorageClass/IngressClass exists, namespace exists,
ValidatingWebhookConfiguration reachable with a non-empty CA bundle. Input shape
should map directly onto helm-expt's `targetFacts.requiredCRDs` /
`requiredSecrets`, so helm-expt can *delete* its kubectl logic and consume one
receipt. Verdict BLOCK on any missing required fact.

**Acceptance.** A receipt over the F3 fixture's declared prerequisites returns
BLOCK naming `secret/app-db-secret` as the unmet fact; PASS once present.

**Non-goals.** Inferring prerequisites from a chart (that is helm-expt's job at
render time); cub-scout consumes a declared list.

**Effort.** Medium. Mostly a new evidence collector + envelope; reuses the
discovery/REST-mapper plumbing `object-set-matches` already uses.

---

### P1 — Observation freshness: stamp `observedAt` + `expiresAt` on receipts

**Suggested labels:** `enhancement`, `receipts`, `helm-expt`

**Problem.** helm-expt treats freshness as part of the claim — its observation
receipts carry `observedAt` + `freshnessTTL: 1h`, and the model demands the UI
show "Observed by cub-scout 4m ago / stale / no receipt yet"
(`helm-expt/docs/reference/chart-recipe-manifest-flow.md`). cub-scout receipts
carry only `verifiedAt`; there is no TTL/expiry, so a workerless ConfigHub
ingesting the receipt cannot tell a fresh green from a stale one. This is the
single most structural mismatch for the "submit observation receipts to a
workerless server" model.

**Important framing.** This does **not** contradict the receipts-immutability
stance ("keep the receipt fresh is a category error",
[`receipts-way-forward.md`](./receipts-way-forward.md)). The receipt stays
immutable; we add an **immutable** `observedAt` (when the live read happened) and
an **immutable** `expiresAt` (a stamped freshness boundary, optionally from a
`--ttl` flag). Staleness is then computed by the *consumer* at read time against
`now`; the artifact never mutates.

**Acceptance.** `receipt verify … --ttl 1h` produces a receipt whose predicate
includes `observedAt` and `expiresAt`; `examples/helm-expt/verify.sh` reports
"freshness/TTL field on receipt: present".

**Non-goals.** Re-verification scheduling; that is the consumer's loop.

**Effort.** Low. Two fields + one flag; no envelope/wire-format change.

---

### P1 — Standalone git/file-as-DRY for `compare three-way`

**Suggested labels:** `enhancement`, `compare`, `helm-expt`

**Problem.** helm-expt's doctrine is that a true PASS = intended ↔ applied/synced
↔ live agree with exact field proof, never just "Synced". cub-scout has
`compare three-way`, but it requires ConfigHub for the DRY leg. helm-expt is
exactly the case that breaks this: it already *has* the rendered desired object
set on disk and no ConfigHub in the loop. cub-scout's own "what's coming next"
already lists git-as-DRY; helm-expt is the motivating consumer to prioritize it.

**Proposal.** Allow `compare three-way --git-path <dir>` / `--source-path <file>`
to supply DRY from rendered YAML or a git checkout, so the three-way agreement
view works standalone. (`object-set-matches` is the receipt sibling; this is the
interactive/compare sibling.)

**Acceptance.** `compare three-way --source-path fixtures/release-objects.yaml
--scope namespace/helm-expt-demo` returns a DRY/WET/LIVE agreement summary with
no ConfigHub connection.

**Effort.** Medium–large. DRY-source abstraction + wiring into the existing
three-way engine.

---

### P2 — Closed-world / pruning verdict (resolve `extra-live-object-coverage`)

**Suggested labels:** `enhancement`, `receipts`, `predicate`

**Problem.** `object-set-matches` carries a permanent self-declared omission:
*"it does not prove that no extra live objects exist outside that desired set."*
The reproduction plants a `drift-not-in-object-set` ConfigMap that the PASS
receipt happily ignores. For an install-equivalence claim ("what we shipped is
what is running, and nothing else"), present-but-not-complete is a real gap.
Compounded by the digest observation above: the "live" subject digest is the
authored-field projection, so it cannot reflect extras either.

**Proposal.** Add an optional closed-world check (`--prune-scope` /
`--no-extras`): enumerate live objects in scope, diff against the desired identity
set, and emit a WATCH/BLOCK plus an `extraObjects[]` evidence list when live has
objects the manifest does not. Resolves the standing omission when enabled.

**Acceptance.** With the check on, the reproduction reports the
`drift-not-in-object-set` ConfigMap as an extra; the omission is dropped from the
receipt.

**Effort.** Medium. Needs careful scoping (ownership/label filters) to avoid
flagging controller-created children.

---

### P2 — Helm/Kustomize provenance back-resolution

**Suggested labels:** `enhancement`, `attribution`, `helm-expt`

**Problem.** The root-cause theme across helm-expt's 15 pain points is "where did
this field value come from?". cub-scout's attribution `gitSource.file:line` works
only for *raw* YAML and is explicitly deferred for templated sources — i.e. it
degrades exactly where Helm/Kustomize (helm-expt's whole domain) lives.

**Proposal.** Extend attribution back-resolution so a live field can be traced to
the Helm template / values key or Kustomize overlay that produced it, not just a
rendered raw-YAML line. Pairs naturally with helm-expt, which holds the
chart→render mapping.

**Effort.** Large. This is the deep one; track as an epic.

---

### P2 — Foreign-receipt bridge + shared digest convention

**Suggested labels:** `enhancement`, `receipts`, `helm-expt`

**Problem.** helm-expt and cub-scout compute their object-set SHAs differently, so
their digests can't be cross-checked, and `--input-attestation` only chains
cub-scout-fingerprinted receipts. helm-expt's README admits its receipts must sit
"adjacent in the run directory until those upstream receipts are emitted or
bridged." The render→package→live story is therefore not one verifiable thread.

**Proposal.** (a) Document and share a canonical rendered-object-set digest
convention both repos compute identically over the same bytes; (b) let
`--input-attestation` (or a sibling `--reference-evidence`) ingest helm-expt's
`InstallerPackageReceipt` / `UserInstallObservationReceipt` by its
`renderedObjectSetSHA256`, so a cub-scout receipt can chain a non-cub-scout
upstream receipt by digest equality.

**Effort.** Medium. Digest convention is small; the bridge needs a foreign-receipt
adapter with explicit trust boundaries (digest match, not fingerprint).

---

## Suggested sequencing

1. **`workloads-converged` + `prerequisites-met`** (both P0) — these close the F3
   false green and let helm-expt drop its parallel verifier. Highest proven value.
2. **Observation freshness** (P1, low effort) — structurally required by the
   workerless-observation model; cheap.
3. **git/file-as-DRY three-way** (P1) — unlocks the entire no-ConfigHub path.
4. Then closed-world, foreign-receipt bridge, and the Helm provenance epic.

The fastest way to keep these honest is to wire `examples/helm-expt/verify.sh`
into CI against an ephemeral kind cluster: today it asserts the false green; as
each predicate lands, the script's gap table flips from ABSENT to a real verdict.
