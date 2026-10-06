---
name: changeorder-read-contract
description: "Read exact-space reported ChangeOrder declarations without evaluating governance."
expected_outcome: "Select prod, preserve declaration ordering and retain unknown evaluated outcomes."
tags: [product-contract, changeorder, authored, connected]
max_turns: 5
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only `cluster/inputs.json`. These are authored SDK-shaped inputs, not captured
CLI output or evidence of a running server. Select the requested exact space from
two same-slug ChangeOrders. Report Stage/State verbatim and the stored stage order
and first-stage prerequisite order. A GET declaration is not a prerequisite,
approval, publication, advancement or runtime health evaluation, even when Stage
is Completed. Handle the separate missing-workflow and denied response without
inventing an ungoverned or approved state. The source pin identifies an inspected
parser contract and does not identify the runtime server version.

Return one bare JSON object in this exact key order: `selected_space`, `order`, `stage`, `state`, `stages`, `review_prerequisites`, `evaluation`, `approval`, `runtime_health`, `absent_workflow_governance`, `denied_evaluation`, `runtime_server_version`, `provenance`, `live_acceptance_proven`.
Use arrays of strings for stages and review_prerequisites, a boolean for
live_acceptance_proven, and strings otherwise. Use `unknown` when the inputs
and explicit contract limits cannot establish a value.
