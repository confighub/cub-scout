# Exact field manager evidence

When the resource and exact field are known, `explain --field-path` returns the
manager evidence for that canonical FieldsV1 path only. It keeps the existing
resource-level mutation summary, but does not use it as a fallback for the
requested path.

```bash
./cub-scout explain Deployment/checkout -n shop \
  --field-path '.spec.template.spec.containers[name="checkout"].image' \
  --format json
```

A compact response may include:

```json
{
  "mutationCause": "manual-edit",
  "mutationManager": "kubectl-edit",
  "fieldAttribution": {
    "path": ".spec.template.spec.containers[name=\"checkout\"].image",
    "cause": "manual-edit",
    "managers": ["kubectl-set"]
  }
}
```

The resource-level fields summarize manager evidence across the object; only
`fieldAttribution` is scoped to the requested path. The sorted manager names
are presence evidence, not write chronology or a human identity. Missing,
malformed, unrecognized-only, or incomplete path evidence returns
`cause: "unknown"` with a reason. Wildcards are rejected. The bounded TUI
resource panel offers the same query with `f` after selecting a resource.

## Captured read-only proof

[`live-proof/`](live-proof/) contains a source-pinned CLI/MCP capture from
`9d3efad1078aa59824a7cc1868fef81d2591d8d2`; `proof.json` records SHA-256
hashes for the raw object, responses, tool catalogs, and binary. The same
Deployment UID and resourceVersion bracket the before/after request. The exact
image path resolves to the observed manager `helm`; `.spec.nonexistent`
resolves to `unknown` with a reason. The saved MCP `tools/list` response
includes `field_path`.

This is a sequential read of the Helm-labelled `team-01/api` scale fixture, not
proof of actual Helm reconciliation or a broader provenance claim. It does not
identify a person or establish write order. The live capture is implementation
evidence; deterministic tests and the separate
[`changed-by-checkout` eval case](../../../evals/changed-by-checkout/) cover the
genuine saved checkout managedFields record (its controller-manager name is
representative; no GitOps controllers were installed). No paid model run was
used.
