# cub-scout CLI Guide

Complete reference for all commands, options, TUI keys, and expected outputs.

---

## Top-Level Commands (14)

| Command | Description | Standalone | Connected |
|---------|-------------|:----------:|:---------:|
| `map` | Interactive TUI explorer | Yes | Yes |
| `trace` | Show GitOps ownership chain | Yes | - |
| `scan` | Scan for CCVEs | Yes | - |
| `snapshot` | Dump cluster state as JSON | Yes | - |
| `import` | Import workloads into ConfigHub | - | Yes |
| `import-argocd` | Import ArgoCD Application | - | Yes |
| `app-space` | Manage App Spaces | - | Yes |
| `remedy` | Execute CCVE remediation | Yes | - |
| `combined` | Git repo + cluster alignment | Yes | Yes |
| `parse-repo` | Parse GitOps repo structure | Yes | - |
| `demo` | Run interactive demos | Yes | - |
| `version` | Print version | Yes | - |
| `completion` | Generate shell completions | Yes | - |
| `setup` | Set up shell config | Yes | - |

---

## `map` — Interactive TUI

**What it does:** Opens an interactive terminal UI showing all cluster resources grouped by owner.

```bash
./cub-scout map
```

**Without cub-scout:**
```bash
kubectl get all -A -o wide
kubectl get all -A -o json | jq '.items[] | select(.metadata.labels["kustomize.toolkit.fluxcd.io/name"])'
kubectl get all -A -o json | jq '.items[] | select(.metadata.labels["argocd.argoproj.io/instance"])'
# ... and manually correlate results
```

**Expected output:**
```
┌─ cub-scout map ──────────────────────────────────────────────────┐
│ CLUSTER: kind-kind                                               │
├──────────────────────────────────────────────────────────────────┤
│ FLUX (12)         ARGOCD (8)        HELM (3)        NATIVE (45)  │
├──────────────────────────────────────────────────────────────────┤
│ > flux-system/Deployment/source-controller          Flux         │
│   flux-system/Deployment/kustomize-controller       Flux         │
│   argocd/Deployment/argocd-server                   ArgoCD       │
│   monitoring/Deployment/prometheus                  Helm         │
│   default/Deployment/nginx                          Native       │
└──────────────────────────────────────────────────────────────────┘
Press ? for help, q to quit
```

**Options:**
| Option | Description |
|--------|-------------|
| `--hub` | Launch ConfigHub hierarchy TUI (requires `cub auth`) |
| `--json` | Output in JSON format |
| `--verbose` | Show additional details |

---

## `map` Subcommands (17)

### `map list` — Plain Text Output

```bash
./cub-scout map list
./cub-scout map list -q "owner=Flux"
./cub-scout map list -q "owner=Native"    # Shadow IT
./cub-scout map list -q "namespace=prod*"
```

**Expected output:**
```
NAMESPACE         KIND          NAME                          OWNER
flux-system       Deployment    source-controller             Flux
flux-system       Deployment    kustomize-controller          Flux
argocd            Deployment    argocd-server                 ArgoCD
monitoring        Deployment    prometheus                    Helm
default           Deployment    nginx                         Native
```

**Options:**
| Option | Description |
|--------|-------------|
| `-q, --query` | Query expression |
| `--namespace` | Filter by namespace |
| `--kind` | Filter by resource kind |
| `--owner` | Filter by owner (Flux, ArgoCD, Helm, ConfigHub, Native) |
| `--since` | Resources changed since duration (1h, 24h, 7d) |
| `--count` | Output count only |
| `--names-only` | Output names only (for scripting) |
| `--json` | JSON output |

---

### `map status` — One-Line Health

```bash
./cub-scout map status
```

**Expected output:**
```
kind-kind: 45 resources | Flux: 12 ok | ArgoCD: 8 ok | Helm: 3 ok | Native: 22 | Issues: 0
```

---

### `map workloads` — Workloads by Owner

```bash
./cub-scout map workloads
```

