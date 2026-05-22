---
name: pilot-cd-gate
description: 'Use when Pilot (or another acceptance-judge tool) is making a PRE-DEPLOY GATE DECISION in a CD pipeline using cub-scout''s structured evidence. Natural phrasings the operator brings to Pilot: "should this deploy go?", "is this release safe to promote?", "what does cub-scout say about this strategy?", "Pilot, gate the apply", "block the deploy if source-truth disagrees", "PASS/WATCH/ASK/BLOCK on payments-api release". Pilot consumes cub-scout''s `compare source-truth --strategy <s>` (with optional `receipt verify --strategy <s> --fail-on any-non-pass` for typed evidence + exit-2 CI gate) and renders a verdict. cub-scout NEVER decides PASS/BLOCK — it produces evidence; Pilot makes the call. Do NOT load for: a Pilot post-deploy verification (use pilot-release-verification — coming in batch B), a real-time event-driven response (use pilot-watch-alert-response), a multi-cluster periodic audit (use pilot-fleet-conformance), or any cub-scout authoring command. Pilot mutates via `cub` / Argo / Flux — never via cub-scout.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub unit get *) Bash(cub link list *) Bash(argocd app get *) Bash(flux get *)

---

# pilot-cd-gate

The pre-deploy CD-gate scenario from **Pilot's** side. A release pipeline asks Pilot: *should this deploy go?* Pilot consumes cub-scout's structured evidence — specifically `compare source-truth --strategy <s>` plus (optionally) a typed `cub-scout receipt verify --strategy <s>` — and renders a verdict that the pipeline gates on.

cub-scout is the **witness**. Pilot is the **judge**. ConfigHub (via `cub`) is the authority. cub-scout never decides PASS / BLOCK — it produces evidence with documented `omissions[]` where evidence is missing, and Pilot makes the call. This skill is about how Pilot reads that evidence; it is NOT about how Pilot itself mutates.

## When to use

Explicit phrasings the user brings to Pilot (which Pilot then satisfies by calling cub-scout):

- "Should this deploy go?"
- "Pilot, gate the apply"
- "Is `payments-api@v1.4.3` safe to promote to prod?"
- "Block the deploy if source-truth disagrees"
- "Run the pre-deploy gate against `git-argo`"
- "PASS/WATCH/ASK/BLOCK on the release ticket"
- "Should we promote this PR's branch?"

Implicit intents:

- The user is **about to deploy** (pre-deploy, not post-deploy)
- The evidence needs to feed a **gate** — a decision that turns into a CD pipeline exit code, a PR check, or a release-ticket field
- The decision should be **strategy-anchored** — the user has declared what the source of truth is (one of the 9 source-truth strategies), and Pilot is checking conformance against that
- The result needs to be **auditable** later — Pilot may want a fingerprinted receipt to attach to the release record

## Do not load for

- A **post-deploy** verification (release just shipped; did it land per Git?) — `pilot-release-verification` (batch B)
- A **real-time** event-driven response (Pilot subscribing to `cub-scout watch`) — [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md)
- A **multi-cluster periodic conformance audit** (not tied to one release event) — [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md)
- A **drift-classification** question after the gate already passed — [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md)
- **Postmortem** evidence assembly — [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md)
- Anything that **mutates** the cluster or ConfigHub. The gate decision is Pilot's; the mutation is `cub` / Argo / Flux's. cub-scout never mutates.

## The Pilot ↔ cub-scout loop

```
   release pipeline
        │
        ▼
   Pilot (acceptance judge)
        │  "should this deploy go?"
        │
        ▼
   cub-scout compare source-truth <subject> --strategy <s> --format json
        │
        │  ──────────────────────────────────────────────────────────
        │  status:       PASS | WATCH | BLOCK | ASK
        │  source_truth: AGREED | MISMATCH | INCOMPLETE | BLOCKED | UNKNOWN
        │  proof_gaps:   [] (string array of free-form missing-evidence entries)
        │  attribution:  cause, managerHint, gitSource per field
        │  ──────────────────────────────────────────────────────────
        │
        ▼
   (optional) cub-scout receipt verify <subject> --strategy <s>
                                --save --out release.receipt.json
        │
        │  fingerprinted in-toto Statement v1 artifact for the release record
        │
        ▼
   Pilot's verdict: PASS / WATCH / ASK / BLOCK
        │
        ▼
   release pipeline reads the verdict
        │  PASS  → continue
        │  WATCH → continue, attach the receipt to the release, alert
        │  ASK   → block, ask a human
        │  BLOCK → halt, alert, halt the promotion
        └──────────────────────────────────────────────────────────
```

