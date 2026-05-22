---
name: pilot-release-verification
description: 'Use when Pilot is RUNNING POST-DEPLOY VALIDATION after a release has shipped — confirming the release actually landed per declared intent. Natural phrasings: "Pilot, verify the release", "did `payments-api@v1.4.3` land cleanly?", "post-deploy verification gate", "Pilot, render the release-quality verdict", "confirm the deploy matches Git", "did everything reconcile after the apply?". Pilot consumes cub-scout''s `compare three-way` (DRY/WET/LIVE per resource in the release scope) + `bindingSource` graph (did Links propagate correctly?) + `history` since the deploy timestamp (what changed in the last N minutes?) + `gitSource` anchor (does the live state match the release SHA?). Pilot renders a release-quality verdict per resource and a release-level rollup. Distinct from `pilot-cd-gate` which fires BEFORE the deploy; this skill fires AFTER. Do NOT load for: pre-deploy gate (use pilot-cd-gate), real-time event response (use pilot-watch-alert-response), rollback decision when verification fails (use pilot-rollback-decision), or incident close-out evidence (use pilot-incident-evidence).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout history *) Bash(cub-scout history *) Bash(cub scout history *) Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout watch *) Bash(cub-scout watch *) Bash(cub scout watch *) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub history *) Bash(cub link list *) Bash(cub unit get *) Bash(argocd app get *) Bash(flux get *)

---

# pilot-release-verification

The post-deploy validation scenario from **Pilot's** side. A release has just shipped (Argo synced, Flux reconciled, Helm applied, ConfigHub unit fired). Pilot is asked: *did it land cleanly?* Pilot consumes cub-scout's evidence at the post-deploy moment + the timeline since deploy + the binding graph + the source-truth anchor, then renders a release-quality verdict.

This is the **after-the-fact** companion to `pilot-cd-gate`. The CD gate fires *before* the deploy and decides whether to proceed; release-verification fires *after* the deploy and confirms it actually landed. Both are query-driven; the timing is the differentiator.

## When to use

Explicit phrasings:

- "Pilot, verify the release"
- "Did `payments-api@v1.4.3` land cleanly?"
- "Post-deploy verification gate"
- "Pilot, render the release-quality verdict"
- "Confirm the deploy matches Git"
- "Did everything reconcile after the apply?"
- "Pilot, sign off on the release"
- "Run the post-deploy gate against the release record"

Implicit intents:

- The deploy **already happened**; reconciliation may still be in flight
- The user wants to **confirm the landing**, not pre-gate the next deploy
- The result feeds a **release record** — typically a release ticket, a deploy webhook, a "did this work?" signal back to the CD pipeline
- The audit-attached path (receipt-saved) is the preferred output — the release record carries the receipt

## Do not load for

