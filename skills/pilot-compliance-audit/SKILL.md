---
name: pilot-compliance-audit
description: 'Use when Pilot is RENDERING A PERIODIC POLICY-CONFORMANCE REPORT for a compliance audit. Natural phrasings: "Pilot, run the quarterly compliance audit", "render the conformance report against policy", "audit our prod fleet against the security baseline", "produce the compliance evidence pack for the auditor", "Pilot, what are this quarter''s policy violations + their evidence?". Pilot consumes cub-scout''s scope-wide `compare source-truth` (per-resource verdicts under the declared strategy) + `scan` findings (risk-pattern matches) + (optional) per-resource receipts persisted to the local store, then synthesizes a compliance-shaped report with violations table + fingerprinted evidence inventory. Differs from `pilot-fleet-conformance` which is the SAME-VERBS-SAME-DAY operational verdict; this skill is the LONGER-CYCLE compliance shape — the output is a quarterly/monthly report with attached evidence inventory, not a real-time gate. Do NOT load for: a single-resource pre-deploy gate (use pilot-cd-gate), real-time event response (use pilot-watch-alert-response), incident close-out evidence (use pilot-incident-evidence), or a rollback decision (use pilot-rollback-decision).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare source-truth *) Bash(cub-scout compare source-truth *) Bash(cub scout compare source-truth *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout scan *) Bash(cub-scout scan *) Bash(cub scout scan *) Bash(./cub-scout patterns *) Bash(cub-scout patterns *) Bash(cub scout patterns *) Bash(./cub-scout fleet outliers *) Bash(cub-scout fleet outliers *) Bash(cub scout fleet outliers *) Bash(./cub-scout views resolve *) Bash(cub-scout views resolve *) Bash(cub scout views resolve *) Bash(./cub-scout views project *) Bash(cub-scout views project *) Bash(cub scout views project *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout audit *) Bash(cub-scout audit *) Bash(cub scout audit *) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub view get *) Bash(cub view list)

---

# pilot-compliance-audit

The periodic compliance-audit scenario from **Pilot's** side. A quarterly/monthly audit window opens; Pilot is asked to render a structured **compliance report** with per-resource violations + their fingerprinted evidence. The downstream consumer is the auditor (internal security/compliance team, external auditor, SOC 2 reviewer, regulator) — not the SRE team running the cluster.

Same cub-scout verbs as `pilot-fleet-conformance`, different framing: that skill is the *operational* verdict (today's snapshot, today's decision); this skill is the *compliance* shape (last quarter's evidence, an auditor's report). The output cadence is longer; the evidence-attachment requirement is stricter.

## When to use

Explicit phrasings:

- "Pilot, run the quarterly compliance audit"
- "Render the conformance report against policy"
- "Audit our prod fleet against the security baseline"
- "Produce the compliance evidence pack for the auditor"
- "Pilot, what are this quarter's policy violations + their evidence?"
- "Generate the SOC 2 evidence inventory from cub-scout"
- "Compliance run for {policy/baseline name}"

Implicit intents:

- The cadence is **periodic** (quarterly, monthly), not real-time
- The audience is the **auditor** — an external or compliance-team reviewer who wants structured violations + fingerprinted evidence per violation
- The report needs to be **fingerprint-stable** — the auditor may re-validate any receipt months later
- The report rolls up across **scope** (View / namespace / cluster / fleet) — not a single resource
- Pilot's verdict per resource maps to a **compliance category** (Compliant / Compliant with caveats / Non-compliant / Insufficient evidence) — slightly different vocabulary from PASS/WATCH/ASK/BLOCK

## Do not load for

- A **single-resource pre-deploy gate** — [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md)
- The **operational** version of the same audit (same-day; not for the auditor) — [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md). Same verbs; different framing.
- A **real-time event-driven** response — [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md)
- An **incident close-out** evidence pack — [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md)
- A **rollback decision** — [`pilot-rollback-decision`](../pilot-rollback-decision/SKILL.md)
- A **promotion safety check** — [`pilot-promotion-gate`](../pilot-promotion-gate/SKILL.md)
- Any **mutating action**. Compliance audits are read-only by design.

## The Pilot ↔ cub-scout compliance loop

