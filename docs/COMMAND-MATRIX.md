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

### Navigation

| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `←`/`h` | Collapse / go to parent |
| `→`/`l` | Expand |
| `Enter` | Select / expand details |
| `Tab` | Next view / focus details |
| `[` | Previous namespace |
| `]` | Next namespace |

### Views

| Key | View |
|-----|------|
| `s` | Status dashboard |
| `w` | Workloads |
| `p` | Pipelines (deployers) |
| `d` | Drift |
| `o` | Orphans |
| `c` | Crashes |
| `i` | Issues |
| `b` | Bypass detection |
| `x` | Sprawl analysis |
| `u` | Suspended resources |
| `a` | Apps |
| `4` | Cluster data (deep-dive) |
| `5`/`A` | App hierarchy |

### Actions

| Key | Action |
|-----|--------|
| `/` | Search |
| `n` | Next search match |
| `N` | Previous search match |
| `f` | Toggle filter mode |
| `r` | Refresh |
| `?` | Help |
| `q` | Quit |
| `:` | Command palette |
| `H` | Switch to ConfigHub TUI |
| `T` | Trace selected |
| `S` | Scan |
| `Q` | Query |
| `I` | Import |
| `M` | Maps |
| `G` | Git sources |
| `D` | Dependencies |

---

## TUI Keyboard Shortcuts (ConfigHub Hub Mode)

### Navigation

| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `←` | Collapse / go to parent |
| `→` | Expand |
| `Enter` | Load entity details |
| `Tab` | Focus details pane |

### Actions

| Key | Action |
|-----|--------|
| `/` | Search |
| `n` | Next match |
| `N` | Previous match |
| `f` | Toggle filter |
| `r` | Refresh |
| `?` | Help |
| `q` | Quit |
| `:` | Command palette |
| `O` | Switch organization |
| `L` | Switch to local cluster TUI |
| `i` | Import wizard |
| `C` | Create unit wizard |
| `X` | Delete wizard |
| `w` | Open in web browser |
| `A` | Activity/all units toggle |
| `P` | Panel view |

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
