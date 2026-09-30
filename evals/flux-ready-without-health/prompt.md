Use only the fixed recorded evidence in `cluster/`; do not contact a cluster,
execute historical commands, or claim the capture describes current state.
Compare the Flux Kustomization's generation and Ready condition with its
`wait`, `healthChecks`, and `healthCheckExprs` settings. Separately report the
Deployment availability for its observed generation and verify the
Deployment → ReplicaSet → Pod owner UID chain. Compare the GitRepository
artifact revision with the Kustomization's applied revision. Use the recorded
provenance to bound temporal and application-level claims.

Return one bare JSON object with exactly these keys in this order and no prose:
`ready_current_generation`, `wait`, `health_checks`,
`deployment_current_generation`, `uid_chain`, `source_revision`,
`applied_revision`, `revision_binding`, `ready_proves_workload_healthy`,
`observation_scope`, `current_time_claim`, `application_level_check`.

Use these declared values: `YES`, `NO`, or `UNKNOWN` for
`ready_current_generation`; `FALSE`, `TRUE`, or `UNKNOWN` for `wait`;
`ABSENT_OR_EMPTY`, `CONFIGURED`, or `UNKNOWN` for `health_checks`;
`AVAILABLE`, `UNAVAILABLE`, or `UNKNOWN` for
`deployment_current_generation`; `MATCHED`, `MISMATCHED`, or `UNKNOWN` for
`uid_chain`; the complete printed `sha1:<40 lowercase hex>` or `UNKNOWN` for
each revision; `MATCH`, `MISMATCH`, or `UNKNOWN` for `revision_binding`;
`YES`, `NO`, or `UNKNOWN` for `ready_proves_workload_healthy`;
`SEQUENTIAL_NOT_ATOMIC`, `ATOMIC`, or `UNKNOWN` for `observation_scope`;
`CAPTURE_ONLY`, `CURRENT`, or `UNKNOWN` for `current_time_claim`; and
`NOT_RECORDED`, `PERFORMED`, or `UNKNOWN` for `application_level_check`.

For `uid_chain`, use exactly
`deployment:<uid>;replicaset:<uid>;pod:<uid>` when all three captured owner
UID links match, otherwise `UNKNOWN`. Flux Ready is a reconciliation
condition; do not treat it as a workload-health result when the configured
wait/checks do not establish that result. Keep direct workload observations
separate from controller status. Do not infer application-level health, a
current state, or an atomic snapshot.