## Strategy required upfront

cub-scout **never infers** the source-truth strategy. Pilot's gate must declare one of the 9 — pass it explicitly via `--strategy`:

| Strategy | What it asserts |
|----------|-----------------|
| `git-argo`           | Argo CD application reading from a Git source; cluster matches the git tree |
| `git-flux`           | Flux reading from a Git source; cluster matches the git tree |
| `helm-argo`          | Argo CD reading from a Helm chart source; cluster matches the rendered chart |
| `helm-flux`          | Flux HelmRelease; cluster matches the rendered chart |
| `kustomize-flux`     | Flux Kustomization; cluster matches the rendered kustomize tree |
| `oci-argo`           | Argo CD reading from an OCI registry; cluster matches the OCI image |
| `oci-flux`           | Flux reading from an OCI registry; cluster matches the OCI image |
| `confighub-oci-argo` | ConfigHub authoring + OCI publish + Argo CD apply; cluster matches the unit revision via OCI |
| `confighub-oci-flux` | ConfigHub authoring + OCI publish + Flux apply; cluster matches the unit revision via OCI |

This is the closed enum from `pkg/agent/source_truth.go`. Unknown values are rejected upfront. See [`source-truth-strategies`](../references/source-truth-strategies.md) for the full reference. If the user can't name a strategy, the gate is premature — Pilot should ask before evaluating.

## Two-tier evidence: `compare source-truth` vs. `receipt verify`

Pilot has two tiers of evidence available, and the choice is a quality / durability trade-off:

| Tier | Verb | Output | When to use |
|------|------|--------|-------------|
| **Live evidence** | `cub-scout compare source-truth <kind>/<name> -n <ns> --strategy <s> --format json` | Ephemeral JSON the pipeline reads once | Most gate decisions. Pilot reads `status` + `source_truth` + `proof_gaps` + `safe_next_action`, renders a verdict, and the JSON is discarded after the decision. |
| **Typed evidence** | `cub-scout receipt verify <kind>/<name> -n <ns> --strategy <s> --save --out release.receipt.json --fail-on any-non-pass` | Persistent in-toto Statement v1 envelope with SHA-256 fingerprint | When the release record needs an immutable receipt attached (audit, compliance, postmortem). `--fail-on any-non-pass` doubles as the exit-2 CI gate. |

The receipt path is **preferred for production gates** because:

1. The artifact is durable — the release record carries the evidence the gate decision was made on, fingerprint-verifiable later.
2. `--fail-on any-non-pass` gives Pilot a native CI exit code (0/2/1) without per-pipeline glue.
3. The receipt is preserved **even if the gate fires** — exit-2 happens AFTER stdout/`--out`/`--save`, so the failure case still produces evidence. (See `examples/receipts/ci-gate/README.md` for the full walkthrough.)
4. The receipt can be re-validated later: `cub-scout receipt validate release.receipt.json` exits 0 (OK) / 1 (mismatch) / 2 (I/O).

## Verdict mapping (cub-scout status → Pilot verdict)

The source-truth contract has **two** enums (snake_case in JSON, per `pkg/agent/source_truth.go:280-287`):

- **`status`** — evidence-quality verdict: `PASS` / `WATCH` / `BLOCK` / `ASK`
- **`source_truth`** — cross-surface agreement: `AGREED` / `MISMATCH` / `INCOMPLETE` / `BLOCKED` / `UNKNOWN`

Pilot's gate verdict typically tracks `status`, with `source_truth` available for drill-down:

| cub-scout `status` | cub-scout `source_truth` (typical) | Pilot gate verdict | Pipeline action |
|--------------------|-----------------------------------|--------------------|-----------------|
| PASS  | AGREED      | **PASS** | continue |
| WATCH | INCOMPLETE  | **WATCH** | continue but attach receipt; alert release channel |
| ASK   | UNKNOWN     | **ASK** | block; ask human (proof gaps the user can fill) |
| BLOCK | MISMATCH    | **BLOCK** | halt; alert |
| BLOCK | BLOCKED     | **BLOCK** | halt; the evidence itself was unreachable (cluster down, ConfigHub down) |

