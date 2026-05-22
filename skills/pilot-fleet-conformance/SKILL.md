---
name: pilot-fleet-conformance
description: 'Use when Pilot (or another acceptance-judge tool) is rendering a FLEET-WIDE CONFORMANCE VERDICT across multiple clusters / namespaces / a ConfigHub View. Natural phrasings: "Pilot, give me the fleet verdict for prod", "is the payments fleet conformant against `git-argo`?", "render the conformance report Pilot can attach to the compliance ticket", "what does Pilot say across all 12 clusters?", "per-cluster PASS/WATCH/ASK/BLOCK rolled up to a fleet verdict". Pilot consumes cub-scout''s `compare three-way --view` + per-resource `compare source-truth --strategy` + `fleet outliers --view`, optionally persisting per-resource receipts via `receipt verify --save`, then synthesizes a fleet-level verdict. Differs from `audit-fleet-conformance` which is the OPERATOR-DRIVEN audit loop; this skill is the JUDGE-DRIVEN companion (Pilot reading the same surfaces but framing the output as a verdict, not a checklist). Do NOT load for: a single-resource pre-deploy gate (use pilot-cd-gate), real-time event-driven response (use pilot-watch-alert-response), or any mutating action.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout fleet outliers *) Bash(cub-scout fleet outliers *) Bash(cub scout fleet outliers *) Bash(./cub-scout views resolve *) Bash(cub-scout views resolve *) Bash(cub scout views resolve *) Bash(./cub-scout views project *) Bash(cub-scout views project *) Bash(cub scout views project *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout scan *) Bash(cub-scout scan *) Bash(cub scout scan *) Bash(./cub-scout summary *) Bash(cub-scout summary *) Bash(cub scout summary *) Bash(kubectl get *) Bash(cub * get) Bash(cub * list) Bash(cub view get *) Bash(cub view list)

---

# pilot-fleet-conformance

The fleet-wide conformance scenario from **Pilot's** side. Pilot is asked for a verdict across a fleet — multiple clusters, multiple namespaces, or a ConfigHub View scope — and synthesizes a single fleet-level PASS / WATCH / ASK / BLOCK from per-resource cub-scout evidence.

This is the **judge-driven** companion to [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md) (the operator-driven audit loop). Same cub-scout verbs feed both; the difference is framing: an operator runs an audit to find what's wrong; Pilot renders a verdict that downstream consumers (compliance dashboards, governance tickets, release boards) act on.

## When to use

Explicit phrasings:

- "Pilot, give me the fleet verdict for prod"
- "Is the payments fleet conformant against `git-argo`?"
- "Render the conformance report Pilot can attach to the compliance ticket"
- "What does Pilot say across all 12 clusters?"
- "Per-cluster PASS/WATCH/ASK/BLOCK rolled up to a fleet verdict"
- "Fleet conformance check on Hub View `monthly-spend-by-team`"
- "Which clusters are out of conformance with their declared strategy?"

Implicit intents:

- The user is **operating across many resources** — namespace, View, or full fleet
- The output needs to be a **single verdict** with structured drill-down, not a per-resource checklist
- The judgment is **strategy-anchored** — the user has declared what the source of truth should be
- The result feeds a **dashboard / ticket / compliance flow**, not an interactive triage

## Do not load for

- A **single-resource pre-deploy gate** — [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md)
- An **operator-driven** audit checklist (same verbs, different framing) — [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md)
- A **real-time event-driven** fleet response (Pilot subscribes to a watch stream) — [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md)
- A **postmortem evidence assembly** — [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md)
- A **drift classification** per resource — [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md)
- Anything that **mutates** the fleet. Pilot's mutation path is `cub` / Argo / Flux, never cub-scout.

## The Pilot ↔ cub-scout fleet loop

