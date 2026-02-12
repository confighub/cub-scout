# Diagrams Index (D2 + SVG)

Human-first index for architecture and concept diagrams.

## Current Status

- Last reviewed: 2026-02-12
- Diagram sources and rendered outputs are in sync by filename.
- Current diagram set was last updated on 2026-01-22.

## How to Read This Folder

- `.d2` files are editable source diagrams.
- `.svg` files are the rendered artifacts used in docs.
- Update `.d2`, then re-render `.svg`.

## Diagram Catalog

| Topic | Source | Rendered | Last Updated |
|-------|--------|----------|--------------|
| Flux architecture | [flux-architecture.d2](flux-architecture.d2) | [flux-architecture.svg](flux-architecture.svg) | 2026-01-22 |
| Ownership detection | [ownership-detection.d2](ownership-detection.d2) | [ownership-detection.svg](ownership-detection.svg) | 2026-01-22 |
| Ownership trace | [ownership-trace.d2](ownership-trace.d2) | [ownership-trace.svg](ownership-trace.svg) | 2026-01-22 |
| Kustomize overlays | [kustomize-overlays.d2](kustomize-overlays.d2) | [kustomize-overlays.svg](kustomize-overlays.svg) | 2026-01-22 |
| Clobbering problem | [clobbering-problem.d2](clobbering-problem.d2) | [clobbering-problem.svg](clobbering-problem.svg) | 2026-01-22 |
| Upgrade tracing | [upgrade-tracing.d2](upgrade-tracing.d2) | [upgrade-tracing.svg](upgrade-tracing.svg) | 2026-01-22 |

## Regenerate SVGs

Requires D2 CLI.

```bash
d2 docs/diagrams/flux-architecture.d2 docs/diagrams/flux-architecture.svg
d2 docs/diagrams/ownership-detection.d2 docs/diagrams/ownership-detection.svg
d2 docs/diagrams/ownership-trace.d2 docs/diagrams/ownership-trace.svg
d2 docs/diagrams/kustomize-overlays.d2 docs/diagrams/kustomize-overlays.svg
d2 docs/diagrams/clobbering-problem.d2 docs/diagrams/clobbering-problem.svg
d2 docs/diagrams/upgrade-tracing.d2 docs/diagrams/upgrade-tracing.svg
```

## Refresh Checklist

1. Confirm diagram labels match current command names and docs terminology.
2. Re-render updated `.d2` sources to `.svg`.
3. Check references in `/docs/README.md` and concept/how-to pages.
4. Update this file's "Last reviewed" date.
