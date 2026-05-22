---
name: pilot-promotion-gate
description: 'Use when Pilot is GATING A VARIANT-TO-VARIANT PROMOTION between two intentional configurations (e.g., staging → prod; canary 5%% → 50%%; blue → green; cell-A → cell-B). Natural phrasings: "Pilot, gate the promotion from staging to prod", "is variant B safe to promote from variant A?", "render the cross-variant safety verdict", "what would change between cells if we promote?", "promotion gate for the canary rollout". Pilot consumes cub-scout''s `compare three-way` per variant + the per-variant `bindingSource` graph + `compare source-truth --strategy <s>` per variant to render a CROSS-VARIANT verdict — not a pre-deploy gate against a single resource (that''s pilot-cd-gate). Pilot picks promotion safety + the field-level diff reasons; the actual promotion execution lives in Pilot''s mutation path. Do NOT load for: a single-resource pre-deploy gate (use pilot-cd-gate), a rollback decision (use pilot-rollback-decision), a real-time event response (use pilot-watch-alert-response), or a post-deploy validation (use pilot-release-verification).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout impact *) Bash(cub-scout impact *) Bash(cub scout impact *) Bash(./cub-scout views resolve *) Bash(cub-scout views resolve *) Bash(cub scout views resolve *) Bash(./cub-scout views project *) Bash(cub-scout views project *) Bash(cub scout views project *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub link list *) Bash(cub unit get *) Bash(argocd app get *) Bash(flux get *)

---

# pilot-promotion-gate

The variant-to-variant promotion-safety scenario from **Pilot's** side. A pipeline (or a human) asks: *is variant B safe to promote from variant A?* — where A and B are two intentional configurations (staging → prod; canary 5% → 50%; blue → green; cell-A → cell-B). Pilot consumes cub-scout's evidence from **both** variants and renders a cross-variant safety verdict.

This is **not** `pilot-cd-gate` — that's the single-resource pre-deploy gate. Promotion is the *cross-variant safety check*: variant A has been running successfully; variant B is the proposed next state; the diff between them needs structured evidence before promotion fires.

## When to use

Explicit phrasings:

- "Pilot, gate the promotion from staging to prod"
- "Is variant B safe to promote from variant A?"
- "Render the cross-variant safety verdict"
- "What would change between cells if we promote?"
- "Promotion gate for the canary rollout"
- "Pilot, decide on the 5% → 50% canary increase"
- "Show me the field-level diff between blue and green"

Implicit intents:

- A **promotion is being staged** (not a fresh deploy and not a rollback) — two variants are both real and Pilot is picking whether to advance
- The decision is **cross-variant** — comparing variant A's evidence to variant B's, not just evaluating B alone
- The result feeds a **promotion ticket / release gate** — the team needs structured evidence the promotion is safe, with field-level diff reasons
- The actual promotion (Argo PromotionStrategy sync, Flux progressive rollout, ConfigHub variant flip) is Pilot's mutation path

## Do not load for

- A **single-resource pre-deploy gate** (no prior variant to compare against) — [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md)
- A **rollback decision** (backward direction; this skill is forward-direction) — [`pilot-rollback-decision`](../pilot-rollback-decision/SKILL.md)
- A **post-deploy validation** after the promotion already fired — [`pilot-release-verification`](../pilot-release-verification/SKILL.md)
- A **real-time event-driven** drift response on either variant — [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md)
- A **fleet-wide conformance audit** across many variants — [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md)
- A **compliance audit** across policies (not a variant decision) — [`pilot-compliance-audit`](../pilot-compliance-audit/SKILL.md)
- Any **mutating action**. The promotion execution is Pilot's; cub-scout doesn't promote.

## The Pilot ↔ cub-scout promotion loop

```
   release pipeline: variant A is running; variant B is the proposed promotion
        │
        ▼
   Pilot (acceptance judge)
        │  "is B safe to promote from A?"
        │
        ├──► For variant A (the current state):
        │      cub-scout compare three-way <r> -n <ns-A> --format json
        │      cub-scout compare source-truth <r> -n <ns-A> --strategy <s> --format json
        │      cub-scout receipt verify <r> -n <ns-A> --strategy <s> \
        │        --out variant-A.receipt.json
        │
        ├──► For variant B (the proposed state):
        │      cub-scout compare three-way <r> -n <ns-B> --format json
        │      cub-scout compare source-truth <r> -n <ns-B> --strategy <s> --format json
        │      cub-scout receipt verify <r> -n <ns-B> --strategy <s> \
        │        --out variant-B.receipt.json
        │
        ├──► Cross-variant binding diff:
        │      cub-scout compare three-way <r> -n <ns-A> --format json \
        │        | jq '.fieldMismatches[].bindingSource'
        │      cub-scout compare three-way <r> -n <ns-B> --format json \
        │        | jq '.fieldMismatches[].bindingSource'
        │      # Pilot compares the bindingSource graphs field-by-field
        │
        └──► Cross-variant chained receipt:
              cub-scout receipt verify <r> -n <ns-B> --strategy <s> \
                --input-attestation variant-A.receipt.json \
                --save \
                --out promotion-target.receipt.json
        │
        ▼
   Pilot's cross-variant verdict:
        │  A=PASS AND B=PASS AND diff is clean    → PROMOTION PASS
        │  A=PASS AND B=PASS but bindingSource    → PROMOTION WATCH (caveats)
        │   shifts in unexpected ways
        │  A=PASS AND B=WATCH                     → PROMOTION ASK (B isn't ready)
        │  A=PASS AND B=BLOCK                     → PROMOTION BLOCK (don't promote)
        │  A=BLOCK                                → PROMOTION BLOCK (fix A first)
        │
        ▼
   release pipeline reads the verdict
        │  PASS  → fire Pilot's promotion mutation path
        │  WATCH → fire with caveats attached to the release ticket
        │  ASK   → halt; the on-call decides
        │  BLOCK → halt; alert; do not promote
        └────────────────────────────────────────────────────
```

