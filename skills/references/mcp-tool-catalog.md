# Reference: MCP tool catalog

The complete list of MCP tools `cub-scout mcp serve` registers, with parameters, behavior, and return shape. The catalog is **closed and read-only by construction** — adding a tool requires a code change plus a passing `mcp_test.go` test.

Source of truth: `cmd/cub-scout/mcp.go` (the registration map literal at line 169 and connected-mode additions starting at line 321) and `cmd/cub-scout/mcp_test.go` (the tool-list lock).

## Mode-aware catalog

The catalog has two tiers:

- **Standalone tools** — registered always; require only a kubeconfig
- **Connected tools** — added when `cub auth status` succeeds; require ConfigHub auth

Total: **10 tools** (5 standalone + 5 connected).

## Standalone tools (5)

These are always available. Run `cub-scout mcp serve --list-tools` to dump the catalog without starting the server.

### `doctor`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout doctor --format json` |
| Required args | — |
| Optional args | `namespace` (string — scope filter); `top` (integer — number of top issues; default 3) |
| Returns | Cluster health summary + top issues + structured `nextSteps[]` |
| When to load (per the registered description) | FIRST standalone tool for "what's wrong?" / "what's broken?" / compact cluster or namespace health summary. Before `explain`, `trace`, or `scan` when the user has not narrowed to one resource. |

### `map`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout map list --json` |
| Required args | — |
| Optional args | `namespace` (string) |
| Returns | Resource inventory with ownership classification per resource |
| When to load | Broad inventory question. "What's running here?" with ownership awareness. NOT a first stop for "what's broken?" |

### `scan`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout scan --json` |
| Required args | — |
| Optional args | `namespace` (string) |
| Returns | Risk/misconfiguration findings, severity-sorted |
| When to load | AFTER `doctor` when the user wants detailed risk findings. Not a governed promotion gate. |

### `trace`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout trace <resource> [-n <ns>] --format json` |
| Required args | `resource` (string — `kind/name` form) |
| Optional args | `namespace` (string) |
| Returns | Ownership and source chain, top-down (controller → source → workload) including secret evidence |
| When to load | AFTER `doctor` or `explain` once narrowed to one resource. To know where a resource came from. |

### `explain`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout explain <resource> [-n <ns>] --format json` |
| Required args | `resource` (string — `kind/name` form) |
| Optional args | `namespace` (string) |
| Returns | Plain-English per-resource report: ownership, health/drift, recent events, structured `nextSteps[]` |
| When to load | AFTER `doctor` or `map` once narrowed to one resource. The Diagnose verb-group's primary entry point. |

## Connected tools (5)

Registered only when `cub-scout mcp serve` detects connected mode (`cub auth status` returns OK or `CONFIGHUB_API_KEY` is set).

### `compare_three_way`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout compare three-way [...] --format json` |
| Required args | — |
| Optional args | `scope` (string — namespace/resource selector), `view` (string — Hub View URL or UUID), `source-path` (string — local git checkout for stage B back-resolution) |
| Returns | DRY (ConfigHub) / WET (rendered) / LIVE (cluster) three-way comparison with per-field agreement and attribution evidence; rolled-up `summary.agreement` (agreed / converging / diverged / partial) |
| When to load | "Does governed state agree with live state?" "Is this change sign-off-ready?" After scope is identified. |

### `compare_source_truth`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout compare source-truth <target> -n <ns> --strategy <s> --format json` |
| Required args | `target` (string — `kind/name`), `namespace` (string), `strategy` (string) |
| Optional args | — |
| Returns | Source-truth evidence document: `declared_strategy`, `status` (PASS/WATCH/BLOCK/ASK), `source_truth` verdict (AGREED/MISMATCH/INCOMPLETE/BLOCKED/UNKNOWN), per-surface evidence, `proof_gaps[]`, `safe_next_action` |
| When to load | "Is this workload's source of truth consistent end-to-end under a declared strategy?" Strategy is **required**, never inferred. The contract refuses to PASS when any required field is missing. NEVER use to approve, repair, or mutate — evidence only. |

The registered MCP `strategy` enum currently lists **four** strategies (`confighub-oci-argo` / `confighub-oci-flux` / `git-argo` / `git-flux`). The CLI itself supports **nine** strategies (Phase 1 + Phase 2 from `#418`). The MCP schema's enum is the Phase 1 subset; passing a Phase 2 strategy (`helm-argo` / `helm-flux` / `kustomize-flux` / `oci-argo` / `oci-flux`) through MCP will be rejected at the schema-validation step. The CLI invocation form does NOT have this restriction. Tracked drift; see [source-truth-strategies](source-truth-strategies.md) for the full enum.

### `confighub_changesets`

| Aspect | Detail |
|---|---|
| Wraps | `cub changeset list --json` (calls `cub`, not cub-scout) |
| Required args | — |
| Optional args | `space` (string — slug/ID); `where` (string — filter expression) |
| Returns | Governed ChangeSet history and receipts from ConfigHub |
| When to load | "What governed write changed this unit?" "Who applied the change?" After `trace` or `confighub_units` has identified the governed object. |

### `confighub_units`

| Aspect | Detail |
|---|---|
| Wraps | `cub unit list --json` (calls `cub`) |
| Required args | — |
| Optional args | `space` (string); `where` (string); `contains` (string — full-text query) |
| Returns | ConfigHub unit inventory + cluster-to-ConfigHub linkage |
| When to load | "Which ConfigHub unit corresponds to this resource?" "Governed unit inventory before drilling into one unit." After `doctor` / `map` / `explain` / `trace` has identified the cluster-side object. |

