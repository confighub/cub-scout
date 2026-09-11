---
name: scout-mcp
description: 'Use when the user wants to expose cub-scout to an AI agent over MCP, wants a deterministic AI-ready snapshot of cluster context, or wants cub-scout running as a read-only event bot — the Integrate verb group. Natural phrasing: "start the MCP server", "expose cub-scout to Claude / Codex / my agent", "give me an AI-ready context pack of this cluster", "set up the MCP gateway", "what MCP tools does cub-scout expose?", "run cub-scout as a bot", "produce JSON context for an LLM to reason about this cluster". Load whenever intent is mcp / mcp-gateway / mcp-serve / context-pack / bot / agent-context / ai-snapshot / llm-context / model-context-protocol. Do NOT load for: actually running a non-integrate verb (those are scout-observe / scout-diagnose / scout-compare / scout-attribute / scout-ingest / scout-govern / scout-verify), or `cub` MCP integration (that''s a separate tool surface).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout mcp *) Bash(cub-scout mcp *) Bash(cub scout mcp *) Bash(./cub-scout context-pack *) Bash(cub-scout context-pack *) Bash(cub scout context-pack *) Bash(./cub-scout bot *) Bash(cub-scout bot *) Bash(cub scout bot *) Bash(./cub-scout version) Bash(cub-scout version) Bash(cub scout version) Bash(./cub-scout status) Bash(cub-scout status) Bash(cub scout status) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl config current-context) Bash(cub auth status) Bash(cub * get) Bash(cub * list)
---

# scout-mcp

The Integrate verb group of cub-scout. Three ways to plug cub-scout into AI workflows:

- **`mcp serve`** — long-running Model Context Protocol gateway exposing cub-scout's verbs as MCP tools for Claude / Codex / any MCP-aware agent
- **`context-pack`** — one-shot deterministic JSON context bundle an agent can consume directly
- **`bot`** — in-cluster event producer using the same read-only watch stream as `cub-scout watch`

Plus the version/status meta-commands the harness uses for cold-start checks.

## When to use

Explicit phrasings:

- "Start the MCP server" / "expose cub-scout to my agent over MCP"
- "Set up cub-scout as an MCP tool for Claude / Codex"
- "Give me an AI-ready context pack of this cluster"
- "What MCP tools does cub-scout expose?"
- "Produce JSON context an LLM can reason about" / "deterministic context snapshot"
- "Run cub-scout as an in-cluster bot"
- "Bootstrap the MCP gateway"
- "Plug cub-scout into the agent"

Implicit intents:

- The user is integrating cub-scout with an LLM / agent framework
- The user wants a *deterministic* (no AI, no inference) snapshot for an LLM to reason over
- The user is debugging MCP tool registration / tool listing / tool calls
- The user is composing cub-scout with `cub`-side MCP tools

## Do not load for

- Running a single cub-scout verb directly from the CLI — load the verb's own skill (`scout-observe`, etc.)
- `cub` MCP integration (the ConfigHub-side MCP surface) — that's a separate skill set
- Generating receipts — [`scout-verify`](../scout-verify/SKILL.md)
- Comparing live state — [`scout-compare`](../scout-compare/SKILL.md)

## Standalone vs connected

- **Standalone (cluster only):** `mcp serve` exposes the read-only standalone tool set (doctor, explain, map, scan, trace, compare drift, etc.). `context-pack` produces a cluster snapshot with attribution evidence but no ConfigHub linkage. `bot` streams watch events using kubeconfig or in-cluster service-account credentials.
- **Connected (cluster + `cub auth login`):** `mcp serve` additionally registers the connected-mode tools (`compare_three_way`, `compare_source_truth`, `confighub_changesets`, `confighub_k8s_resources`, `confighub_k8s_types`, `confighub_live_status`, `confighub_releases`, `confighub_resources`, `confighub_unit_events`, `confighub_units`, `confighub_unit_get`). Connected `context-pack` includes ConfigHub unit linkage and connected attribution (`bindingSource`).
- **Offline (no cluster):** `mcp serve` refuses to start without a reachable cluster. `context-pack` works against debug bundles via `--bundle <path>`.

## Tool boundary

- **Allowed (read-only):** `mcp serve` (long-running but no mutation), `context-pack` (deterministic snapshot), `bot` (long-running event stream but no mutation), `version`, `status`; `kubectl get/describe`, `kubectl config current-context`; `cub * get/list`, `cub auth status`.
- **Not allowed:** `mcp serve` exposes ONLY the read-only cub-scout verbs as tools. The MCP tool catalog explicitly excludes any mutating verb (`demo`, `import apply`, `compare --suggest --apply`) — see `cmd/cub-scout/mcp.go` `RegisterTools` for the closed list. `context-pack` never writes; it reads + emits.
- **MCP and the read-only triad:** the MCP gateway is the third party in the architectural triad. cub-scout (evidence) ↔ MCP (transport) ↔ Pilot or agent (judge / actor). The MCP layer NEVER decides; it just streams cub-scout's structured output.