When wrapped in a receipt, `ASK` maps to receipt verdict `WATCH` per the locked synthesis in `docs/proposals/receipts-way-forward.md`. The receipt-level `INCONCLUSIVE` verdict is reserved for receipts that themselves couldn't be built (missing predicate inputs, e.g., source-truth-pass without `--strategy`); it does NOT appear in `compare source-truth`'s own `status`.

## Worked example

A release pipeline is gating on `payments-api@v1.4.3` going to `prod`. The team's declared strategy is `git-argo` (Argo CD reading from the org's Git repo).

```bash
# Step 1 — Pilot asks cub-scout for live evidence
$ cub-scout compare source-truth deploy/payments-api -n prod \
    --strategy git-argo \
    --format json
```

```json
{
  "declared_strategy": "git-argo",
  "status": "WATCH",
  "source_truth": "INCOMPLETE",
  "outlier": "none",
  "surfaces": {
    "confighub": null,
    "controller": {
      "kind": "Argo",
      "source": "https://github.com/org/payments",
      "revision_or_digest": "abc123",
      "health": "Synced"
    },
    "runtime": {
      "resource": "Deployment/payments-api in prod",
      "field": "spec.template.spec.containers[0].image",
      "value": "ghcr.io/org/payments-api:v1.4.2",
      "health": "Current"
    }
  },
  "proof_gaps": [
    "git-source-anchor unresolved: argocd CLI not installed; tracer fell back to label-only ownership"
  ],
  "safe_next_action": "install argocd CLI in the pipeline container and re-run, or accept WATCH if policy allows"
}
```

Pilot reads this:
- `status: WATCH` → not a hard PASS; gate fires WATCH (continue but flag).
- `proof_gaps[0]` (a free-form string entry) → the operator can fix the missing git-source-anchor by installing argocd CLI in the pipeline container.
- `surfaces.controller.health: Synced` → Argo is in control; no manual-edit signal at this layer.

Pilot's CD pipeline policy says WATCH is non-blocking but must attach a receipt to the release ticket. So Pilot escalates the call to:

```bash
# Step 2 — Pilot generates a typed receipt to attach to the release
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --save \
    --out release.receipt.json \
    --fail-on BLOCK
# exit 0 because verdict is WATCH (not in --fail-on BLOCK set)
# stdout shows the JSON; release.receipt.json is on disk; the local store has the artifact too
```

The release ticket gets the WATCH verdict + the `release.receipt.json` artifact attached. Later, an audit query can:

```bash
$ cub-scout receipt validate ./release.receipt.json
# exit 0 — fingerprint still matches
$ cub-scout receipt show ./release.receipt.json --format ascii
# renders the same evidence Pilot read
```

The receipt is **fingerprint-stable** — the same release.receipt.json can be archived for years and validated indefinitely (subject to the underlying evidence sources still existing for cross-checks).

## Standalone vs connected

Pilot's CD gate runs in two flavors:

| Flavor | Verbs available | Notes |
|--------|-----------------|-------|
| **Standalone** (cluster only, no ConfigHub auth) | `receipt verify --predicate applied-matches-spec`, `receipt verify --predicate no-manual-edits-since` | Limited — cannot use `source-truth-pass`. Useful for non-ConfigHub-managed deployments. |
| **Connected** (cluster + `cub auth login`) | All of the above + `receipt verify --strategy <s>` (source-truth-pass), `compare source-truth`, `compare three-way`, `confighub-unit://` subject in receipt | Full surface. Recommended for production gates. |

A standalone gate is still valuable. The receipt **omits** the `confighub-unit://` subject and records a documented `OmissionConfigHubUnitSubject` entry — that's structural honesty about what's missing, not a verdict downgrade. The verdict itself follows the predicate's normal rules: `applied-matches-spec` can still PASS in standalone mode when the controller-resolved git anchor and attribution are sufficient (per `pkg/agent/receipt_predicates.go:106-110`). Pilot may layer its own conservative policy on top (e.g., "treat standalone receipts as WATCH even on PASS verdicts") — that's a Pilot-side choice, not a cub-scout-imposed ceiling.

Run `cub-scout status` in the pipeline preamble to confirm the mode; if standalone and the gate requires a strategy, Pilot's policy should escalate to ASK rather than proceed.

