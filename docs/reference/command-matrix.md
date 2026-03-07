# cub-scout Command Matrix

Complete reference of all commands, options, TUI keys, and availability.

> This matrix is aligned to current `cub-scout --help` output.
> Dynamic availability still depends on cluster access, auth state, and installed companion CLIs.

---

## Top-Level Commands

| Command | Description |
|---------|-------------|
| `app-space` | Manage App Spaces |
| `apply` | Apply a proposal from JSON (GUI) |
| `bundle` | Work with debug bundles |
| `catalog` | Manage bundle catalogs |
| `combined` (`compare`) | Show Git repo + cluster alignment, or resource LIVE snapshot mode |
| `completion` | Generate shell completion script |
| `connect` | Quickly configure kube context from server URL or kubeconfig |
| `debug` | Guided GitOps debugging wizard |
| `demo` | Run interactive demos |
| `discover` | Discover resources (alias for `map workloads`) |
| `drift` | Detect drift between desired and live state |
| `fleet` | Fleet-level connected insights |
| `gitops` | GitOps status and diagnostics |
| `graph` | Resource graph operations |
| `health` | Check cluster issues (alias for `map issues`) |
| `import` | Import workloads into ConfigHub |
| `import-argocd` | Import an ArgoCD Application into ConfigHub |
| `import-cluster-aggregator` | Aggregate imports from multiple clusters (GUI) |
| `impact` | Connected blast-radius preview for one unit |
| `map` | Interactive map of resources and ownership |
| `parse-repo` | Parse a GitOps repository structure |
| `patterns` | Pattern detection engine |
| `remedy` | Execute remediation for risk findings |
| `scan` | Scan for risk issues and stuck states |
| `setup` | Set up shell completions and configuration |
| `snapshot` | Dump cluster state as GSF JSON |
| `status` | Show connection status and cluster info |
| `trace` | Trace any resource to its Git source |
| `tree` | Show hierarchical views of resources |
| `version` | Print version information |

---

## `connect` Options

| Option | Description |
|--------|-------------|
| `[server-url]` | Kubernetes API endpoint (defaults to `https://` if scheme omitted) |
| `--server` | Explicit server URL (alternative to positional arg) |
| `--token` | Bearer token auth (or use `K8S_BEARER_TOKEN`) |
| `--username`, `--password` | Basic auth credentials |
| `--client-cert`, `--client-key` | TLS client certificate auth |
| `--from-kubeconfig` | Import context from an existing kubeconfig |
| `--from-context` | Context name within `--from-kubeconfig` |
| `--context` | Name for created/imported destination context |
| `--kubeconfig` | Destination kubeconfig path |
| `--skip-verify` | Skip Kubernetes API connectivity check |
| `--map` | Launch `cub-scout map` immediately after connect |

---

## `map` Subcommands

| Command | Description | TUI Key | Notes |
|---------|-------------|:-------:|-------|
| `map` (default) | Interactive TUI | - | Local cluster explorer |
| `map --hub` | ConfigHub hierarchy TUI | `H` | Requires ConfigHub auth/context |
| `map list` | Plain text resource list | - | Scriptable output |
| `map status` | One-line health check | `s` | CI-friendly status summary |
| `map workloads` | Workloads by owner | `w` | Ownership-focused view |
| `map deployers` | Deployers (Deployments) | `p` | Deployer slice |
| `map cronjobs` | CronJob schedule/run view | - | Read-only operator visibility |
| `map jobs` | Job run history view | - | Read-only operator visibility |
| `map actions <kind/name>` | Action previews (runbook) | - | No mutation; preview only |
| `map activity` | Unified activity timeline | - | Flux/Argo/Helm/Event signals |
| `map previews` | Preview env detection | - | PR/Forgejo/Gitea heuristics |
| `map orphans` | Unmanaged + explicit AppSet-link orphans | `o` | Native/orphan focus |
| `map crashes` | Failing pods/deployments | `c` | Crash/failure focus |
| `map issues` | Resources with problems | `i` | Consolidated problems |
| `map drift` | Desired vs actual state | `d` | Drift-focused slice |
| `map bypass` | Factory bypass detection | `b` | Governance/debug slice |
| `map sprawl` | Configuration sprawl | `x` | Config-sprawl slice |
| `map deep-dive` | All cluster data with LiveTree | `4` | Deep inspection |
| `map app-hierarchy` | Inferred ConfigHub model | `5`/`A` | Inference view |
| `map dashboard` | Unified health dashboard | - | Summary dashboard |
| `map fleet` | Multi-cluster fleet view | - | Connected/fleet workflows |
| `map hub` | ConfigHub hierarchy | `H` | Hub view shortcut |
| `map queries` | Saved queries | - | Query workflow support |

