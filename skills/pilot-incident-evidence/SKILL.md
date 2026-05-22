---
name: pilot-incident-evidence
description: 'Use when Pilot is ASSEMBLING THE INCIDENT EVIDENCE PACK during incident close-out — Pilot renders a structured, fingerprinted evidence package the postmortem and the audit ticket both consume. Natural phrasings: "Pilot, build the evidence pack for last night''s outage", "render the incident verdict + evidence for the SEV-2", "Pilot, what does cub-scout have on this incident?", "freeze the evidence before the cluster moves on", "assemble the postmortem inputs from cub-scout". Pilot composes `trace` + `explain` + attribution + recent K8s events + `bundle` + `history` + (crucially) per-affected-resource `receipt verify --save` with chained `--input-attestation` for multi-stage incidents. Differs from `operator-incident-evidence` which is OPERATOR-DRIVEN (the SRE assembling for their own postmortem); this skill is JUDGE-DRIVEN (Pilot rendering an evidence pack + verdict for downstream consumption). Do NOT load for: pager-active triage (use triage-unhealthy-workload), a real-time event-driven response (use pilot-watch-alert-response), a periodic fleet audit (use pilot-fleet-conformance), or any mutating remediation. cub-scout never mutates; Pilot''s mutation path is `cub` / Argo / Flux.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout compare *) Bash(cub-scout compare *) Bash(cub scout compare *) Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout bundle *) Bash(cub-scout bundle *) Bash(cub scout bundle *) Bash(./cub-scout history *) Bash(cub-scout history *) Bash(cub scout history *) Bash(./cub-scout audit *) Bash(cub-scout audit *) Bash(cub scout audit *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout snapshot *) Bash(cub-scout snapshot *) Bash(cub scout snapshot *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get events *) Bash(kubectl logs *) Bash(cub * get) Bash(cub * list) Bash(cub history *) Bash(cub link list *) Bash(cub unit get *) Bash(argocd app get *) Bash(flux get *)

---

# pilot-incident-evidence

The incident-evidence scenario from **Pilot's** side. After the incident is past peak (pager no longer firing), Pilot is asked to **render an evidence pack + verdict** that downstream consumers — the postmortem doc, the release-engineering meeting, the audit ticket, the compliance review — all consume.

This is the **judge-driven** companion to [`operator-incident-evidence`](../operator-incident-evidence/SKILL.md) (the SRE-driven postmortem assembly). Same cub-scout verbs feed both; the difference is framing: the operator wants to understand the incident; Pilot wants to render a verdict + a fingerprinted evidence pack on the incident's close-out.

The receipts v2 chained-attestation surface (`--input-attestation`, shipped in `#463`) is load-bearing here: a multi-stage incident produces a **chain** of per-stage receipts, each referencing the prior, and the final verdict receipt has the full chain inline. cub-scout never mutates; Pilot's "what should we change" recommendation is its own surface, not cub-scout's.

## When to use

Explicit phrasings:

- "Pilot, build the evidence pack for last night's outage"
- "Render the incident verdict + evidence for the SEV-2"
- "Pilot, what does cub-scout have on this incident?"
- "Freeze the evidence before the cluster moves on"
- "Assemble the postmortem inputs from cub-scout"
- "Pilot, attach the verdict + fingerprinted receipts to the audit ticket"
- "Chain the receipts so the postmortem has the full delivery path"

Implicit intents:

- The incident is **past peak** (pager no longer firing); time pressure has eased
- Pilot is rendering for an **audience** — postmortem doc, release-engineering meeting, compliance audit
- The evidence needs to be **immutable** (fingerprinted) — it shouldn't shift if someone fixes the cluster afterwards
- The output is a **verdict + an evidence pack**, not just a description
- Multi-stage incidents (deploy → reconciliation → drift → revert) want **chained receipts**, not isolated artifacts

## Do not load for