Shows Deployments, StatefulSets, DaemonSets grouped by owner.

---

### `map deployers` — GitOps Deployers

```bash
./cub-scout map deployers
```

**Without cub-scout:**
```bash
kubectl get kustomizations -A
kubectl get helmreleases -A
kubectl get applications -A
```

---

### `map orphans` — Unmanaged Resources

```bash
./cub-scout map orphans
```

**Expected output:**
```
ORPHAN RESOURCES (not managed by GitOps)
═══════════════════════════════════════════════════════════════════

NAMESPACE         KIND          NAME                    AGE
default           Deployment    debug-pod               3d
default           ConfigMap     test-config             5d

Total: 2 orphaned resources
```

---

### `map crashes` — Failing Pods

```bash
./cub-scout map crashes
```

Lists pods in CrashLoopBackOff, Error, ImagePullBackOff.

---

### `map issues` — Resources with Problems

```bash
./cub-scout map issues
```

Shows resources with conditions != Ready.

---

### `map drift` — Desired vs Actual

```bash
./cub-scout map drift
```

Shows resources where live state differs from last-applied configuration.

---

### `map bypass` — Factory Bypass Detection

```bash
./cub-scout map bypass
```

Detects changes made outside GitOps (kubectl edits to managed resources).

---

### `map sprawl` — Configuration Sprawl

```bash
./cub-scout map sprawl
```

Analyzes configuration sprawl across namespaces.

---

### `map dashboard` — Unified Dashboard

```bash
./cub-scout map dashboard
```

Combined health + ownership view.

---

### `map deep-dive` — All Cluster Data

```bash
./cub-scout map deep-dive
```

Maximum detail for all GitOps resources with LiveTree views:
- Flux: GitRepositories, Kustomizations, HelmReleases
- ArgoCD: Applications, AppProjects, ApplicationSets
- Helm: Releases decoded from secrets
- Deployment → ReplicaSet → Pod trees

---

### `map app-hierarchy` — Inferred Structure

```bash
./cub-scout map app-hierarchy
```

Infers ConfigHub-style hierarchy from cluster analysis.

---

### `map queries` — Saved Queries

```bash
./cub-scout map queries
```

List and manage saved queries.

---

### `map fleet` — Multi-Cluster View

```bash
./cub-scout map fleet
```

Fleet view grouped by app and variant. Requires ConfigHub labels.

---

### `map hub` — ConfigHub Hierarchy

```bash
./cub-scout map --hub
./cub-scout map hub
```

Interactive TUI for ConfigHub hierarchy. Requires `cub auth login`.

---

## `trace` — Ownership Chain

```bash
./cub-scout trace deploy/nginx -n production
./cub-scout trace --app guestbook
./cub-scout trace pod/nginx-abc123 -n prod --reverse
```

**Expected output:**
```
TRACE: Deployment/nginx in production

  ✓ GitRepository/flux-system
    │ URL: https://github.com/myorg/infra
    │ Revision: main@sha1:abc123
    │
    └─▶ ✓ Kustomization/apps
          │ Path: ./apps/production
          │
          └─▶ ✓ Deployment/nginx
                Status: Managed by Flux
```

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace of the resource |
| `--app` | Trace ArgoCD app by name |
| `-r, --reverse` | Reverse trace (walk ownerRefs up) |
| `--json` | Output as JSON |

---

## `scan` — Configuration Issues

```bash
./cub-scout scan
./cub-scout scan -n production
./cub-scout scan --file manifest.yaml
```

