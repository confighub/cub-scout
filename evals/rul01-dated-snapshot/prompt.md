---
name: rul01-dated-snapshot
description: "Answer repeated questions from one dated Kubernetes observation without renewing it."
expected_outcome: "Preserve the same capture binding and recorded condition while reporting age only at the two explicit test clocks; current state remains unknown."
tags: [benchmark-v1, RUL-01, reuse-limits, raw-recording]
max_turns: 4
timeout_seconds: 90
allowed_tools: [Read, Grep]
---

Use only the files in `cluster/`. They contain one retained Pod response, a
neutral receipt bound to its exact bytes and request, and two authored as-of
test clocks. These are historical fixture inputs, not live access. The receipt
timestamps are logged to one-second precision although the measured request
elapsed 0.01204633410088718 seconds. Do not claim the request lasted zero
seconds or that the printed second is an exact instant.

Report the resource identity and UID, the recorded `PodScheduled` condition,
the resource creation timestamp, and the condition transition timestamp. The
capture time is the logged **request end** time; it is separate from the two
resource timestamps and from the authored test clocks. Calculate age from that
request-end second at each test clock. The clocks are explicit arithmetic
inputs, not current time. Repeating the question does not refresh the snapshot.
Current live state is unknown.

Return exactly one minified single-line JSON object (no whitespace outside
strings) and no prose, with precisely these keys
in this order, all values strings:
`evidence_binding`, `resource_identity`, `resource_uid`,
`recorded_scheduling_state`, `resource_created_at_utc`,
`condition_transition_at_utc`, `capture_request_started_at_utc`,
`capture_request_ended_at_utc`, `timestamp_precision`, `capture_time_basis`,
`snapshot_sha256`, `test_clock_1_utc`, `age_at_clock_1_seconds`,
`test_clock_2_utc`, `age_at_clock_2_seconds`, `capture_time_reuse`,
`current_live_state`, `scope`, `evidence`.

Allowed values: `evidence_binding` is `VERIFIED` or `UNKNOWN`;
`recorded_scheduling_state` is `POD_SCHEDULED_TRUE`,
`POD_SCHEDULED_FALSE`, or `UNKNOWN`; `timestamp_precision` is
`ONE_SECOND_LOGGED` or `UNKNOWN`; `capture_time_basis` is `REQUEST_END`,
`RESOURCE_CREATION`, `CONDITION_TRANSITION`, or `UNKNOWN`;
`capture_time_reuse` is `SAME_SNAPSHOT_NO_REFRESH`, `REFRESHED_OBSERVATION`,
or `UNKNOWN`; `current_live_state` is `UNKNOWN`, `LIVE_PRESENT`, or
`LIVE_ABSENT`; `scope` is `HISTORICAL_CAPTURE_ONLY`, `CURRENT_CLUSTER`, or
`UNKNOWN`. Use UTC RFC3339 seconds for timestamp fields and base-10 integer
seconds for ages. Use literal `UNKNOWN` when a binding is absent or inconsistent;
never infer a missing time from file metadata or your own clock.

Field encoding: `resource_identity` is
`apiVersion|kind|namespace|name` from the raw object's top level and metadata;
`resource_uid` is metadata.uid. The two resource timestamps come from
metadata.creationTimestamp and the unique PodScheduled condition's
lastTransitionTime. The capture start/end, one-second precision, and
`REQUEST_END` basis come from the receipt. `snapshot_sha256` is the receipt's
hash after verifying it against the raw bytes. Each age is the whole-second
difference between the associated authored clock and receipt endedAt. All
timestamps must be the exact UTC second strings in the corresponding evidence;
do not substitute one timestamp field for another.

For `evidence`, join these exact filenames with `+` in order:
`after-pod.json+capture-time-receipt.json+test-clocks.json`.
