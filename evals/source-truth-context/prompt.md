---
name: source-truth-context
description: "Interpret selected Kubernetes context and incomplete source-truth evidence conservatively."
expected_outcome: "Treat alpha as a context label, keep ConfigHub separate, and never turn a denied controller read into PASS."
tags: [product-contract, context, recorded, source-truth]
max_turns: 5
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only `cluster/source-truth.json`. It is a synthetic contract fixture based
on deterministic tests, not a live-cluster or MCP capture.

Return one bare JSON object with exactly these string keys: `selected_context`,
`context_identity`, `confighub_status`, `controller_status`, `verdict`,
`config_hub_selected_by_kube_context`. Use `LABEL_ONLY` or `STABLE_IDENTITY`,
`PRESENT` or `UNAVAILABLE`, `PASS` or `NOT_PASS`, and `YES` or `NO` as
appropriate. A forbidden controller observation is unavailable evidence.
