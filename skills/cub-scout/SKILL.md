---
name: cub-scout
description: Use when working in the cub-scout repo or when answering what cub-scout can do versus ConfigHub/cub. Covers cub-scout's read-only observation role, connected comparison workflows, attribution layer (mutation cause + git source + ConfigHub binding source), Model Context Protocol (MCP) surfaces, Git preview boundaries, and how to verify current command truth before claiming capability.
---

# cub-scout

Start here when the task is about:

- what `cub scout` does today
- how `cub scout` differs from `cub`
- whether a workflow is standalone, connected, or ConfigHub/cub
- AI / Model Context Protocol (MCP) usage of `doctor`, `explain`, `trace`, `map`, `scan`
- attribution evidence on field mismatches (`cause`, `managerHint`, `gitSource`, `bindingSource`)
- Git preview versus render/import boundaries

## Read first

1. `AI-README-FIRST.md`
2. `HANDOVER.md`
3. `docs/reference/commands.md`
4. `docs/reference/cli-contract.md`
5. `docs/reference/json-contracts.md`

For capability-triage and demo conversations such as "can cub scout do X?" or "should I use cub scout or kubectl here?", also read `references/capability-assistant.md`.

If the user is asking you to *use* cub-scout against a real cluster (not work on the repo), load `docs/ai/cub-scout-tasks.md` instead — that file is task-oriented with concrete command flows for operator scenarios.

## Product value in one breath

**cub scout observes and explains; it never decides.** It is the read-only Kubernetes and GitOps observer — it surfaces evidence about ownership, health, drift, and provenance, but it never mutates the cluster and never makes authority calls about what *should* be true. ConfigHub (driven by `cub`) is the authority; cub-scout is the witness.

`cub scout` is the preferred documented form. In this repo, use `./cub-scout ...` for exact local commands.

## Capability groups (the verb map)

cub-scout commands fall into seven groups; see `README.md` § Capability Map for the full per-command table. The lens:

- **Observe** — `doctor`, `map`, `trace`, `tree`, `scan`, `graph`, `snapshot`, `watch`, `status` (cluster only)
- **Diagnose** — `explain`, `debug`, `suggest-remedy`, `patterns`, `gitops status` (cluster only)
- **Compare** — `compare`, `compare drift` (standalone); `compare three-way`, `compare source-truth` (connected)
- **Attribute** — `cause`/`managerHint`/`gitSource`/`bindingSource` surfacing on `compare` + `explain` output (mix of standalone + connected)
- **Ingest** — `import --git-path` (preview); `import argocd`, `import cluster-aggregator`, `import apply`, `app` (connected)
- **Govern** — `history`, `impact`, `fleet outliers`, `summary`, `views`, `audit`, `bundle`, `catalog` (connected)
- **Integrate** — `setup`, `quickstart`, `mcp serve`, `context-pack`, `version`

Standalone vs connected is preserved per-command — never blur the two.

## Tool boundaries

### Use `cub scout` for

- Observe: `doctor`, `explain`, `trace`, `map`, `scan`
- Diagnose: `explain`, `patterns`, `suggest-remedy`
- Compare: `compare three-way`, `compare source-truth`, `compare drift`
- Attribute: read provenance fields on the JSON output of `compare` and `explain`
- Model Context Protocol (MCP) serving through `mcp serve`
- Local Git structure preview through `import --git-path` and `import parse-repo`

### Use `cub` for

- ConfigHub intended-state workflows
- spaces, units, targets, workers
- `cub gitops discover`
- `cub gitops import`
- `cub link list / get` (the data feed for cub-scout's connected attribution layer)

### Do not blur these

- `cub scout import --git-path` is a local structure/import-preview flow
- `cub gitops import` is target + render-target based
- SDK renderers are implementation detail for `cub`, not an implied `cub scout` feature
- Attribution evidence on `compare`/`explain` JSON is read-only enrichment — never implies a write

## High-signal shipped capabilities

- **Attribution layer** (`#435`): `cause` + `managerHint` (managedFields-based controller-drift vs manual-edit), `gitSource{repoUrl, revision, path, file, line}` (Argo/Flux tracer + opt-in `--source-path` for raw-YAML back-resolution), `incomingBindings[]` + `bindingSource` (connected, via `cub link list`). Documented in `docs/reference/json-contracts.md` § Field Mutation Attribution Contract.
- **Source-truth contract** (`#393`, `#418`): `compare source-truth` with Phase 1 + Phase 2 strategies (9 total: `confighub-oci-argo`, `confighub-oci-flux`, `git-argo`, `git-flux`, `helm-flux`, `helm-argo`, `kustomize-flux`, `oci-flux`, `oci-argo`).
- **Views integration** (`#391`): `views resolve`, `views open`, `views project --with-reality`, `compare three-way --view`.
- **`doctor` / `explain`**: `--presentation` and `--hint-mode` for AI-friendly output.
- **Argo truth-and-guidance**: truthful ownership for ApplicationSet-managed resources, three-way disagreement, phase-aware hints.
- **`compare three-way`**: connected DRY/WET/LIVE, `--fail-on` conformance exit codes, agreement summary, `--source-path` opt-in for stage-B back-resolution.
- **MCP gateway** (`mcp serve`): `doctor` as the first standalone troubleshooting tool, including for local access uncertainty (wrong context, stale kubeconfig, API reachability).
- **Secret evidence** across trace, Crossplane, `map issues`, and TUI.
- **Git preview** with ApplicationSet git-generator support.

## Queue source

Do not rely on this file for current milestone state.

Use `HANDOVER.md` plus live GitHub issues for the active queue so the skill does not become a second source of truth.

## Verification rule

Do not invent command surfaces.

Verify from local help before claiming capability:

```bash
./cub-scout --help
./cub-scout doctor --help
./cub-scout explain --help
./cub-scout compare three-way --help
./cub-scout compare source-truth --help
./cub-scout import --help
./cub-scout views --help
./cub-scout mcp serve --help
```

When the workflow crosses into ConfigHub:

```bash
cub gitops --help
cub gitops import --help
cub link --help
cub link list --help
```

## Safety rule

- `cub-scout` is cluster read-only by default
- connected import writes inventory/state to ConfigHub, not cluster manifests
- prefer preview or dry-run paths first
- attribution evidence is enrichment, never mutation
