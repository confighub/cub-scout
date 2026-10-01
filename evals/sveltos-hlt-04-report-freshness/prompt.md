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

Return one canonical, minified, single-line JSON object: no Markdown fences,
no leading/trailing text, and no whitespace outside JSON strings. Use exactly
these top-level keys in this order: `records`, `check_receipt`,
`check_receipt_consumed`, `last_transition_time_role`,
`applied_release_digest_proven`, `evidence_scope`,
`renewal_proves_new_check`, `future_timestamp_proves_freshness`.

`records` must contain one row for each `R01` through `R08`, in that order,
with exactly these string-valued keys in this order: `id`,
`held_before_observed_at`, `held_after_observed_at`, `computed_observed_at`,
`write_applied`, `health_status`, `sync_status`, and `revision`. Use source
values where present and `MISSING` when an observedAt field is absent. Use
`YES`, `NO`, or `UNKNOWN` for `write_applied` and each of the four proof/consumption
fields. For health use `Healthy`, `Degraded`, `Progressing`, or `Unknown`; for
sync use `Synced`, `OutOfSync`, or `Unknown`. Timestamp and revision fields are
their literal source strings or `MISSING` when absent.

For `check_receipt`, choose exactly one of `AUTHORED_SYNTHETIC_NOT_CONTROLLER_EVIDENCE`,
`CONTROLLER_EXECUTION_RECEIPT`, `NO_RECEIPT`, or `UNKNOWN`. For
`last_transition_time_role`, choose exactly one of
`CONDITION_TRANSITION_NOT_CHECK_EXECUTION`, `CHECK_EXECUTION_TIME`,
`NOT_PRESENT`, or `UNKNOWN`. For `evidence_scope`, choose exactly one of
`SYNTHETIC_SOURCE_CONTRACT_ONLY`, `LIVE_CONTROLLER_EVIDENCE`,
`MIXED_EVIDENCE`, or `UNKNOWN`. These are vocabulary options, not claims about
the records. Preserve case-level differences and ground every value in the
files; do not infer a check execution or release digest from a refreshed report
timestamp. Do not add properties or prose.