```
   compliance dashboard / release board / governance ticket
        │
        ▼
   Pilot (acceptance judge)
        │  "render the fleet verdict for view:monthly-spend-by-team"
        │
        ├──► cub-scout views resolve <url-or-uuid>          # confirm scope
        │
        ├──► cub-scout fleet outliers --view <uuid>         # cross-cluster outliers
        │
        ├──► cub-scout compare three-way --view <uuid>      # per-resource DRY/WET/LIVE
        │     --format json
        │
        ├──► for each resource in scope:
        │      cub-scout compare source-truth <r> --strategy <s>
        │     --format json                                  # per-resource verdict
        │
        └──► (optional, for audit attachment)
              for each resource in scope:
                cub-scout receipt verify <r> --strategy <s>
                  --save                                    # fingerprinted artifact
              cub-scout receipt list --format json          # the artifact inventory
        │
        ▼
   Pilot's fleet verdict = max-severity over per-resource verdicts
        │
        ▼
   downstream consumer reads the verdict
        │  PASS  → conformant fleet, no action needed
        │  WATCH → conformant with caveats; attach receipts; review at next sync
        │  ASK   → human-needed; the per-resource ASKs surface proof gaps
        │  BLOCK → halt promotion / release; quarantine the divergent resources
        └────────────────────────────────────────────────────
```

## Scope sources

Pilot's fleet verdict needs a defined scope. cub-scout exposes three scope shapes:

| Scope | Verb path | Use when |
|-------|-----------|----------|
| **View** | `views resolve <url-or-uuid>` → `--view <uuid>` on the audit verbs | The team declared the scope in ConfigHub (preferred for governance) |
| **Namespace** | `--scope namespace/<ns>` | One namespace's workloads |
| **Cluster** | `--scope cluster` | Entire kubeconfig context |
| **Multi-cluster** | `fleet outliers --view <uuid>` | Cross-cluster comparison against a View baseline (connected mode only) |

Views are declared via `cub view create` (out of cub-scout's scope — `cub` operation). cub-scout reads them via `views resolve`. If the team doesn't have a View, Pilot can still render a fleet verdict over a namespace or cluster scope; the scope is just less semantically anchored.

## Verdict synthesis (per-resource → fleet)

Pilot's synthesis policy is **max-severity** by default:

| Per-resource verdicts | Fleet verdict |
|-----------------------|---------------|
| All PASS | **PASS** |
| Some WATCH, rest PASS | **WATCH** |
| Some ASK, rest PASS/WATCH | **ASK** (humans needed for the proof gaps) |
| Any BLOCK | **BLOCK** (one divergent resource halts the fleet verdict) |

`fleet outliers` adds a cross-cluster dimension: if all per-cluster resource counts match the View baseline, the fleet is **outlier-free**; if one cluster diverges, Pilot's verdict downgrades to at least WATCH and may escalate to BLOCK depending on the missing resource's role.

This skill does NOT prescribe the synthesis policy beyond max-severity — Pilot may want a different aggregation (majority, weighted by criticality). The cub-scout side is verdict-producing per resource; the fleet roll-up is Pilot's call.

## Step-by-step

### Step 1 — resolve the scope

```bash
$ cub-scout views resolve https://hub.confighub.com/views/payments-prod-conformance
View: payments-prod-conformance
  scope: cluster
  columns: [team, service, replicas, source_truth_strategy]
  resolved at: 2026-05-22T10:30:00Z
```

If no View applies, fall back to a namespace scope; the rest of the loop is identical except `--scope namespace/<ns>` replaces `--view <uuid>`.

### Step 2 — outliers (true fleet shape)

```bash
$ cub-scout fleet outliers --view payments-prod-conformance
View: payments-prod-conformance
Clusters: 12

  payments-api   EXPECTED 3 deploys / cluster   prod-euw1: 2 (outlier: missing payments-api-worker)
  payments-db    EXPECTED 1 deploy / cluster    ALL match
  payments-cron  EXPECTED 4 deploys / cluster   staging-use2: 3 (outlier: missing payments-cron-billing)
```

`fleet outliers` emits per-row mismatches. Each outlier is at minimum a **WATCH** for Pilot's fleet verdict (the per-cluster resource count doesn't match the View baseline); it escalates to **BLOCK** if the missing resource is in the View's critical-set (a separate policy Pilot enforces).

