---
name: ai-agent-readonly-context
description: 'Use when the user is WIRING CUB-SCOUT INTO AN AI AGENT (Claude / Codex / a custom LLM workflow) and needs the patterns for read-only context, MCP gateway tools, deterministic context snapshots, and the prompt-shape that makes cub-scout''s output reliable for LLMs. Natural phrasing: "set up cub-scout as a Claude tool", "expose cub-scout to my agent over MCP", "give the LLM a deterministic snapshot of this cluster", "what cub-scout patterns work well for AI agents?", "MCP config for cub-scout in Claude Code", "best practices for wiring cub-scout into Codex / Cursor / Continue", "AI-ready cluster context", "prompt template for cub-scout output". Composes `mcp serve` + `context-pack` + `--presentation ai` + the structured JSON contracts. Do NOT load for: a specific verb invocation (load the corresponding scout-* skill), authoring an MCP server in general (this is about wiring CUB-SCOUT specifically), or any mutating workflow (cub-scout never mutates; the read-only triad is the whole point of this skill).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout mcp *) Bash(cub-scout mcp *) Bash(cub scout mcp *) Bash(./cub-scout context-pack *) Bash(cub-scout context-pack *) Bash(cub scout context-pack *) Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout status) Bash(cub-scout status) Bash(cub scout status) Bash(./cub-scout version) Bash(cub-scout version) Bash(cub scout version)
---

# ai-agent-readonly-context

The agent-integration loop. The user is wiring cub-scout into an AI agent — Claude Code, Codex, Cursor, Continue, a custom MCP-aware LLM — and needs the patterns that make cub-scout's output reliable for LLMs.

Three patterns matter here, plus the prompt shape that ties them together:

1. **MCP gateway** (`mcp serve`) — long-running tool surface the agent calls live
2. **context-pack** (`context-pack --format json`) — deterministic one-shot snapshot the agent reasons over
3. **`--presentation ai`** — output format with bracket notation + uppercase markers that LLMs parse reliably

Plus the read-only invariant: every cub-scout tool an agent can call is **categorically non-mutating**. cub-scout is the safest possible AI surface because the worst-case prompt-injection still can't apply / edit / delete anything.

## When to use

Explicit phrasings:

- "Set up cub-scout as a Claude tool"
- "Expose cub-scout to my agent over MCP"
- "Give the LLM a deterministic snapshot of this cluster"
- "What cub-scout patterns work well for AI agents?"
- "MCP config for cub-scout in Claude Code / Codex / Cursor / Continue"
- "Best practices for wiring cub-scout into [agent framework]"
- "AI-ready cluster context for my LLM workflow"
- "Prompt template for cub-scout output"
- "How do I make cub-scout JSON parseable by an LLM?"

Implicit intents:

