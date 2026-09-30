---
name: recorded-explain-mcp
expected_outcome: "Report separately what the recorded ownership labels classify and which manager owns the requested managed field path."
tags: [recorded-explain, mcp-diagnostic, prepared-not-run]
max_turns: 4
timeout_seconds: 90
allowed_tools: [Read, Glob, Grep, mcp__plugin_recorded-mcp-probe_cub-scout__explain]
---

Read `cluster/deployments.yaml` and `cluster/recording.json`. These are identical immutable recorded inputs in both eval arms, not current cluster state. In the plugin arm, call the available `explain` MCP tool exactly once for `apps/v1 Deployment shop/checkout` and the field `.spec.template.spec.containers[name="checkout"].image`; in the no-plugin arm, derive the answer from the same raw fixture and do not attempt an unavailable MCP call. You may read the raw fixture to verify the returned facts.

Keep two kinds of evidence separate. `resourceOwner` is the resource-level ownership classification supported by labels/owner references on the recorded Kubernetes object. `fieldManagers` is the manager name recorded for the exact requested path in `managedFields`. Do not infer the field manager from the resource owner, or the resource owner from a field-manager string. Neither value identifies a human or proves a literal command.

Return one plain JSON object only, with exactly these keys (any order): `resourceOwner`, `fieldPath`, `fieldManagers`, `recordedInputSha256`, `resourceRead`. Derive the ownership classification, selected exact field path, and manager list from the evidence available in your arm. Copy the hash from `cluster/recording.json`. Set `resourceRead` to `null`: this is recorded evidence and no live read occurred. Do not claim freshness, desired/live drift, delivery completion, human identity, or reduced work from the tool call. If evidence is missing or ambiguous, report `UNKNOWN` for that value.
