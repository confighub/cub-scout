---
name: trace-case-sensitive-slugs
description: "Interpret Trace delivery evidence without joining case-distinct ConfigHub slugs."
expected_outcome: "Reject opposite-case unit/target slugs; preserve exact no-match omissions and stronger ID rules."
tags: [product-contract, trace, confighub, identity, synthetic]
max_turns: 4
timeout_seconds: 90
allowed_tools: [Read, Grep]
---

Use only the two JSON files in `cluster/`. They are a synthetic typed Trace
output and mocked bounded connected rows, not a live ConfigHub response.

The selected object is associated with unit slug `PaymentsAPI` and target slug
`West`. The bounded event row names unit `paymentsapi`; the release row names
target `west`; the live-status row also names `paymentsapi`. Explain which rows
Trace attached, what the omissions mean, and
whether UUID casing or conflicting IDs change the slug rule.

Return one bare JSON object with exactly these string keys in this order:
`unit_slug`, `target_slug`, `wrong_case_unit_event`, `wrong_case_release`,
`wrong_case_live_status`, `unit_event_evidence`, `release_evidence`,
`live_status_evidence`, `identity_rule`, `uuid_id_case`.

Use `EXCLUDED` for all three wrong-case rows, `NONE_WITH_NO_MATCH_OMISSION` for
evidence sections, `SLUGS_CASE_SENSITIVE_IDS_STRONGER` for the matching rule,
and `NORMALIZED` for UUID ID casing. Do not infer a successful delivery or
claim these fixture rows came from a real server.
