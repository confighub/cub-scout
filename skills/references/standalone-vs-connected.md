# Reference: standalone vs connected mode

The **mode axis**. cub-scout works without ConfigHub (`standalone`) and with ConfigHub auth (`connected`). What you can ask depends on which mode you're in. Most skills reference this distinction — this doc is the canonical place for the rule.

## The axis

| Aspect | Standalone | Connected |
|---|---|---|
| Trigger | The `cub` CLI is absent, or `cub auth status` does not report an authenticated session | `cub auth status` reports an authenticated session. This holds in either invocation form (`cub scout ...` or `cub-scout ...`); running as the plugin is not enough on its own |
| Required inputs | Just a kubeconfig context | kubeconfig + ConfigHub auth |
| Cluster reads | All read verbs work | All read verbs work |
| ConfigHub reads | Refused with a clear error | Available via `cub * get/list`, `cub unit get`, `cub link list` |
| Source-of-truth evidence | Cluster + git anchor (from controller tracer) | Cluster + git anchor + ConfigHub Unit + ConfigHub Link bindings |
| Receipts | Single subject (`k8s-live://`), `OmissionConfigHubUnitSubject` recorded | Dual subjects (`k8s-live://` + `confighub-unit://`) when linked |

The mode is **per-invocation**, not per-cluster. The same cluster can be observed standalone or connected from different shells; the difference is whether `cub auth login` has been run for the current shell.

## Detection

```bash
$ cub-scout status
ConfigHub:  ● Connected
Cluster:    default
Context:    prod-use2
Worker:     (none for this cluster)
```

vs.

```bash
$ cub-scout status
ConfigHub:  ○ Online (not authenticated)
            Run: cub auth login
Cluster:    default
Context:    prod-use2
```

`status` prints one of `● Connected`, `● Connected (auth expired)`, `○ Online (not authenticated)` or `○ Offline`. `Cluster` is `$CLUSTER_NAME` or the literal `default`, not the kube context.

**The mode line is not the pre-flight.** It describes `hub.confighub.com`; whether connected commands will actually run is `confighub_reads` in `cub-scout status --json` (with `confighub_reads_reason`), and the text output adds a line whenever the two disagree. Both disagreements are real: `● Connected` with `confighub_reads: false` (credentials exist, every connected command refuses), and `○ Offline` with `confighub_reads: true` (`cub` reads a self-hosted or air-gapped ConfigHub it can reach). Read `confighub_reads`; fall back to the mode line only against a binary that predates the field (v2.12.0 and earlier).

Every command that reads ConfigHub by running `cub` gates on the same check — `pkg/hub.RequireCubConnected()`, which runs `cub auth status`: `compare source-truth`, `views resolve`, `views project`, `compare three-way`, `import argocd`, the MCP gateway's connected tools, `history`, `audit list`, `receipt verify`, `doctor`'s three-way hint, `gitops status --with-confighub`, and the ConfigHub subjects in `watch` receipts. `cub auth status` is the check that notices an expired session; `cub auth get-token` prints a stored token and exits 0 even after expiry. The standalone and plugin forms run the same check, so they agree. Each refusal names its cause: reads turned off, `cub` not installed, or `cub`'s own reason for not being authenticated. (Converged after v2.12.0. A v2.12.0 or earlier binary routes the second group through `hub.NewClient().RequireConnected()`, which probes hub.confighub.com and then accepts an expired token — against those versions, a refusal saying `cub auth login` may be unfixable by logging in.) `pkg/hub.QuickMode()` is a display helper for the TUI header; it never consults `cub` and must not be used as a gate.

## What works in standalone

Every cub-scout verb that operates on cluster state alone — the read-only Kubernetes + GitOps observer surface. Specifically:

| Verb group | Verbs that work standalone |
|---|---|
| Observe | `doctor`, `map`, `trace`, `tree`, `scan`, `graph`, `snapshot`, `watch`, `bot`, `status` |
| Diagnose | `explain`, `debug`, `suggest-remedy`, `patterns`, `gitops status` |
| Compare | `compare drift --file <yaml>`, `compare three-way --source-path <local-checkout>` (stage B back-resolution); the resource-mode `compare` works but DRY layer is absent |
| Attribute | The full `cause` / `managerHint` / `gitSource` evidence on `compare` + `explain` JSON works — `bindingSource` requires connected mode |
| Adopt Existing Config | `import --dry-run` + `import --from-bundle` + `import --git-path` + `import argocd` + `import cluster-aggregator` + `import parse-repo` (preview-first) |
| Govern | `bundle`, `catalog` against on-disk artifacts only |
| Integrate | `mcp serve` (standalone tool set), `context-pack`, `bot` |
| Verify | `receipt verify --predicate applied-matches-spec` / `--predicate no-manual-edits-since`; `receipt show / validate / list` |

## What unlocks with connected

Everything that needs ConfigHub-side authority or cross-cluster snapshots:

