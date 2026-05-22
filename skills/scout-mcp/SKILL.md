---
name: scout-mcp
description: 'Use when the user wants to expose cub-scout to an AI agent over MCP, or wants a deterministic AI-ready snapshot of cluster context — the Integrate verb group. Natural phrasing: "start the MCP server", "expose cub-scout to Claude / Codex / my agent", "give me an AI-ready context pack of this cluster", "set up the MCP gateway", "what MCP tools does cub-scout expose?", "produce JSON context for an LLM to reason about this cluster". Load whenever intent is mcp / mcp-gateway / mcp-serve / context-pack / agent-context / ai-snapshot / llm-context / model-context-protocol. Do NOT load for: actually running a verb (those are scout-observe / scout-diagnose / scout-compare / scout-attribute / scout-ingest / scout-govern / scout-verify), or `cub` MCP integration (that''s a separate tool surface).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout mcp *) Bash(cub-scout mcp *) Bash(cub scout mcp *) Bash(./cub-scout context-pack *) Bash(cub-scout context-pack *) Bash(cub scout context-pack *) Bash(./cub-scout version) Bash(cub-scout version) Bash(cub scout version) Bash(./cub-scout status) Bash(cub-scout status) Bash(cub scout status) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl config current-context) Bash(cub auth status) Bash(cub * get) Bash(cub * list)
---

# scout-mcp

The Integrate verb group of cub-scout. Two ways to plug cub-scout into AI workflows:

- **`mcp serve`** — long-running Model Context Protocol gateway exposing cub-scout's verbs as MCP tools for Claude / Codex / any MCP-aware agent
- **`context-pack`** — one-shot deterministic JSON context bundle an agent can consume directly

Plus the version/status meta-commands the harness uses for cold-start checks.

## When to use

Explicit phrasings:

- "Start the MCP server" / "expose cub-scout to my agent over MCP"
- "Set up cub-scout as an MCP tool for Claude / Codex"
- "Give me an AI-ready context pack of this cluster"
- "What MCP tools does cub-scout expose?"
- "Produce JSON context an LLM can reason about" / "deterministic context snapshot"
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

- **Standalone (cluster only):** `mcp serve` exposes the read-only standalone tool set (doctor, explain, map, scan, trace, compare drift, etc.). `context-pack` produces a cluster snapshot with attribution evidence but no ConfigHub linkage.
- **Connected (cluster + `cub auth login`):** `mcp serve` additionally registers the connected-mode tools (`compare_three_way`, `compare_source_truth`, `history`, `impact`, source-truth, views). Connected `context-pack` includes ConfigHub unit linkage and connected attribution (`bindingSource`).
- **Offline (no cluster):** `mcp serve` refuses to start without a reachable cluster. `context-pack` works against debug bundles via `--bundle <path>`.

## Tool boundary

- **Allowed (read-only):** `mcp serve` (long-running but no mutation), `context-pack` (deterministic snapshot), `version`, `status`; `kubectl get/describe`, `kubectl config current-context`; `cub * get/list`, `cub auth status`.
- **Not allowed:** `mcp serve` exposes ONLY the read-only cub-scout verbs as tools. The MCP tool catalog explicitly excludes any mutating verb (`demo`, `import apply`, `compare --suggest --apply`) — see `cmd/cub-scout/mcp.go` `RegisterTools` for the closed list. `context-pack` never writes; it reads + emits.
- **MCP and the read-only triad:** the MCP gateway is the third party in the architectural triad. cub-scout (evidence) ↔ MCP (transport) ↔ Pilot or agent (judge / actor). The MCP layer NEVER decides; it just streams cub-scout's structured output.

## The verb menu