## What "variant" means here

This skill handles three common variant shapes:

| Variant shape | Variant A | Variant B | cub-scout reads |
|---------------|-----------|-----------|------------------|
| **Environment promotion** | `deploy/api -n staging` | `deploy/api -n prod` | Same kind/name, different namespaces |
| **Canary progression** | `deploy/api -n prod` (HPA limit 5%) | `deploy/api -n prod` (HPA limit 50%) | Same resource, different Argo-Rollouts step |
| **Blue-green flip** | `deploy/api-blue -n prod` | `deploy/api-green -n prod` | Different names, same role; Pilot compares both against the shared upstream Unit |
| **Cell rollout** | `deploy/api -n prod-use1` | `deploy/api -n prod-euw1` | Same resource in different cluster cells |

For each shape, Pilot's cross-variant call is structurally the same: read both variants' three-way + source-truth + bindingSource graphs; compute the diff; render a verdict. cub-scout doesn't model variants as a first-class concept — it reads the kind/name/namespace pair Pilot passes. The variant pairing is Pilot's job.

## Why `bindingSource` matters for promotion

The `bindingSource` per field (connected-mode only; from `#435` C2) records *which ConfigHub Link supplied this field's value*. Cross-variant safety often hinges on whether the same upstream Link feeds both variants:

- **A and B reference the same upstream Unit at the same path** → safe; the promotion is just propagating an already-canonical value
- **A and B reference different upstream Units** → caveat; the promotion is *also* a Link-graph shift, which may surprise the operator
- **A has a bindingSource but B has `null`** → ASK; B might be unmanaged or the Link wasn't promoted alongside

This is the load-bearing reason promotion-gate is distinct from `pilot-cd-gate`: a single-variant gate doesn't see the cross-variant Link-graph shift.

## Step-by-step

### Step 1 — per-variant evidence

```bash
# Variant A (current)
$ cub-scout compare three-way deploy/api -n staging --format json > variant-A-3way.json
$ cub-scout compare source-truth deploy/api -n staging --strategy git-argo --format json > variant-A-st.json
$ cub-scout receipt verify deploy/api -n staging --strategy git-argo \
    --save \
    --out variant-A.receipt.json

# Variant B (proposed)
$ cub-scout compare three-way deploy/api -n prod --format json > variant-B-3way.json
$ cub-scout compare source-truth deploy/api -n prod --strategy git-argo --format json > variant-B-st.json
$ cub-scout receipt verify deploy/api -n prod --strategy git-argo \
    --save \
    --out variant-B.receipt.json
```

### Step 2 — extract field-level diff between variants

```bash
# Pilot's cross-variant diff — fieldMismatches by path
$ jq -s '
  {
    A_fields: .[0].fieldMismatches | map({path, value: .compareThreeWay.live, bindingSource}),
    B_fields: .[1].fieldMismatches | map({path, value: .compareThreeWay.live, bindingSource})
  }
' variant-A-3way.json variant-B-3way.json
```

Pilot reads:
- Fields present in A but not B (deleted)
- Fields present in B but not A (new)
- Fields in both with different values (changed)
- Fields where the `bindingSource` shifted (cross-variant Link-graph drift)

The last category is the most important and the hardest to surface — only the cross-variant comparison can see it.

### Step 3 — chained receipt records the cross-variant relationship

```bash
$ cub-scout receipt verify deploy/api -n prod \
    --strategy git-argo \
    --input-attestation variant-A.receipt.json \
    --save \
    --out promotion-target.receipt.json
```

The promotion-target receipt's `inputAttestations[]` records variant A's receipt — the audit trail says "promotion B was gated against the prior variant A's evidence." If A's receipt is later tampered with, the chain construction (`VerifiedAttestationRef` API-boundary verify) refuses.

### Step 4 — Pilot's verdict synthesis