- A **pre-deploy gate** (the deploy hasn't happened yet) — [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md)
- A **real-time event-driven** response on subsequent drift — [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md)
- A **rollback decision** when verification failed — [`pilot-rollback-decision`](../pilot-rollback-decision/SKILL.md). This skill says "did the release land?"; rollback-decision picks what to do if it didn't.
- A **postmortem evidence pack** for a broken release — [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md)
- A **promotion safety check** between two variants — [`pilot-promotion-gate`](../pilot-promotion-gate/SKILL.md)
- A **periodic compliance audit** — [`pilot-compliance-audit`](../pilot-compliance-audit/SKILL.md)
- Any **mutating action**. Verification is read-only by design.

## The Pilot ↔ cub-scout release-verification loop

```
   CD pipeline: argocd app sync / flux reconcile / cub unit apply just fired at <deploy-time>
        │
        ▼
   (reconciliation in flight; allow a settle window — typically 30-180s)
        │
        ▼
   Pilot (acceptance judge in post-deploy mode)
        │  "did the release land cleanly?"
        │
        ├──► cub-scout doctor          # cluster-health sanity check
        │
        ├──► For each resource in the release scope:
        │      cub-scout compare three-way <r> -n <ns> --format json
        │      # DRY/WET/LIVE convergence check
        │
        │      cub-scout compare source-truth <r> -n <ns> --strategy <s> --format json
        │      # strategy-typed verdict against the release SHA
        │
        │      cub-scout history <r> -n <ns> --since <deploy-time> --format json
        │      # what changed since the deploy fired
        │
        │      cub-scout receipt verify <r> -n <ns> --strategy <s> \
        │        --at-commit <release-sha> \
        │        --save \
        │        --out release-verify-<r>.receipt.json
        │      # fingerprinted post-deploy verdict, pinned to the release SHA
        │
        └──► Convergence-still-in-flight detection:
              cub-scout compare three-way <r> -n <ns> --format json \
                | jq '.summary.agreement'
              # "converging" = give it more time
              # "diverged"   = the release did NOT land cleanly
              # "agreed"     = clean landing
        │
        ▼
   Pilot's release-quality verdict:
        │  All PASS, summary.agreement = "agreed"        → RELEASE PASS
        │  All PASS but summary.agreement = "converging" → RELEASE WATCH (wait + re-check)
        │  Any WATCH                                     → RELEASE WATCH (note in release record)
        │  Any BLOCK (manual-edit during deploy window)  → RELEASE BLOCK → escalate to rollback
        │  Any INCONCLUSIVE                              → RELEASE ASK   → human checks
        │
        ▼
   release record gets the verdict + per-resource receipts
        │  PASS  → close the release ticket as Verified
        │  WATCH → re-verify after another settle window; or close with caveats
        │  BLOCK → route to pilot-rollback-decision
        │  ASK   → page on-call; the proof gaps explain what's missing
        └────────────────────────────────────────────────────
```

## Settle window — the load-bearing timing detail

Reconciliation isn't instant. Argo CD application syncs typically converge in 10-60s for healthy resources; Flux Kustomizations in 20-180s; Helm in similar windows. Cron-style or autoscaled workloads can take longer.

Pilot's verification policy MUST account for a settle window:

| Resource type | Typical settle | Verification call timing |
|---------------|----------------|--------------------------|
| Deployment / ReplicaSet | 30-60s after sync | First verify after sync wait; retry on `converging` |
| StatefulSet | 60-180s (per-pod rolling) | Same; longer retry window |
| CronJob | next schedule (variable) | Don't verify until first job runs; or skip CronJobs from this gate |
| Helm release | 30-120s for full chart converge | Wait for `helm status` Deployed; then verify |

Pilot's policy choice: short-poll (re-verify every N seconds until `agreed` or timeout) vs single-shot (verify once after a fixed wait). cub-scout supports both — the verb is the same; the loop is Pilot's.

## What `compare three-way`'s `summary.agreement` field tells you

The post-deploy convergence signal:

| `summary.agreement` | Meaning |
|---------------------|---------|
| `agreed` | All resources in scope have DRY=WET=LIVE for every field. **Release landed cleanly.** |
| `converging` | Most resources agree; a small subset is in active reconciliation (`controller-drift` causes). **Release is in flight; wait and re-verify.** |
| `diverged` | At least one resource has a hard mismatch with no `controller-drift` cause. **Release did NOT land cleanly; investigate or rollback.** |
| `partial` | Mixed; the rollup couldn't pick a single label. **Read per-resource details.** |

`converging` is the most important post-deploy state — it's the "wait, the controller is still working on it" signal. Pilot's verification policy should treat `converging` as "not done yet" rather than a soft pass.

## Step-by-step

### Step 1 — pre-flight health check

```bash
$ cub-scout doctor
```

If the cluster itself is unhealthy (API server flapping, controller flat-lined), the verification call won't be trustworthy. Doctor's output is a sanity signal — verify cluster is in a state where reconciliation *could* succeed before evaluating whether it *did*.

### Step 2 — wait the settle window, then verify

```bash
# Adjust the sleep to your release type's typical settle
$ sleep 60

# Per-resource three-way + source-truth
$ for r in <list-of-released-resources>; do
    cub-scout compare three-way $r --format json > verify/$r-3way.json
    cub-scout compare source-truth $r --strategy git-argo --format json > verify/$r-st.json
  done
```

### Step 3 — pin to the release SHA via `--at-commit`

```bash
$ for r in <list>; do
    cub-scout receipt verify $r --strategy git-argo \
      --at-commit <release-sha> \
      --save \
      --out verify/$r.receipt.json
  done
```

`--at-commit <sha>` pins the spec anchor to the release's SHA. A PASS verdict confirms the live state matches **this release**, not some other state. The receipt is fingerprint-stable and lands in the release record.

### Step 4 — timeline check

```bash
$ cub-scout history deploy/payments-api -n prod --since "<deploy-time>" --format json
```

Reveals what ChangeSet entries fired in the deploy window. Should match the release's intent — if extra ChangeSets fired (manual edits during deploy, controller flaps), Pilot's verdict should downgrade.

### Step 5 — Pilot's release-quality verdict synthesis

| Per-resource verdicts | `summary.agreement` | Pilot release verdict | Pipeline action |
|------------------------|---------------------|-----------------------|-----------------|
| All PASS | `agreed` | **RELEASE PASS** | Close the release ticket as Verified |
| All PASS | `converging` | **RELEASE WATCH (wait)** | Re-verify after another settle window |
| Mix of PASS + WATCH | any | **RELEASE WATCH** | Note caveats in the release record; consider closing with caveats |
| Any BLOCK | `diverged` | **RELEASE BLOCK** | Route to [`pilot-rollback-decision`](../pilot-rollback-decision/SKILL.md) |
| Any BLOCK | `converging` | **RELEASE ASK** | Wait; the controller may still resolve the divergence |
| Any INCONCLUSIVE | any | **RELEASE ASK** | Page on-call; proof gaps explain what's missing |

### Step 6 — attach receipts to the release record

```bash
# The receipt path each resource's verdict landed at
$ cub-scout receipt list --format json \
    | jq '[.[] | select(.verifiedAt > "<deploy-time>")]' \
    > verify/release-receipts.json
```

The release record (release ticket, deploy webhook payload, change-management entry) gets the per-resource receipts attached. Six months later, an auditor can `cub-scout receipt validate` any individual receipt to confirm it's tamper-evident.

## Worked example

Release `payments-api@v1.4.3` (SHA `9e7d12fa`) just fired at `2026-05-22T14:30:00Z`. Strategy: `git-argo`. Settle window: 60s.

```bash
# Pre-flight
$ cub-scout doctor 2>&1 | tail -3
✓ API server reachable
✓ All required controllers Ready
✓ 2 namespaces selected for compare

# Wait the settle window
$ sleep 60

# Per-resource three-way
$ cub-scout compare three-way deploy/payments-api -n prod --format json \
    | jq '.summary'
{
  "agreement": "agreed",
  "agreed": 7,
  "converging": 0,
  "diverged": 0,
  "scope": "resource:Deployment/payments-api/prod"
}

# Source-truth + receipt
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --at-commit 9e7d12fa \
    --save \
    --out release-v1.4.3.receipt.json
# Verdict: PASS

# Timeline check
$ cub-scout history deploy/payments-api -n prod \
    --since 2026-05-22T14:30:00Z --format json | jq 'length'
1   # one ChangeSet entry (the release itself); no surprise edits
```

Pilot's release-quality verdict:

```json
{
  "release_id": "release-2026-05-22-payments-api-v1.4.3",
  "release_sha": "9e7d12fa",
  "deploy_time": "2026-05-22T14:30:00Z",
  "verify_time": "2026-05-22T14:31:00Z",
  "settle_window_seconds": 60,
  "release_verdict": "PASS",
  "summary_agreement": "agreed",
  "per_resource": [
    {"resource": "Deployment/payments-api in prod", "verdict": "PASS", "receipt": "release-v1.4.3.receipt.json"}
  ],
  "post_deploy_changesets": 1,
  "rationale": "DRY=WET=LIVE; source-truth PASS against release SHA; no surprise changes",
  "next_step": "close release ticket as Verified; attach release-v1.4.3.receipt.json"
}
```

The release ticket gets the verdict + the receipt. Six months later: `cub-scout receipt validate release-v1.4.3.receipt.json` exits 0 — fingerprint stable.

## Standalone vs connected

**Connected mode preferred** for full verification:

- `compare source-truth --strategy` — connected
- `history` — connected (ChangeSet timeline)
- `bindingSource` field on `compare three-way` — connected
- `cub link list` for Link-graph cross-check — connected
- `receipt verify --strategy source-truth-pass` — connected

Standalone mode is **partial**:
- `compare three-way --git-path <local-checkout>` works without ConfigHub for raw-YAML comparison
- `receipt verify --predicate applied-matches-spec --at-commit <sha>` produces a per-resource verdict
- No `history` (no ChangeSet timeline) — Pilot has to supply deploy-time from elsewhere
- No `bindingSource` — Link-graph verification skipped

A standalone post-deploy gate is still valuable but narrower. Pilot's policy should call out the limitation in the release record.

## Tool boundary

- **Allowed (cub-scout):** Compare verbs (read-only), `history`, `doctor`, `explain`, `trace`, `receipt verify` / `show` / `validate` / `list`, `watch` (for the optional event-driven verification track). All read-only.
- **Allowed (cluster):** `kubectl get/describe`. NOT `kubectl apply / edit / patch / delete / rollout undo`.
- **Allowed (ConfigHub):** `cub * get / list`, `cub history`, `cub link list`, `cub unit get`. NOT mutating `cub` verbs.
- **Allowed (controllers):** `argocd app get`, `flux get`. NOT `argocd app sync`, `flux reconcile` (those fired *before* this skill ran).
- **Verification is read-only by design.** If the verdict is BLOCK, the rollback path is [`pilot-rollback-decision`](../pilot-rollback-decision/SKILL.md) — and even there, cub-scout's role is evidence, not the controller sync.

## References

- [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md) — pre-deploy companion (this skill is the post-deploy half)
- [`pilot-rollback-decision`](../pilot-rollback-decision/SKILL.md) — when this skill returns BLOCK
- [`pilot-promotion-gate`](../pilot-promotion-gate/SKILL.md) — cross-variant; this skill is single-release
- [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md) — event-driven companion; useful as an alternative to short-poll re-verify
- [`scout-compare`](../scout-compare/SKILL.md) — Compare verbs (cub-scout side)
- [`scout-verify`](../scout-verify/SKILL.md) — receipts with `--at-commit`
- [`scout-govern`](../scout-govern/SKILL.md) — `history`
- Source-truth contract: `#393`, `#418` (Phase 2 strategies)
- Receipts: `#446` (v1), `#463` (v2 surface)

## Constraints

- **Settle window** is mandatory. Verification too early returns `converging` and Pilot's policy has to re-verify. Pilot's choice between short-poll (every N seconds) vs single-shot (fixed wait) is Pilot-side; cub-scout exposes the verbs.
- `--at-commit <sha>` pins the spec anchor revision; the rest of the predicate evaluation (attribution, source-truth derivation) runs against the **live** cluster state. Useful for "did we land *this specific* release?" rather than "is the live state plausible against *some* release?"
- For CronJobs or workloads that don't materialize a Pod until their schedule fires, verification within minutes of deploy may not be meaningful. Pilot's policy should either skip those resources from the per-release gate or delay verification until the first execution.
- The release-quality verdict is **per-release**, not per-resource. A single BLOCK resource flips the whole release to BLOCK by default; Pilot may pick a different roll-up policy (e.g., "ignore drift on `deploy/legacy-tool`") — that's Pilot-side.
- Verification's `--at-commit` pinning is meaningful only against `source-truth-pass` predicate. `applied-matches-spec` doesn't strictly require it (the controller-resolved anchor is sufficient when present).