```
   quarterly compliance window opens (cron / manual trigger)
        │
        ▼
   Pilot (acceptance judge in compliance-report mode)
        │  "render Q2 2026 compliance report for view:prod-baseline"
        │
        ├──► cub-scout views resolve <baseline-view>   # scope confirmation
        │
        ├──► For each resource in scope:
        │      cub-scout compare source-truth <r> -n <ns> --strategy <s> --format json
        │      cub-scout scan -n <ns> --format json   # risk-pattern matches per resource
        │      cub-scout receipt verify <r> -n <ns> --strategy <s> \
        │        --save                              # fingerprinted, persisted to local store
        │
        ├──► Cross-cluster (if View has cluster scope):
        │      cub-scout fleet outliers --view <baseline-view> --format json
        │
        ├──► Audit-trail layer:
        │      cub-scout audit list --since 90d --format json    # ConfigHub audit entries
        │      cub-scout receipt list --format json              # full receipt inventory
        │
        └──► (optional) chain ALL per-resource receipts under one aggregate:
              # When #448 aggregate-with-discovery half ships, this collapses to one call.
              # Until then: produce one receipt per resource; the report inventory links each.
        │
        ▼
   Pilot's compliance report:
        │  Compliant resources:               <list with PASS receipts>
        │  Compliant-with-caveats resources:  <list with WATCH receipts + proof_gaps>
        │  Non-compliant resources:           <list with BLOCK receipts + violation reason>
        │  Insufficient-evidence resources:   <list with INCONCLUSIVE receipts + omissions>
        │  Audit-trail (last 90d):            <ConfigHub change events that touched the scope>
        │  Receipt inventory:                 <fingerprint-stable file paths per resource>
        │
        ▼
   auditor reads the report
        │  Auditor can `cub-scout receipt validate <path>` any individual entry to confirm tamper-free
        └────────────────────────────────────────────────────
```

## Verdict mapping (cub-scout → compliance vocabulary)

Auditors typically don't speak PASS / WATCH / ASK / BLOCK. Map cub-scout's receipt verdicts to compliance vocabulary at the report-rendering layer:

| Receipt verdict | Compliance category | Auditor-facing meaning |
|------------------|---------------------|------------------------|
| `PASS` | **Compliant** | Evidence supports the claim against the declared strategy / baseline |
| `WATCH` | **Compliant with caveats** | Evidence is ambiguous; proof_gaps listed; the auditor can decide whether the gaps invalidate compliance |
| `BLOCK` | **Non-compliant** | Evidence contradicts the claim — a real violation |
| `INCONCLUSIVE` | **Insufficient evidence** | The receipt couldn't be built (missing strategy, missing input); the auditor needs the cube-scout-side fix or a manual review |

`ASK` is a source-truth `status` value, not a receipt verdict. When it surfaces during the audit, the wrapping receipt has verdict `WATCH` (per `pkg/agent/receipt_predicates.go:396-407`); Pilot's compliance report should list the auditor-facing meaning consistently.

## Step-by-step

### Step 1 — resolve the audit scope

A **View** is the canonical compliance scope (declared in ConfigHub via `cub view create`):

```bash
$ cub-scout views resolve https://hub.confighub.com/views/prod-baseline-q2-2026
View: prod-baseline-q2-2026
  scope: cluster
  columns: [team, service, source_truth_strategy, last_audited]
  resolved at: 2026-05-22T22:00:00Z
```

Scope sources for compliance: ConfigHub Views (preferred — declared scope is auditable), namespace, cluster. Avoid ad-hoc resource lists for periodic compliance — they're harder to defend later.

### Step 2 — per-resource source-truth + scan + receipt

```bash
# Per-resource source-truth verdict against the baseline's declared strategy
$ for r in $(cub-scout views project --view prod-baseline-q2-2026 --format json \
              | jq -r '.[] | "\(.kind)/\(.name) -n \(.namespace)"'); do
    cub-scout compare source-truth $r --strategy git-argo --format json \
      > evidence/$r-source-truth.json
  done

# Risk-pattern scan (CVE, expiring certs, dangling resources, etc.)
$ cub-scout scan --format json > evidence/scan-findings.json

# Fingerprinted per-resource receipts (immutable; survive the audit window)
$ for r in $(cub-scout views project --view prod-baseline-q2-2026 --format json \
              | jq -r '.[] | "\(.kind)/\(.name) -n \(.namespace)"'); do
    cub-scout receipt verify $r --strategy git-argo --save
  done
```

