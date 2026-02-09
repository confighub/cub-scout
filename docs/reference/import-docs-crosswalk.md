# Import Docs Crosswalk

This page maps archived import docs to the current canonical docs.

Use this page when you find an old import doc in `docs/archive/` and want the maintained equivalent.

## Canonical Docs (Current)

- [How To: Import Workloads to ConfigHub](../howto/import-to-confighub.md)
- [Pipeline Source Resolution](pipeline-source-resolution.md)
- [GitOps Patterns Reference](gitops-patterns.md)
- [View Hierarchies with tree](../howto/tree-hierarchies.md)
- [Trace Context Troubleshooting](../howto/trace-context-troubleshooting.md)
- [Commands Reference](commands.md)
- [Roadmap](../roadmap.md)

## Archive-to-Current Mapping

| Archived doc | Use this now | Notes |
|---|---|---|
| `docs/archive/IMPORTING-WORKLOADS.md` | `docs/howto/import-to-confighub.md`, `docs/reference/commands.md` | Old wizard/flags examples are historical. Validate against current command docs. |
| `docs/archive/JOURNEY-IMPORT.md` | `docs/howto/import-to-confighub.md`, `docs/howto/trace-context-troubleshooting.md` | Journey narrative kept for background only. |
| `docs/archive/IMPORT-FROM-LIVE.md` | `docs/reference/pipeline-source-resolution.md`, `docs/reference/gitops-patterns.md` | LIVE-only inference guidance is historical framing, not contract text. |
| `docs/archive/IMPORT-FROM-SOURCES.md` | `docs/reference/gitops-patterns.md`, `docs/roadmap-1x-connected-upsell.md` | TUI vs GUI split remains directional; connected details evolve in 1.x. |
| `docs/archive/IMPORT-GIT-REFERENCE-ARCHITECTURES.md` | `docs/reference/gitops-patterns.md`, `docs/howto/tree-hierarchies.md` | Architecture examples are reference patterns, not strict runtime guarantees. |

## Scope Notes

- Archive docs are preserved for context and examples.
- Canonical behavior for current releases is defined by docs outside `docs/archive/`.
- For future connected features (for example ApplicationSet generator visualization), treat archive docs as provisional and track current roadmap/issues.