- **Pager-active triage** (the incident is still firing) — [`triage-unhealthy-workload`](../triage-unhealthy-workload/SKILL.md)
- A **real-time event-driven response** as the incident unfolds — [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md)
- A **single-resource pre-deploy gate** that would have prevented the incident — [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md)
- A **periodic fleet audit** — [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md)
- A **drift-classification deep-dive** in isolation — [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md)
- An **operator-driven** evidence assembly (SRE writing their own postmortem) — [`operator-incident-evidence`](../operator-incident-evidence/SKILL.md)
- Any **mutating action**. cub-scout doesn't revert / fix / restart. Pilot's mutation path is `cub` / Argo / Flux / kubectl.

## The Pilot ↔ cub-scout incident-evidence loop

```
   incident SEV-2 fired at 14:00, resolved at 16:00 — close-out at 16:30
        │
        ▼
   Pilot (acceptance judge)
        │  "render the evidence pack + verdict for incident-2026-05-22-001"
        │
        ├──► Per affected resource:
        │      cub-scout trace <r> -n <ns> --format json     # owner + lineage
        │      cub-scout explain <r> -n <ns> --presentation ai
        │      cub-scout compare three-way <r> -n <ns>      # DRY/WET/LIVE + attribution
        │        --format json
        │      cub-scout history <r> -n <ns> --since 4h     # ChangeSet timeline
        │      cub-scout receipt verify <r> -n <ns>
        │        --predicate no-manual-edits-since
        │        --since <pre-incident timestamp>
        │        --save                                      # was a manual edit involved?
        │
        ├──► Cluster-level capture:
        │      cub-scout bundle --since 4h                   # debug-bundle archive (replay-able)
        │      cub-scout snapshot                            # GSF JSON snapshot (offline)
        │      cub-scout audit list --since 4h               # ConfigHub audit entries
        │
        ├──► (multi-stage) Chain the receipts:
        │      # Stage 1: pre-incident receipt
        │      cub-scout receipt verify <r> ... --out stage1.receipt.json
        │      # Stage 2: at-incident receipt, chained to stage 1
        │      cub-scout receipt verify <r> ... \
        │        --input-attestation stage1.receipt.json \
        │        --out stage2.receipt.json
        │      # Stage 3: post-resolution receipt, chained to stage 2
        │      cub-scout receipt verify <r> ... \
        │        --input-attestation stage2.receipt.json \
        │        --out stage3.receipt.json
        │
        └──► (optional) cub-scout receipt list --format json > incident-receipt-inventory.json
        │
        ▼
   Pilot's evidence pack + verdict:
        │
        │  evidence_pack = {
        │    incident_id: "incident-2026-05-22-001",
        │    verdict: <max-severity over all receipts>,
        │    timeline: [
        │      {stage: "pre", receipt: "stage1.receipt.json"},
        │      {stage: "at", receipt: "stage2.receipt.json"},
        │      {stage: "post", receipt: "stage3.receipt.json"}
        │    ],
        │    bundle: "incident-bundle.tar.gz",
        │    snapshot: "incident-snapshot.json",
        │    audit_entries: [...],
        │    next_step: "<Pilot's recommendation, e.g., open Git PR to remove kubectl-edit access>"
        │  }
        │
        ▼
   downstream consumer (postmortem / audit / compliance) reads the pack
        └────────────────────────────────────────────────────
```

## The four-tier evidence shape

Pilot's incident pack composes four tiers of cub-scout evidence, in increasing audit durability:

| Tier | Verb | Durability | Shape |
|------|------|------------|-------|
| **1. Ephemeral** | `compare three-way`, `explain`, `trace`, `history` | One-shot JSON; valid at this moment | Live evidence — the cluster state at incident close-out |
| **2. Bundle snapshot** | `bundle`, `snapshot` | Tar / JSON archive replay-able offline | Cluster-wide state freeze; survives the cluster being rolled |
| **3. Fingerprinted receipt** | `receipt verify --save` | Immutable per-resource artifact; SHA-256 fingerprint over RFC 8785 canonical JSON | Typed evidence; verdict per predicate; verifiable later via `receipt validate` |
| **4. Chained receipts** | `receipt verify --input-attestation <prior>` | Per-stage receipts linked by `inputAttestations[]`; each ref's fingerprint verified at chain construction | Multi-stage incident timeline as a tamper-evident DAG |