## The verb menu

| Question | Verb | Notes |
|---|---|---|
| "Start the MCP gateway" | `cub-scout mcp serve` | Long-running stdio transport. |
| "What MCP tools are registered?" | MCP `tools/list` request | Dumps the tool catalog through the standard MCP discovery request. Useful for agent registration debugging. |
| "Produce deterministic AI context" | `cub-scout context-pack --format json` | One-shot. Cluster snapshot + ownership classifications + attribution evidence in a single JSON. |
| "Pack a bundle as AI context" | `cub-scout context-pack --bundle <path>` | Same shape, sourced from a debug bundle instead of the live cluster. Offline-friendly. |
| "Run cub-scout as an event bot" | `cub-scout bot --webhook <url>` | In-cluster-friendly watch stream with env-var config for manifests. |
| "Check cub-scout version" | `cub-scout version` | Build tag. Agents that registered cub-scout tools use this to detect breaking-change ranges. |
| "Confirm mode" | `cub-scout status` | Standalone vs connected. Agents call this first to know which connected tools are available. |

## The loop

1. **Decide MCP vs context-pack vs bot.** Long-running agent tool gateway → `mcp serve`. Single-shot reasoning over a snapshot → `context-pack`. In-cluster event producer → `bot`.
2. **Check the mode.** `cub-scout status` confirms standalone vs connected. Tell the agent which tool subset is live.
3. **Start the surface.** For MCP, `mcp serve` registers tools with the agent. For context-pack, `--format json` and pipe to the agent. For bot mode, set destination env vars or flags and deploy/run the process.
4. **Compose with sibling skills.** The MCP gateway is plumbing — once it's running, every other verb-group skill (`scout-observe`, `scout-diagnose`, etc.) is reachable via MCP tools and uses the same loops described in those skills.
5. **Hand off mutations.** No matter what an agent asks the MCP gateway to do, the gateway refuses to call a mutating verb. Mutations route through `cub` skills (in [`confighub/confighub-skills`](https://github.com/confighub/confighub-skills)) or direct `kubectl` with the user driving.

## Worked examples

### A: MCP STDIO for Claude Code

```jsonc
// .claude/mcp_servers.json (or equivalent for your agent host)
{
  "mcpServers": {
    "cub-scout": {
      "command": "cub-scout",
      "args": ["mcp", "serve"],
      "env": {
        "KUBECONFIG": "${env:HOME}/.kube/config"
      }
    }
  }
}
```

The agent now sees cub-scout's read-only tools (`doctor`, `explain`, `trace`, `compare_three_way`, `confighub_live_status`, etc.) and can call them like any other MCP tool.

### B: list the tool catalog

Use the standard MCP `tools/list` request from the agent host. The standalone
catalog contains `doctor`, `explain`, `map`, `scan`, and `trace`; connected mode
adds the ConfigHub tools listed below.

Every tool is a thin wrapper over the underlying CLI verb with `--format json` — see `docs/reference/json-contracts.md` for the response shape.

### C: deterministic context pack for an LLM

```bash
$ cub-scout context-pack --format json > cluster-ctx.json
$ jq '.ownership | group_by(.type) | map({type: .[0].type, count: length})' cluster-ctx.json
[
  {"type": "argo",     "count": 47},
  {"type": "flux",     "count": 12},
  {"type": "helm",     "count": 8},
  {"type": "native",   "count": 23},
  {"type": "unknown",  "count": 3}
]
```

The pack is **deterministic** — same cluster state + same cub-scout version → same JSON bytes. Safe to feed to an LLM with prompt caching, and safe to diff in CI.

### D: connected MCP for promotion workflows

When the agent is connected (`cub auth status` returns OK), `mcp serve` additionally registers:

- `compare_three_way` — DRY/WET/LIVE comparison
- `compare_source_truth` — strategy-typed verdict (requires `target` + `namespace` + `strategy`)
- `confighub_changesets` — governed ChangeSet history from ConfigHub
- `confighub_k8s_types` — Resource-backed intended Kubernetes type survey
- `confighub_k8s_resources` — Resource-backed intended Kubernetes resource reader
- `confighub_live_status` — Space live-status writeback with freshness
- `confighub_releases` — bounded ConfigHub Release history
- `confighub_resources` — generic ConfigHub Resource entity query
- `confighub_unit_events` — bounded ConfigHub UnitEvent history
- `confighub_units` — ConfigHub unit + fleet inventory
- `confighub_unit_get` — exact ConfigHub unit details + applied/live revision

These are the typical acceptance-kernel inputs. The agent composes them;
cub-scout produces evidence; the downstream judge decides.

## Output evidence

### `mcp serve` (the gateway)

- Long-running. STDIO transport.
- Lists tools via the standard MCP `tools/list` request.
- Each tool call returns `content[0].text` containing the wrapped CLI verb's `--format json` output verbatim.
- Optional `structuredContent` field on selected connected-mode tools adds parsed evidence + read-only trust guidance for Pilot-shaped consumers.

### `context-pack` (the snapshot)

| Field | Meaning |
|---|---|
| `version` | cub-scout build tag + context-pack schema version |
| `capturedAt` | RFC 3339 timestamp |
| `cluster` | kubeconfig context + server URL |
| `mode` | `standalone` or `connected` |
| `resources[]` | One entry per workload, with kind / name / namespace / ownership classification |
| `attribution[]` | Per-resource attribution evidence (cause / managerHint / gitSource / bindingSource) |
| `omissions[]` | What the snapshot deliberately did NOT include (e.g., crashlooping pods skipped, unsupported kinds) |

The snapshot is the *evidence base* a downstream LLM reasons over. It is NOT a recommendation; the LLM forms recommendations.

## MCP tool catalog (current)

The closed list of MCP tools cub-scout registers. The set is verified by `cmd/cub-scout/mcp_test.go` against the connected/standalone modes.

| Tool name | Mode | Wraps |
|---|---|---|
| `doctor` | standalone | `cub-scout doctor --format json` plus optional `--with-confighub` delivery evidence flags |
| `map` | standalone | `cub-scout map list --json` |
| `scan` | standalone | `cub-scout scan --json` |
| `trace` | standalone | `cub-scout trace <resource> --format json` |
| `explain` | standalone | `cub-scout explain <resource> --format json` |
| `gitops_status` | standalone | `cub-scout gitops status --format json` |
| `compare_three_way` | connected | `cub-scout compare three-way --format json` |
| `compare_source_truth` | connected | `cub-scout compare source-truth <target> -n <ns> --strategy <s> --format json` |
| `confighub_changesets` | connected | `cub changeset list --json` (calls `cub`, not cub-scout) |
| `confighub_k8s_resources` | connected | `cub k8s get <type> [<name> ...] -o json` (calls `cub`) |
| `confighub_k8s_types` | connected | `cub k8s types [<type>] -o json` (calls `cub`) |
| `confighub_live_status` | connected | `cub space list -o json --select Slug,SpaceID,Annotations,Labels` (calls `cub`) |
| `confighub_releases` | connected | `cub release list --space <space> -o json` (calls `cub`) |
| `confighub_resources` | connected | `cub resource list --space <space> -o json` (calls `cub`) |
| `confighub_unit_events` | connected | `cub unit-event list [unit] --space <space> -o json` (calls `cub`) |
| `confighub_units` | connected | `cub unit list --json` (calls `cub`) |
| `confighub_unit_get` | connected | `cub unit get --json <unit>` (calls `cub`) |

6 standalone + 11 connected = **17 tools total**. The catalog is intentionally narrow: every tool is read-only, every tool has a stable JSON contract, every tool is exercised by an MCP integration test.

For the full reference (per-tool parameters, return shape, when-to-load semantics, deferred verbs not in the catalog), see [`references/mcp-tool-catalog.md`](../references/mcp-tool-catalog.md).

## References

- Command reference: [`docs/reference/commands.md`](../../docs/reference/commands.md) § mcp, § context-pack
- MCP tool descriptions audit: `cub-scout/#377`
- Connected MCP trust surface: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § "MCP connected trust guidance"
- Context-pack format: [`docs/howto/context-pack-v2.md`](../../docs/howto/context-pack-v2.md)
- AI integration walkthrough: [`examples/ai-integration/`](../../examples/ai-integration/)
- MCP gateway example: [`examples/mcp-gateway/`](../../examples/mcp-gateway/)
- Bot deployment example: [`examples/bot/`](../../examples/bot/)

## Constraints

- The MCP server never starts a mutating tool. The tool catalog is closed and read-only by construction; adding a tool requires a code change + a passing test.
- `context-pack` is **deterministic** — same input, same bytes. Do not introduce non-determinism (random IDs, timestamps in non-`capturedAt` fields, map iteration order).
- For long-running agent sessions, use MCP STDIO; `bot` is the separate in-cluster event-stream entrypoint.
- `cub-scout mcp serve` is the cub-scout-side MCP. `cub` may also expose an MCP surface; the two are separate, and an agent can register both.
