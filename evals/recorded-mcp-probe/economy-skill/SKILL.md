---
name: recorded-field-attribution
description: Use when asked which manager is recorded for a field on a known Kubernetes resource, especially when the resource and exact canonical field path are supplied. Route a known exact-path question directly to the recorded `explain` MCP tool instead of doing broad rediscovery first. This is a preference for compact evidence, not a requirement to call a tool.
---

# Recorded field attribution

For a known resource and exact canonical field path, prefer one direct call to
the available `explain` MCP tool with the resource and `field_path`. The returned
field-attribution evidence is scoped to that path; it is not a substitute for
the ordinary resource summary. Do not call inventory, health, lineage, or
comparison tools first solely to rediscover manager evidence.

Do not guess a canonical path. If the resource or path is not known, use only
the recorded evidence already available to identify it, or ask for the missing
detail. Raw recorded object evidence may still be read when it is needed to
check a result or understand an unknown response; the compact tool result does
not prohibit that drill-down.

Treat missing, malformed, unrecognized, or ambiguous path evidence as unknown,
without substituting resource-level manager summaries. Manager strings do not
identify a person or prove the latest writer. Resource ownership classification
and field-manager evidence are separate facts. This package is recorded-only:
never fall back to a live cluster, kubectl, shell, or an unrelated source.
