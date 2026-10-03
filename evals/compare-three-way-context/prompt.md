---
name: compare-three-way-context
description: "Interpret scoped comparison omissions and exact View membership without false convergence claims."
expected_outcome: "Identify selected Alpha evidence, preserve its follow-up selector, retain denied Pod/discovery uncertainty, and reject cross-space slug-only membership."
tags: [product-contract, context, synthetic, comparison, mcp]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only `cluster/evidence.json`, a synthetic example. It is not a captured CLI
or MCP response, current cluster observation, real ConfigHub server result or
model-run receipt. Do not use network or live tools.

An operator asks: "Which context and image did this comparison read? Does the
Pod omission allow complete rollout proof? Does empty denied cluster discovery
prove no workloads? Can the same-slug candidate belong to the selected View
unit in another space? Is the source revision an applied revision? What exact
follow-up selector must be retained? Does this establish cluster identity,
end-to-end agreement, or dollar savings?"

Return exactly one bare JSON object with these string keys:
`context`, `image`, `rollout_coverage`, `denied_inventory`, `view_membership`,
`revision_role`, `followup_selector`, `stable_cluster_identity`, `agreement`,
`dollar_savings`.
Use `INCOMPLETE` for denied rollout coverage; `UNKNOWN` for denied inventory;
`EXCLUDED` for an identity-mismatching View candidate; `DECLARED_TARGET` for the
source revision; `NOT_PROVEN` for cluster identity and savings; and `PARTIAL`
for agreement. Keep the exact context argument in `followup_selector`.
