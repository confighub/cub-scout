# Reference: MCP tool catalog

The complete list of MCP tools `cub-scout mcp serve` registers, with parameters, behavior, and return shape. The catalog is **closed and read-only by construction** — adding a tool requires a code change plus a passing `mcp_test.go` test.

Source of truth: `cmd/cub-scout/mcp.go` (`newMCPGatewayWithMode`) and `cmd/cub-scout/mcp_test.go` (the tool-list lock).

## Mode-aware catalog

The catalog has two tiers:

- **Standalone tools** — registered always; live reads require kubeconfig; release checks also require a digest-pinned OCI bundle or local layout
- **Connected tools** — added when `cub auth status` succeeds; require ConfigHub auth

Total: **18 tools** (7 standalone + 11 connected; `release_check` is unreleased v2.11).

## Standalone tools (7)

These are always available. Use the standard MCP `tools/list` request to dump
the catalog from a running server.

### `doctor`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout doctor --format json` |
| Required args | — |
| Optional args | `namespace` (string — scope filter); `top` (integer — number of top issues; default 3); `with_confighub` (boolean); `confighub_space` (string); `confighub_since` (string); `confighub_stale_after` (string) |
| Returns | Cluster health summary + rollout evidence + optional bounded delivery evidence + top issues + structured `nextSteps[]` |
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
| Optional args | `namespace` (string); `bounded`, `refresh` (boolean); `api_version`, `context` (strings, required for bounded reads); unreleased v2.11 `expected_revision` (immutable commit/digest, requires bounded) |
| Returns | Plain-English per-resource report: ownership, health/drift, recent events, structured `nextSteps[]` |
| When to load | AFTER `doctor` or `map` once narrowed to one resource. The Diagnose verb-group's primary entry point. |

### `gitops_status`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout gitops status --format json` |
| Required args | — |
| Optional args | `namespace` (string); `with_confighub` (boolean); `confighub_space` (string); `confighub_since` (string); `confighub_stale_after` (string) |
| Returns | GitOps/controller backend, transport, sources, deployers, source/build/apply/sync stages, delivery evidence when requested, and `controllerCoverage[]` |
| When to load | "Is this deployed?" "Is delegated delivery healthy?" "Which controller families did cub-scout actually inspect?" "Is missing status absence or an RBAC/API omission?" Evidence only; never use it to force sync or declare application success by itself. |

### `release_check`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout release check --format json` |
| Required | `bundle` (digest-pinned OCI configuration), `controller` (Kind/name), `api_version`, `controller_namespace`, `context` (target kube context) |
| Optional | `controller_context` (default target context), `oci_layout` (read-only local layout), `max_objects` (integer 1-100, default 100) |
| Returns | Stage verdicts for bundle/controller/configuration/workloads, actual request counts, dated resource reads, omissions and existing fingerprinted configuration/workload receipts |
| When to load | "Did this exact configuration release reach this target?" or "Where is this release waiting?" Scope and immutable bundle are required; no broad discovery, rendering, mutation, running-image or application-success claim |

Each invocation is a fresh bounded check, not a shared cache. Unsupported or
ambiguous controller/source/target shapes remain inconclusive. See the
[example and read budgets](../../examples/oci-release-check/).

## Connected tools (11)

Registered only when `cub-scout mcp serve` detects connected mode and the `cub`
CLI is available.

### `compare_three_way`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout compare three-way [...] --format json` |
| Required args | `scope` (string — namespace/resource selector) |
| Optional args | `namespace` (string) |
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

The registered MCP `strategy` enum is generated from the same
`agent.AllStrategies()` registry as the CLI, so Phase 1 and Phase 2 strategy
values stay in parity. See [source-truth-strategies](source-truth-strategies.md)
for the full enum.

### `confighub_changesets`

| Aspect | Detail |
|---|---|
| Wraps | `cub changeset list --json` (calls `cub`, not cub-scout) |
| Required args | — |
| Optional args | `space` (string — slug/ID); `where` (string — filter expression) |
| Returns | Governed ChangeSet history and receipts from ConfigHub |
| When to load | "What governed write changed this unit?" "Who applied the change?" After `trace` or `confighub_units` has identified the governed object. |

### `confighub_k8s_types`

| Aspect | Detail |
|---|---|
| Wraps | `cub k8s types [<type>] -o json` (calls `cub`, not cub-scout) |
| Required args | Either `space` or `target` |
| Optional args | `type` (string — `all`, kubectl-style type, Kind, or full ConfigHub resource type); `space` (string — slug/ID, or `*` for an explicit all-spaces read); `target` (string array — `space-slug/target-slug`); `namespace` (string); `where` (string — ConfigHub entity filter); `where_resource` (string — stored-resource configuration filter) |
| Returns | ConfigHub Resource type summaries: resource type, API version, kind, resource count, unit count, and space count |
| When to load | "Which Kubernetes types does ConfigHub already hold?" "Which custom resources exist before I fetch bodies?" Cheapest ConfigHub-side survey before `confighub_k8s_resources`; not live cluster health. |

### `confighub_k8s_resources`