| Verb group | Connected-only verbs |
|---|---|
| Compare | `compare three-way` (DRY/WET/LIVE), `compare source-truth --strategy <s>` |
| Attribute | Per-field `bindingSource` (which ConfigHub Link supplies this value) |
| Govern | `history`, `impact`, `fleet outliers`, `summary list/slack`, `views resolve`, `audit list` |
| Verify | `receipt verify --strategy <s>` (source-truth-pass predicate requires connected source-truth) |
| Integrate | MCP gateway registers the connected tool set (`compare_three_way`, `compare_source_truth`, `confighub_changesets`, `confighub_k8s_resources`, `confighub_k8s_types`, `confighub_live_status`, `confighub_releases`, `confighub_resources`, `confighub_unit_events`, `confighub_units`, `confighub_unit_get`) in addition to the standalone catalog |

The MCP catalog is **mode-aware**: `mcp serve` registers the standalone catalog always and adds the connected catalog when `cub auth status` reports OK.

## Trust-boundary differences

Standalone is the **safer** posture for many investigation contexts:

- No ConfigHub credentials in the shell
- No accidental ConfigHub reads
- Receipts work but explicitly record `OmissionConfigHubUnitSubject` to flag missing connected-mode evidence — no silent absence

Connected is the **richer** posture:

- DRY/WET/LIVE three-way evidence
- Strategy-typed source-truth verdicts (the contract Pilot consumes)
- Per-field binding provenance into the ConfigHub Link graph
- ConfigHub GUI deep-links (`confighubUrl` on compare/trace/explain)
- Cross-cluster fleet outliers + connected snapshots

A connected-mode receipt has strictly more evidence than a standalone one. The receipt's `omissions[]` field makes the difference explicit: standalone receipts always carry `OmissionConfigHubUnitSubject`; connected receipts may not (if the resource has a ConfigHub unit linkage).

## Standalone-first contract

cub-scout's worked examples in every skill **lead with standalone**, then enrich with connected. This is deliberate:

- Most operators try cub-scout standalone first (no auth setup)
- Standalone mode is the cluster-side floor — what works without ConfigHub
- Connected mode is the enrichment layer — what additional evidence ConfigHub adds

The skills' "Standalone vs connected" section makes this explicit per-skill. Don't reverse the order.

## Graceful degradation rules

Per `CLAUDE.md` § "Key Principles":

> 6. **Graceful degradation** — works without cluster, ConfigHub, or internet

Standalone is one axis; **offline** (no cluster) is another. Some verbs work offline against bundles or files:

| Mode | What works |
|---|---|
| Standalone + live cluster | Cluster reads, no ConfigHub |
| Connected + live cluster | Cluster reads + ConfigHub reads |
| Standalone + bundle / file | `bundle inspect/replay/diff/timeline/summarize`, `compare drift --file`, `catalog list`, `context-pack --bundle <path>`, `receipt show / validate / list` |
| Connected + bundle / file | Same as above; ConfigHub auth has no effect on bundle-side reads |
| No cluster, no bundle, no ConfigHub | `version`, `--help` |

Most operators don't need to think about the bundle / file mode — it's the CI / agent / forensic path. But skills like [`operator-incident-evidence`](../operator-incident-evidence/SKILL.md) lean on it.

## CI usage

For CI pipelines that need ConfigHub-side evidence (source-truth verdicts, audit trails, governance gates):

```bash
# cub-scout has no API-key mode of its own: it uses the cub CLI's login.
# Authenticate cub non-interactively by whatever means your ConfigHub
# deployment supports (see `cub auth --help`), then run in the same environment.
cub auth status
cub-scout compare source-truth deploy/api -n prod --strategy git-argo --format json
```

For CI pipelines that just need cluster reads:

```bash
# Standalone mode — no env var needed
cub-scout doctor -n prod --format json
cub-scout compare drift --file desired.yaml -n prod
```

The `--format json` flag is mode-agnostic; the contract shape per verb is the same regardless of mode (connected adds fields, doesn't reshape existing ones).

## Skills that consume this reference

Every skill mentions the standalone-vs-connected boundary in its "Standalone vs connected" section. Specific skills that pivot on the mode:

- [`scout-compare`](../scout-compare/SKILL.md) — `compare three-way` + `compare source-truth` require connected
- [`scout-govern`](../scout-govern/SKILL.md) — most Govern verbs require connected; `bundle` / `catalog` work standalone
- [`scout-verify`](../scout-verify/SKILL.md) — `applied-matches-spec` / `no-manual-edits-since` work standalone; `source-truth-pass` requires connected
- [`scout-attribute`](../scout-attribute/SKILL.md) — `bindingSource` is connected-only
- [`audit-fleet-conformance`](../audit-fleet-conformance/SKILL.md) — fleet-wide audits require connected
- [`confighub-source-truth`](../confighub-source-truth/SKILL.md) — connected is the explicit precondition
- [`observe-confighub-managed`](../observe-confighub-managed/SKILL.md) — the connected enrichment layer for ConfigHub-managed resources

## References

- Code: `pkg/hub/connected.go` `RequireCubConnected()`, `pkg/hub/client.go` `RequireConnected()`, `pkg/hub/mode.go` `QuickMode()` (display only), `cmd/cub-scout/status.go`
- Mode flag on receipts: `BuildReceiptInput.Connected` in `pkg/agent/receipt_build.go`
- Connected-mode gate examples: `cmd/cub-scout/source_truth.go` and `cmd/cub-scout/views.go` (`requireConfigHubFor`), `cmd/cub-scout/history.go` (`RequireConnected`)
- Receipts contract on standalone: `docs/reference/json-contracts.md` § Receipt Contract — `OmissionConfigHubUnitSubject` handling
