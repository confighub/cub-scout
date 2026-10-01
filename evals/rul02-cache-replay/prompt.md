---
name: rul02-cache-replay
description: "Interpret a bounded resource cache replay and separate configured inputs from observations returned by the reader."
expected_outcome: "Report identity, content, cache and observation-time changes without asserting unobserved automatic invalidation or freshness."
tags: [benchmark-v1, RUL-02, cache, identity, synthetic-replay]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the two files in `cluster/`. The replay JSON contains authored clock
and response configuration under each `input`, actual HTTP requests and
responses under `requests`, and the object and bounded-read evidence actually
returned by the reader. Configured responses on a cache hit were not served.
This is an authored local API replay with a fixed clock, not a live-cluster
recording. Do not use network or live cluster access.

Compare the first read, unexpired reads, refreshes, and the read after expiry.
Keep resource identity (UID), image content digest, and observation/expiry
times distinct. For each cache hit, distinguish the configured response from
the object returned and from actual HTTP requests. Report missing UID and
digest as unknown; do not infer them from the name or mutable tag. Explain what
the failed refresh and following ordinary read establish about reuse. The
record does not establish push/automatic invalidation, present-day cluster
state, or a freshness guarantee outside the recorded fixed-clock steps.

Return one bare JSON object with exactly these string-valued keys in this
order, without prose or additional properties:
`initial_read`, `initial_uid`, `initial_digest`, `initial_observed_at`,
`initial_expires_at`, `repeat_read`, `repeat_uid`, `repeat_digest`,
`repeat_observed_at`, `repeat_expires_at`,
`changed_response_before_refresh_uid`, `changed_response_before_refresh_digest`,
`changed_response_before_refresh_served`, `returned_before_refresh_uid`,
`returned_before_refresh_digest`, `refresh_read`, `refresh_uid`, `refresh_digest`,
`same_uid_refresh_read`, `same_uid_refresh_uid`, `same_uid_refresh_digest`,
`after_expiry_read`, `after_expiry_uid`,
`after_expiry_digest`, `missing_uid`, `missing_digest`, `failed_refresh`,
`read_after_failed_refresh`, `automatic_invalidation`, `push_invalidation`,
`freshness_guarantee`, `evidence`.

Use only the declared values: read results are `MISS`, `HIT`, or `REFRESH`;
`changed_response_before_refresh_served` is `NO`; missing identity values are
`UNKNOWN`; failed operations are `ERROR`; both `automatic_invalidation` and
`push_invalidation` are `NOT_DEMONSTRATED`; and `freshness_guarantee` is
`NOT_ESTABLISHED`. UID, digest, and timestamps must be copied exactly from the
supplied JSON. Digests are the full `sha256:<64 hex>` values from immutable
image references. The
evidence value is exactly `rul02-cache-replay.json+capture-scope.json`.
