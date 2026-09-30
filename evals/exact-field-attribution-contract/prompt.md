---
name: exact-field-attribution-contract
description: "Read captured CLI/MCP and object evidence for one exact managedFields path; distinguish absent path from resource-level rollup."
expected_outcome: "Emit the recorded manager for the exact image path, preserve unknown for an absent path, and state the evidence boundary."
tags: [product-contract, attribution, exact-field]
max_turns: 10
timeout_seconds: 300
allowed_tools: [Read, Glob, Grep, Skill]
---

Read the source-pinned files in `evidence/exact-field-attribution/`. Use `after-explain.json` and `after-absent-path.json` for the bounded explain results; compare with `after-object.json` and `proof.json` as needed. These are captured responses, not independent live confirmation. Do not call MCP or claim that the Helm label proves Helm reconciliation. The observations are sequential, not atomic.

Return one JSON object only, with exactly these keys in this order; JSON whitespace is allowed:

- `resource`: `Deployment/api` (the captured CLI `resource` value)
- `namespace`: `team-01` (the captured CLI `namespace` value)
- `field`: exact `fieldAttribution` path, cause and managers from `after-explain.json`
- `absentField`: path, cause and reason from `after-absent-path.json`
- `humanActor`: `unknown`
- `latestWriter`: `unknown`
- `evidenceBoundary`: `recorded snapshot; sequential, not atomic; Helm-labelled fixture is not proof of Helm reconciliation`

Do not use the resource-level `mutationCause` or `mutationManager` as a substitute for `fieldAttribution`, and do not add any extra keys or claims.