### Step 3 — per-resource three-way + source-truth

```bash
# Three-way conformance across the View (DRY/WET/LIVE per resource)
$ cub-scout compare three-way --view payments-prod-conformance --format json \
    | jq '.summary'
{
  "agreement": "partial",
  "agreed": 18,
  "converging": 1,
  "diverged": 3,
  "scope": "view:payments-prod-conformance"
}

# Strategy-anchored source-truth per resource
$ for r in $(cub-scout views project --view payments-prod-conformance --format json \
              | jq -r '.[] | "\(.kind)/\(.name) -n \(.namespace)"'); do
    cub-scout compare source-truth $r --strategy git-argo --format json \
      | jq '{resource: "'"$r"'", status: .status, source_truth: .source_truth, proof_gaps: .proof_gaps}'
  done
```

Each per-resource output has `status` (PASS/WATCH/BLOCK/ASK) + `source_truth` (AGREED/MISMATCH/INCOMPLETE/BLOCKED/UNKNOWN) + `proof_gaps[]` (a string array of free-form missing-evidence entries) + `safe_next_action` (a free-form human-readable next step). Pilot collects these into the synthesis matrix. Field names are snake_case per `pkg/agent/source_truth.go:280-287`.

### Step 4 — persist fleet evidence as receipts (audit-quality)

```bash
# Per-resource receipts, saved to the immutable local store
$ for r in $(cub-scout views project --view payments-prod-conformance --format json \
              | jq -r '.[] | "\(.kind)/\(.name) -n \(.namespace)"'); do
    cub-scout receipt verify $r --strategy git-argo --save
  done

# Audit inventory: every fleet receipt with verdict BLOCK or WATCH
$ cub-scout receipt list --format json \
    | jq '[.[] | select(.verdict == "BLOCK" or .verdict == "WATCH")]'
```

The receipt store is **immutable** (each filename is canonical and `O_EXCL`-protected; duplicate saves return `os.ErrExist` and never overwrite). The fleet audit produces a fingerprint-stable evidence pack that can be archived for compliance review.

## Worked example

Pilot is asked for the daily fleet verdict on View `payments-prod-conformance`, strategy `git-argo`:

```bash
$ cub-scout fleet outliers --view payments-prod-conformance
View: payments-prod-conformance
Clusters: 12

  payments-api   EXPECTED 3 deploys / cluster   prod-euw1: 2 (outlier)
  payments-cron  EXPECTED 4 deploys / cluster   staging-use2: 3 (outlier)

$ cub-scout compare three-way --view payments-prod-conformance --format json \
    | jq '.summary'
{
  "agreement": "partial",
  "agreed": 18,
  "converging": 1,
  "diverged": 3
}

$ # per-resource source-truth (abbreviated)
$ ... 18 PASS, 1 WATCH, 3 BLOCK
```

Pilot's synthesis:
- 22 resources in scope
- 18 PASS + 1 WATCH + 3 BLOCK per source-truth
- 2 cluster-level outliers (counted as WATCH)
- Max-severity verdict → **BLOCK**

Pilot's verdict surface to the compliance dashboard:

```json
{
  "fleet": "payments-prod",
  "view": "payments-prod-conformance",
  "strategy": "git-argo",
  "verdict": "BLOCK",
  "rollup": {
    "PASS": 18,
    "WATCH": 1,
    "ASK": 0,
    "BLOCK": 3,
    "outliers": 2
  },
  "blockers": [
    "Deployment/payments-api in prod-use1 (source_truth=MISMATCH)",
    "Deployment/payments-worker in prod-use1 (source_truth=MISMATCH)",
    "Deployment/payments-db in prod-euw1 (source_truth=MISMATCH)"
  ],
  "evidence_path": "/path/to/audit/receipts/2026-05-22/",
  "next_step": "investigate the 3 BLOCK resources via pilot-patch-and-drift"
}
```

