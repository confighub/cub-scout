---
name: recorded-explain-contract
description: "Interpret exact-object facts from a complete immutable recorded Kubernetes input. This case does not invoke cub-scout CLI or MCP."
expected_outcome: "Return the requested exact JSON fields; do not claim live observation or time-dependent results."
tags: [recorded-explain, product-contract, prepared-not-run]
max_turns: 8
timeout_seconds: 300
allowed_tools: [Read, Glob, Grep, Skill]
---

Read `cluster/deployments.yaml`, the complete recorded Kubernetes Deployments List, and `cluster/recording.json` for its byte hash and counts. Answer only for exact `apps/v1 Deployment shop/checkout`, not similarly named resources. Treat the recording as immutable input; it is not evidence of current cluster state. There is no trusted capture-time metadata, no desired-state export, and no controller/source/API evidence beyond the recorded object. This is a fixture interpretation case: do not claim you invoked `cub-scout`, CLI, TUI, or MCP.

Return one compact JSON object only with these keys in this order: `owner`, `health`, `healthMeasurement`, `mutationCause`, `mutationManager`, `fieldPath`, `recordedInputSha256`, `resourceRead`. Copy facts from the selected recorded object. For `healthMeasurement`, include only `status` and `scope`. Set `recordedInputSha256` to the SHA-256 in `cluster/recording.json`. Set `resourceRead` to `null` because no live read occurred. Use the exact managed field `.spec.template.spec.containers[name="checkout"].image`.

Do not infer current rollout freshness, delivery completion, a human actor, literal command arguments, or desired/live drift from this file. If evidence is insufficient for a requested field, use `UNKNOWN` rather than guessing.
