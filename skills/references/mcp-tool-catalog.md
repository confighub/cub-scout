# Reference: MCP tool catalog

The complete list of MCP tools `cub-scout mcp serve` registers, with parameters, behavior, and return shape. The catalog is **closed and read-only by construction** — adding a tool requires a code change plus a passing `mcp_test.go` test.

Source of truth: `cmd/cub-scout/mcp.go` (`newMCPGatewayWithMode`) and `cmd/cub-scout/mcp_test.go` (the tool-list lock).

## Mode-aware catalog

The catalog has two tiers:

- **Standalone tools** — registered always; live reads require kubeconfig; release checks also require a digest-pinned OCI bundle or local layout
- **Connected tools** — added when the ConfigHub session check passes; require a cub login (`cub auth login`). Since v2.14.0 that check is made through the ConfigHub SDK and needs no `cub` binary; `cub-scout status --json` reports it as `confighub_reads`. With `CUB_SCOUT_CONFIGHUB_READER=cub` it is `cub auth status`.

Total: **21 tools** (8 standalone + 13 connected).

## Standalone tools (8)

These are always available. Use the standard MCP `tools/list` request to dump
the catalog from a running server.

### `doctor`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout doctor --format json` |
| Required args | — |
| Optional args | `context` (nonempty Kubernetes context, forwarded as `--kube-context`); `namespace` (string — scope filter); `top` (integer — number of top issues; default 3); `with_confighub` (boolean); `confighub_space` (string); `confighub_since` (string); `confighub_stale_after` (string) |
| Returns | Cluster health summary + rollout evidence + optional bounded delivery evidence + top issues + structured `nextSteps[]`; selected `kubernetesContext` label and partial-coverage `warnings[]` |
| When to load (per the registered description) | FIRST standalone tool for "what's wrong?" / "what's broken?" / compact cluster or namespace health summary. Before `explain`, `trace`, or `scan` when the user has not narrowed to one resource. |

### `map`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout map list --json` |
| Required args | — |
| Optional args | `namespace` (string); `context` (nonempty exact kubeconfig context for live inventory); opt-in preview `cluster_identity` (boolean: one extra bounded Namespace GET; excludes compact and ownership-only modes) |
| Returns | Resource inventory with ownership classification per resource; opt-in preview `cluster_identity: true` uses a separate envelope with observed Namespace-instance identity, per-resource instance references/omissions, identity-reader-only cost and independent collection omissions, including unavailable inventory. Default response remains unchanged. |
| When to load | Broad inventory question. "What's running here?" with ownership awareness. NOT a first stop for "what's broken?" |

### `scan`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout scan --json` |
| Required args | — |
| Optional args | `namespace` (string); `context` (nonempty Kubernetes context, forwarded as `--kube-context`) |
| Returns | Risk/misconfiguration findings, severity-sorted; selected `kubernetesContext` label and provider coverage warnings |
| When to load | AFTER `doctor` when the user wants detailed risk findings. Not a governed promotion gate. |

### `trace`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout trace <resource> [-n <ns>] --format json` |
| Required args | `resource` (string — `kind/name` form) |
| Optional args | `namespace` (string); `context` (nonempty exact kubeconfig context for all Kubernetes trace reads) |
| Returns | Ownership and source chain, top-down (controller → source → workload) including secret evidence |
| When to load | AFTER `doctor` or `explain` once narrowed to one resource. To know where a resource came from. |

### `explain`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout explain <resource> [-n <ns>] --format json` |
| Required args | `resource` (string — `kind/name` form) |
| Optional args | `namespace` (string); `field_path` (exact canonical managedFields path); `bounded`, `refresh` (boolean); `api_version`, `context` (strings, required for bounded reads); unreleased v2.11 `expected_revision` (immutable commit/digest, requires bounded) |
| Returns | Plain-English per-resource report: ownership, health/drift, recent events, structured `nextSteps[]`; with `field_path`, also a path-scoped manager-evidence block while keeping the normal resource summary |
| When to load | For a known resource and exact canonical field path, call directly with `field_path`; do not run doctor, map, trace, or compare first solely to rediscover manager evidence. If the resource/scope is unknown or the user needs health/inventory, load `doctor`/`map` first. Missing or ambiguous field evidence remains unknown and does not fall back to resource-level summary. Manager evidence does not identify a person or latest writer. |

