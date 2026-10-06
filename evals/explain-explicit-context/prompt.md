---
name: explain-explicit-context
description: "Select enriched Explain evidence from an exact context and keep denial unknown."
expected_outcome: "Select alpha despite ambient beta; retain unknown under denial and context identity limits."
tags: [product-contract, context, explain, authored]
max_turns: 5
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only `cluster/inputs.json`, authored contract inputs rather than captured
Scout output. The two contexts contain identical same-name workloads but their
Applications name different source URLs. The request explicitly selects a
context and namespace. Interpret the intended Explain contract, including the
separate denied object read and omitted-context request. A forbidden read
provides no ownership or health observation.

Return one bare JSON object in this exact key order: `selected_context`,
`namespace`, `owner`, `source`, `ambient_beta_used`, `denied_owner`,
`denied_health`, `omitted_context`, `stable_cluster_identity`,
`live_acceptance_proven`. Use boolean values for `ambient_beta_used`,
`stable_cluster_identity`, and `live_acceptance_proven`; strings otherwise.
Use `unknown` for evidence unavailable through denial. Describe omitted context
using the contract's stated mode. Do not infer stable IDs or live acceptance
from authored files.
