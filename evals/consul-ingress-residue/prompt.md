---
name: consul-ingress-residue
description: "HLT-03 receipt-backed source case; prepared, not run."
tags: [benchmark-v1, HLT-03, health, consul, argocd]
max_turns: 4
timeout_seconds: 90
allowed_tools: [Read, Grep]
---

Review the supplied source provenance, receipt, and recorded Argo child Application capture for
`hashicorp-consul-secure-mesh-existing-secrets-parity` in namespace `argocd`.
Report what the receipt says about the overall outcome and workload result,
then report the child Application's sync/health and any specifically identified
residual object with its recorded sync/health. State whether the evidence gives
a cause for that residual and whether the capture is current, atomic, or a full
Kubernetes object snapshot.

Use the exact child Application identity above; do not substitute the root
Application or infer a cause from the resource kind or status. The receipt and
child capture are separate recorded evidence sources. They are historical and
not necessarily simultaneous. Do not make claims about current cluster state.

Return one bare JSON object with exactly these keys, in any order and with no
additional prose:
`receipt_outcome`, `workload_outcome`, `child_sync`, `child_health`,
`residual_identity`, `residual_sync`, `residual_health`, `residual_cause`,
`observation_scope`.

For outcome and status fields, use the declared enums `PASS`, `WATCH`, `FAIL`,
`SYNCED`, `OUT_OF_SYNC`, `HEALTHY`, `PROGRESSING`, `DEGRADED`, or `UNKNOWN` as
appropriate. For `residual_identity`, use the exact
`kind/namespace/name` identity established by the evidence, or `UNKNOWN`.
`residual_cause` is `RECORDED` only when the evidence states a cause for the
residual, otherwise `UNKNOWN`. `observation_scope` must be one of
`HISTORICAL_SEQUENTIAL_NONATOMIC_NOT_CURRENT_NOT_FULL_K8S_SNAPSHOT`, `CURRENT`,
`ATOMIC`, or `UNKNOWN`.