| Variant A verdict | Variant B verdict | bindingSource diff | Pilot promotion verdict |
|--------------------|--------------------|--------------------|-------------------------|
| PASS | PASS | none | **PASS** |
| PASS | PASS | shifted but explicable | **WATCH** |
| PASS | PASS | shifted without rationale | **ASK** (human decides) |
| PASS | WATCH | — | **ASK** (B isn't ready) |
| PASS | BLOCK | — | **BLOCK** |
| PASS | INCONCLUSIVE | — | **ASK** (B can't be evaluated) |
| WATCH / BLOCK | any | — | **BLOCK** (fix A first; don't promote a sick variant) |
| INCONCLUSIVE | any | — | **ASK** (A's evidence isn't reliable for the gate) |

The decision is **cross-variant** — both variants' verdicts and the diff between them all feed the promotion verdict. cub-scout doesn't synthesize this; Pilot does.

## Worked example

Promotion: `deploy/payments-api` from `staging` to `prod`. Declared strategy: `git-argo`.

```bash
# Per-variant receipts
$ cub-scout receipt verify deploy/payments-api -n staging \
    --strategy git-argo --save --out staging.receipt.json
# verdict: PASS

$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo --save --out prod.receipt.json
# verdict: WATCH; proof_gaps lists git-source-anchor missing for one container

# Cross-variant chained receipt
$ cub-scout receipt verify deploy/payments-api -n prod \
    --strategy git-argo \
    --input-attestation staging.receipt.json \
    --save \
    --out promotion-target.receipt.json
```

Pilot's verdict synthesis:
- staging.verdict = PASS
- prod.verdict = WATCH (a per-container source-anchor gap)
- bindingSource diff: identical (same upstream Unit in both variants)
- → Pilot promotion verdict: **ASK** (B isn't fully ready; missing source-anchor on one container)

Pilot's verdict pack:

```json
{
  "decision_type": "promotion",
  "from_variant": "staging",
  "to_variant": "prod",
  "from_receipt": "staging.receipt.json",
  "to_receipt": "prod.receipt.json",
  "chained_target": "promotion-target.receipt.json",
  "from_verdict": "PASS",
  "to_verdict": "WATCH",
  "binding_source_diff": "none",
  "promotion_verdict": "ASK",
  "rationale": "B (prod) has a WATCH verdict with proof_gaps; on-call should decide whether to proceed",
  "next_step": "route to on-call; promotion mutation lives in Pilot's stack, not cub-scout"
}
```

## Standalone vs connected

**Connected mode strongly recommended** for promotion-gate:

- `compare source-truth --strategy` — connected (per-variant strategy verdict)
- `bindingSource` field on each mismatch — connected (Link-graph data)
- `impact` for blast-radius — connected

Standalone mode degrades:
- Receipt verdicts can still be produced via `--predicate applied-matches-spec`
- bindingSource is null per field; cross-variant Link-graph diff isn't available
- The promotion-gate result is narrower; Pilot's policy should escalate to ASK when bindingSource isn't readable.

## Tool boundary

- **Allowed (cub-scout):** Compare verbs (read-only), `impact`, `views resolve` / `project`, `receipt verify` / `show` / `validate` / `list`, `explain`, `trace`. All read-only.
- **Allowed (cluster):** `kubectl get/describe`. NOT `kubectl apply / edit / patch / delete`.
- **Allowed (ConfigHub):** `cub * get / list`, `cub unit get`, `cub link list`. NOT any mutating `cub` verb.
- **Allowed (controllers):** `argocd app get`, `flux get`. NOT `argocd app sync`, `flux reconcile`.
- **Pilot's promotion execution** (Argo PromotionStrategy, Flux progressive rollout, ConfigHub variant flip, blue-green service swap) is Pilot's mutation path, deliberately not named here.

## References

- [`scout-compare`](../scout-compare/SKILL.md) — Compare verbs (cub-scout side)
- [`scout-attribute`](../scout-attribute/SKILL.md) — `bindingSource` semantics
- [`scout-verify`](../scout-verify/SKILL.md) — chained receipts via `--input-attestation`
- [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md) — single-resource pre-deploy companion
- [`pilot-rollback-decision`](../pilot-rollback-decision/SKILL.md) — backward direction
- [`pilot-release-verification`](../pilot-release-verification/SKILL.md) — post-promotion validation
- [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md) — fleet-wide variant audit (related but different framing)
- Attribution layer: `#435` (C1+C2 `bindingSource`)
- Chained receipts: `#448` (parent), `#463` (chained-half shipped)

## Constraints

- cub-scout doesn't model variants as a first-class concept. The variant pairing (which two resources are being compared) is **Pilot's job**; cub-scout reads the kind/name/namespace pairs Pilot passes.
- `bindingSource` requires connected mode AND both variants being ConfigHub-linked. Variants without Link coverage produce `null` bindingSource — the cross-variant Link-graph diff isn't surfaced. Pilot should escalate to ASK in that case.
- The promotion-target receipt's `inputAttestations[]` typically references variant A's receipt. If multi-step promotion (A → A' → B), Pilot may chain multiple prior receipts. The chain ordering is **Pilot's choice** — cub-scout treats `inputAttestations[]` as a set (RFC 8785 canonical JSON orders the array, but the meaning is set-shaped).
- Promotion mutation (the actual flip from A to B) is Pilot's mutation path. cub-scout's role ends at the promotion-target receipt; how Pilot fires the controller sync / variant flip is outside this skill.
