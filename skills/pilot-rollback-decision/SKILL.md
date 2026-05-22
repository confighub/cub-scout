---
name: pilot-rollback-decision
description: 'Use when Pilot is DECIDING WHEN AND WHICH REVISION TO ROLL BACK TO after an incident or a broken release. Natural phrasings: "Pilot, should we roll back?", "what revision should Pilot pick for the rollback?", "is the previous SHA safe to roll back to?", "render the rollback verdict + target", "Pilot, choose the rollback revision and explain why". Pilot consumes cub-scout''s `history` (ConfigHub ChangeSet timeline) + `impact` (blast-radius preview) + `compare source-truth --strategy <s>` (per-candidate-revision evidence) + `receipt verify --predicate applied-matches-spec --at-commit <sha>` (pin to a specific revision for fingerprinted evaluation). Pilot picks the rollback target and renders a safety verdict; the actual rollback is Pilot''s mutation path (controller sync to the chosen revision), not cub-scout''s. Do NOT load for: pre-deploy gate (use pilot-cd-gate), real-time event response (use pilot-watch-alert-response), drift-classification on the current state (use pilot-patch-and-drift), or incident close-out evidence assembly (use pilot-incident-evidence — which captures what happened; this skill picks where to go next).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout history *) Bash(cub-scout history *) Bash(cub scout history *) Bash(./cub-scout impact *) Bash(cub-scout impact *) Bash(cub scout impact *) Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub history *) Bash(cub unit get *) Bash(cub link list *) Bash(argocd app get *) Bash(flux get *)

---

# pilot-rollback-decision

The rollback-decision scenario from **Pilot's** side. After an incident, a broken release, or a failed verification, Pilot is asked: *should we roll back, and to which revision?* Pilot consumes cub-scout's `history` + `impact` + per-candidate-revision evidence and picks the rollback target with a safety verdict.

cub-scout is the **witness**; Pilot is the **judge**. ConfigHub (via `cub`) is the authority. cub-scout never picks a target or executes the rollback — it produces structured evidence per candidate revision, and Pilot decides. The mutation (controller sync to the chosen revision) is Pilot's surface, not cub-scout's.

## When to use

Explicit phrasings:

- "Pilot, should we roll back?"
- "What revision should Pilot pick for the rollback?"
- "Is the previous SHA safe to roll back to?"
- "Render the rollback verdict + target"
- "Pilot, choose the rollback revision and explain why"
- "How far back should the rollback go?"
- "Show me the safety verdict for rolling back to `<sha>`"

Implicit intents:

- A **rollback is being considered** (not a forward fix); time pressure may still be active but the decision needs structured evidence, not heuristic
- The user wants a **target revision** plus a safety verdict (not just "roll back somewhere")
- The result feeds a **decision record** — postmortem, change ticket, release rollback approval
- The mutation itself is Pilot's call; cub-scout's job is to make the safest target *legible*

## Do not load for

