Review the three staged evidence files in `cluster/`: the Sveltos fleet status
documentation excerpt, the related known-behaviours excerpt, and a separate
ConfigHub OCI delivery receipt. Use only what these artifacts establish. Keep
the documented status reading, the receipt's publication/profile evidence, and
any runtime-artifact binding distinct. Do not assume that records from
different documents, clusters, or dates describe the same observation. Do not
claim a current live state, a person, or an exact runtime Sveltos digest unless
the supplied evidence directly supports that claim.

Return one bare JSON object, no Markdown or surrounding explanation, with
exactly these keys in any order:
`status_cluster`, `status_revision`, `status_revision_basis`, `receipt_cluster`,
`receipt_profile_matches_approved_revision`, `receipt_workload_evidence`,
`runtime_sveltos_digest`, `cross_artifact_join`.

For the basis use `reported_exactly`, `inferred_from_timestamps`, or `UNKNOWN`.
For `receipt_profile_matches_approved_revision`, use a JSON boolean or the
string `UNKNOWN` if unavailable. For `receipt_workload_evidence`, use
`helmrelease_chart_deployed`, `none`, or `UNKNOWN`. For
`runtime_sveltos_digest`, use a full digest only when explicitly bound to the
runtime Sveltos observation; otherwise use `UNKNOWN`. For
`cross_artifact_join`, use `ESTABLISHED` only with explicit evidence joining
these artifacts to the same runtime observation; otherwise use
`UNESTABLISHED`.
