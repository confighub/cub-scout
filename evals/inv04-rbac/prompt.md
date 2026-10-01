---
name: inv04-rbac
description: "Recorded namespaced Deployment list results under readable and denied RBAC scopes."
expected_outcome: "Report the visible identities and scoped empty result, keep denied data unknown, and avoid global completeness or orphan claims."
tags: [benchmark-v1, INV-04, inventory, rbac, recorded]
max_turns: 6
timeout_seconds: 120
allowed_tools: [Read, Grep]
---

Use only the four supplied files in `cluster/`. They contain three sequential,
bounded namespaced Deployment list API responses and a compact capture-scope
record. They are not one atomic or cluster-wide snapshot. Do not use network,
cluster access, or other tools to fill gaps.

Report visible Deployment identities from the readable populated
response, including each UID and object resourceVersion. Report the Deployment
count for the readable-empty namespace. Interpret each response status and
error identity; do not treat an error as an empty response. Determine what, if
anything, the denied response establishes about Deployment count and
ownership. Assess whether these three scoped requests establish a complete
cluster inventory. Raw objects and
missing ownership markers do not prove an orphan or globally unmanaged state.

Return a bare JSON object with exactly these string-valued keys, each once, in
any order, without surrounding prose:
`populated_namespace`, `visible_deployments`, `empty_namespace`,
`empty_deployment_count`, `denied_namespace`, `denied_result`,
`denied_deployment_count`, `denied_ownership`, `readable_objects_orphan_status`,
`inventory_completeness`, `evidence`.

Encoding contract (these formats do not reveal the fixture answers):

- `visible_deployments`: comma-separated entries in response item order, each
  encoded as `namespace/name@uid#resourceVersion`.
- `empty_deployment_count`: decimal count scoped to the readable-empty request.
- `denied_result`: `FORBIDDEN` or `UNKNOWN`.
- `denied_deployment_count` and `denied_ownership`: use `UNKNOWN` when the
  denied response cannot establish them.
- `readable_objects_orphan_status`: `NOT_ESTABLISHED` when the evidence does not
  establish orphan status.
- `inventory_completeness`: `PARTIAL`, `COMPLETE`, or `UNKNOWN`.
- `evidence`: supplied filenames joined with `+` in this order:
  `capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json`.