- A **pre-deploy gate** (the deploy hasn't happened yet) — [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md)
- A **real-time event-driven** response — [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md)
- A **drift classification** on current state without a rollback decision — [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md)
- A **postmortem evidence pack** (what happened) — [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md). That skill captures the incident's evidence; **this** skill picks where to go next.
- A **promotion** between two intentional variants (not a rollback) — [`pilot-promotion-gate`](../pilot-promotion-gate/SKILL.md)
- A **post-deploy** validation that didn't fail (just confirming the release landed) — [`pilot-release-verification`](../pilot-release-verification/SKILL.md)
- Any **mutating action**. The rollback execution path is Pilot's. cub-scout doesn't roll anything back.

## The Pilot ↔ cub-scout rollback loop

```
   broken release at HEAD = <bad-sha>
        │
        ▼
   Pilot (acceptance judge)
        │  "should we roll back, and to which revision?"
        │
        ├──► cub-scout history <r> -n <ns> --since 7d --format json
        │     # ConfigHub ChangeSet timeline — candidate prior SHAs
        │
        ├──► cub-scout impact <r> --format json
        │     # blast-radius of the current state + each candidate
        │
        ├──► For each candidate revision (call it <candidate-sha>):
        │      cub-scout receipt verify <r> -n <ns> \
        │        --strategy <s> \
        │        --at-commit <candidate-sha> \
        │        --out candidate-<sha>.receipt.json
        │      # fingerprinted, per-candidate verdict
        │      cub-scout compare three-way <r> -n <ns> --format json
        │        # what fields would change between current and candidate
        │
        └──► (optional) chain the candidates:
              cub-scout receipt verify <r> -n <ns> \
                --input-attestation candidate-<sha1>.receipt.json \
                --input-attestation candidate-<sha2>.receipt.json \
                --strategy <s> \
                --at-commit <chosen-sha> \
                --save                          # the chosen-target receipt
        │
        ▼
   Pilot's rollback verdict, by candidate:
        │
        │  candidate.verdict == PASS    → safe candidate
        │  candidate.verdict == WATCH   → safe but with caveats; check proof_gaps
        │  candidate.verdict == BLOCK   → unsafe target (would re-introduce a divergence)
        │  candidate.verdict == INCONCLUSIVE → can't evaluate; pick another candidate
        │
        ▼
   Pilot picks the first PASS candidate (most recent prior SHA), records
   the chosen-target receipt, and routes to Pilot's mutation path
   (controller sync to <chosen-sha>; implementation lives outside cub-scout).
        └────────────────────────────────────────────────────
```

## Candidate selection (most-recent-first PASS)

Pilot's default policy is **most-recent-first**: walk the ChangeSet timeline backwards from the current HEAD, evaluate each candidate revision with `--at-commit`, pick the first one that produces a PASS receipt for the declared strategy.

This is *not* "roll back to the last green build" — the last green build may have been green for the *wrong* reasons (cached evidence, fewer resources in scope, transient luck). cub-scout's per-candidate `receipt verify --at-commit <sha>` evaluates the candidate against the *current* cluster state with the *current* strategy. Pilot's verdict pack carries that exact pairing.

Alternative policies (Pilot-side; cub-scout doesn't enforce):

| Policy | When to use |
|--------|-------------|
| Most-recent PASS | Default; the smallest possible rollback to a verifiable state |
| Most-recent PASS with no INCOMPLETE proof gaps | Stricter; useful when source-truth evidence quality matters |
| Last release-tagged SHA | When the team distinguishes "release-tagged" SHAs from intermediate commits |
| Operator-named SHA | Pilot's interface to a human decision — the operator passes `--at-commit <sha>` directly |

Pilot's policy choice is *its* business; cub-scout's job is to make each candidate's safety legible via fingerprinted receipts.

## Step-by-step

### Step 1 — enumerate candidates from history

```bash
$ cub-scout history deploy/payments-api -n prod --since 7d --format json
[
  {"timestamp": "2026-05-22T14:00:00Z", "actor": "ci-bot", "change": "image: v1.4.2 → v1.4.3", "changeset": "CS-4821", "sha": "9e7d12fa"},
  {"timestamp": "2026-05-22T09:30:00Z", "actor": "ci-bot", "change": "replicas: 2 → 3", "changeset": "CS-4818", "sha": "8b6c11e9"},
  {"timestamp": "2026-05-21T17:15:00Z", "actor": "ci-bot", "change": "image: v1.4.1 → v1.4.2", "changeset": "CS-4815", "sha": "7a5b10d8"}
]
```

The ChangeSet timeline lists candidate prior SHAs in reverse-chronological order. Pilot's candidate set is `[8b6c11e9, 7a5b10d8, …]` (skip the current HEAD `9e7d12fa` which is presumed broken). Connected mode required (`cub auth login`) for `history`.

### Step 2 — per-candidate receipt with `--at-commit`

```bash
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --at-commit 8b6c11e9 \
    --save \
    --out candidate-8b6c11e9.receipt.json
# Pilot reads receipt.predicate.verdict directly
```

`--at-commit <sha>` overrides the controller-resolved revision in the spec anchor; the resulting receipt asserts "the live state at this moment matches the candidate revision per the declared strategy." A PASS verdict means rolling back to this SHA would NOT introduce drift; a BLOCK means it would.

Repeat per candidate. Each receipt is fingerprint-stable; Pilot's verdict pack carries the full set.

### Step 3 — blast radius per candidate

```bash
$ cub-scout impact deploy/payments-api --format json
```

`impact` is connected-mode only. It returns the blast-radius preview (downstream units, dependent workloads). Pilot weights candidates: a candidate that would change many downstream units is higher-risk than one that changes few — even if both verify PASS individually.

### Step 4 — chain the candidates into the chosen-target receipt

Once Pilot picks `<chosen-sha>`, the chosen-target receipt references each candidate evaluated:

```bash
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --at-commit <chosen-sha> \
    --input-attestation candidate-<sha1>.receipt.json \
    --input-attestation candidate-<sha2>.receipt.json \
    --save \
    --out rollback-target.receipt.json
```

The chosen-target receipt's `inputAttestations[]` records every candidate Pilot considered — even the ones it rejected. The chain shape is **why Pilot picked this target**, not just **which target Pilot picked**. Audit later can walk the chain and confirm the decision process.

### Step 5 — Pilot's verdict + handoff

Pilot's verdict pack:

```json
{
  "decision_type": "rollback",
  "resource": "Deployment/payments-api in prod",
  "current_sha": "9e7d12fa",
  "candidates_evaluated": [
    {"sha": "8b6c11e9", "verdict": "PASS", "receipt": "candidate-8b6c11e9.receipt.json"},
    {"sha": "7a5b10d8", "verdict": "PASS", "receipt": "candidate-7a5b10d8.receipt.json"}
  ],
  "chosen_sha": "8b6c11e9",
  "chosen_receipt": "rollback-target.receipt.json",
  "rationale": "most-recent PASS; smallest possible rollback to a verifiable state",
  "blast_radius": "1 unit affected (payments-api); no downstream Link impact",
  "next_step": "route to Pilot's controller-sync path with target=8b6c11e9; cub-scout does not execute the sync"
}
```

The downstream consumer (release rollback approver, change ticket) reads the verdict + chosen receipt. Pilot's mutation path (the controller sync to `8b6c11e9`) is **out of scope** for cub-scout — typically a `cub` / Argo / Flux operation in Pilot's stack.

## Worked example

Production `deploy/payments-api` at HEAD `9e7d12fa` is failing health checks. Pilot is asked to decide a rollback target.

```bash
# Step 1 — enumerate candidates
$ cub-scout history deploy/payments-api -n prod --since 24h --format json \
    | jq '.[].sha'
"9e7d12fa"  # current; broken
"8b6c11e9"  # 4h ago
"7a5b10d8"  # 21h ago

# Step 2 — verify each candidate
$ for sha in 8b6c11e9 7a5b10d8; do
    cub-scout receipt verify deploy/payments-api -n prod \
      --strategy git-argo \
      --at-commit $sha \
      --save \
      --out candidate-$sha.receipt.json
  done

# Step 3 — read verdicts
$ for sha in 8b6c11e9 7a5b10d8; do
    jq -r '.predicate.verdict' candidate-$sha.receipt.json
  done
PASS    # 8b6c11e9 → safe candidate
PASS    # 7a5b10d8 → also safe (older)

# Step 4 — chain candidates into chosen-target receipt
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --at-commit 8b6c11e9 \
    --input-attestation candidate-8b6c11e9.receipt.json \
    --input-attestation candidate-7a5b10d8.receipt.json \
    --save \
    --out rollback-target.receipt.json

# Step 5 — blast radius
$ cub-scout impact deploy/payments-api --format json | jq '.affected_units | length'
1
```

Pilot's verdict: roll back to `8b6c11e9` (most-recent PASS), blast radius 1 unit, evidence chain attached. Routes to Pilot's controller-sync path with target SHA; cub-scout's role ends at the verdict.

## Standalone vs connected

Most of this skill **requires connected mode** (`cub auth login`):

- `history` — connected (ChangeSet timeline lives in ConfigHub)
- `impact` — connected (blast-radius needs the Link graph)
- `receipt verify --strategy source-truth-pass` — connected
- Per-candidate evaluation can still produce a per-candidate receipt in standalone mode using `--predicate applied-matches-spec`, but without the ChangeSet timeline, Pilot has to supply candidate SHAs from somewhere else (release tags, the operator's memory, git log).

Run `cub-scout status` upfront; if standalone and `history` is needed, Pilot's policy should escalate to ASK ("operator must supply candidate SHAs").

## Tool boundary

- **Allowed (cub-scout):** `history`, `impact`, Compare verbs (read-only), `receipt verify` / `show` / `validate` / `list`, `explain`, `trace`. All read-only.
- **Allowed (cluster):** `kubectl get/describe`. NOT `kubectl apply / edit / patch / delete / rollout undo`.
- **Allowed (ConfigHub):** `cub * get / list`, `cub unit get`, `cub link list`, `cub history`. NOT `cub * create / update / delete`.
- **Allowed (controllers):** `argocd app get`, `flux get`. NOT `argocd app sync`, `flux reconcile`.
- **Pilot's rollback execution** (the controller sync, the unit revision update) is Pilot's mutation path, deliberately not enumerated here. cub-scout's role ends at the chosen-target receipt.

## References

- [`scout-govern`](../scout-govern/SKILL.md) — `history`, `impact` (cub-scout side)
- [`scout-verify`](../scout-verify/SKILL.md) — receipts; `--at-commit` for revision-pinned evaluation; `--input-attestation` for chained candidates
- [`scout-compare`](../scout-compare/SKILL.md) — Compare verbs
- [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md) — pre-deploy companion
- [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md) — captures *what happened*; this skill picks *where to go next*
- [`pilot-promotion-gate`](../pilot-promotion-gate/SKILL.md) — variant-to-variant safety (forward direction)
- [`pilot-release-verification`](../pilot-release-verification/SKILL.md) — post-deploy validation
- Chained receipts: `#448` (parent), `#463` (chained-half shipped)
- ConfigHub ChangeSet timeline: `cub-scout history` reference in commands.md

## Constraints

- `--at-commit <sha>` is the only knob for pinning the candidate-evaluation to a specific revision. It overrides the controller-resolved revision in the spec anchor; the rest of the predicate evaluation (attribution, source-truth derivation) runs against the **current** cluster state.
- Candidate enumeration via `history` requires connected mode AND the resource being ConfigHub-tracked. Resources not in ConfigHub's ChangeSet timeline can't be candidate-walked through `history`; Pilot has to supply candidate SHAs another way.
- The chosen-target receipt's `inputAttestations[]` is **not** ordered — RFC 8785 canonical JSON treats arrays as ordered, so Pilot should choose a deterministic order (e.g., evaluation order, or oldest-to-newest) for reproducibility.
- `impact` blast-radius is a *snapshot* — it reflects the Link graph at evaluation time, which may shift after the rollback. Pilot's policy should treat it as a hint, not a guarantee.
- cub-scout never executes the rollback. The chosen-target receipt is the handoff artifact; the controller sync (or unit revision update, or whatever Pilot's stack uses) is the mutation path Pilot owns.