| Aspect | Detail |
|---|---|
| Wraps | `cub k8s get <type> [<name> ...] -o json` (calls `cub`, not cub-scout) |
| Required args | `type`, plus either `space` or `target` |
| Optional args | `names` (string array); `space` (string — slug/ID, or `*` for an explicit all-spaces read); `target` (string array — `space-slug/target-slug`); `namespace` (string); `where` (string — ConfigHub entity filter); `where_resource` (string — stored-resource configuration filter); `show` (`list`, `detail`, or `data`) |
| Returns | ConfigHub-stored intended Kubernetes resources, joined to Space, Unit, Target, namespace, name, kind, API version, and resource type. With `show=data` or serialized output, bodies are included when `cub` provides them. |
| When to load | "What Kubernetes resources does ConfigHub say should exist for this space/target?" "Fetch intended Deployment YAML without hitting every cluster." Pair with `gitops_status`, `trace`, `compare_three_way`, or `compare_source_truth` for live/controller proof. |

### `confighub_live_status`

| Aspect | Detail |
|---|---|
| Wraps | `cub space list -o json --select Slug,SpaceID,Annotations,Labels` |
| Required args | `space` (string — slug, or `*` for an explicit all-spaces read) |
| Optional args | — |
| Returns | Space rows plus additive `structuredContent.liveStatuses[]` parsed from `confighub.com/live-status` and `structuredContent.omissions[]` for missing/malformed writeback |
| When to load | "Did the evented status feedback report sync, health, operation, revision, and freshness for this space?" Evidence only; controller/runtime truth still belongs to the underlying systems. |

### `confighub_releases`

| Aspect | Detail |
|---|---|
| Wraps | `cub release list --space <space> -o json` |
| Required args | `space` (string — slug/ID, or `*` for an explicit all-spaces read) |
| Optional args | `where` (string — filter expression, usually time- or target-bounded) |
| Returns | ConfigHub Release rows for release/OCI bundle history |
| When to load | "Which release or OCI bundle was published for this space/target/time window?" Pair with `gitops status`, `trace`, or `compare_source_truth` for controller/runtime evidence. |

### `confighub_resources`

| Aspect | Detail |
|---|---|
| Wraps | `cub resource list --space <space> -o json` (calls `cub`, not cub-scout) |
| Required args | `space` (string — slug/ID, or `*` for an explicit all-spaces read) |
| Optional args | `where` (string — Resource/entity/Data filter); `contains` (string); `select` (string); `filter` (string — ConfigHub filter slug/ID); `view` (string — ConfigHub view slug/ID); `raw_data` (boolean) |
| Returns | ConfigHub Resource entity rows for indexed resources extracted from Unit data, including resource metadata and selected fields/data when requested |
| When to load | "Which indexed resources match this fleet-wide predicate?" "Find resources by ResourceType, ResourceName, TargetID, Unit/Space labels, or Data paths." Lower-load alternative to iterating Units; not live cluster health. |

### `confighub_unit_events`

| Aspect | Detail |
|---|---|
| Wraps | `cub unit-event list [unit] --space <space> -o json` |
| Required args | `space` (string — slug/ID, or `*` for an explicit all-spaces read) |
| Optional args | `unit` (string); `where` (string — filter expression, usually time-bounded) |
| Returns | ConfigHub UnitEvent rows for source/action evidence |
| When to load | "What event or source action happened for this unit or space?" Read-only history; not an event-consumer subscription and does not advance cursors. |

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
| `patterns_detect` | Specialized; CLI invocation is the better surface for this verb |
| `compare_drift` (file vs live) | Requires a local YAML file argument — awkward to expose over MCP (the file path is relative to the cub-scout process, not the agent) |
| `compare` (resource mode) | Subsumed by `compare_three_way` in connected mode |
| `history` | Use `confighub_changesets` instead — same conceptual surface, different naming for the MCP tier |
| `impact` | Considered but deferred; the connected `impact` blast-radius surface is rich enough that MCP exposure needs design |
| `fleet_outliers` | Considered but deferred; fleet-scope tools need careful agent-side framing |
| `views_resolve` | Considered but deferred |
| `receipt verify / show / validate / list` | Considered but deferred; the receipt surface is locally-driven by design — emitting receipts mid-MCP-conversation needs design (covered in `#446` v2 work) |
| `cub k8s source / collect / refresh` | Not exposed through Scout MCP: `source` is browser-opening operator UX, while `collect` and `refresh` can write ConfigHub state unless carefully dry-run scoped |
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

`cub-scout mcp serve` currently supports stdio transport:

| Transport | Flag | Use case |
|---|---|---|
| STDIO (default) | (no flag) | Local agent hosts (Claude Code, Codex, Cursor, Continue). Lower latency, simpler sandboxing. |

The catalog is discovered with the standard MCP `tools/list` request.

## Tool execution path

Each tool's `BuildArgs` function transforms the MCP arguments into a cub-scout CLI argv. The MCP server then executes the cub-scout CLI as a subprocess (via the `runner` / `connectedRunner` injection points) and returns the stdout as `content[0].text` in the MCP response. The CLI's `--format json` flag drives the structured output.

For connected tools that wrap `cub` (not cub-scout) — `confighub_changesets`,
`confighub_k8s_resources`, `confighub_k8s_types`, `confighub_live_status`,
`confighub_releases`, `confighub_resources`, `confighub_unit_events`,
`confighub_units`, and `confighub_unit_get` — the runner is `connectedRunner`
instead of `runner`. Same execution model; different binary on PATH.

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
