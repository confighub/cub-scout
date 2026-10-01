---
name: trace-context-binding
description: "Interpret explicitly scoped Trace evidence, source revisions and denied reads without false completeness claims."
expected_outcome: "Identify Alpha's declared source, incomplete Events coverage, and denied observation as unknown; do not claim applied revision, stable cluster identity or dollar savings."
tags: [product-contract, context, recorded, trace, mcp]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the three files in `cluster/`. They are actual Scout MCP outputs and a
receipt from controlled local HTTP fixtures, not a current Kubernetes cluster.
No network or live tools.

An operator asks: "Which context and source did the successful Trace read?
Is that source revision the applied revision? Are its Events complete? Does the
denied result mean the selected cluster has no resources? Do these results
prove stable cluster identity or agent dollar savings?"

Return one bare JSON object with exactly these string keys in this order:
`context`, `source_url`, `source_revision`, `revision_role`, `events_coverage`,
`denied_context`, `denied_inventory`, `stable_cluster_identity`, `dollar_savings`.
Use `DECLARED_TARGET` or `APPLIED` for revision role; `COMPLETE` or `INCOMPLETE`
for coverage; `KNOWN` or `UNKNOWN` for denied inventory; and `PROVEN` or
`NOT_PROVEN` for the last two fields. Do not fill denied gaps from Alpha or the
ambient Beta context.