- The user is **integrating**, not running cub-scout interactively
- The user values **determinism** (same input → same bytes)
- The user values **read-only safety** (prompt injection can't escalate to cluster mutation)
- The user may be combining cub-scout with `cub` / `kubectl` / other tools in the agent's tool set

## Do not load for

- A specific verb invocation — load the matching `scout-*` skill
- Authoring an MCP server in general (this is about wiring cub-scout *specifically*) — use the MCP-server-authoring docs from the agent host
- Mutating workflows. cub-scout never mutates; the read-only triad is the whole point of this skill.
- ConfigHub-side MCP integration — that's `cub`, a separate tool surface

## The patterns

### Pattern 1 — MCP gateway (live tool calls)

`cub-scout mcp serve` is a long-running MCP server exposing cub-scout's verbs as tools. Register it in your agent host's MCP config:

```jsonc
// .claude/mcp_servers.json (Claude Code) — STDIO transport (preferred)
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

```jsonc
// HTTP transport — for agent hosts that don't support STDIO
{
  "mcpServers": {
    "cub-scout": {
      "url": "http://localhost:7755",
      "command": "cub-scout",
      "args": ["mcp", "serve", "--port", "7755"]
    }
  }
}
```

The agent host calls `tools/list` to discover tools and `tools/call` to invoke them. cub-scout's `mcp_test.go` covers the exact catalog:

**Standalone-mode tools** (5, always available):
- `doctor` — cluster-level diagnostic with structured nextSteps
- `map` — resource inventory with ownership classification
- `scan` — risk + misconfiguration findings
- `trace` — ownership + source chain (one resource)
- `explain` — plain-English per-resource report

**Connected-mode tools** (5, added when `cub auth status` succeeds):
- `compare_three_way` — DRY/WET/LIVE with rolled-up agreement
- `compare_source_truth` — strategy-typed verdict (target + namespace + strategy required)
- `confighub_changesets` — governed ChangeSet history from ConfigHub
- `confighub_units` — ConfigHub unit + fleet inventory
- `confighub_unit_get` — exact ConfigHub unit details + applied/live revision

Use `cub-scout mcp serve --list-tools` to dump the registered catalog without starting the server — useful for agent registration debugging. See [`references/mcp-tool-catalog.md`](../references/mcp-tool-catalog.md) for per-tool parameters, return shape, and the list of verbs intentionally NOT in the catalog.

### Pattern 2 — context-pack (deterministic snapshot)

`cub-scout context-pack --format json` produces a **one-shot deterministic** JSON snapshot of the cluster. Same input → same bytes. Safe to feed an LLM with prompt caching; safe to diff in CI.

```bash
$ cub-scout context-pack --format json > cluster-ctx.json
$ wc -l cluster-ctx.json
2347 cluster-ctx.json

$ jq '.ownership | group_by(.type) | map({type: .[0].type, count: length})' cluster-ctx.json
[
  {"type": "argo",    "count": 47},
  {"type": "flux",    "count": 12},
  {"type": "helm",    "count": 8},
  {"type": "native",  "count": 23},
  {"type": "unknown", "count": 3}
]
```

The pack carries:
- `version` — cub-scout build tag + context-pack schema version
- `capturedAt` — RFC 3339 timestamp (the ONLY non-deterministic field; everything else is reproducible)
- `cluster` — kubeconfig context + server URL
- `mode` — `standalone` or `connected`
- `resources[]` — per-workload kind / name / namespace / ownership classification
- `attribution[]` — per-resource attribution evidence (`cause`, `managerHint`, `gitSource`, `bindingSource`)
- `omissions[]` — explicit non-claims (e.g., resources skipped due to CrashLoop, unsupported kinds)

Feed it to the LLM as a system message context block:

```
You have read-only access to a Kubernetes cluster snapshot via the JSON below.
Cluster: <cluster name>
Mode: <standalone|connected>
Captured at: <RFC3339>

<cluster-ctx.json>

You can also call the following MCP tools for deeper reads: ...
```

### Pattern 3 — `--presentation ai`

For agent prompts that consume ASCII output (not JSON), `--presentation ai` produces a structured form LLMs parse reliably. Uppercase markers; bracket notation; consistent label-value separators.

```bash
$ cub-scout explain deploy/api -n prod --presentation ai
[explain]
Resource: Deployment/api in prod
Owner: Argo CD application "payments-api" [label:argocd.argoproj.io/instance]
Drift: Detected
Mutation cause: manual-edit [manager: kubectl-edit; controller co-signal: argocd-controller]
Git source: https://github.com/org/platform-config @abc123 path=apps/prod/api
Events: ImagePullBackOff (5 mins ago, repeating)
Recommended actions:
  - [read-only] Image ghcr.io/org/api:v2.3.0 cannot be pulled; check ghcr.io status
  - [read-only] If image is correct, verify imagePullSecrets
[end explain]
```

vs. the default human form:

```bash
$ cub-scout explain deploy/api -n prod
Resource: Deployment/api in prod
  Owner: Argo CD application "payments-api"
    (label:argocd.argoproj.io/instance)
  ...
```

The AI form is denser, less ambiguous, and parseable with regex if needed.

Available on `doctor` / `explain` / `trace` (per `#352` / `#359`). The `--presentation paired` flag emits both forms for human + AI dual-purpose output.

## The read-only invariant

This is **the** value proposition for AI integration. cub-scout's MCP tool catalog is **closed and read-only by construction**:

- Every tool wraps a cub-scout verb with `--format json`
- Every verb is non-mutating
- The tool catalog is verified by `cmd/cub-scout/mcp_test.go` — adding a tool requires a code change + a passing test
- The receipt capability's `FilterNextSteps` (`pkg/agent/receipt_predicates.go`) strips mutating `actionType` / `nextCommand` from any structured next-step hint before emit
- `TestReceiptPackageReadOnlyClient` statically greps the receipt source for any mutating K8s client method

Consequence: **even with a fully-compromised prompt**, an LLM calling cub-scout's MCP tools cannot apply / edit / patch / delete anything. The blast radius is reads.

This is unusual — most operational tools mix read and write surfaces. cub-scout's #410 / #428 architectural-triad work made the read-only carve-out a code-enforced invariant, not a convention. That's what makes cub-scout the safest Kubernetes MCP surface for AI agents today.

## Worked example: Claude Code integration

```jsonc
// .claude/mcp_servers.json
{
  "mcpServers": {
    "cub-scout": {
      "command": "cub-scout",
      "args": ["mcp", "serve"]
    }
  }
}
```

```jsonc
// .claude/skills.json — load the relevant scout-* skills
{
  "skills": [
    "scout-observe", "scout-diagnose", "scout-compare", "scout-attribute",
    "scout-verify", "scout-mcp",
    "triage-unhealthy-workload", "investigate-drift"
  ]
}
```

User prompt: *"deploy/api in prod is broken, what's going on?"*

Agent's likely loop:
1. Trigger `triage-unhealthy-workload` skill
2. Call MCP tool `doctor` (scope: namespace=prod)
3. Call MCP tool `explain` (`deploy/api -n prod`)
4. Call MCP tool `trace` (`deploy/api -n prod`)
5. Read structured `nextSteps[]` + `events[]` from the responses
6. Synthesize human-language explanation; route the mutation suggestion to the user

The agent NEVER calls a mutating MCP tool because **there are none registered**. The user drives `kubectl rollout undo` / `argocd app rollback` from a separate context if needed.

## Configuration tips

- **Prefer STDIO over HTTP** for agent integration — lower latency, easier to sandbox, no port management.
- **Set `KUBECONFIG` explicitly** in the MCP env block so the server picks up the right cluster context. Don't rely on the agent host inheriting it.
- **For multi-cluster contexts**, run one `mcp serve` instance per cluster on different ports / paths, OR use the `--context <name>` flag to switch contexts in-process. cub-scout doesn't support multi-context-in-one-server today (a single `mcp serve` is one context).
- **Cache `context-pack` aggressively.** It's deterministic; an LLM with prompt caching benefits enormously from caching the snapshot across multiple queries.
- **Use connected mode** if the agent is doing source-truth / governance work. Run `cub auth status` first; the agent should refuse to claim source-truth verdicts in standalone mode.

## Tool boundary

- **Allowed:** `mcp serve` (long-running but no mutation), `context-pack`, `status`, `version`, plus the read-only verbs the agent will indirectly call through MCP.
- **Not allowed:** any mutating verb. The MCP gateway never registers one; the static guards in cub-scout enforce it.
- **MCP and the triad:** cub-scout (evidence) ↔ MCP (transport) ↔ Pilot or agent (judge / actor). The MCP layer is plumbing; it never decides.

## References

- [`scout-mcp`](../scout-mcp/SKILL.md) — the verb-group skill for `mcp serve` and `context-pack`
- MCP tool descriptions audit: `#377`
- `--presentation` flag: `#352` (doctor / explain), `#359` (trace)
- Connected MCP trust surface: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § "MCP connected trust guidance"
- Context-pack format: [`docs/howto/context-pack-v2.md`](../../docs/howto/context-pack-v2.md)
- Examples: [`examples/ai-integration/`](../../examples/ai-integration/), [`examples/mcp-gateway/`](../../examples/mcp-gateway/), [`examples/ai-agent-quest/`](../../examples/ai-agent-quest/)
- Read-only triad: `#410`, `#428`

## Constraints

- The MCP tool catalog is **closed**. Adding tools requires a code change + a passing `mcp_test.go` test. Don't claim cub-scout has a mutating MCP tool — it doesn't, by design.
- `context-pack` is **deterministic** (same input, same bytes) EXCEPT for the `capturedAt` timestamp. If you need byte-identical output across runs, set a fixed `capturedAt` in your wrapper.
- For agent hosts that don't speak MCP yet, fall back to invoking cub-scout verbs directly via shell tools with `--format json --presentation ai`. The structured output is the same.
- This skill's read-only invariant doesn't extend to `cub`. If the agent has BOTH cub-scout AND cub MCP tools registered, the cub side IS mutation-capable. Use a separate allowed-tools policy for that surface.
