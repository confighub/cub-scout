---
name: trace-rendered-diff-contract
description: "Interpret a synthetic offline local-rendered Trace diff while preserving evidence limits."
expected_outcome: "Return the changed authored field and exact live-read facts; do not call the local file controller desired state or predict reconciliation."
tags: [product-contract, trace, rendered-diff]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Read only `evidence/desired.yaml` and `evidence/comparison.json`. This is a
synthetic teaching fixture shaped like the Trace output contract; it is not a
captured cluster response, a source revision receipt, or a model-evaluation
result. No command, network, cluster, ConfigHub, or renderer is needed.

An operator asks: "What differs, which live object was read, and does this say
what Flux or Argo will apply next?"

Return one bare JSON object with exactly these keys in this order:
`status`, `comparison`, `coverage`, `resource`, `live_read`, `difference`,
`limits`. `status`, `comparison`, and `coverage` must preserve the fixture's
values. `resource` must contain the exact API version, kind, namespace, name,
UID, and resourceVersion. `live_read` must identify the timestamp and separate
scope-resolution discovery reads from bounded-reader discovery/object reads.
`difference` must identify the authored field and desired/live values.
`limits` must say the local rendered object is not controller desired state,
the comparison covers one object only, and it does not predict reconciliation.