### `gitops_status`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout gitops status --format json` |
| Required args | — |
| Optional args | `namespace` (string); `context` (nonempty exact kubeconfig context); `with_confighub` (boolean); `confighub_space` (string); `confighub_since` (string); `confighub_stale_after` (string) |
| Returns | GitOps/controller backend, transport, sources, deployers, source/build/apply/sync stages, delivery evidence when requested, and `controllerCoverage[]` |
| When to load | "Is this deployed?" "Is delegated delivery healthy?" "Which controller families did cub-scout actually inspect?" "Is missing status absence or an RBAC/API omission?" Evidence only; never use it to force sync or declare application success by itself. |

### `gitops_settings`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout gitops settings --format json --view summary` (the `view` argument selects another view) |
| Required args | — |
| Optional args | `setting` (array of `name` or `name=value`; all must match); `project` (array of Argo CD project names; Flux objects are left out when set); `namespace` (string, the deployer objects' namespace); `context` (nonempty exact kubeconfig context); `view` (`summary` default, `deployers`, `groups`, `settings`, `all`) |
| Returns | The delivery settings each Argo CD Application and Flux Kustomization/HelmRelease declares: auto-sync, self-heal, prune, sync and apply options, suspension. `summary` maps each setting to its values and each value to the deployers that have it; `unset` marks deployers counted under a controller default. Every view keeps the scope, counts, reads and notes that say whether it is complete |
| When to load | "Which Applications sync automatically without self-heal?" "Which Flux objects are suspended?" "Which deployers share this option?" Declared settings only: no verdict, and nothing about whether a controller acted. An unknown argument, a wrong type, an empty string or an empty array is refused, not ignored: leave a filter out to ask for everything. Preview since v2.13.4: shapes may still change |

### `release_check`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout release check --format json` |
| Required | `bundle` (digest-pinned OCI configuration), `controller` (Kind/name), `api_version`, `controller_namespace`, `context` (target kube context) |
| Optional | `controller_context` (default target context), `oci_layout` (read-only local layout), `max_objects` (integer 1-100, default 100), `check_running_image` (bool, opt-in running-image identity), `max_pods` (integer 1-200, default 50) |
| Returns | Stage verdicts for bundle/controller/configuration/workloads (plus `running-image` when `check_running_image` is set), actual request counts, dated resource reads, omissions and existing fingerprinted configuration/workload receipts |
| When to load | "Did this exact configuration release reach this target?", "Where is this release waiting?", or "Is the image actually running the one I intended?" (set `check_running_image`). Scope and immutable bundle are required; a mutable tag stays UNKNOWN, and there is no application-success claim. The configuration-bundle digest and the container-image digest are never compared |

Each invocation is a fresh bounded check, not a shared cache. Unsupported or
ambiguous controller/source/target shapes remain inconclusive. See the
[example and read budgets](../../examples/oci-release-check/).

## Connected tools (13)

Registered only when `cub-scout mcp serve` finds a ConfigHub session the server
accepts. No `cub` binary is needed for that, or for the tools marked as read
through the SDK below. A tool whose read still runs `cub` fails at the call on
a machine without it, and says so.

### `compare_three_way`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout compare three-way [...] --format json` |
| Required args | one of `scope` (resource/namespace/cluster selector) or `view` (View UUID/URL) |
| Optional args | `namespace` (string), `context` (nonempty exact kubeconfig context; no target inference) |
| Returns | DRY (ConfigHub) / WET (rendered) / LIVE (cluster) three-way comparison with per-field agreement and attribution evidence; rolled-up `summary.agreement` (agreed / converging / diverged / partial), context label and explicit omissions; incomplete coverage does not certify agreement |
| When to load | "Does governed state agree with live state?" "Is this change sign-off-ready?" After scope is identified. |

### `compare_source_truth`

| Aspect | Detail |
|---|---|
| Wraps | `cub-scout compare source-truth <target> -n <ns> --strategy <s> --format json` |
| Required args | `target` (string — `kind/name`), `namespace` (string), `strategy` (string) |
| Optional args | `context` (nonempty exact kubeconfig context) |
| Returns | Source-truth evidence document: `declared_strategy`, `status` (PASS/WATCH/BLOCK/ASK), `source_truth` verdict (AGREED/MISMATCH/INCOMPLETE/BLOCKED/UNKNOWN), per-surface evidence, `proof_gaps[]`, `safe_next_action` |
| When to load | "Is this workload's source of truth consistent end-to-end under a declared strategy?" Strategy is **required**, never inferred. The contract refuses to PASS when any required field is missing. NEVER use to approve, repair, or mutate — evidence only. |

The registered MCP `strategy` enum is generated from the same
`agent.AllStrategies()` registry as the CLI, so Phase 1 and Phase 2 strategy
values stay in parity. See [source-truth-strategies](source-truth-strategies.md)
for the full enum.

### `confighub_changeorder_get`

Read one known ChangeOrder's reported Stage/State and stored workflow/prerequisite
declarations. Requires exact `changeorder` and exact `space` strings; matching
qualified slugs are accepted, wildcard/options/mismatched paths and environment
fallback are refused. The connected runner performs one
`cub changeorder get <order> -o json --space <space>` (since v2.14.0 read through
the ConfigHub SDK by default for an order named by slug; an order named by ID
runs `cub`), verifies returned identity and space, and uses the same projection as `history changeorder` (ASCII/JSON/MD
and `--tui`). Unknown evaluated outcomes stay unknown, including Completed,
missing workflow and denied reads. No controller health, prerequisite execution,
approval, publish eligibility or advancement decision is inferred. `readContract`
pins inspected SDK v0.6.8 source, not runtime server version. Genuine captures
and live CLI/TUI acceptance remain pending.

### `confighub_changesets`

| Aspect | Detail |
|---|---|
| Wraps | The ChangeSet list of the space, as `cub changeset list -o json --space <space>` prints it (read through the ConfigHub SDK by default since v2.14.0, with no `cub` process; the same JSON `cub` prints) |
| Required args | `space` (string — slug/ID, or `*` for an explicit all-spaces read; cub has no default space) |
| Optional args | `where` (string — filter expression) |
| Returns | Governed ChangeSet history and receipts from ConfigHub |
| When to load | "What governed write changed this unit?" "Who applied the change?" After `trace` or `confighub_units` has identified the governed object. |

### `confighub_k8s_types`

| Aspect | Detail |
|---|---|
| Wraps | `cub k8s types [<type>] -o json` (runs `cub`; needs the `cub` binary) |
| Required args | Either `space` or `target` |
| Optional args | `type` (string — `all`, kubectl-style type, Kind, or full ConfigHub resource type); `space` (string — slug/ID, or `*` for an explicit all-spaces read); `target` (string array — `space-slug/target-slug`); `namespace` (string); `where` (string — ConfigHub entity filter); `where_resource` (string — stored-resource configuration filter) |
| Returns | ConfigHub Resource type summaries: resource type, API version, kind, resource count, unit count, and space count |
| When to load | "Which Kubernetes types does ConfigHub already hold?" "Which custom resources exist before I fetch bodies?" Cheapest ConfigHub-side survey before `confighub_k8s_resources`; not live cluster health. |

### `confighub_k8s_resources`

| Aspect | Detail |
|---|---|
| Wraps | `cub k8s get <type> [<name> ...] -o json` (runs `cub`; needs the `cub` binary) |
| Required args | `type`, plus either `space` or `target` |
| Optional args | `names` (string array); `space` (string — slug/ID, or `*` for an explicit all-spaces read); `target` (string array — `space-slug/target-slug`); `namespace` (string); `where` (string — ConfigHub entity filter); `where_resource` (string — stored-resource configuration filter); `show` (`list`, `detail`, or `data`) |
| Returns | ConfigHub-stored intended Kubernetes resources, joined to Space, Unit, Target, namespace, name, kind, API version, and resource type. With `show=data` or serialized output, bodies are included when `cub` provides them. |
| When to load | "What Kubernetes resources does ConfigHub say should exist for this space/target?" "Fetch intended Deployment YAML without hitting every cluster." Pair with `gitops_status`, `trace`, `compare_three_way`, or `compare_source_truth` for live/controller proof. |

### `confighub_live_status`

| Aspect | Detail |
|---|---|
| Wraps | Two reads, as `gitops status --with-confighub` makes them: `cub space list -o json --select Slug,SpaceID,Annotations,Labels` (with `--where "Slug = '<space>'"` unless `space` is `*`; runs `cub`), and the Releases created in the window, as `cub release list --space <space> -o json --where "CreatedAt > '<cutoff>'"` prints them (SDK for one space) |
| Required args | `space` (string — slug, or `*` for an explicit all-spaces read) |
| Optional args | `since` (string, default `24h`): the Release window. An older Release's status is not read |
| Returns | `{spaces, releases, releasesSince}` as read (or `releasesError`), plus `structuredContent.liveStatuses[]`: since v2.14.2, each Space and Target's newest Release `LiveStatus` (`recordedOn: release`), as ConfigHub records it from 2026-10-03, and the `confighub.com/live-status` annotation older servers kept (`recordedOn: space-annotation`); `structuredContent.omissions[]` names a space with neither, or a release list that could not be read |
| When to load | "Did the evented status feedback report sync, health, operation, revision, and freshness for this space?" Evidence only; controller/runtime truth still belongs to the underlying systems. |

### `confighub_releases`

| Aspect | Detail |
|---|---|
| Wraps | The Release list of the space, as `cub release list --space <space> -o json` prints it (read through the ConfigHub SDK by default since v2.14.0, with no `cub` process; the same JSON `cub` prints; `space: "*"` runs `cub`) |
| Required args | `space` (string — slug/ID, or `*` for an explicit all-spaces read) |
| Optional args | `where` (string — filter expression, usually time- or target-bounded) |
| Returns | ConfigHub Release rows for release/OCI bundle history |
| When to load | "Which release or OCI bundle was published for this space/target/time window?" Pair with `gitops status`, `trace`, or `compare_source_truth` for controller/runtime evidence. |

### `confighub_resources`

| Aspect | Detail |
|---|---|
| Wraps | The Resource list, as `cub resource list --space <space> -o json` prints it (read through the ConfigHub SDK by default since v2.14.0, with no `cub` process; the same JSON `cub` prints; with `select`, `filter`, `view` or `raw_data` it runs `cub`) |
| Required args | `space` (string — slug/ID, or `*` for an explicit all-spaces read) |
| Optional args | `where` (string — Resource/entity/Data filter); `contains` (string); `select` (string); `filter` (string — ConfigHub filter slug/ID); `view` (string — ConfigHub view slug/ID); `raw_data` (boolean) |
| Returns | ConfigHub Resource entity rows for indexed resources extracted from Unit data, including resource metadata and selected fields/data when requested |
| When to load | "Which indexed resources match this fleet-wide predicate?" "Find resources by ResourceType, ResourceName, TargetID, Unit/Space labels, or Data paths." Lower-load alternative to iterating Units; not live cluster health. |

### `confighub_unit_events`

| Aspect | Detail |
|---|---|
| Wraps | The UnitEvent list of the space or of one Unit, as `cub unit-event list [unit] --space <space> -o json` prints it (read through the ConfigHub SDK by default since v2.14.0, with no `cub` process; the same JSON `cub` prints; one Unit named by ID or as `<space>/<slug>`, or one Unit with `space: "*"`, runs `cub`) |
| Required args | `space` (string — slug/ID, or `*` for an explicit all-spaces read) |
| Optional args | `unit` (string); `where` (string — filter expression, usually time-bounded) |
| Returns | ConfigHub UnitEvent rows for source/action evidence |
| When to load | "What event or source action happened for this unit or space?" Read-only history; not an event-consumer subscription and does not advance cursors. |

### `confighub_units`

| Aspect | Detail |
|---|---|
| Wraps | The Unit list, as `cub unit list -o json --space <space>` prints it (read through the ConfigHub SDK by default since v2.14.0, with no `cub` process; the same JSON `cub` prints) |
| Required args | `space` (string — slug/ID, or `*` for an explicit all-spaces read; cub has no default space) |
| Optional args | `where` (string); `contains` (string — full-text query) |
| Returns | ConfigHub unit inventory + cluster-to-ConfigHub linkage |
| When to load | "Which ConfigHub unit corresponds to this resource?" "Governed unit inventory before drilling into one unit." After `doctor` / `map` / `explain` / `trace` has identified the cluster-side object. |

### `confighub_unit_get`

| Aspect | Detail |
|---|---|
| Wraps | One Unit, as `cub unit get -o json <unit> --space <space>` prints it (read through the ConfigHub SDK by default since v2.14.0, with no `cub` process; the same JSON `cub` prints; a Unit named by ID, or as `<space>/<slug>` together with `space`, runs `cub`) |
| Required args | `unit` (string — slug with `space`, `<space>/<slug>`, or unit ID) |
| Optional args | `space` (string — required when `unit` is a bare slug; `*` is refused unless `unit` is an ID) |
| Returns | Exact ConfigHub unit details: intended state, last-applied revision, live revision, ConfigHub URL |
| When to load | ONLY after the unit slug/ID is known. "Show me the intended/applied/live revision for unit X." If unit is unknown, use `confighub_units` first. |

### `confighub_attestations`

| Aspect | Detail |
|---|---|
| Wraps | One revision and the attestation list of its space, as `cub revision get <unit> <n> --space <space> -o json` and `cub attestation list --space <space> -o json` print them (read through the ConfigHub SDK by default since v2.14.0; each response is limited to 1 MiB) |
| Required args | `space_id` (string), `unit` (string — unit slug), `revision` (integer ≥ 1): the exact origin already observed for a resource |
| Optional args | `unit_id` (string) |
| Returns | The attestations ConfigHub records directly against that revision, with observed expiry and revocations |
| When to load | ONLY once `trace` or `explain --with-confighub` has given the exact space, unit and revision. It reports claims as recorded. It cannot evaluate approval or inherited coverage, and it says nothing about runtime health |

## What's NOT in the catalog

The closed catalog is verified by `cmd/cub-scout/mcp_test.go`. The following cub-scout CLI verbs are intentionally NOT exposed as MCP tools (currently):

| Verb | Why not |
|---|---|
| `patterns_detect` | Specialized; CLI invocation is the better surface for this verb |
| `compare_drift` (file vs live) | Requires a local YAML file argument — awkward to expose over MCP (the file path is relative to the cub-scout process, not the agent) |
| `compare` (resource mode) | Subsumed by `compare_three_way` in connected mode |
| `history <resource>` | Use `confighub_changesets` instead — same conceptual surface, different naming for the MCP tier |
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

`confighub_changeorder_get` transforms successful cub JSON through its source-pinned
identity-checked projection before returning it. Failed cub stdout is not exposed
through the generic retained Scout JSON error path.

For connected tools that wrap `cub` (not cub-scout) — `confighub_changeorder_get`, `confighub_changesets`,
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


Recorded `map` pagination (opt-in preview): optional integer `page_size` (1..500)
and nonempty `cursor` bound to the same recording, scope and size. Incompatible
with `summary`. Paged responses use `map-list-recorded-page.v1` in text and
`structuredContent.data`, with full-scope totals and per-page returned count.
This limits records, not bytes/tokens; default recorded responses are unchanged.


Recorded `map` report-data budget (opt-in preview): optional integer
`max_report_json_bytes` (1..4194304), for full/summary/page canonical JSON data.
Oversize is an explicit error without clipping fields or resources. This excludes
MCP text/structured duplication, protocol/transport bytes, tokens and input work;
a successful MCP result may exceed this number. Default responses are unchanged.