Each receipt lands in the immutable local store (`$CUB_SCOUT_RECEIPTS_DIR → $XDG_DATA_HOME → $HOME/.local/share/cub-scout/receipts`) with a canonical filename. Auditors can validate any receipt months later with `cub-scout receipt validate`.

### Step 3 — connected-mode audit trail layer

```bash
$ cub-scout audit list --since 90d --format json > evidence/confighub-audit.json
$ cub-scout fleet outliers --view prod-baseline-q2-2026 --format json > evidence/fleet-outliers.json
```

`audit list` (connected) captures break-glass accept/reject entries from ConfigHub — the human-decision audit trail that paired with the per-resource verdicts during the quarter. `fleet outliers` flags cross-cluster divergences against the baseline.

### Step 4 — compose the report

Pilot's compliance report shape:

```json
{
  "report_type": "quarterly_compliance",
  "audit_window": "Q2 2026 (2026-04-01 → 2026-06-30)",
  "scope": "view:prod-baseline-q2-2026",
  "declared_strategy": "git-argo",
  "rollup": {
    "compliant": 184,
    "compliant_with_caveats": 12,
    "non_compliant": 3,
    "insufficient_evidence": 1
  },
  "non_compliant": [
    {
      "resource": "Deployment/payments-cron in prod-use1",
      "verdict": "BLOCK",
      "reason": "source_truth=MISMATCH; manual-edit detected on container image",
      "receipt": "/path/to/receipts/2026-Q2/payments-cron.receipt.json"
    },
    {"resource": "Deployment/legacy-api in prod-euw1", "verdict": "BLOCK", "reason": "fleet outlier: missing per-cluster baseline", "receipt": "/path/to/..." }
  ],
  "compliant_with_caveats": [
    {"resource": "Deployment/api in prod-use1", "verdict": "WATCH", "proof_gaps": ["git-source-anchor unresolved: argocd CLI not installed"], "receipt": "/path/to/..."}
  ],
  "insufficient_evidence": [
    {"resource": "Deployment/legacy-tool in prod-use1", "verdict": "INCONCLUSIVE", "omissions": ["managedFields"], "receipt": "/path/to/..."}
  ],
  "audit_trail_entries": "evidence/confighub-audit.json (90 entries)",
  "scan_findings": "evidence/scan-findings.json (4 critical, 12 warning, 31 info)",
  "fleet_outliers": "evidence/fleet-outliers.json (2 outliers)",
  "evidence_inventory": "evidence/receipt-list.json (200 receipts; all fingerprint-verifiable)"
}
```

### Step 5 — handoff + later re-validation

The auditor receives the report + the `evidence/` directory. To validate any individual receipt:

```bash
$ cub-scout receipt validate evidence/non-compliant/payments-cron.receipt.json
# exit 0 = fingerprint intact; 1 = mismatch; 2 = I/O

$ cub-scout receipt show evidence/non-compliant/payments-cron.receipt.json --format ascii
# Renders the same evidence Pilot's report cited — tamper-evident
```

Pilot's report is **only as durable as the receipt store** — auditors typically want the evidence/ directory archived externally (S3, GitHub Release artifact, ConfigHub bundle catalog) for the retention period.

## Worked example

Q2 2026 compliance run; scope = `view:prod-baseline-q2-2026`; strategy = `git-argo`.

