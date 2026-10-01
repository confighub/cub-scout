---
name: rul03-context
description: "Interpret a denied Kubernetes list alongside a readable explicit context without cross-context inference."
expected_outcome: "Report per-context observations and keep denied inventory unknown."
tags: [benchmark-v1, RUL-03, kubernetes, context, recorded]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the four files in `cluster/`. `capture-scope.json` and
`observer-context-map.json` are capture metadata; the two `.body` files are the
raw Kubernetes API responses. The observations were sequential, not an atomic
snapshot. Do not use network or live cluster access, and do not fill one
context's gaps from the other.

Compare the explicitly named context for each request with the captured
current context before and after. Interpret an HTTP error as the response it
actually is; do not turn denied access into an empty inventory. For the
readable list, report the exact observed Deployment identity. Do not infer
health, ownership, a fleet-wide result, or any implicit default-context query.

Return one bare JSON object with exactly these string-valued keys in this
order, without prose or additional properties:
`denied_context`, `denied_http_status`, `denied_list_result`,
`denied_inventory`, `readable_context`, `readable_list_count`,
`deployment_namespace`, `deployment_name`, `deployment_uid`,
`default_context_before`, `default_context_after`, `observation_scope`,
`evidence`.

Use `FORBIDDEN` or `READABLE_LIST` for `denied_list_result`; use `UNKNOWN` or
`KNOWN` for `denied_inventory`; set `observation_scope` to
`SEQUENTIAL_NON_ATOMIC`. `evidence` is the supplied filenames joined by `+` in
this exact order: `capture-scope.json+observer-context-map.json+rul03-denied-deployments.body+rul03-readable-deployments.body`.
