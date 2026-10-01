---
name: pre02-node-selector
description: "Interpret a recorded Pod scheduling prerequisite transition without extending claims to health or cloud configuration."
expected_outcome: "Identify the observed selector prerequisite from equal raw evidence and distinguish scheduling from application health."
tags: [benchmark-v1, PRE-02, kubernetes, prerequisites, recorded]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the supplied files in `cluster/`. They contain six raw Kubernetes API
responses and a capture-scope index. The responses are sequential, not an
atomic snapshot. The capture context and limitations are stated in the index.
Do not use network or live cluster access, and do not use evidence outside
these files.

Identify the concrete prerequisite that blocked scheduling in the before
observations. Compare the Pod UID, scheduling condition, authored selector,
matching-node count, and UID-correlated scheduler event across the phases. In
the after observations, report the exact scheduled node and label used to
satisfy the selector. A historical `FailedScheduling` event may remain after
the Pod is scheduled; do not treat it as the current scheduling state.

Scheduling is not application health. Do not infer a cloud API, secret,
capacity limitation, readiness, or controller health beyond what the supplied
responses explicitly establish. Keep conclusions within this single captured
cluster and sequential observation.

Return exactly one compact JSON object as the complete response. It must have
the following string-valued keys in this exact order, with no additional keys,
duplicate keys, whitespace, or surrounding prose:

`prerequisite`, `before_pod_uid`, `before_scheduling`, `selector`,
`before_matching_nodes`, `before_event`, `after_pod_uid`, `after_scheduling`,
`after_node`, `after_selector_label`, `historical_failed_scheduling_event`,
`health_scope`, `cloud_api`, `secret`, `capacity`, `evidence`.

Use these exact vocabularies where applicable:

- `prerequisite`: `NODE_SELECTOR_LABEL`, `CLOUD_API`, `SECRET`, or `UNKNOWN`.
- `before_scheduling`: `FALSE_UNSCHEDULABLE`, `TRUE`, or `UNKNOWN`.
- `before_event`: `UID_CORRELATED_SELECTOR_MISMATCH`, `NO_MATCHING_EVENT`, or `UNKNOWN`.
- `after_scheduling`: `TRUE`, `FALSE`, or `UNKNOWN`.
- `historical_failed_scheduling_event`: `RETAINED_NOT_CURRENT_FAILURE`, `ABSENT`, or `UNKNOWN`.
- `health_scope`: `SCHEDULING_ONLY_NO_HEALTH_CONCLUSION` or `UNKNOWN`.
- `cloud_api`: `NOT_OBSERVED` or `UNKNOWN`.
- `secret`: `NOT_OBSERVED` or `UNKNOWN`.
- `capacity`: `NOT_INFERRED` or `UNKNOWN`.

UIDs and the scheduled node name must be copied exactly from the observed
objects. `selector` and `after_selector_label` use `key=value` form.
`before_matching_nodes` is the exact decimal count. `evidence` is the supplied
filename list joined by `+` in this order:
`capture-scope.json+before-pod.json+before-nodes.json+before-events.json+after-pod.json+after-nodes.json+after-events.json`.
