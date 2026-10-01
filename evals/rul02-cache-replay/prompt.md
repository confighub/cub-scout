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
returned by the reader. Treat configured responses as authored state; compare
them with the request trace to determine what the reader actually received.
This is an authored local API replay with a fixed clock, not a live-cluster
recording. Do not use network or live cluster access.

Use the neutral step IDs to locate the evidence: step-01 is the initial read;
step-02 the first unexpired repeat; step-03 the next unexpired read after the
configured response changes; step-04 the first explicit refresh; step-05 the
second explicit refresh; step-06 the ordinary read after the TTL boundary;
step-07 the identity-field input; step-08 the refresh using the final response
input; step-09 the following ordinary read. For each, keep resource identity (UID), image content digest, cache
result, and observation/expiry times distinct. For step-03, separately report
the configured response, whether that response was served, and the object
returned. For identity fields, copy observed values; when absent, represent
them as `UNKNOWN`, and do not infer them from a name or mutable tag. Use the
complete record to assess whether automatic or push invalidation and a
freshness guarantee are demonstrated; do not infer current live state.

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

Select from these declared alternatives based on the supplied evidence:
read results are `MISS`, `HIT`, or `REFRESH`;
`changed_response_before_refresh_served` is `YES`, `NO`, or `UNKNOWN`;
`failed_refresh` and `read_after_failed_refresh` are `ERROR`, `SUCCESS`, or
`UNKNOWN`; `automatic_invalidation` and `push_invalidation` are
`DEMONSTRATED`, `NOT_DEMONSTRATED`, or `UNKNOWN`; and `freshness_guarantee` is
`ESTABLISHED`, `NOT_ESTABLISHED`, or `UNKNOWN`. UID, digest, and timestamps
must be copied exactly from the supplied JSON. Digests are the full
`sha256:<64 hex>` values from immutable image references. The evidence value
is exactly `rul02-cache-replay.json+capture-scope.json`.
