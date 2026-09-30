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
