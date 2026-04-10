---
name: cub-scout
description: Use when working in the cub-scout repo or when answering what cub-scout can do versus ConfigHub/cub. Covers cub-scout's read-only observation role, connected comparison workflows, MCP surfaces, Git preview boundaries, and how to verify current command truth before claiming capability.
---

# cub-scout

Start here when the task is about:

- what `cub-scout` does today
- how `cub-scout` differs from `cub`
- whether a workflow is standalone, connected, or ConfigHub/cub
- AI/MCP usage of `doctor`, `explain`, `trace`, `map`, `scan`
- Git preview versus render/import boundaries

## Read first

1. `AI-README-FIRST.md`
2. `HANDOVER.md`
3. `docs/reference/commands.md`
4. `docs/reference/cli-contract.md`
5. `docs/reference/json-contracts.md`

If the user is asking you to *use* cub-scout against a real cluster (not work on the repo), load `docs/ai/cub-scout-tasks.md` instead — that file is task-oriented with concrete command flows for operator scenarios.

## Product value in one breath

`cub-scout` is the read-only Kubernetes and GitOps observer that helps operators and AI agents understand what is broken, who owns it, where it came from, and what to do next without starting from the Argo UI or mutating the cluster.

## Tool boundaries

### Use `cub-scout` for

- `doctor`, `explain`, `trace`, `map`, `scan`
- connected comparison such as `compare three-way`
- MCP serving through `mcp serve`
- local Git structure preview through `import --git-path` and `parse-repo`

### Use `cub` for

- ConfigHub intended-state workflows
- spaces, units, targets, workers
- `cub gitops discover`
- `cub gitops import`

### Do not blur these

- `cub-scout import --git-path` is a local structure/import-preview flow
- `cub gitops import` is target + render-target based
- SDK renderers are implementation detail for `cub`, not an implied `cub-scout` feature

## High-signal shipped capabilities

- `doctor` / `explain` with `--presentation` and `--hint-mode`
- truthful Argo ownership and phase-aware hints
- connected `compare three-way` with conformance + agreement summary
- secret evidence across trace, Crossplane, map issues, and TUI trace
- MCP standalone tools with `doctor` as the first troubleshooting tool
- Git preview with ApplicationSet git-generator support

## Open queue bias

Check GitHub and handover first, but current priority is:

1. `#370` structured action-typed hints in JSON and MCP
2. `#368` broader troubleshooting wedge versus Argo GUI
3. lower-priority polish / compat (`#359`, `#360`, `#362`)

## Verification rule

Do not invent command surfaces.

Verify from local help before claiming capability:

```bash
./cub-scout --help
./cub-scout doctor --help
./cub-scout explain --help
./cub-scout compare three-way --help
./cub-scout import --help
./cub-scout mcp serve --help
```

When the workflow crosses into ConfigHub:

```bash
cub gitops --help
cub gitops import --help
```

## Safety rule

- `cub-scout` is cluster read-only by default
- connected import writes inventory/state to ConfigHub, not cluster manifests
- prefer preview or dry-run paths first
