# cub-scout Command Matrix

Complete reference of all commands, options, TUI keys, and availability.

**Legend:**
- **Standalone**: Works with just kubectl access
- **Connected**: Requires ConfigHub auth (`cub auth login`)
- **Full Product**: Available in ConfigHub Pro

---

## Top-Level Commands

| Command | Description | Standalone | Connected | Full Product |
|---------|-------------|:----------:|:---------:|:------------:|
| `map` | Interactive TUI explorer | Yes | Yes | Yes |
| `trace` | Show GitOps ownership chain | Yes | - | - |
| `scan` | Scan for CCVEs | Yes | - | Yes |
| `snapshot` | Dump cluster state as JSON | Yes | - | - |
| `import` | Import workloads into ConfigHub | - | Yes | Yes |
| `import-argocd` | Import ArgoCD Application | - | Yes | Yes |
| `app-space` | Manage App Spaces | - | Yes | Yes |
| `remedy` | Execute CCVE remediation | Yes | - | Yes |
| `combined` | Git repo + cluster alignment | Yes | Yes | Yes |
| `parse-repo` | Parse GitOps repo structure | Yes | - | - |
| `demo` | Run interactive demos | Yes | - | - |
| `version` | Print version | Yes | - | - |
| `completion` | Generate shell completions | Yes | - | - |
| `setup` | Set up shell config | Yes | - | - |

---

## `map` Subcommands

| Command | Description | TUI Key | Standalone | Connected |
|---------|-------------|:-------:|:----------:|:---------:|
| `map` (default) | Interactive TUI | - | Yes | Yes |
| `map --hub` | ConfigHub hierarchy TUI | `H` | - | Yes |
| `map list` | Plain text resource list | - | Yes | - |
| `map status` | One-line health check | `s` | Yes | - |
| `map workloads` | Workloads by owner | `w` | Yes | - |
| `map deployers` | GitOps deployers | `p` | Yes | - |
| `map orphans` | Unmanaged resources | `o` | Yes | - |
| `map crashes` | Failing pods/deployments | `c` | Yes | - |
| `map issues` | Resources with problems | `i` | Yes | - |
| `map drift` | Desired vs actual state | `d` | Yes | - |
| `map bypass` | Factory bypass detection | `b` | Yes | - |
| `map sprawl` | Configuration sprawl | `x` | Yes | - |
| `map deep-dive` | All cluster data with LiveTree | `4` | Yes | Yes |
| `map app-hierarchy` | Inferred ConfigHub model | `5`/`A` | Yes | - |
| `map dashboard` | Unified health dashboard | - | Yes | - |
| `map fleet` | Multi-cluster fleet view | - | - | Yes |
| `map hub` | ConfigHub hierarchy | `H` | - | Yes |
| `map queries` | Saved queries | - | Yes | - |

---

## `map` Options

| Option | Description | Applies To |
|--------|-------------|------------|
| `--hub` | Launch ConfigHub hierarchy TUI | `map` |
| `--json` | Output in JSON format | `map`, `map list` |
| `--verbose` | Show additional details | `map` |
| `-q, --query` | Query expression | `map list` |
| `--namespace` | Filter by namespace | `map list` |
| `--kind` | Filter by resource kind | `map list` |
| `--owner` | Filter by owner type | `map list` |
| `--since` | Resources changed since | `map list` |
| `--count` | Output count only | `map list` |
| `--names-only` | Output names only | `map list` |

---

## `trace` Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace of the resource |
| `--app` | Trace ArgoCD app by name |
| `-r, --reverse` | Reverse trace (walk ownerRefs up) |
| `--json` | Output as JSON |

---

## `scan` Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to scan |
| `--state` | State scan only (stuck reconciliations) |
| `--kyverno` | Kyverno scan only (PolicyReports) |
| `--timing-bombs` | Expiring certs, quota limits |
| `--dangling` | Dangling/orphan resources |
| `--include-unresolved` | Include Trivy/Kyverno findings |
| `--file` | YAML file to scan (static analysis) |
| `--list` | List all KPOL policies |
| `--threshold` | Duration threshold for stuck (default: 5m) |
| `--json` | Output as JSON |
| `--verbose` | Show detailed output |

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
| `--list` | List auto-fixable CCVEs |
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
| `-n, --namespace` | Namespace to scan |
| `--suggest` | Generate Hub/App Space proposal |
| `--apply` | Create App Space and Units |
| `--dry-run` | Show without making changes |
| `--json` | Output as JSON |

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
| `demo ccve` | CCVE-2025-0027 demo (~2 min) |
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
| `←`/`h` | Collapse / go to parent |
| `→`/`l` | Expand |
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
| `S` | Scan | Scan for CCVEs |
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
| `P` | Panel | WET↔LIVE side-by-side view |
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