### `confighub_unit_get`

| Aspect | Detail |
|---|---|
| Wraps | `cub unit get --json <unit>` (calls `cub`) |
| Required args | `unit` (string — slug/ID) |
| Optional args | `space` (string) |
| Returns | Exact ConfigHub unit details: intended state, last-applied revision, live revision, ConfigHub URL |
| When to load | ONLY after the unit slug/ID is known. "Show me the intended/applied/live revision for unit X." If unit is unknown, use `confighub_units` first. |

## What's NOT in the catalog

The closed catalog is verified by `cmd/cub-scout/mcp_test.go`. The following cub-scout CLI verbs are intentionally NOT exposed as MCP tools (currently):

| Verb | Why not |
|---|---|
| `gitops_status` | Surfaced through `doctor` already; separate MCP tool would duplicate |
| `patterns_detect` | Specialized; CLI invocation is the better surface for this verb |
| `compare_drift` (file vs live) | Requires a local YAML file argument — awkward to expose over MCP (the file path is relative to the cub-scout process, not the agent) |
| `compare` (resource mode) | Subsumed by `compare_three_way` in connected mode |
| `history` | Use `confighub_changesets` instead — same conceptual surface, different naming for the MCP tier |
| `impact` | Considered but deferred; the connected `impact` blast-radius surface is rich enough that MCP exposure needs design |
| `fleet_outliers` | Considered but deferred; fleet-scope tools need careful agent-side framing |
| `views_resolve` | Considered but deferred |
| `receipt verify / show / validate / list` | Considered but deferred; the receipt surface is locally-driven by design — emitting receipts mid-MCP-conversation needs design (covered in `#446` v2 work) |
| Any mutating verb | Categorically out of band — see the read-only invariant below |

If a user asks for one of these via MCP, the answer is "use the CLI" (or the corresponding scout-* skill from this repo). The catalog is intentionally narrow.

## Common annotations

Every tool's `Descriptor.Annotations` is the same object:

```go
readOnly := &mcpToolAnnotations{ReadOnlyHint: true}
```

The `ReadOnlyHint: true` is a discovery hint for MCP-aware agent hosts that filter tools by safety class. Combined with the static catalog itself being closed and the receipt-package read-only-triad guards, the invariant is layered:

1. The catalog only contains read-only verbs (closed by design)
2. The `ReadOnlyHint` advertises this to discovery clients
3. Per-tool descriptions reinforce it (`evidence only` / `DO NOT use to approve, repair, or mutate`)
4. `scripts/check-readonly.sh` + `TestReceiptPackageReadOnlyClient` enforce it at code level

See [`read-only-triad`](read-only-triad.md) for the broader invariant.

## Transport choices

`cub-scout mcp serve` supports two transports:

| Transport | Flag | Use case |
|---|---|---|
| STDIO (default) | (no flag) | Local agent hosts (Claude Code, Codex, Cursor, Continue). Lower latency, simpler sandboxing. |
| HTTP | `--port <n>` | Hosted agent platforms that don't speak STDIO; multi-process orchestration |

Both transports register the same catalog. The `mcp serve --list-tools` debug command dumps the catalog JSON without starting either transport — useful for agent registration debugging.

## Tool execution path

Each tool's `BuildArgs` function transforms the MCP arguments into a cub-scout CLI argv. The MCP server then executes the cub-scout CLI as a subprocess (via the `runner` / `connectedRunner` injection points) and returns the stdout as `content[0].text` in the MCP response. The CLI's `--format json` flag drives the structured output.

For connected tools that wrap `cub` (not cub-scout) — `confighub_changesets` / `confighub_units` / `confighub_unit_get` — the runner is `connectedRunner` instead of `runner`. Same execution model; different binary on PATH.

## Errors

| Error condition | MCP response |
|---|---|
| Missing required argument | `BuildArgs` returns `fmt.Errorf("missing required argument: <name>")` → MCP error response |
| Schema validation fails (e.g., unknown strategy enum value) | MCP server rejects pre-execution |
| CLI subprocess fails | stderr captured + included in MCP tool response with `isError: true` |
| Tool not registered | MCP `tools/call` returns "unknown tool" error |

## Skills that consume this reference

- [`scout-mcp`](../scout-mcp/SKILL.md) — the verb-group skill for `mcp serve` and `context-pack`; lists the catalog summary
- [`ai-agent-readonly-context`](../ai-agent-readonly-context/SKILL.md) — wiring patterns + the read-only invariant value prop
- [`scout-compare`](../scout-compare/SKILL.md) — `compare_three_way` and `compare_source_truth` are MCP tools; the CLI invocations have richer flags
- [`scout-govern`](../scout-govern/SKILL.md) — `confighub_*` MCP tools are the connected-mode entry points

## References

- Code: `cmd/cub-scout/mcp.go` (catalog), `cmd/cub-scout/mcp_test.go` (catalog lock + per-tool invocation tests)
- Audit: `#377` (MCP tool descriptions audit — cold-test sharpening lessons)
- Doctor added as MCP tool: `#369`
- Compare-three-way MCP tool: added in the connected-mode trust-surface work
- Read-only triad: `#410` / `#428`
- Examples: [`examples/mcp-gateway/`](../../examples/mcp-gateway/), [`examples/ai-integration/`](../../examples/ai-integration/), [`examples/ai-agent-quest/`](../../examples/ai-agent-quest/)