---

## `map` Options

| Option | Description | Applies To |
|--------|-------------|------------|
| `--hub` | Launch ConfigHub hierarchy TUI | `map` |
| `--json` | Output in JSON format | `map`, `map list` |
| `--verbose` | Show additional details | `map` |
| `-q, --query` | Query expression | `map list` |
| `--namespace` | Filter by namespace | `map list`, `map cronjobs`, `map jobs`, `map activity`, `map previews`, `map actions` |
| `--kind` | Filter by resource kind | `map list` |
| `--owner` | Filter by owner type | `map list`, `map cronjobs`, `map jobs`, `map activity` |
| `--since` | Resources changed / timeline window | `map list`, `map activity` |
| `--count` | Output count only | `map list` |
| `--names-only` | Output names only | `map list` |
| `--stale-after` | Preview staleness threshold | `map previews` |
| `--format` | Output format (`ascii`, `json`, `md`) | `map list`, `map hooks`, `map cronjobs`, `map jobs`, `map actions`, `map activity`, `map previews` |

---

## `tree` Subcommands

| Command | Description | Notes |
|---------|-------------|-------|
| `tree` or `tree runtime` | Deployment → ReplicaSet → Pod trees | Runtime hierarchy view |
| `tree ownership` | Resources grouped by GitOps owner | Ownership hierarchy |
| `tree git` | Git repository structure | Git source structure |
| `tree patterns` | Detected GitOps patterns (D2, Arnie, etc.) | Pattern view |
| `tree config` | ConfigHub Unit relationships | Requires ConfigHub/cub context |
| `tree suggest` | Suggested Hub/AppSpace organization | Suggestion workflow |

---

## `tree` Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Filter by namespace |
| `-A, --all` | Include system namespaces |
| `--space` | ConfigHub space for config view |
| `--edge` | Edge type: clone (inheritance) or link (dependencies) |
| `--json` | Output as JSON |

---

## `trace` Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace of the resource |
| `--app` | Trace ArgoCD app by name |
| `-r, --reverse` | Reverse trace (walk ownerRefs up) |
| `--diff` | Show what would change on next reconciliation |
| `--artifacts` | Include source artifact provenance fields |
| `--format` | Output format (`ascii`, `json`, `md`) |
| `--json` | Output as JSON |

---

## `scan` Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to scan |
| `--state` | State scan only (stuck reconciliations) |
| `--kyverno` | Kyverno scan only (PolicyReports) |
| `--timing-bombs` | Expiring certs, quota limits |
| `--dangling` | Dangling/orphan resources (includes Argo ApplicationSet-link checks) |
| `--include-unresolved` | Include Trivy/Kyverno findings |
| `--file` | YAML file to scan (static analysis) |
| `--list` | List all KPOL policies |
| `--threshold` | Duration threshold for stuck (default: 5m) |
| `--json` | Output as JSON |
| `--verbose` | Show detailed output |

---

## `impact` Options

| Option | Description |
|--------|-------------|
| `--format` | Output format (`ascii`, `json`, `md`) |
| `--json` | Output as JSON |

---

## `fleet outliers` Options