```bash
$ mkdir -p evidence/{non-compliant,with-caveats,inconclusive}

# Per-resource receipts (saved to immutable store + linked into evidence/)
$ for r in $(cub-scout views project --view prod-baseline-q2-2026 --format json \
              | jq -r '.[] | "\(.kind)/\(.name) -n \(.namespace)"'); do
    cub-scout receipt verify $r --strategy git-argo --save 2>/dev/null
  done

# Categorize by verdict
$ cub-scout receipt list --format json \
    | jq '[.[] | select(.verdict == "BLOCK")]' > evidence/non-compliant.json
$ cub-scout receipt list --format json \
    | jq '[.[] | select(.verdict == "WATCH")]' > evidence/with-caveats.json
$ cub-scout receipt list --format json \
    | jq '[.[] | select(.verdict == "INCONCLUSIVE")]' > evidence/inconclusive.json

# Audit-trail + scan + fleet outliers
$ cub-scout audit list --since 90d --format json > evidence/audit.json
$ cub-scout scan --format json > evidence/scan.json
$ cub-scout fleet outliers --view prod-baseline-q2-2026 --format json > evidence/outliers.json

# Rollup
$ jq 'length' evidence/{non-compliant,with-caveats,inconclusive}.json
3
12
1
```

Pilot's report cites the 3 non-compliant resources by name + their receipts + their violation reasons; the WATCH/INCONCLUSIVE lists fill in the caveats; the audit-trail / scan / outliers layers give the auditor the cross-axis evidence. Total artifact count: 200 receipts (one per resource in scope) + 3 supporting JSON files. All fingerprint-verifiable.

## Standalone vs connected

**Connected mode is effectively required** for compliance audits:

- `compare source-truth --strategy` — connected
- `views resolve` / `views project` — connected
- `fleet outliers` — connected
- `audit list` — connected
- `receipt verify --strategy source-truth-pass` — connected

Standalone-mode compliance is *narrower* (no View scope; no ConfigHub audit trail; no fleet-outlier dimension) and rarely useful for an external auditor. If standalone is the only option, Pilot's report should call out the limitation explicitly so the auditor knows what's missing.

## Tool boundary

- **Allowed (cub-scout):** Compare verbs (read-only), `scan`, `patterns`, `fleet outliers`, `views resolve` / `project`, `receipt verify` / `show` / `validate` / `list`, `map`, `audit list`. All read-only.
- **Allowed (cluster):** `kubectl get/describe`. NOT `kubectl apply / edit / patch / delete`.
- **Allowed (ConfigHub):** `cub * get / list`, `cub view get / list`. NOT `cub * create / update / delete`.
- **Compliance audits are read-only by design.** The audit produces evidence; remediation (re-aligning the non-compliant resources) is a separate operational decision, typically routed through `pilot-patch-and-drift` or `pilot-rollback-decision` — those skills handle Pilot's mutation path. This skill stops at the report.

## References

- [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md) — same verbs, operational (same-day) framing instead of compliance (periodic) framing
- [`scout-compare`](../scout-compare/SKILL.md) — Compare verbs
- [`scout-verify`](../scout-verify/SKILL.md) — receipt persistence + validation
- [`scout-govern`](../scout-govern/SKILL.md) — fleet, views, audit, summary
- [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md) — operator-side counterpart
- [`scout-attribute`](../scout-attribute/SKILL.md) — per-field attribution for violation explainability
- Source-truth contract: `#393`, `#418` (Phase 2 strategies)
- Receipts v1+v2: `#446` (v1), `#463` (v2 surface)
- Read-only triad: [`references/read-only-triad.md`](../references/read-only-triad.md)

## Constraints

- The audit is **point-in-time**: the report reflects the cluster + ConfigHub state at evaluation. Compliance frameworks that require continuous monitoring should pair this skill with [`pilot-watch-alert-response`](../pilot-watch-alert-response/SKILL.md) for the event-driven side.
- Receipts are **fingerprint-stable but not signed** in v1/v2 (`#463`); cryptographic signing is a future hardening direction. Auditors who require non-repudiation need the future signing layer — flag this in the report.
- The local receipt store at `$XDG_DATA_HOME/cub-scout/receipts` is **per-host**. For multi-host compliance pipelines, Pilot should archive each run's `evidence/` directory externally (S3, ConfigHub bundle catalog, GitHub Release artifact) for the retention period.
- `scan` covers ~46 built-in risk patterns; bespoke policies (custom CIS benchmarks, SOC 2 controls) need either custom patterns (out of cub-scout scope) or a policy engine (Kyverno, OPA Gatekeeper) that this skill doesn't replace.
- Compliance vocabulary (Compliant / Non-compliant) is the report-layer translation of receipt verdicts (PASS / BLOCK). cub-scout doesn't speak compliance vocabulary natively; Pilot's report renders the translation.
