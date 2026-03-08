# Complete CLI Reference (A-Z)

Alphabetical command index for `cub-scout`.

Source of truth:
- command inventory: `cub-scout --help`
- stable contracts/schemas: [CLI Contract Reference](cli-contract.md)
- usage examples: [Command Reference](commands.md) and [CLI Guide](../../CLI-GUIDE.md)

---

## Top-Level Commands (Alphabetical)

| Command | What It Does | Primary Docs | Demo / Workflow |
|---------|---------------|--------------|-----------------|
| `app-space` | Manage ConfigHub App structures | [CLI Guide](../../CLI-GUIDE.md) | [Import demos](../../examples/argo-import-confighub-demo/) |
| `apply` | Apply a proposal JSON (GUI workflow) | [CLI Guide](../../CLI-GUIDE.md) | [Import from live](../../examples/import-from-live/) |
| `audit` | Break-glass audit trail tools (`audit list`) | [Command Reference](commands.md#audit-list) | [Connect and compare](../../examples/connect-and-compare/) |
| `bundle` | Inspect/replay/diff/timeline debug bundles | [Command Reference](commands.md) | [Artifact workflows](../../examples/workflows/) |
| `catalog` | Manage bundle catalogs | [Command Reference](commands.md) | [Artifact workflows](../../examples/workflows/) |
| `combined` | Git + cluster/bundle compare (alias: `compare`) | [Command Reference](commands.md#combined) | [Connect and compare](../../examples/connect-and-compare/) |
| `completion` | Generate shell completion script | [Command Reference](commands.md) | - |
| `connect` | Create/import kube context and optionally launch map | [Command Reference](commands.md#connect) | [New user puzzle quest](../../examples/new-user-puzzle-quest/) |
| `context-pack` | Export deterministic AI context JSON | [How-to](../howto/context-pack-v2.md) | [AI integration](../../examples/ai-integration/) |
| `debug` | Guided GitOps debugging wizard | [CLI Guide](../../CLI-GUIDE.md) | [Platform example](../../examples/platform-example/) |
| `demo` | Run fixture demos | [CLI Guide](../../CLI-GUIDE.md) | [Demos index](../../examples/demos/) |
| `discover` | Alias for workload discovery (`map workloads`) | [Command Reference](commands.md#discover) | [Running demos](../howto/running-demos.md) |
| `doctor` | One-command cluster health summary | [Command Reference](commands.md#doctor) | [Connect and compare](../../examples/connect-and-compare/) |
| `drift` | Detect desired vs live drift | [CLI Guide](../../CLI-GUIDE.md) | [Drift examples](../../examples/drift/) |
| `explain` | Plain-English ownership + lineage for one resource | [Command Reference](commands.md#explain) | [New user puzzle quest](../../examples/new-user-puzzle-quest/) |
| `fleet` | Fleet-level connected insights (`fleet outliers`) | [Command Reference](commands.md#fleet-outliers) | [Fleet import](../../examples/fleet-import/) |
| `gitops` | GitOps diagnostics (`gitops status`) | [CLI Guide](../../CLI-GUIDE.md) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `graph` | Resource graph export/explain | [Command Reference](commands.md#graph-v06) | [Graph export](../../examples/graph-export/) |
| `health` | Alias for health issues (`map issues`) | [Command Reference](commands.md#health) | [Running demos](../howto/running-demos.md) |
| `help` | Command help and subcommand discovery | [CLI Guide](../../CLI-GUIDE.md) | - |
| `history` | Connected ChangeSet timeline for a resource | [Command Reference](commands.md#history) | [Connect and compare](../../examples/connect-and-compare/) |
| `impact` | Connected blast-radius preview for one unit | [Command Reference](commands.md#impact) | [Connect and compare](../../examples/connect-and-compare/) |
| `import` | Import workloads into ConfigHub | [Command Reference](commands.md#import) | [Argo import demo](../../examples/argo-import-confighub-demo/) |
| `import-argocd` | Import an ArgoCD Application | [CLI Guide](../../CLI-GUIDE.md) | [Argo import demo](../../examples/argo-import-confighub-demo/) |
| `import-cluster-aggregator` | Aggregate multi-cluster imports (GUI path) | [CLI Guide](../../CLI-GUIDE.md) | [Fleet import](../../examples/fleet-import/) |
| `map` | Interactive TUI and list/status/ownership views | [Command Reference](commands.md#map) | [New user puzzle quest](../../examples/new-user-puzzle-quest/) |
| `mcp` | Read-only MCP gateway (`mcp serve`) | [Command Reference](commands.md#mcp) | [MCP gateway](../../examples/mcp-gateway/) |
| `parse-repo` | Parse GitOps repository structure | [CLI Guide](../../CLI-GUIDE.md) | [Combined git+live](../../examples/combined-git-live/) |
| `patterns` | Pattern detection engine | [Command Reference](commands.md) | [Patterns fixtures](../../test/fixtures/patterns/) |
| `quickstart` | Guided first-run tour | [Command Reference](commands.md#quickstart) | [New user puzzle quest](../../examples/new-user-puzzle-quest/) |
| `remedy` | Execute remediation for findings | [CLI Guide](../../CLI-GUIDE.md) | [Running demos](../howto/running-demos.md) |
| `scan` | Risk/misconfiguration scanning | [Command Reference](commands.md#scan) | [Lifecycle hazards](../../examples/lifecycle-hazards/) |
| `setup` | Install/configure shell setup helpers | [Command Reference](commands.md#setup) | - |
| `snapshot` | Dump cluster state as GSF JSON | [CLI Guide](../../CLI-GUIDE.md) | [AI integration](../../examples/ai-integration/) |
| `status` | Show connection mode + cluster info | [Command Reference](commands.md#status) | [Connect and compare](../../examples/connect-and-compare/) |
| `summary` | Connected summary storage tools (`list`, `slack`) | [Command Reference](commands.md#summary-list) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `trace` | Trace ownership/source chain | [Command Reference](commands.md#trace) | [kro composition](../../examples/kro-composition/) |
| `tree` | Runtime/ownership/git/composition hierarchy views | [Command Reference](commands.md#tree) | [kro composition](../../examples/kro-composition/) |
| `version` | Print version/build info | [CLI Guide](../../CLI-GUIDE.md) | - |
| `watch` | Stream events to webhook/file sinks | [Command Reference](commands.md#watch) | [Watch webhook](../../examples/watch-webhook/) |

---

## High-Value Subcommands (Alphabetical)

| Subcommand | Purpose | Docs | Demo |
|------------|---------|------|------|
| `audit list` | Break-glass accept/reject audit trail | [Command Reference](commands.md#audit-list) | [Connect and compare](../../examples/connect-and-compare/) |
| `bundle diff` | Compare two bundles | [Command Reference](commands.md) | [Workflows](../../examples/workflows/) |
| `bundle inspect` | Inspect bundle metadata/files | [Command Reference](commands.md) | [Workflows](../../examples/workflows/) |
| `bundle replay` | Re-render bundle sections | [Command Reference](commands.md) | [Workflows](../../examples/workflows/) |
| `bundle summarize` | Summarize bundle evidence | [Command Reference](commands.md) | [Workflows](../../examples/workflows/) |
| `bundle timeline` | Time-series view over catalogs | [Command Reference](commands.md) | [Workflows](../../examples/workflows/) |
| `compare three-way` | DRY/WET/LIVE comparison (connected) | [Command Reference](commands.md#compare-three-way) | [Connect and compare](../../examples/connect-and-compare/) |
| `fleet outliers` | Cluster divergence report | [Command Reference](commands.md#fleet-outliers) | [Fleet import](../../examples/fleet-import/) |
| `gitops status` | GitOps pipeline health | [Command Reference](commands.md) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `map hooks` | Lifecycle hook inventory | [Command Reference](commands.md#map-hooks) | [Lifecycle hazards](../../examples/lifecycle-hazards/) |
| `map list` | Resource ownership inventory | [Command Reference](commands.md#map-list) | [Running demos](../howto/running-demos.md) |
| `map meaning` | Experimental meaning-first grouping | [Command Reference](commands.md#map-meaning) | [kro composition](../../examples/kro-composition/) |
| `map orphans` | Unmanaged/orphan resources | [Command Reference](commands.md#map-orphans) | [Orphans](../../examples/orphans/) |
| `mcp serve` | Read-only MCP server over stdio | [Command Reference](commands.md#mcp) | [MCP gateway](../../examples/mcp-gateway/) |
| `patterns detect` | Run pattern detection | [Command Reference](commands.md) | [Patterns fixtures](../../test/fixtures/patterns/) |
| `summary list` | Query persisted summary records | [Command Reference](commands.md#summary-list) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `summary slack` | Post summary digest to Slack | [Command Reference](commands.md#summary-slack) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `tree composition` | Crossplane/kro composition hierarchy | [README](../../README.md) | [kro composition](../../examples/kro-composition/) |

---

## Verification Tip

For any command path, verify local runtime behavior directly:

```bash
cub-scout <command> --help
```