Downstream consumer (compliance dashboard) reads the verdict, halts promotion, escalates to the team with the per-resource breakdown.

## Standalone vs connected

Most of this skill **requires connected mode** (`cub auth login`):

- `compare three-way` (DRY layer needs ConfigHub) — connected
- `compare source-truth` — connected (the contract is meaningless without all 3 surfaces)
- `views resolve` / `views project` — connected
- `fleet outliers` — connected (cross-cluster snapshots live in ConfigHub)
- `receipt verify --strategy <s>` — connected (source-truth-pass predicate requires it)

Standalone-mode fleet conformance is narrower:
- Per-resource `compare drift` against files (no DRY layer)
- `receipt verify --predicate applied-matches-spec` (cluster-only)
- No View scope; falls back to namespace / cluster scope

Pilot should run `cub-scout status` in the pipeline preamble; if standalone and the fleet verdict requires `--strategy`, escalate to ASK with a "connected mode required" message.

## CI / dashboard integration

For periodic fleet audits, the verbs are CI-shaped:

```bash
# Daily cron — render the fleet verdict, exit non-zero if BLOCK
$ cub-scout compare three-way --view payments-prod-conformance --fail-on warning
# exit 2 if any resource in the View has a hard mismatch
```

For receipts-attached audits (preferred for compliance):

```bash
# Per-resource receipt save + collect into audit branch
$ ./fleet-audit.sh > audit-results.txt
$ cub-scout receipt list --format json > audit-receipts.json
$ git add audit-receipts.json && git commit -m "fleet audit $(date -u +%F)"
```

Pilot's typical batch shape: run per-resource `receipt verify --strategy <s> --save` overnight; in the morning, walk `cub-scout receipt list --format json` for the verdict roll-up.

## Tool boundary

- **Allowed (cub-scout):** Compare verbs; `fleet outliers`; `views resolve` / `project`; `receipt verify` / `show` / `validate` / `list`; `scan`; `map`; `summary list`. All read-only.
- **Allowed (cluster):** `kubectl get`. NOT `kubectl apply/edit/patch/delete`.
- **Allowed (ConfigHub):** `cub * get / list`, `cub view get / list`. NOT `cub * create / update / delete / sync`.
- **Pilot itself may mutate** via `cub` or controller verbs, but the fleet-conformance verdict is Pilot's evidence-driven judgment; cub-scout never decides on Pilot's behalf.

## References

- [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md) — operator-side counterpart (same verbs, different framing)
- [`scout-compare`](../scout-compare/SKILL.md) — the four Compare verbs (cub-scout side)
- [`scout-govern`](../scout-govern/SKILL.md) — fleet outliers, views, summary
- [`scout-verify`](../scout-verify/SKILL.md) — receipt persistence
- [`confighub-source-truth`](../confighub-source-truth/SKILL.md) — strategy-typed verdict
- [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md) — single-resource pre-deploy gate companion
- [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md) — drill-down for the BLOCK resources Pilot flags
- [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md) — event-driven companion (this skill is periodic-driven)
- Views integration: `#391` (parent), `#414` (compare three-way --view), `#403` (views resolve), `#420` (views project --with-reality)
- Source-truth contract: `#393`, `#418` (Phase 2 strategies)

## Constraints

- Connected mode is required for the View + ConfigHub + fleet surfaces.
- The strategy is **caller-declared** per resource (or uniform across the View when the team declares fleet-wide policy). cub-scout never infers it.
- Synthesis policy (max-severity, majority, weighted, etc.) is **Pilot's** choice. cub-scout provides per-resource verdicts; the roll-up shape is Pilot-side.
- `fleet outliers` requires the View to be **declared with cluster scope** (`cub view create --scope cluster`). Namespace-scoped Views can't be cross-clustered.
- Per-resource receipts in a fleet audit can be high-volume. The local receipt store handles large-N writes (each filename is canonical so collisions are predictable), but downstream consumers should expect `receipt list` to return many entries.