| Question | Verb | Notes |
|---|---|---|
| "Start the MCP gateway" | `cub-scout mcp serve` | Long-running. STDIO transport by default; `--port` for HTTP. |
| "What MCP tools are registered?" | `cub-scout mcp serve --list-tools` | Dumps the tool catalog as JSON without starting the server. Useful for agent registration debugging. |
| "Produce deterministic AI context" | `cub-scout context-pack --format json` | One-shot. Cluster snapshot + ownership classifications + attribution evidence in a single JSON. |
| "Pack a bundle as AI context" | `cub-scout context-pack --bundle <path>` | Same shape, sourced from a debug bundle instead of the live cluster. Offline-friendly. |
| "Check cub-scout version" | `cub-scout version` | Build tag. Agents that registered cub-scout tools use this to detect breaking-change ranges. |
| "Confirm mode" | `cub-scout status` | Standalone vs connected. Agents call this first to know which connected tools are available. |

## The loop

1. **Decide MCP vs context-pack.** Long-running agent → `mcp serve`. Single-shot reasoning over a snapshot → `context-pack`.
2. **Check the mode.** `cub-scout status` confirms standalone vs connected. Tell the agent which tool subset is live.
3. **Start the surface.** For MCP, `mcp serve` registers tools with the agent. For context-pack, `--format json` and pipe to the agent.
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

The agent now sees cub-scout's read-only tools (`doctor`, `explain`, `trace`, `compare_drift`, `compare_three_way`, etc.) and can call them like any other MCP tool.

### B: list the tool catalog

```bash
$ cub-scout mcp serve --list-tools | jq '.tools[] | .name'
"doctor"
"explain"
"map_workloads"
"map_status"
"trace"
"scan"
"compare_drift"
"compare_three_way"
"compare_source_truth"
"gitops_status"
"patterns_detect"
...
```

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
- `compare_source_truth` — strategy-typed verdict
- `history` — ChangeSet timeline
- `impact` — blast-radius
- `views_resolve` — Hub View resolution

These are the typical Pilot acceptance-kernel inputs. The agent composes them; cub-scout produces evidence; Pilot decides.

## Output evidence

### `mcp serve` (the gateway)

- Long-running. STDIO or HTTP transport.
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

| Tool name | Mode | Verb |
|---|---|---|
| `doctor` | standalone | `cub-scout doctor` |
| `explain` | standalone | `cub-scout explain` |
| `map_workloads` | standalone | `cub-scout map workloads` |
| `map_status` | standalone | `cub-scout map status` |
| `trace` | standalone | `cub-scout trace` |
| `scan` | standalone | `cub-scout scan` |
| `compare_drift` | standalone | `cub-scout compare drift` |
| `gitops_status` | standalone | `cub-scout gitops status` |
| `patterns_detect` | standalone | `cub-scout patterns detect` |
| `compare_three_way` | connected | `cub-scout compare three-way` |
| `compare_source_truth` | connected | `cub-scout compare source-truth` |
| `history` | connected | `cub-scout history` |
| `impact` | connected | `cub-scout impact` |
| `fleet_outliers` | connected | `cub-scout fleet outliers` |
| `views_resolve` | connected | `cub-scout views resolve` |

The catalog is intentionally narrow: every tool is read-only, every tool has a stable JSON contract, every tool is exercised by an MCP integration test.

## References

- Command reference: [`docs/reference/commands.md`](../../docs/reference/commands.md) § mcp, § context-pack
- MCP tool descriptions audit: `cub-scout/#377`
- Connected MCP trust surface: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § "MCP connected trust guidance"
- Context-pack format: [`docs/howto/context-pack-v2.md`](../../docs/howto/context-pack-v2.md)
- AI integration walkthrough: [`examples/ai-integration/`](../../examples/ai-integration/)
- MCP gateway example: [`examples/mcp-gateway/`](../../examples/mcp-gateway/)

## Constraints

- The MCP server never starts a mutating tool. The tool catalog is closed and read-only by construction; adding a tool requires a code change + a passing test.
- `context-pack` is **deterministic** — same input, same bytes. Do not introduce non-determinism (random IDs, timestamps in non-`capturedAt` fields, map iteration order).
- For long-running agent sessions, prefer MCP STDIO over HTTP unless the agent host requires HTTP — STDIO has lower latency and is sandboxable.
- `cub-scout mcp serve` is the cub-scout-side MCP. `cub` may also expose an MCP surface; the two are separate, and an agent can register both.