**Expected output:**
```
CCVE SCAN: kind-kind
═══════════════════════════════════════════════════════════════════

CRITICAL (1)
───────────────────────────────────────────────────────────────────
[CCVE-2025-0001] GitRepository not ready
  Resource: flux-system/GitRepository/apps
  Message:  authentication required
  Fix:      kubectl create secret generic git-credentials ...

WARNING (2)
───────────────────────────────────────────────────────────────────
[CCVE-2025-0005] Application out of sync
  Resource: argocd/Application/guestbook

═══════════════════════════════════════════════════════════════════
Summary: 1 critical, 2 warning, 0 info
```

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to scan |
| `--state` | State scan only (stuck reconciliations) |
| `--kyverno` | Kyverno scan only (PolicyReports) |
| `--timing-bombs` | Expiring certs, quota limits |
| `--dangling` | Orphan HPAs, Services, Ingress, NetworkPolicy |
| `--include-unresolved` | Include Trivy/Kyverno findings |
| `--file` | YAML file to scan (static analysis, no cluster) |
| `--list` | List all KPOL policies in database |
| `--threshold` | Duration threshold for stuck (default: 5m) |
| `--json` | Output as JSON |
| `--verbose` | Detailed output |

---

## `snapshot` — Export State as JSON

```bash
./cub-scout snapshot -o state.json
./cub-scout snapshot -o - | jq '.entries[] | select(.owner.type == "Native")'
```

**Options:**
| Option | Description |
|--------|-------------|
| `-o, --output` | Output file (default: stdout) |
| `-n, --namespace` | Filter by namespace |
| `-k, --kind` | Filter by kind |

---

## `remedy` — Execute Remediation

```bash
./cub-scout remedy CCVE-2025-0687 -n production --dry-run
./cub-scout remedy --all --dry-run -n production
./cub-scout remedy --list
```

**Options:**
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

## `import` — Import Workloads

```bash
./cub-scout import -n production
./cub-scout import -n production --dry-run
./cub-scout import --wizard
```

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to import |
| `-w, --wizard` | Launch interactive TUI wizard |
| `--dry-run` | Preview without making changes |
| `--json` | Output as JSON |
| `-y, --yes` | Skip confirmation |
| `--no-log` | Disable logging to file |

---

## `import-argocd` — Import ArgoCD App

```bash
./cub-scout import-argocd --list
./cub-scout import-argocd guestbook --dry-run
./cub-scout import-argocd guestbook --show-yaml
```

**Options:**
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

## `combined` — Git + Cluster Alignment

```bash
./cub-scout combined --git-url https://github.com/org/repo --namespace demo
./cub-scout combined --git-url https://github.com/org/repo --suggest --apply
```

**Options:**
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

## `parse-repo` — Parse GitOps Repo

```bash
./cub-scout parse-repo --url https://github.com/fluxcd/flux2-kustomize-helm-example
./cub-scout parse-repo --path ./my-gitops-repo
```

**Options:**
| Option | Description |
|--------|-------------|
| `--url` | Git repository URL |
| `--path` | Local path to parse |
| `--json` | Output as JSON |

---

## `app-space` — Manage App Spaces

```bash
./cub-scout app-space list
./cub-scout app-space create
```

---

## `demo` — Interactive Demos

```bash
./cub-scout demo --list
./cub-scout demo quick
./cub-scout demo ccve
./cub-scout demo query
./cub-scout demo scenario bigbank
./cub-scout demo quick --cleanup
```

---

## `version` / `completion` / `setup`

```bash
./cub-scout version
./cub-scout completion bash > /etc/bash_completion.d/cub-scout
./cub-scout completion zsh > "${fpath[1]}/_cub-scout"
./cub-scout setup
```

---

## TUI Keyboard Shortcuts

Press `?` in the TUI to see help.

### Local Cluster Mode

#### Navigation
| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `←`/`h` | Collapse / go to parent |
| `→`/`l` | Expand |
| `Enter` | Cross-references (panel view) |
| `Tab` | Cycle views |
| `[` | Previous namespace |
| `]` | Next namespace |
| `/` | Search |
| `r` | Refresh data |

#### Views (17)
| Key | View | Description |
|-----|------|-------------|
| `s` | Status | Dashboard overview |
| `w` | Workloads | Workloads by owner |
| `a` | Apps | Grouped by app label + variant |
| `p` | Pipelines | GitOps deployers (Flux, ArgoCD) |
| `d` | Drift | Resources diverged from desired |
| `o` | Orphans | Native (unmanaged) resources |
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

