---
name: rul04-image-identity
description: "Assess a tag-only StatefulSet image intent against captured live owner and runtime identity evidence."
expected_outcome: "Report the exact healthy workload and runtime image identity, but UNKNOWN for intended immutable image identity because authored intent is tag-only."
tags: [benchmark-v1, RUL-04, image-identity, statefulset, recorded]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the four files in `cluster/`. They contain the literal authored
StatefulSet manifest, raw StatefulSet GET, raw namespace PodList, and factual
capture scope. The reads were sequential and are not an atomic snapshot. Do not
use network, cluster access, or other tools to fill gaps.

Determine the captured StatefulSet and Pod identities from their actual UID and
owner-reference fields. Report the Pod's runtime image and exact `imageID`, and
assess its observed state and readiness. Decide whether the authored intended
image and runtime image can be related with the identity evidence provided.
Keep intended image identity, runtime image identity, workload health, and
applied-source binding as separate findings. The local OCI reference describes
an authored configuration bundle; assess what the captured files establish
about any controller applying it.

Return a bare JSON object with exactly these string-valued keys, each once, in
any order, without surrounding prose:
`image_identity_verdict`, `unknown_reason`, `intended_image`,
`statefulset_uid`, `pod_name`, `pod_uid`, `pod_owner_uid`, `runtime_image`,
`runtime_image_id`, `runtime_identity_status`, `workload_health`,
`applied_source_binding`, `observation_scope`, `evidence`.

Encoding contract:

- `intended_image`, `statefulset_uid`, `pod_name`, `pod_uid`, `pod_owner_uid`,
  `runtime_image`, and `runtime_image_id` must match the supplied raw files.
- `unknown_reason`: `IMMUTABLE_INTENT_DIGEST_UNAVAILABLE`,
  `RUNTIME_IDENTITY_UNAVAILABLE`, `NONE`, or `OTHER`.
- `image_identity_verdict`: `MATCH`, `MISMATCH`, or `UNKNOWN`.
- `runtime_identity_status`: `OBSERVED` or `MISSING`; `workload_health`:
  `READY_RUNNING`, `NOT_READY`, or `UNKNOWN`.
- `applied_source_binding`: `VERIFIED` or `UNKNOWN`.
- `observation_scope`: `SEQUENTIAL_NON_ATOMIC`.
- `evidence`: supplied filenames joined with `+` in this order:
  `capture-scope.json+desired-statefulset.yaml+pods.json+statefulset.json`.
