# Concepts Guide

Human-first entry point for cub-scout concept docs.

## Start Here

If you only read 3 docs, read these in order:

1. [mental-model.md](mental-model.md) — What cub-scout does and how to think about it
2. [gitops-overview.md](gitops-overview.md) — GitOps basics and loop
3. [live-cluster-inference.md](live-cluster-inference.md) — How cub-scout infers structure from live data

Then go deeper with:

- [architecture.md](architecture.md) — Internal model and contract framing
- [state-and-snapshots.md](state-and-snapshots.md) — Session vs shareable state
- [tui-vs-gui.md](tui-vs-gui.md) — Scope boundaries between cub-scout and ConfigHub
- [clobbering-problem.md](clobbering-problem.md) — Why GitOps layering causes silent overrides
- [alternatives.md](alternatives.md) — Where cub-scout fits vs adjacent tools

## Doc Status

`Status` meaning:
- `Current (Primary)` = recommended first-stop conceptual docs
- `Current (Deep Dive)` = accurate but more specialized context

| Doc | Status | Last Reviewed |
|-----|--------|---------------|
| [mental-model.md](mental-model.md) | Current (Primary) | 2026-02-12 |
| [gitops-overview.md](gitops-overview.md) | Current (Primary) | 2026-02-12 |
| [live-cluster-inference.md](live-cluster-inference.md) | Current (Primary) | 2026-02-12 |
| [architecture.md](architecture.md) | Current (Deep Dive) | 2026-02-12 |
| [state-and-snapshots.md](state-and-snapshots.md) | Current (Deep Dive) | 2026-02-12 |
| [tui-vs-gui.md](tui-vs-gui.md) | Current (Deep Dive) | 2026-02-12 |
| [clobbering-problem.md](clobbering-problem.md) | Current (Deep Dive) | 2026-02-12 |
| [alternatives.md](alternatives.md) | Current (Deep Dive) | 2026-02-12 |

## Notes

- File age alone does not mean a doc is stale.
- Historical material is kept under [../archive/](../archive/).
- If command behavior differs from docs, the executable contract and golden tests win.