Pilot uses tier-1 + tier-2 for the narrative; tier-3 + tier-4 for the **immutable** part of the pack. The chained-receipts tier is what makes a multi-stage incident's evidence trustworthy: tampering with any upstream receipt invalidates every downstream receipt's fingerprint.

## Step-by-step

### Step 1 — define scope and timeline

- What resources were involved? (`deploy/payments-api`, maybe `deploy/payments-worker`, the upstream `service/payments`)
- What time window? (incident fired 14:00, resolved 16:00; capture from 13:00 to 16:30 to cover pre-and-post state)
- Was there a **deploy** during the window? a **manual edit**? a **controller reconciliation flap**?

### Step 2 — resource-level evidence (per affected resource)

```bash
# Owner + lineage
$ cub-scout trace deploy/payments-api -n prod --format json > payments-api-trace.json
$ cub-scout explain deploy/payments-api -n prod --presentation ai > payments-api-explain.txt

# DRY/WET/LIVE + per-field attribution
$ cub-scout compare three-way deploy/payments-api -n prod --format json > payments-api-3way.json

# ChangeSet timeline
$ cub-scout history deploy/payments-api -n prod --since 4h --format json > payments-api-history.json

# Manual-edits check
$ cub-scout receipt verify deploy/payments-api -n prod \
    --predicate no-manual-edits-since \
    --since 2026-05-22T13:00:00Z \
    --save \
    --out payments-api-manual-edits.receipt.json
```

The four outputs together answer: who owns it, how is it diverging now, what changed recently, and was a kubectl-* writer involved.

### Step 3 — cluster-level capture (offline-replay-able)

```bash
$ cub-scout bundle --since 4h > incident-bundle.tar.gz
$ cub-scout snapshot > incident-snapshot.json
$ cub-scout audit list --since 4h --format json > incident-audit.json
```

The bundle archives enough cluster state to replay offline (`cub-scout bundle replay` later). The snapshot is GSF JSON for tooling. `audit list` collects connected-mode ConfigHub audit entries.

### Step 4 — multi-stage chained receipts (the load-bearing piece)

For a multi-stage incident (e.g., deploy at 13:50 → drift at 14:00 → revert at 15:30), capture per-stage receipts and chain them:

```bash
# Stage 1: pre-incident baseline
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --at-commit <pre-incident-sha> \
    --save \
    --out incident/stage1-pre.receipt.json

# Stage 2: at peak (during the incident)
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --input-attestation incident/stage1-pre.receipt.json \
    --save \
    --out incident/stage2-at.receipt.json

# Stage 3: post-resolution
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --input-attestation incident/stage2-at.receipt.json \
    --save \
    --out incident/stage3-post.receipt.json
```

The chain forms a tamper-evident DAG: `stage3` references `stage2` references `stage1`, and each parent's fingerprint is verified at chain construction time. Tampering with `stage1.receipt.json` after the fact would refuse to chain into `stage2` — chain construction errors out upfront rather than silently propagating bad evidence.