## CI integration shapes

```yaml
# GitHub Actions shape — cub-scout produces evidence; Pilot's policy
# engine (whatever it is in your stack) reads it and renders a verdict.
# cub-scout side is the read-only piece; the verdict-rendering step is
# tool-neutral.
- name: cub-scout evidence
  run: |
    cub-scout compare source-truth deploy/payments-api -n prod \
      --strategy git-argo \
      --format json > evidence.json
- name: Pilot verdict
  id: verdict
  run: |
    # Pilot consumes evidence.json and writes one of PASS / WATCH / ASK / BLOCK.
    # The exact mechanism (CLI / library / HTTP call) is Pilot-side; cub-scout
    # only produces the evidence file. Substitute your acceptance kernel here.
    pilot_verdict=$(./scripts/pilot-decide evidence.json)
    echo "verdict=$pilot_verdict" >> $GITHUB_OUTPUT
- name: Halt on BLOCK or ASK
  if: steps.verdict.outputs.verdict == 'BLOCK' || steps.verdict.outputs.verdict == 'ASK'
  run: exit 1
```

```yaml
# Simpler: skip Pilot, use the receipt --fail-on path directly
- name: cub-scout receipt gate
  run: |
    cub-scout receipt verify deploy/payments-api -n prod \
      --strategy git-argo \
      --fail-on any-non-pass \
      --save \
      --out release.receipt.json
    # exit 0 → continue; exit 2 → gate fired (release.receipt.json is still on disk)
- name: Upload receipt (always)
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: release-receipt
    path: release.receipt.json
```

See [`examples/receipts/ci-gate/README.md`](../../examples/receipts/ci-gate/README.md) for the full GitHub Actions / GitLab CI / Jenkins walkthrough.

## Tool boundary

- **Allowed (cub-scout side):** `compare source-truth`, `compare three-way`, `receipt verify` / `show` / `validate`, `explain`, `trace`, `doctor`. All read-only.
- **Allowed (cluster side):** `kubectl get/describe`. **NOT** `kubectl apply/edit/patch/delete`.
- **Allowed (ConfigHub side, connected):** `cub * get / list`, `cub unit get`, `cub link list`. **NOT** any `cub * create / update / delete`.
- **Allowed (controller side):** `argocd app get`, `flux get`. **NOT** `argocd app sync`, `flux reconcile`.
- **Pilot itself may mutate** — but through `cub` / Argo / Flux / whatever, not through cub-scout. This skill's allowed-tools list is what cub-scout authorizes for Pilot's evidence-gathering calls.

## References

- [`scout-compare`](../scout-compare/SKILL.md) — the four Compare verbs (cub-scout side)
- [`scout-verify`](../scout-verify/SKILL.md) — the receipt verbs (cub-scout side)
- [`confighub-source-truth`](../confighub-source-truth/SKILL.md) — strategy-typed verdict workflow
- [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md) — real-time companion (event-driven, this skill is query-driven)
- [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md) — drift classification after the gate passes
- [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md) — evidence assembly when the gate didn't catch it
- Source-truth contract: `#393`, `#418` (Phase 2 strategies), `#409` (Phase 3 deferred)
- Receipts: `#446` (v1), `#463` (v2: `--fail-on`, chained, watch emit)
- Worked CI walkthrough: [`examples/receipts/ci-gate/README.md`](../../examples/receipts/ci-gate/README.md)

## Constraints

- Connected mode (`cub auth login`) is **required** for `--strategy source-truth-pass`. Standalone gates fall back to `applied-matches-spec` or `no-manual-edits-since`, which are narrower.
- The strategy is **caller-declared**, never inferred. If the team can't name one, the gate is premature.
- The gate decision is Pilot's, but cub-scout's `--fail-on` flag implements the same shape (exit 2 = non-PASS) for gateways that don't want a Pilot dependency.
- Receipts are **immutable** — once persisted to the store, `SaveStatement` uses `O_EXCL` and refuses to overwrite. Re-running the gate produces a new receipt with a new fingerprint.
- The receipt-level verdict `INCONCLUSIVE` is distinct from `compare source-truth`'s `status: ASK`. INCONCLUSIVE means the receipt itself couldn't be built; ASK means the receipt was built but the evidence quality couldn't reach PASS.
