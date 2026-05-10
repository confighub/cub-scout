---
name: cub-scout
description: Use when working in the cub-scout repo or when answering what cub-scout can do versus ConfigHub/cub. Covers cub-scout's read-only observation role, connected comparison workflows, Model Context Protocol (MCP) surfaces, Git preview boundaries, and how to verify current command truth before claiming capability.
---

# cub-scout

Start here when the task is about:

- what `cub scout` does today
- how `cub scout` differs from `cub`
- whether a workflow is standalone, connected, or ConfigHub/cub
- AI / Model Context Protocol (MCP) usage of `doctor`, `explain`, `trace`, `map`, `scan`
- Git preview versus render/import boundaries

## Read first

1. `AI-README-FIRST.md`
2. `HANDOVER.md`
3. `docs/reference/commands.md`
4. `docs/reference/cli-contract.md`
5. `docs/reference/json-contracts.md`

For capability-triage and demo conversations such as "can cub scout do X?" or
"should I use cub scout or kubectl here?", also read
`references/capability-assistant.md`.

If the user is asking you to *use* cub-scout against a real cluster (not work on the repo), load `docs/ai/cub-scout-tasks.md` instead — that file is task-oriented with concrete command flows for operator scenarios.

## Product value in one breath

**cub scout observes and explains; it never decides.** It is the read-only
Kubernetes and GitOps observer — it surfaces evidence about ownership,
health, and drift, but it never mutates the cluster and never makes
authority calls about what *should* be true. ConfigHub (driven by `cub`) is
the authority; cub-scout is the witness.

`cub scout` is the preferred documented form. In this repo, use
`./cub-scout ...` for exact local commands.

## Tool boundaries

### Use `cub scout` for

- `doctor`, `explain`, `trace`, `map`, `scan`
- connected comparison such as `compare three-way`
- Model Context Protocol (MCP) serving through `mcp serve`
- local Git structure preview through `import --git-path` and `parse-repo`

### Use `cub` for

- ConfigHub intended-state workflows
- spaces, units, targets, workers
- `cub gitops discover`
- `cub gitops import`

### Do not blur these

- `cub scout import --git-path` is a local structure/import-preview flow
- `cub gitops import` is target + render-target based
- SDK renderers are implementation detail for `cub`, not an implied `cub scout` feature

## High-signal shipped capabilities

- `doctor` / `explain` with `--presentation` and `--hint-mode`
- truthful Argo ownership and phase-aware hints
- connected `compare three-way` with conformance + agreement summary
- secret evidence across trace, Crossplane, map issues, and TUI trace
- MCP standalone tools with `doctor` as the first troubleshooting tool, including cluster-access uncertainty such as wrong context, stale kubeconfig, or API reachability
- Git preview with ApplicationSet git-generator support

## Queue source

Do not rely on this file for current milestone state.

Use `HANDOVER.md` plus live GitHub issues for the active queue so the skill does
not become a second source of truth.

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
