# `cub` vs `cub scout`: Family Boundary

> **Audience:** operators, AI tool authors, and anyone deciding which tool to reach for first.
> **Status:** Authoritative for the `v2.x` line.
> **Related:** [`why-connected-mode.md`](why-connected-mode.md#interface-boundaries-authoritative), [`v2.0.0-plugin-plan.md`](../releases/v2.0.0-plugin-plan.md).

## One-Sentence Split

- **`cub`** is the authority. It owns intended state and governed execution.
- **`cub scout`** is the explorer. It reads, explains, compares, and proves.

## When To Reach For Which

| You want to... | Use |
|---|---|
| See what's broken in a cluster | `cub scout doctor` |
| Explain one resource | `cub scout explain` |
| Trace ownership to its source | `cub scout trace` |
| Compare ConfigHub intent to live state | `cub scout compare three-way` |
| Map workloads / find issues in inventory | `cub scout map workloads` / `cub scout map issues` |
| Run config/risk scans | `cub scout scan` |
| Expose read-only evidence to AI | `cub scout mcp serve` |
| Create, update, apply, or approve governed units | `cub unit ...` |
| Manage spaces, targets, workers | `cub space ...` / `cub target ...` / `cub worker ...` |
| Run a governed ChangeSet | `cub changeset ...` |
| Discover GitOps resources in a cluster | `cub gitops discover` |
| Import governed state from a cluster via render targets | `cub gitops import` |
| Log into ConfigHub | `cub auth login` |

The rule of thumb: **read and prove → `cub scout`. Write and govern → `cub`.**

## Product Boundary

### `cub scout` is for

- observation
- explanation
- comparison
- troubleshooting
- proof
- AI investigation routing

### `cub scout` is not for

- approvals
- governed execution ownership
- apply / mutate control-plane authority
- replacing provider-owned action surfaces

### `cub` is for

- spaces, units, targets, workers, and ConfigHub state
- governed ChangeSets and receipts
- `cub gitops discover` and `cub gitops import` (cluster → ConfigHub import via render targets)
- authority over intended state and mutation

### `cub` is not for

- standalone Kubernetes observation without ConfigHub
- ownership classification of Flux/Argo/Helm/native resources
- read-only ASCII/JSON explanation surfaces tuned for AI agents
- local Git-path structure import previews

## Overlaps That Look Confusing (And Aren't)

### Import paths

Two different import surfaces exist. They are complementary, not interchangeable:

- **`cub-scout import --git-path ./repo`** — parses a Git repository locally, produces an import **preview** (structure, proposals, duplicate-safe identifiers). No cluster, no render target, no write. Use to answer "what would this repo import as?"
- **`cub gitops import --space <space> <target-slug> <render-target-slug>`** — discovers ArgoCD/Flux resources **in a live cluster**, renders via ArgoCD API or Flux renderer, and writes the result as ConfigHub units. Use to actually perform the import.

`cub scout` scouts. `cub gitops import` renders and writes. The two can feed each other: scout to understand, `cub gitops import` to execute.

### Comparison paths

- **`cub scout compare three-way`** — DRY (ConfigHub intent) vs WET (rendered deployer state) vs LIVE (cluster). Read-only. Produces an agreement summary, per-resource pattern classification, and canonical ConfigHub trust URLs. Use for "does governed state agree with live state?"
- **`cub unit ...`** (`cub unit get`, `cub unit list`) — exact governed facts for one or many units. No cluster comparison. Use for "what is the last applied revision of unit X?"

Use `cub scout compare three-way` as the read entry point, then `cub unit get` for the exact governed facts once the scope is narrowed.

### Health and status

`cub scout doctor` and `cub scout explain` surface cluster health and ownership. `cub` does not compete with these. If you see a `cub` command that prints cluster health, it is using `cub scout` output under the hood or it is a different concept (governed unit status, not live cluster status).

## MCP Boundary

`cub scout mcp serve` is the **explorer and investigation gateway** for AI agents.

It is:

- the MCP front door for understanding and proving ConfigHub-connected reality
- the read-first AI investigation gateway
- the evidence coordinator across live systems and connected context
- the routing and explanation layer for troubleshooting workflows

It is **not**:

- the full ConfigHub API over MCP
- the approval or apply gateway
- the control plane
- the mutation owner for ConfigHub or provider systems

Mutation and governed execution stay with `cub` and the owning provider surfaces.

## Invocation Forms

During the `v2.x` line, `cub scout` can be invoked three ways:

1. **Plugin form (preferred):** `cub scout ...` — via `cub plugin install confighub/cub-scout`
2. **Standalone binary:** `cub-scout ...` — via Homebrew, krew, direct download, or source build
3. **Local checkout:** `./cub-scout ...` — when working inside the `cub-scout` repository

All three invoke the same binary with the same arguments. Flags, exit codes, JSON contracts, and MCP tool names are identical.

See the [migration guide](../releases/v2.0.0-migration-guide.md) for switching between forms.

## Authority and Execution

- `cub` holds credentials and context (`cub auth login`, `cub context ...`).
- `cub scout` in plugin form inherits `CUB_TOKEN`, `CUB_SERVER`, `CUB_CONTEXT`, `CUB_SPACE` from the parent `cub` process.
- `cub scout` in standalone form talks to `cub auth get-token` or reads its own token store when connected features are used.
- Neither form of `cub scout` ever performs ConfigHub writes. Writes go through `cub`.

## The Read-First Contract

Both product lines preserve:

1. **Read-only by default.** `cub scout` never modifies cluster state. Connected imports write inventory to ConfigHub, not manifests to the cluster.
2. **Parse, don't guess.** Ownership comes from labels and annotations, not heuristics.
3. **Deterministic facts over optimistic status.** If status is uncertain, say so and show the evidence.
4. **Graceful degradation.** Missing metadata surfaces as "unknown," never as a false claim.

See [`CLAUDE.md`](../../CLAUDE.md) and [`docs/semantic-contract.md`](../semantic-contract.md) for the full principles.

## Quick Reality Check

When in doubt, run:

```bash
cub --help
cub gitops --help
cub unit --help
cub scout --help           # or: cub-scout --help
cub scout doctor --help
```

If a command surface is not shown in local help, it does not exist. Do not infer capabilities from prose.

## See Also

- [`v2.0.0-plugin-plan.md`](../releases/v2.0.0-plugin-plan.md) — the plugin switchover plan
- [`v2.0.0-migration-guide.md`](../releases/v2.0.0-migration-guide.md) — step-by-step migration
- [`plugin-install.md`](../howto/plugin-install.md) — how to install the `cub scout` plugin
- [`host-plugin-compatibility.md`](../reference/host-plugin-compatibility.md) — version compatibility matrix
- [`why-connected-mode.md`](why-connected-mode.md) — connected-mode rationale and paid value boundary
