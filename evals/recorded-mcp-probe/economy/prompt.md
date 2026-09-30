---
name: recorded-explain-mcp
expected_outcome: "Report separately what the recorded ownership labels classify and which manager owns the requested managed field path."
tags: [recorded-explain, mcp-diagnostic, economy, prepared-not-run]
max_turns: 8
timeout_seconds: 90
allowed_tools: [Read, Glob, Grep, mcp__plugin_recorded-mcp-probe_cub-scout__explain]
---

Answer the question using the evidence you judge sufficient. `cluster/deployments.yaml` and `cluster/recording.json` are the same immutable historical input in both eval arms, not current cluster state. You may inspect the recording or use available tools; no particular tool, file-reading sequence, or amount of reading is required. Do not assume live state.

For `apps/v1 Deployment shop/checkout`, report the resource-level ownership classification and the manager names recorded for the exact field path `.spec.template.spec.containers[name="checkout"].image`. Keep those evidence types separate: `resourceOwner` comes from ownership labels/owner references on the object, while `fieldManagers` comes from `managedFields` for that exact path. A resource owner does not establish a field manager. Neither identifies a person, proves a literal command, or establishes the latest writer. If the evidence for a value is absent or ambiguous, use `UNKNOWN` for that value.

Return one plain JSON object only, with exactly these keys (any order): `resourceOwner`, `fieldPath`, `fieldManagers`, `recordedInputSha256`, `resourceRead`. Copy the field path and hash from the question/input. Set `resourceRead` to `null`: this task is about recorded evidence, and no live read occurred. Do not claim freshness, desired/live drift, delivery completion, or that one arm did less work. No additional prose.
