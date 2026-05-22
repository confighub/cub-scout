---
name: audit-fleet-conformance
description: 'Use when the user wants to verify CONFORMANCE ACROSS A FLEET — multiple namespaces, multiple clusters, or a defined View scope. Natural phrasing: "which clusters are conformant?", "audit fleet conformance against this View", "show me all outliers in prod fleet", "compare every service against its source-truth strategy", "scope-wide three-way check for drift", "fleet-level compliance audit", "which deployments drift across the cluster?". Composes `compare three-way --view` + `compare source-truth` + `fleet outliers` for multi-resource / multi-cluster checks. Requires connected mode for the View + ConfigHub surfaces. Do NOT load for: a single resource (use investigate-drift), a triage (use triage-unhealthy-workload), incident evidence for one event (use operator-incident-evidence), or anything mutating.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout fleet outliers *) Bash(cub-scout fleet outliers *) Bash(cub scout fleet outliers *) Bash(./cub-scout views resolve *) Bash(cub-scout views resolve *) Bash(cub scout views resolve *) Bash(./cub-scout views project *) Bash(cub-scout views project *) Bash(cub scout views project *) Bash(./cub-scout scan *) Bash(cub-scout scan *) Bash(cub scout scan *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout summary *) Bash(cub-scout summary *) Bash(cub scout summary *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(kubectl get *) Bash(cub * get) Bash(cub * list) Bash(cub unit list *) Bash(cub view get *) Bash(cub view list)
---

# audit-fleet-conformance

The fleet-wide conformance loop. Multiple resources, possibly multiple clusters, need a structured "are these in agreement with their declared source of truth?" verdict. Composes `compare three-way --view` + `compare source-truth --strategy` + `fleet outliers` (+ optionally `receipt verify --save` for evidence persistence).

Requires **connected mode** (`cub auth login`) for the View, source-truth, and fleet surfaces.

## When to use

Explicit phrasings:

- "Which clusters are conformant against View `monthly-spend-by-team`?"
- "Audit fleet conformance against this Hub View"
- "Show me all outliers in the prod fleet"
- "Compare every service against its declared source-truth strategy"
- "Scope-wide three-way check for drift across `namespace/payments`"
- "Fleet-level compliance audit"
- "Which deployments drift across the cluster?"

Implicit intents:

- The user is doing **compliance** / **governance**, not real-time response
- The scope is **multi-resource** (namespace, View, scope=cluster) — not a single workload
- The user wants a **per-resource verdict** with rolled-up statistics
- The result may feed Pilot acceptance, a release-gate dashboard, or a periodic audit log

## Do not load for

- A single resource investigation — [`investigate-drift`](../investigate-drift/SKILL.md)
- A time-pressed triage — [`triage-unhealthy-workload`](../triage-unhealthy-workload/SKILL.md)
- One-off incident evidence — [`operator-incident-evidence`](../operator-incident-evidence/SKILL.md)
- Per-strategy source-truth verdict workflow specifically — [`confighub-source-truth`](../confighub-source-truth/SKILL.md) zooms in on that
- Any mutating remediation. cub-scout never mutates.

## The loop

1. **Define the scope.** A View (`--view <uuid-or-url>`), a namespace (`--scope namespace/<ns>`), a cluster (`--scope cluster`), or the whole fleet via `fleet outliers`.
2. **Pick the verb.** `compare three-way --view` for DRY/WET/LIVE conformance per resource. `compare source-truth --strategy <s>` for strategy-typed verdict per resource. `fleet outliers --view` for cross-cluster comparison against a View baseline.
3. **Roll up the verdict.** `--format json` for machine-readable per-resource verdicts; ASCII for human review. The `summary.agreement` field on three-way output (`agreed` / `converging` / `diverged` / `partial`) is the headline.
4. **Persist the audit** (optional). `receipt verify <kind>/<name> --strategy <s> --save` per resource if the audit needs fingerprinted evidence. Useful for periodic compliance reports.

## Step-by-step

### Step 1 — define the scope

A **View** is the canonical scope for fleet-wide audits. Views are declared in ConfigHub (`cub view ...`) and resolved by cub-scout from a UUID or a hub.confighub.com URL:

```bash
$ cub-scout views resolve https://hub.confighub.com/views/monthly-spend-by-team
View: monthly-spend-by-team
  scope: cluster
  columns: [team, service, replicas, monthly_cost_estimate]
  resolved at: 2026-05-22T10:30:00Z
```

If no View applies, fall back to a namespace or cluster scope:

```bash
$ cub-scout compare three-way --scope namespace/payments --format json | jq '.summary.agreement'
"diverged"
```

### Step 2 — three-way conformance

```bash
$ cub-scout compare three-way --view monthly-spend-by-team --format json
```

The output structures per-resource agreement (DRY/WET/LIVE) + rolls up to a scope-level `summary.agreement` field with four states:

| State | Meaning |
|-------|---------|
| `agreed` | All resources in scope have DRY=WET=LIVE for every field. The fleet matches its intent. |
| `converging` | Most resources agree; a small subset is in active reconciliation (controller-drift causes). The fleet is recovering. |
| `diverged` | At least one resource has a hard mismatch with no controller-drift cause. Investigate. |
| `partial` | Mixed; the rollup couldn't pick a single label. Read the per-resource detail. |

Per-resource `fieldMismatches[]` carries the same attribution evidence as the single-resource case (see [`investigate-drift`](../investigate-drift/SKILL.md)).

### Step 3 — strategy-typed source-truth across the fleet

When the audit is **strategy-anchored** (e.g., "all of `payments` should be Argo-reading-from-Git"), iterate `compare source-truth` per resource with the declared strategy:

