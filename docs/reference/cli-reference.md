# Complete CLI Reference (A-Z)

Alphabetical command catalog for `cub-scout`.

Use this file to answer "what commands exist?"

For other questions:
- exact usage examples: [Command Reference](commands.md)
- stable flags, exit codes, and schemas: [CLI Contract Reference](cli-contract.md)
- workflow guidance: [CLI Guide](../../CLI-GUIDE.md)

Source of truth:
- `cub-scout --help`
- `cub-scout <command> --help`

---

## Top-Level Commands (Alphabetical)

| Command | What It Does | Primary Docs | Demo / Workflow |
|---------|---------------|--------------|-----------------|
| `app` | Manage ConfigHub Apps | [Command Reference](commands.md#app) | [Argo import demo](../../examples/argo-import-confighub-demo/) |
| `audit` | Break-glass audit trail tools | [Command Reference](commands.md#audit-list) | [Connect and compare](../../examples/connect-and-compare/) |
| `bundle` | Inspect, replay, diff, and summarize debug bundles | [Command Reference](commands.md#bundle-inspect-v015) | [Artifact workflows](../../examples/workflows/) |
| `catalog` | Manage bundle catalogs | [Command Reference](commands.md#catalog-list-v015) | [Artifact workflows](../../examples/workflows/) |
| `compare` | Compare Git, bundle, live, or connected intent/render/live state; in connected mode each field mismatch carries attribution evidence (`cause`, `managerHint`, `gitSource`, `bindingSource`) | [Command Reference](commands.md#compare) | [Connect and compare](../../examples/connect-and-compare/) |
| `context-pack` | Export deterministic AI context JSON | [How-to](../howto/context-pack-v2.md) | [AI integration](../../examples/ai-integration/) |
| `debug` | Guided GitOps debugging wizard | [Command Reference](commands.md#debug) | [Platform example](../../examples/platform-example/) |
| `doctor` | One-command cluster health summary | [Command Reference](commands.md#doctor) | [Connect and compare](../../examples/connect-and-compare/) |
| `explain` | Plain-English ownership and lineage for one resource | [Command Reference](commands.md#explain) | [New user puzzle quest](../../examples/new-user-puzzle-quest/) |
| `fleet` | Fleet-level connected insights | [Command Reference](commands.md#fleet-outliers) | [Fleet import](../../examples/fleet-import/) |
| `gitops` | GitOps pipeline health and diagnostics | [Command Reference](commands.md#gitops-v014) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `graph` | Resource graph export and explanation | [Command Reference](commands.md#graph-v06) | [Graph export](../../examples/graph-export/) |
| `help` | Help for any command path | [CLI Guide](../../CLI-GUIDE.md#verify-behavior-locally) | - |
| `history` | Connected ChangeSet timeline for one resource | [Command Reference](commands.md#history) | [Connect and compare](../../examples/connect-and-compare/) |
| `impact` | Connected blast-radius preview for one unit | [Command Reference](commands.md#impact) | [Connect and compare](../../examples/connect-and-compare/) |
| `import` | Import workloads into ConfigHub | [Command Reference](commands.md#import) | [Argo import demo](../../examples/argo-import-confighub-demo/) |
| `map` | Interactive TUI and list/status/ownership views | [Command Reference](commands.md#map) | [New user puzzle quest](../../examples/new-user-puzzle-quest/) |
| `mcp` | Read-only MCP gateway | [Command Reference](commands.md#mcp) | [MCP gateway](../../examples/mcp-gateway/) |
| `patterns` | Pattern detection engine | [Command Reference](commands.md#patterns-v07) | [Patterns fixtures](../../test/fixtures/patterns/) |
| `quickstart` | Guided first-run tour | [Command Reference](commands.md#quickstart) | [New user puzzle quest](../../examples/new-user-puzzle-quest/) |
| `receipt` | Create and verify typed, fingerprinted evidence receipts (#446) | [Command Reference](commands.md#receipt) | [Receipts](../../examples/receipts/) |
| `suggest-remedy` | Describe a suggested remediation for a risk finding (read-only). Legacy `remedy` is accepted as an alias. | [Command Reference](commands.md#suggest-remedy) | [Running demos](../howto/running-demos.md) |
| `scan` | Risk and stuck-state scanning | [Command Reference](commands.md#scan) | [Lifecycle hazards](../../examples/lifecycle-hazards/) |
| `setup` | Shell setup and quick cluster connect helpers | [Command Reference](commands.md#setup) | - |
| `snapshot` | Dump cluster state as GSF JSON | [Command Reference](commands.md#snapshot) | [AI integration](../../examples/ai-integration/) |
| `status` | Show connection mode and cluster info | [Command Reference](commands.md#status) | [Connect and compare](../../examples/connect-and-compare/) |
| `summary` | Connected summary storage tools | [Command Reference](commands.md#summary-list) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `trace` | Trace ownership and source chain | [Command Reference](commands.md#trace) | [kro composition](../../examples/kro-composition/) |
| `tree` | Runtime, ownership, git, and composition hierarchies | [Command Reference](commands.md#tree) | [kro composition](../../examples/kro-composition/) |
| `version` | Print version/build information | [Command Reference](commands.md#version) | - |
| `views` | Resolve, open, and project ConfigHub Views (#391) | [Command Reference](commands.md#views) | - |
| `watch` | Stream observation events to webhook/file sinks | [Command Reference](commands.md#watch) | [Watch webhook](../../examples/watch-webhook/) |

---

## High-Value Subcommands (Alphabetical)

| Subcommand | Purpose | Docs | Demo |
|------------|---------|------|------|
| `audit list` | Break-glass accept/reject audit trail | [Command Reference](commands.md#audit-list) | [Connect and compare](../../examples/connect-and-compare/) |
| `compare drift` | Desired vs live drift detection | [Command Reference](commands.md#compare-drift) | [Drift examples](../../examples/drift/) |
| `compare source-truth` | Read-only source-truth evidence for Pilot acceptance (#393) | [Command Reference](commands.md#compare-source-truth) | - |
| `compare three-way` | Connected DRY/WET/LIVE comparison; `--source-path <local-checkout>` opts into stage-B `gitSource.file:line` back-resolution for raw-YAML sources | [Command Reference](commands.md#compare-three-way) | [Connect and compare](../../examples/connect-and-compare/) |
| `fleet outliers` | Cluster divergence report | [Command Reference](commands.md#fleet-outliers) | [Fleet import](../../examples/fleet-import/) |
| `gitops status` | GitOps pipeline health | [Command Reference](commands.md#gitops-v014) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `import apply` | Apply an import proposal JSON | [Command Reference](commands.md#import-apply) | [Import from live](../../examples/import-from-live/) |
| `import argocd` | Import one ArgoCD Application | [Command Reference](commands.md#import-argocd) | [Argo import demo](../../examples/argo-import-confighub-demo/) |
| `import cluster-aggregator` | Aggregate multiple import proposals | [Command Reference](commands.md#import-cluster-aggregator) | [Fleet import](../../examples/fleet-import/) |
| `import parse-repo` | Parse GitOps repo structure | [Command Reference](commands.md#import-parse-repo) | [Combined git+live](../../examples/combined-git-live/) |
| `map hooks` | Lifecycle hook inventory | [Command Reference](commands.md#map-hooks) | [Lifecycle hazards](../../examples/lifecycle-hazards/) |
| `map list` | Resource ownership inventory | [Command Reference](commands.md#map-list) | [Running demos](../howto/running-demos.md) |
| `map meaning` | Experimental meaning-first grouping | [Command Reference](commands.md#map-meaning) | [kro composition](../../examples/kro-composition/) |
| `map orphans` | Unmanaged/orphan resources | [Command Reference](commands.md#map-orphans) | [Orphans](../../examples/orphans/) |
| `mcp serve` | Read-only MCP server over stdio | [Command Reference](commands.md#mcp) | [MCP gateway](../../examples/mcp-gateway/) |
| `quickstart demo` | Run fixture-backed demos | [Command Reference](commands.md#quickstart-demo) | [Demos index](../../examples/demos/) |
| `receipt list` | Walk the local receipt store ($CUB_SCOUT_RECEIPTS_DIR → $XDG_DATA_HOME → $HOME/.local/share); JSON / ASCII output, sortable | [Command Reference](commands.md#receipt-list) | [Receipts](../../examples/receipts/) |
| `receipt show` | Render a saved receipt (ASCII / JSON). Does NOT verify the fingerprint — works on tampered receipts for forensic inspection | [Command Reference](commands.md#receipt-show) | [Receipts](../../examples/receipts/) |
| `receipt validate` | Recompute the receipt fingerprint and compare. Exit 0 OK / 1 mismatch / 2 I/O | [Command Reference](commands.md#receipt-validate) | [Receipts](../../examples/receipts/) |
| `receipt verify` | Build a typed, fingerprinted receipt asserting a predicate (applied-matches-spec / source-truth-pass / no-manual-edits-since / object-set-matches / workloads-converged / prerequisites-met). `--file <path>` verifies rendered install object sets or workload convergence; `--fail-on <verdict>` for CI gating (exit 2); `--input-attestation <path>` for chained receipts; `--save` to persist to the immutable local store | [Command Reference](commands.md#receipt-verify) | [Receipts](../../examples/receipts/) |
| `setup completion` | Generate shell completion script | [Command Reference](commands.md#setup-completion) | - |
| `setup connect` | Import or create a kubeconfig context | [Command Reference](commands.md#setup-connect) | [New user puzzle quest](../../examples/new-user-puzzle-quest/) |
| `summary list` | Query persisted summary records | [Command Reference](commands.md#summary-list) | [Connected summary storage](../../examples/connected-summary-storage/) |
| `summary slack` | Post summary digest to Slack | [Command Reference](commands.md#summary-slack) | [Connected summary storage](../../examples/connected-summary-storage/) |

---

## Verification Tip

For any command path, verify local runtime behavior directly:

```bash
cub-scout <command> --help
```