#### Actions
| Key | Action | Description |
|-----|--------|-------------|
| `Q` | Saved Queries | Filter with saved queries |
| `T` | Trace | Trace ownership chain |
| `S` | Scan | Scan for CCVEs |
| `I` | Import | Import wizard |

#### Command Palette (`:`)
Press `:` to run shell commands:
```
:kubectl get pods
:cub-scout scan
:flux get kustomizations
```
- `↑`/`↓` — Navigate history (last 20)
- `Enter` — Execute
- `Esc` — Cancel

#### Help and Mode Switching
| Key | Action |
|-----|--------|
| `?` | Show help overlay |
| `H` | Switch to ConfigHub TUI |
| `q` | Quit |

### ConfigHub Hub Mode

#### Navigation
| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `←`/`h` | Collapse |
| `→`/`l` | Expand |
| `Enter` | Load details |
| `Tab` | Focus details pane |

#### Search & Filter
| Key | Action |
|-----|--------|
| `/` | Start search |
| `n`/`N` | Next/previous match |
| `f` | Toggle filter |

#### Actions
| Key | Action |
|-----|--------|
| `a` | Activity view |
| `B` | Toggle Hub/AppSpace |
| `M` | Three Maps view |
| `P` | Panel view (WET↔LIVE) |
| `c` | Create resource |
| `d`/`x` | Delete resource |
| `i` | Import workloads |
| `o` | Open in browser |
| `O` | Switch organization |
| `r` | Refresh |
| `?` | Help |
| `L` | Switch to local TUI |
| `q` | Quit |

---

## Query Syntax

```bash
./cub-scout map list -q "owner=Flux"
./cub-scout map list -q "owner=Native"           # Shadow IT
./cub-scout map list -q "namespace=prod*"        # Wildcard
./cub-scout map list -q "kind=Deployment"
./cub-scout map list -q "owner=Flux AND namespace=production"
./cub-scout map list -q "owner=Flux OR owner=ArgoCD"
./cub-scout map list -q "labels[app]=nginx"
```

**Operators:**
| Operator | Example | Description |
|----------|---------|-------------|
| `=` | `owner=Flux` | Exact match |
| `!=` | `owner!=Native` | Not equal |
| `~=` | `name~=nginx.*` | Regex match |
| `=a,b` | `owner=Flux,ArgoCD` | IN list |
| `=prefix*` | `namespace=prod*` | Wildcard |
| `AND` | `kind=Deployment AND owner=Flux` | Both match |
| `OR` | `owner=Flux OR owner=ArgoCD` | Either matches |

**Fields:**
| Field | Values |
|-------|--------|
| `owner` | Flux, ArgoCD, Helm, ConfigHub, Native |
| `namespace` | Any namespace |
| `kind` | Deployment, Service, ConfigMap, etc. |
| `name` | Resource name |
| `status` | Ready, NotReady, Failed, Pending, Unknown |
| `cluster` | Cluster name |
| `labels[key]` | Label value |

---

## Ownership Detection

| Owner | Detection Method |
|-------|------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| **ArgoCD** | `app.kubernetes.io/instance` + `argocd.argoproj.io/instance` labels |
| **Helm** | `app.kubernetes.io/managed-by: Helm` label |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | None of the above (kubectl-applied) |

**Priority:** Flux > ArgoCD > Helm > ConfigHub > Native

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
| 1 | Error (check stderr) |
| 2 | No cluster connection |

---

## See Also

- [README.md](README.md) — Project overview
- [docs/COMMAND-MATRIX.md](docs/COMMAND-MATRIX.md) — Complete reference table
- [docs/SCAN-GUIDE.md](docs/SCAN-GUIDE.md) — CCVE scanning deep dive
- [docs/ALTERNATIVES.md](docs/ALTERNATIVES.md) — Comparison with other tools
- [CONTRIBUTING.md](CONTRIBUTING.md) — How to contribute