| Option | Description |
|--------|-------------|
| `--format` | Output format (`ascii`, `json`, `md`) |
| `--json` | Output as JSON |

---

## `snapshot` Options

| Option | Description |
|--------|-------------|
| `-o, --output` | Output file (default: stdout) |
| `-n, --namespace` | Filter by namespace |
| `-k, --kind` | Filter by kind |

---

## `import` Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to import |
| `-w, --wizard` | Launch interactive TUI wizard |
| `--dry-run` | Preview without making changes |
| `--json` | Output as JSON |
| `-y, --yes` | Skip confirmation |
| `--no-log` | Disable logging to file |

---

## `import-argocd` Options

| Option | Description |
|--------|-------------|
| `--list` | List available ArgoCD Applications |
| `--dry-run` | Preview without making changes |
| `--show-yaml` | Show YAML content |
| `--disable-sync` | Disable auto-sync after import |
| `--delete-app` | Delete ArgoCD Application after import |
| `--space` | ConfigHub space to import into |
| `--argocd-namespace` | Namespace where ArgoCD is installed |
| `--raw` | Keep raw YAML with runtime fields |
| `--test-rollout` | Test by triggering rollout restart |
| `--test-update` | Test by adding annotation |
| `-y, --yes` | Skip confirmation |

---

## `remedy` Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to operate in |
| `--all` | Fix all auto-fixable issues |
| `--dry-run` | Show what would be changed (default: true) |
| `--force` | Skip confirmation for high-risk actions |
| `--file` | YAML file to scan and fix |
| `--list` | List auto-fixable risk issues |
| `--json` | Output as JSON |
| `--audit` | Log actions to audit file (default: true) |
| `--audit-file` | Audit log file path |
| `--timeout` | Timeout for each action (default: 30s) |

---

## `combined` Options

| Option | Description |
|--------|-------------|
| `--git-url` | Git repository URL |
| `--git-path` | Local path to Git repo |
| `--git-url-compare` | Right-side Git repository URL for Git↔Git compare |
| `--git-path-compare` | Right-side local Git repository path for Git↔Git compare |
| `-n, --namespace` | Namespace to scan |
| `--bundle` | Debug bundle directory as offline cluster snapshot |
| `--format` | Resource compare format (`ascii`, `json`, `md`) |
| `--suggest` | Generate Hub/App Space proposal |
| `--apply` | Create App Space and Units |
| `--dry-run` | Show without making changes |
| `--json` | Output as JSON |

Resource compare mode uses one positional argument:
- `cub-scout compare <kind/name> -n <namespace> --format ascii|json|md`
- Linked resources show DRY/WET/LIVE sections; unlinked resources degrade to LIVE-only with notes.
- Linked resource output includes mismatch highlights across DRY/WET/LIVE when values diverge.

---

## `app-space` Subcommands

| Command | Description | Connected |
|---------|-------------|:---------:|
| `app-space list` | List App Spaces | Yes |
| `app-space create` | Create an App Space | Yes |

---

## `demo` Subcommands

| Command | Description |
|---------|-------------|
| `demo list` | List available demos |
| `demo quick` | Quick demo (~30 sec) |
| `demo ccve` | Risk issue demo (~2 min) |
| `demo query` | Query language demo |
| `demo scenario <name>` | Narrative scenario demo |
| `demo --cleanup` | Remove demo resources |

---

## TUI Keyboard Shortcuts (Local Cluster Mode)

Press `?` in the TUI to see this help.

### Navigation

| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `←` | Collapse / go to parent |
| `→` | Expand |
| `Enter` | Cross-references (in panel view) |
| `Tab` | Cycle views |
| `[` | Previous namespace |
| `]` | Next namespace |
| `/` | Search |
| `r` | Refresh data |

### Views

