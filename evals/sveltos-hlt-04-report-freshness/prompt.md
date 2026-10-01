---
name: sveltos-hlt-04-report-freshness
description: "Analyze timestamp and status fields in a pinned synthetic producer replay."
expected_outcome: "Report each held/computed timestamp and producer write result; keep health and release identity distinct."
tags: [benchmark-v1, HLT-04, sveltos, report-freshness, synthetic, recorded]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the files in `cluster/`. They contain an authored synthetic source
replay, its fixture inputs and outputs, source metadata, and a separately
authored check-execution receipt. Treat all timestamps and resources as
synthetic. Do not use network, live cluster access, or other context.

Compare the held report with the computed report for each record. Report what
was held before and after, the computed `observedAt`, whether a write patch was
produced, and the report's health, sync status, and revision. Explain what the
separate receipt does and does not establish, and state the limits of the
revision and timestamp evidence. Keep each record distinct; do not generalize
the synthetic behavior to a live Sveltos run.

Return one compact JSON object with exactly these top-level keys and order:
`records`, `check_receipt`, `check_receipt_consumed`,
`last_transition_time_role`, `applied_release_digest_proven`, `evidence_scope`.
`records` must contain one row for each `R01` through `R08`, in that order,
with exactly these string-valued keys in order: `id`,
`held_before_observed_at`, `held_after_observed_at`, `computed_observed_at`,
`write_applied`, `health_status`, `sync_status`, and `revision`. Use the
source values where present and `MISSING` when an observedAt field is absent.
Do not add prose or additional properties.
