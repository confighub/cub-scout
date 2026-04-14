# AI Read Me First

This is the repo-specific cold-start guide for Claude, Codex, and other AI coding agents.

If your AI host supports repo-local skills, load [skills/cub-scout/SKILL.md](skills/cub-scout/SKILL.md) after this file.

If you are starting work in this repo, read files in this order:

1. `AI-README-FIRST.md`
2. `HANDOVER.md`
3. `CLAUDE.md`
4. `docs/reference/commands.md`
5. `docs/reference/cli-contract.md`
6. `docs/reference/json-contracts.md`

Use these for different AI scenarios:
- `skills/cub-scout/references/capability-assistant.md` — capability-assistant profile (answering "can cub-scout do X?")
- `docs/ai/cub-scout-tasks.md` — task skill for *using* cub-scout to investigate a real cluster
- `docs/howto/using-cub-scout-from-ai-tool.md` — demo/operator flows

## Tool Boundaries

### `cub-scout`

`cub-scout` is the read-only Kubernetes and GitOps observer.

Use it for:
- `doctor`, `explain`, `trace`, `map`, `scan`
- connected comparison such as `compare three-way`
- local Git structure discovery with `import --git-path` and `parse-repo`
- MCP serving via `mcp serve`

Important:
- `cub-scout` is cluster read-only by default
- connected imports write inventory/state to ConfigHub, not cluster manifests
- `import --git-path` is a structure/import-preview path, not a manifest renderer

### `cub`

`cub` is the ConfigHub CLI for intended-state workflows.

Use it for:
- spaces, units, targets, workers, and ConfigHub state
- `cub gitops discover`
- `cub gitops import`

Current local CLI truth:
- `cub gitops discover --space <space> <target-slug>`
- `cub gitops import --space <space> <target-slug> <render-target-slug>`

Important:
- `cub gitops import` is target + render-target based
- it is not a local `--git-path` renderer

### `confighub/sdk`

The SDK contains renderer and bridge implementation details used by `cub`.

Do not claim that `cub-scout` can do SDK/renderer work locally unless the current repo code and CLI help actually expose that path.

## Current High-Signal Shipped Capabilities

As of 2026-04-09, these areas are fully or materially shipped:

- `doctor` / `explain`
  - `--presentation human|ai|paired`
  - `--hint-mode default|beginner|operator`
- Argo truth-and-guidance track
  - truthful `explain` ownership for ApplicationSet-managed resources
  - connected three-way disagreement surfacing
  - phase-aware next-step hints
- `compare three-way`
  - connected DRY/WET/LIVE comparison
  - `--fail-on` conformance exit codes
  - agreement/convergence summary in CLI + JSON
- MCP gateway
  - `mcp serve` exposes `doctor` as the first standalone troubleshooting tool, including when the problem might be local access uncertainty such as wrong context, stale kubeconfig, or API reachability
  - standalone tool set: `doctor`, `explain`, `map`, `scan`, `trace`
  - connected mode adds read-only ConfigHub query tools
- Secrets track
  - trace secret evidence
  - Crossplane `ProviderConfig` secret evidence
  - `map issues` secret findings
  - TUI secret panel
- Git import track
  - `import --git-path` local preview
  - ApplicationSet git-generator support in parser
  - path-centric duplicate-safe proposal identifiers

## Current Open Queue

Verify live state before acting, but the current open follow-ons are:

- `#370` structured action-typed next-step hints in JSON and MCP outputs
- `#368` broader “beat Argo CD GUI” troubleshooting umbrella
- `#364` investigated render-integration follow-on
- `#359`, `#360`, `#362` lower-priority polish / compat / test stability

## Non-Negotiables

1. Do not invent command surfaces.
2. Verify current behavior from local help before claiming support.
3. Prefer `./cub-scout` in local repo workflows.
4. Keep cluster read-only behavior separate from ConfigHub writes.
5. Treat `cub-scout` and `cub gitops import` as complementary, not interchangeable.
6. Preserve deterministic facts over optimistic guidance.

## Quick Reality Checks

Use these before answering capability or workflow questions:

```bash
./cub-scout version
./cub-scout --help
./cub-scout doctor --help
./cub-scout explain --help
./cub-scout compare three-way --help
./cub-scout import --help
./cub-scout parse-repo --help
```

When the question crosses into ConfigHub or renderer workflows:

```bash
cub version
cub gitops --help
cub gitops import --help
```

## Best Next Read Based On Intent

- implementing or reviewing code: `HANDOVER.md`
- checking exact flags or stable command surfaces: `docs/reference/commands.md` and `docs/reference/cli-contract.md`
- checking JSON outputs or MCP-adjacent facts: `docs/reference/json-contracts.md`
- capability-assistant or demo flow: `docs/howto/using-cub-scout-from-ai-tool.md`