| Key | View | Description |
|-----|------|-------------|
| `s` | Status | Dashboard overview |
| `w` | Workloads | Workloads by owner |
| `a` | Apps | Grouped by app label + variant |
| `p` | Pipelines | GitOps deployers (Flux, ArgoCD) |
| `h` | History | Connected ChangeSet timeline |
| `d` | Drift | Resources diverged from desired state |
| `o` | Orphans | Native resources (not GitOps-managed) |
| `c` | Crashes | Failing pods |
| `i` | Issues | Unhealthy resources |
| `u` | sUspended | Paused/forgotten resources |
| `b` | Bypass | Factory bypass detection |
| `x` | Sprawl | Config sprawl analysis |
| `D` | Dependencies | Upstream/downstream relationships |
| `G` | Git sources | Forward trace from Git |
| `4` | Cluster Data | All data sources TUI reads |
| `5`/`A` | App Hierarchy | Inferred ConfigHub model |
| `M` | Maps | Three Maps view |

### Actions

| Key | Action | Description |
|-----|--------|-------------|
| `Q` | Saved Queries | Filter resources with saved queries |
| `T` | Trace | Trace ownership chain for selected |
| `S` | Scan | Scan for risk issues |
| `I` | Import | Import wizard (bring workloads to ConfigHub) |

### Command Palette (`:`)

Press `:` to open the command palette. Type any shell command and press Enter.

```
:kubectl get pods
:cub-scout scan
:flux get kustomizations
```

- `↑`/`↓` — Navigate command history (last 20 commands)
- `Enter` — Execute command
- `Esc` — Cancel

Output appears inline (max 8 lines). Press `Esc` to dismiss.

### Help and Mode Switching

| Key | Action |
|-----|--------|
| `?` | Show help overlay (press any key to dismiss) |
| `H` | Switch to ConfigHub hierarchy (requires `cub auth login`) |
| `q` | Quit |

---

## TUI Keyboard Shortcuts (ConfigHub Hub Mode)

Press `?` in the TUI to see this help.

### Navigation

| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `←`/`h` | Collapse node |
| `→`/`l` | Expand node |
| `Enter` | Load details in right pane |
| `Tab` | Switch focus to details pane |

### Search & Filter

| Key | Action |
|-----|--------|
| `/` | Start search |
| `n`/`N` | Next/previous match |
| `f` | Toggle filter mode |

### Actions

| Key | Action | Description |
|-----|--------|-------------|
| `a` | Activity | Recent changes view |
| `B` | Toggle | Hub/AppSpace view |
| `M` | Maps | Three Maps view (GitOps + ConfigHub + Repos) |
| `P` | Panel | DRY↔WET↔LIVE side-by-side view |
| `c` | Create | Create new resource |
| `d`/`x` | Delete | Delete selected resource |
| `i` | Import | Import workloads from Kubernetes |
| `o` | Open | Open in browser |
| `O` | Switch | Switch organization |
| `r` | Refresh | Refresh data |

### Command Palette (`:`)

Press `:` to open the command palette. Type queries or shell commands.

**Query examples:**
```
:owner=Native              # Orphaned resources
:owner=Flux OR owner=ArgoCD   # GitOps managed
:namespace=prod*           # Prod namespaces
:labels[app]=nginx         # By label
```

**Command examples:**
```
:cub-scout map orphans
:cub-scout scan
:cub-scout trace
```

### Help and Mode Switching

| Key | Action |
|-----|--------|
| `?` | Show help overlay |
| `L` | Switch to local cluster TUI |
| `q` | Quit |

---

## Query Syntax

```bash
# Field operators
field=value           # Exact match
field!=value          # Not equal
field~=pattern        # Regex match
field=val1,val2       # IN list
field=prefix*         # Wildcard

# Logical operators
expr AND expr         # Both match
expr OR expr          # Either matches

# Available fields
kind, namespace, name, owner, status, cluster, labels[key]

# Owner values
Flux, ArgoCD, Helm, ConfigHub, Native

# Status values
Ready, NotReady, Failed, Pending, Unknown
```

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig |
| `CLUSTER_NAME` | `default` | Name for this cluster |

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error |
| 2 | No cluster connection |