The verify step inside `cub-scout receipt verify --input-attestation` is enforced **at the API boundary** via the `VerifiedAttestationRef` typed wrapper (#463 Codex round-6 P1 fix). Programmatic callers cannot bypass the verify step; the chain integrity property is locked in code.

### Step 5 — Pilot's verdict synthesis

Pilot's verdict for the incident pack is **max-severity over the chained receipts**:

| Stage receipts' verdicts | Pilot's incident verdict |
|--------------------------|--------------------------|
| All PASS | **PASS** (no incident; close as no-op) |
| At least one WATCH, rest PASS | **WATCH** (close with a flag) |
| At least one ASK | **ASK** (postmortem owners need to fill proof gaps) |
| Any BLOCK | **BLOCK** (real incident; write a postmortem) |

The chained shape encodes the timeline: PASS → BLOCK → PASS reads as "we had a manual-edit episode that we recovered from"; PASS → BLOCK → BLOCK reads as "we never recovered intent; the live state is still divergent."

### Step 6 — assemble the pack for downstream consumption

```json
{
  "incident_id": "incident-2026-05-22-001",
  "scope": "Deployment/payments-api in prod",
  "window": "13:00–16:30",
  "verdict": "BLOCK",
  "timeline": [
    {"stage": "pre",  "at": "13:00", "receipt": "stage1-pre.receipt.json",  "verdict": "PASS"},
    {"stage": "at",   "at": "14:00", "receipt": "stage2-at.receipt.json",   "verdict": "BLOCK"},
    {"stage": "post", "at": "16:30", "receipt": "stage3-post.receipt.json", "verdict": "PASS"}
  ],
  "bundle": "incident-bundle.tar.gz",
  "snapshot": "incident-snapshot.json",
  "audit_entries": "incident-audit.json",
  "narrative": "explain + trace + 3way outputs (txt + json)",
  "root_cause_hypothesis": "manual-edit on image during pager-active triage (kubectl-edit; verified manager string)",
  "recommendation": "open Git PR amending apps/prod/payments-api/deployment.yaml; remove ad-hoc kubectl-edit access from on-call IAM"
}
```

The downstream consumer (postmortem author, audit reviewer) reads the verdict + the chained timeline + the cluster-state artifacts. Every receipt is fingerprint-verifiable; `cub-scout receipt validate` exit-coded for CI.

## Worked example

Incident `incident-2026-05-22-001`: `deploy/payments-api` in prod fired SEV-2 at 14:00. Resolved by 16:00. Pilot is rendering the close-out pack.

```bash
# Pre-incident baseline (the controller said this was good at 13:00)
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --at-commit 9e7d12fa \
    --save \
    --out incident/stage1-pre.receipt.json
# verdict: PASS, fingerprint: sha256:abc...

# At incident peak (14:00)
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --input-attestation incident/stage1-pre.receipt.json \
    --save \
    --out incident/stage2-at.receipt.json
# verdict: BLOCK, evidence shows manual-edit on container image; fingerprint: sha256:def...

# Post-resolution (16:30)
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --input-attestation incident/stage2-at.receipt.json \
    --save \
    --out incident/stage3-post.receipt.json
# verdict: PASS, the post-resolution Git revision matches LIVE; fingerprint: sha256:789...

# Bundle + snapshot for offline replay
$ cub-scout bundle --since 4h > incident/bundle.tar.gz
$ cub-scout snapshot > incident/snapshot.json

# Connected-mode timeline
$ cub-scout history deploy/payments-api -n prod --since 4h --format json > incident/history.json
$ cub-scout audit list --since 4h --format json > incident/audit.json

# Receipt inventory for the incident namespace
$ cub-scout receipt list --format json > incident/receipt-inventory.json
```

Pilot's verdict pack (rendered as JSON for the audit ticket):

```json
{
  "incident_id": "incident-2026-05-22-001",
  "verdict": "BLOCK",
  "scope": "Deployment/payments-api in prod",
  "window": "13:00–16:30",
  "chain": ["stage1-pre", "stage2-at", "stage3-post"],
  "verdict_per_stage": ["PASS", "BLOCK", "PASS"],
  "root_cause": "manual-edit on container image during pager triage",
  "verified_manager": "kubectl-edit",
  "recovered_at": "16:00",
  "evidence_paths": {
    "receipts": "incident/stage{1,2,3}-*.receipt.json",
    "bundle": "incident/bundle.tar.gz",
    "snapshot": "incident/snapshot.json",
    "history": "incident/history.json",
    "audit": "incident/audit.json"
  },
  "recommended_follow_ups": [
    "Open Git PR codifying the image bump as intent",
    "Review on-call IAM (remove kubectl-edit on prod deployments)",
    "Open issue against #449 — investigate whether watch alert fired pre-pager"
  ]
}
```

The audit ticket gets the verdict + the chained-receipts manifest. Six months later, an auditor can:

```bash
$ cub-scout receipt validate incident/stage1-pre.receipt.json   # exit 0 — fingerprint intact
$ cub-scout receipt validate incident/stage2-at.receipt.json    # exit 0 — fingerprint intact
$ cub-scout receipt validate incident/stage3-post.receipt.json  # exit 0 — fingerprint intact
$ cub-scout receipt show incident/stage2-at.receipt.json --format ascii
# Same evidence Pilot rendered the verdict from; tamper-evident.
```

If any of the receipts had been tampered with between then and now, `receipt validate` would exit 1 — the audit would immediately know the chain was untrustworthy.

## Standalone vs connected

Most of this skill works in **both modes**:

- Standalone: tier-1 + tier-2 + tier-3 (with `--predicate applied-matches-spec` and `--predicate no-manual-edits-since`); chained receipts work. Receipts will carry an `OmissionConfigHubUnitSubject` entry.
- Connected: adds `--strategy source-truth-pass` (tier-3 quality boost), `cub-scout audit list`, `cub-scout history`, `bindingSource` in attribution. Recommended for production incident packs.

Pilot should run `cub-scout status` in the pack-assembly preamble; if standalone, Pilot's pack records the limitation explicitly (the receipts themselves already record it via omissions).

## Tool boundary

- **Allowed (cub-scout):** `trace`, `explain`, all Compare verbs, `doctor`, `bundle`, `history`, `audit`, `receipt verify` / `show` / `validate` / `list`, `snapshot`, `map`. All read-only.
- **Allowed (cluster):** `kubectl get/describe`, `kubectl get events`, `kubectl logs`. NOT `kubectl apply / edit / patch / delete` / `rollout undo`.
- **Allowed (ConfigHub):** `cub * get / list`, `cub history`, `cub link list`, `cub unit get`. NOT `cub * create / update / delete`.
- **Allowed (controllers):** `argocd app get`, `flux get`. NOT `argocd app sync`, `flux reconcile`.
- **Pilot's recommendation** ("open a Git PR", "remove kubectl-edit access") is **a recommendation only**. cub-scout doesn't implement the recommendation; Pilot's mutation path is `cub` / Argo / kubectl. This skill produces evidence and a verdict, not a remediation.

## References

- [`operator-incident-evidence`](../operator-incident-evidence/SKILL.md) — operator-driven counterpart (SRE writing their own postmortem)
- [`scout-verify`](../scout-verify/SKILL.md) — receipts + chained `--input-attestation`
- [`scout-observe`](../scout-observe/SKILL.md) — `bundle`, `snapshot`, `watch` (the source of replay-able evidence)
- [`scout-attribute`](../scout-attribute/SKILL.md) — `cause` / `managerHint` / `gitSource` per field
- [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md) — drift-classification companion (per-field, not per-incident)
- [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md) — real-time companion (the events that may have fired during the incident; this skill replays them in the bundle)
- [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md) — what should have caught the incident pre-deploy (postmortem may recommend tightening this)
- Chained receipts: `#448` (issue), `#463` (chained-half shipped); `VerifiedAttestationRef` typed wrapper enforces verify at API boundary
- in-toto Statement v1, RFC 8785 canonical JSON, SHA-256 fingerprint: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § Receipt Contract

## Constraints

- The incident pack's **immutability** depends on the receipt store's `O_EXCL` atomic file create (cub-scout side). The store at `$XDG_DATA_HOME/cub-scout/receipts` is per-host; Pilot should archive receipts to a durable store (S3, ConfigHub bundle catalog) for long-term audit retention.
- Chained receipts ONLY verify chain integrity at construction time — `cub-scout receipt validate` checks one receipt's fingerprint; verifying a multi-stage chain requires walking each `inputAttestations[]` entry and validating it independently. A future helper may automate this; today it's a manual loop.
- `--input-attestation` requires the **upstream receipt to be a valid in-toto Statement v1 envelope with a verifiable fingerprint**. Tampered upstreams refuse to chain at the verify-time API boundary (`VerifiedAttestationRef` typed wrapper); Pilot must capture pristine receipts at each stage.
- `bundle` archives can be large for high-resource clusters. Pilot may want to scope `bundle --namespace <ns>` to the affected namespace rather than the whole cluster.
- The verdict is **per-incident**, not per-resource. A multi-resource incident produces N chains, and Pilot's incident-level verdict is the max-severity over all chains.