```bash
$ for d in $(cub-scout map workloads --namespace payments --format json | jq -r '.[] | "\(.kind)/\(.name)"'); do
    cub-scout compare source-truth "$d" -n payments --strategy git-argo --format json | jq '{name: "'"$d"'", status: .status, verdict: .source_truth}'
  done
```

Each output is a one-line verdict per resource. Group by status (PASS / WATCH / BLOCK / ASK / INCONCLUSIVE) to produce the audit matrix. The `INCOMPLETE` verdict on a `source_truth` field is recoverable (missing proof gaps); the `MISMATCH` verdict is not.

### Step 4 — cross-cluster outliers (true fleet scope)

`fleet outliers --view` does the multi-cluster comparison. It walks each cluster's snapshot for the View's columns and flags rows where one cluster deviates:

```bash
$ cub-scout fleet outliers --view monthly-spend-by-team
View: monthly-spend-by-team
Clusters: 12

  payments  EXPECTED 3 deploys / cluster   prod-euw1: 2 (outlier: missing payments-worker)
  checkout  EXPECTED 5 deploys / cluster   ALL match
  search    EXPECTED 4 deploys / cluster   staging-use2: 3 (outlier: missing search-replica)
```

Each row names the expectation (`EXPECTED`) and the offending cluster (`outlier`). Fleet outliers requires **connected mode** because the cross-cluster snapshots live in ConfigHub.

### Step 5 — persist as receipts (optional)

If the audit feeds a compliance dashboard or a Pilot acceptance kernel, generate a typed evidence artifact per resource:

```bash
$ for d in $(cub-scout map workloads --namespace payments --format json | jq -r '.[] | "\(.kind)/\(.name)"'); do
    cub-scout receipt verify "$d" -n payments --strategy git-argo --save
  done
$ cub-scout receipt list --format json | jq '[.[] | select(.verdict == "BLOCK")]'
```

See [`scout-verify`](../scout-verify/SKILL.md) for the receipt surface; the `source-truth-pass` predicate is the audit-aligned one.

## Worked example

```bash
$ cub-scout compare three-way --view monthly-spend-by-team --format json | jq '.summary'
{
  "agreement": "partial",
  "agreed": 18,
  "converging": 1,
  "diverged": 3,
  "scope": "view:monthly-spend-by-team"
}

$ cub-scout fleet outliers --view monthly-spend-by-team
View: monthly-spend-by-team
Clusters: 12

  payments  EXPECTED 3 deploys / cluster   prod-euw1: 2 (outlier: missing payments-worker)
```

Reading: 22 resources in the View; 18 conformant, 1 reconciling, 3 truly divergent, 1 multi-cluster outlier (a missing deploy in `prod-euw1`).

For the three divergent resources, drill into each with `compare three-way <kind>/<name>` to see the per-field attribution. For the outlier, drill into `cub view get monthly-spend-by-team` to confirm the expectation and then `cub-scout explain` the missing resource's cluster context.

## Connected-mode requirement

Most of this skill **requires** `cub auth login`. Specifically:

- `compare three-way` (DRY layer needs ConfigHub)
- `compare source-truth` (the contract is meaningless without all three surfaces)
- `views resolve`
- `fleet outliers`

The `compare drift` standalone variant works without ConfigHub for file-vs-live diffs, but it's narrower in scope than a fleet audit. Run `cub-scout status` first to confirm the mode; if standalone, tell the user the audit needs auth.

## CI integration

For periodic compliance checks (e.g., a daily audit job), the verbs are CI-shaped:

```bash
# Exit non-zero if any resource in scope is diverged (i.e., gate on conformance)
$ cub-scout compare three-way --view monthly-spend-by-team --fail-on warning
```

`--fail-on warning|info` (added in `compare three-way`) maps the agreement state to exit codes. The flag itself is read-only — it just decides whether to fail the pipeline, not whether to apply a fix.

Receipts are the more durable artifact:

```bash
# Save one receipt per workload in scope; commit them to an audit branch
$ ./audit.sh > audit-results.txt
$ cub-scout receipt list --format json > audit-receipts.json
$ git add audit-receipts.json && git commit -m "daily audit $(date -u +%F)"
```

## Tool boundary

- **Allowed:** the four Compare verbs; `fleet outliers`; `views resolve / project`; `scan` (risk-classification overlay); `map`; `summary list` (connected snapshot reads); `receipt verify --save` for evidence persistence.
- **Not allowed:** any mutation to cluster state or ConfigHub. cub-scout's `--fail-on` flag fails CI; it does not patch. ApplicationSet generation, unit creation, target binding are all `cub` operations.

## References

- [`scout-compare`](../scout-compare/SKILL.md) — the four Compare verbs
- [`scout-govern`](../scout-govern/SKILL.md) — `fleet outliers`, `views resolve`, `summary`
- [`scout-verify`](../scout-verify/SKILL.md) — typed audit evidence
- [`confighub-source-truth`](../confighub-source-truth/SKILL.md) — the strategy-typed verdict variant
- Views integration: `#391` (parent), `#414` (compare three-way --view), `#403` (views resolve)
- Source-truth contract: `#393`, `#418` (Phase 2 strategies)
- Fleet outliers: roadmap-rendered-manifest-and-argo.md workstream F

## Constraints

- Connected mode required for the View + ConfigHub + fleet surfaces. Standalone audits are limited to per-resource three-way + drift.
- Views need to be **declared in ConfigHub** before they can be referenced. Authoring Views is a `cub view create` operation; out of cub-scout scope.
- The audit produces evidence; remediation of any outlier is the user's call. cub-scout's `receipt verify --save` is the suggested persistence path for the audit trail.
