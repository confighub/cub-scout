# JSON Contracts and Output Model

Start here for machine-readable output contracts.

## TL;DR

1. JSON is the canonical data contract.
2. ASCII/Markdown are deterministic renderings of the same facts.
3. There is no single monolithic schema for every command in current releases.
4. The old v0.14 schema doc is historical and archived.

## Fast Entry Points

| If you need... | Read this |
|----------------|-----------|
| JSON vs ASCII meaning model | [../semantic-contract.md](../semantic-contract.md) |
| Stable command and flag surface | [cli-contract.md](cli-contract.md) |
| Exact command usage and JSON-capable flags | [commands.md](commands.md) |
| Historical v0.14 schema document | [../archive/v0.14-json-schema.md](../archive/v0.14-json-schema.md) |

## Field Naming Conventions

JSON field casing is **per-surface and frozen by compatibility**. No global normalization.

| Surface | Convention | Example fields |
|---------|------------|----------------|
| CLI commands (map, trace, bundle summarize) | camelCase | `formatVersion`, `capturedAt`, `driftCount` |
| Debug bundle metadata | camelCase | `cubScoutVersion`, `createdAt`, `gitContext` |
| Versioned schema artifacts (graph, catalog, bundle-diff/timeline, checkpoints) | snake_case + `schema_version` | `schema_version`, `join_mode`, `bundle_count` |

Existing surfaces keep their original field names — no renames for style consistency.
When fields cross surface boundaries, mapping is explicit (e.g., metadata `createdAt` → catalog `created_at`).

## Current Contract Sources by Surface

| Surface | Primary contract doc | Schema version signal |
|---------|----------------------|-----------------------|
| Graph export/explain | [graph-contract.md](graph-contract.md) | `graph.v1` |
| Patterns | [patterns-contract.md](patterns-contract.md) | `patterns.v1` |
| Bundle diff | [../bundle-diff.md](../bundle-diff.md) | `bundle-diff.v1` |
| Bundle timeline | [../bundle-timeline.md](../bundle-timeline.md) | `bundle-timeline.v1` |
| Catalog | [../catalog.md](../catalog.md) | `catalog.v1` |
| Evidence export | [evidence-export-v1.md](evidence-export-v1.md) | `evidence-export.v1` |
| General CLI JSON behavior | [cli-contract.md](cli-contract.md) + [commands.md](commands.md) | Command-specific |
| GitOps checkpoint proposal schemas | [gitops-checkpoint-schemas.md](gitops-checkpoint-schemas.md) | `change-intent.v1`, `execution-report.v1`, `change-interaction-card.v1`, `decision-receipt.v1`, `execution-receipt.v1`, `outcome-receipt.v1` |

## Tree / Map / Trace / Drift JSON Today

These surfaces are documented as command contracts and deterministic output behavior, not a single shared JSON schema file.

Use this sequence:

1. Command behavior and flags: [commands.md](commands.md)
2. JSON vs ASCII model: [../semantic-contract.md](../semantic-contract.md)
3. Stability + source-of-truth rule: [cli-contract.md](cli-contract.md)
4. Real output fixtures: `test/golden/`

Useful golden directories:

- `test/golden/map-list-json/`
- `test/golden/map-deployers-json/`
- `test/golden/ownership/`
- `test/golden/trace/`
- `test/golden/map-status/`
- `test/golden/bundle-summarize/`

## Historical Note

`v0.14-json-schema.md` is preserved for historical reference. It should not be treated as the canonical contract for current releases.

## Related Repo Tooling (Codex Handoff)

The Codex task handoff schema added for automation handoffs lives at:

- `tools/codex-task-output/codex-task-output.schema.json`
