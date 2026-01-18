# cub-scout CLI Reference

Complete command-line reference for `cub-scout`.

---

## Quick Start

```bash
# Clone and build
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
./run.sh

# Or install permanently
./run.sh --install
```

**Prerequisites:** Go 1.24+, kubectl configured with cluster access

---

## Command Overview

| Command | Description |
|---------|-------------|
| [`app-space`](#app-space) | Manage App Spaces |
| [`apply`](#apply) | Apply a proposal from JSON (GUI) |
| [`combined`](#combined) | Show Git repo + cluster aligned |
| [`completion`](#completion) | Generate shell completion script |
| [`fleet`](#fleet) | Aggregate imports from multiple clusters |
| [`hierarchy`](#hierarchy) | Interactive ConfigHub hierarchy explorer |
| [`import`](#import) | Import workloads into ConfigHub |
| [`import-argocd`](#import-argocd) | Import ArgoCD Application into ConfigHub |
| [`map`](#map) | Interactive resource map and queries |
| [`parse-repo`](#parse-repo) | Parse GitOps repository structure |
| [`scan`](#scan) | Scan for CCVEs and stuck states |
| [`snapshot`](#snapshot) | Dump cluster state as GSF JSON |
| [`trace`](#trace) | Show GitOps ownership chain |
| [`version`](#version) | Print version information |

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `CLUSTER_NAME` | Cluster identifier (default: `default`) |
| `KUBECONFIG` | Path to kubeconfig file (default: `~/.kube/config`) |

---

## app-space

Create, list, and manage App Spaces in ConfigHub.

```bash
cub-scout app-space [command]
```

### Subcommands

#### app-space create

```bash
cub-scout app-space create <name> [flags]
```

| Flag | Description |
|------|-------------|
| `--label` | Labels in `key=value` format (can be repeated) |
| `--set-context` | Set as current context after creation |
| `-h, --help` | Help for create |

#### app-space list

```bash
cub-scout app-space list [flags]
```

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `-h, --help` | Help for list |

### Examples

```bash
# Create an App Space
cub-scout app-space create payments-team

# Create and set as current context
cub-scout app-space create payments-team --set-context

# Create with labels
cub-scout app-space create payments-team --label team=payments --label owner=platform

# List all App Spaces
cub-scout app-space list
cub-scout app-space list --json
```

---

## apply

Apply a Hub/App Space proposal to create resources in ConfigHub. GUI companion to `import`.

```bash
cub-scout apply [proposal.json] [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview what would be created without making changes |
| `--no-log` | Disable logging to file |
| `-h, --help` | Help for apply |

### Examples

```bash
# Single cluster: generate, edit, apply
cub-scout import --json > proposal.json
# (GUI displays, user edits)
cub-scout apply proposal.json

# Fleet: multiple clusters -> unified proposal -> apply
cub-scout import-cluster-aggregator cluster*.json --suggest --json | cub-scout apply -

# Dry-run to preview
cub-scout apply proposal.json --dry-run
```

---

## combined

Parse a Git repo and scan a cluster, showing alignment between them.

```bash
cub-scout combined [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--git-url` | Git repository URL to parse |
| `--git-path` | Local path to Git repository |
| `-n, --namespace` | Namespace to scan in cluster |
| `--suggest` | Generate Hub/App Space model proposal |
| `--apply` | Create App Space and Units in ConfigHub |
| `--dry-run` | Show what would be created without making changes |
| `--json` | Output as JSON |
| `-h, --help` | Help for combined |

### Examples

```bash
# Combine Git repo with current cluster
cub-scout combined --git-url https://github.com/org/gitops-repo --namespace demo

# Generate Hub/App Space proposal
cub-scout combined --git-url https://github.com/org/gitops-repo --namespace demo --suggest

# Preview what would be created (dry-run)
cub-scout combined --namespace demo --suggest --apply --dry-run

# Apply: create App Space and Units in ConfigHub
cub-scout combined --namespace demo --suggest --apply

# Use local Git repo with JSON output
cub-scout combined --git-path ./my-repo --namespace demo --suggest --json
```

---

## completion

Generate shell completion script.

```bash
cub-scout completion [bash|zsh|fish|powershell]
```

### Setup

**Bash:**
```bash
source <(cub-scout completion bash)
# Or add to ~/.bashrc:
cub-scout completion bash >> ~/.bashrc
```

**Zsh:**
```bash
source <(cub-scout completion zsh)
# Or install to fpath:
cub-scout completion zsh > "${fpath[1]}/_cub-scout"
```

**Fish:**
```bash
cub-scout completion fish | source
# Or install:
cub-scout completion fish > ~/.config/fish/completions/cub-scout.fish
```

**PowerShell:**
```powershell
cub-scout completion powershell | Out-String | Invoke-Expression
```

---

## fleet

Aggregate import data from multiple clusters into a fleet view. GUI/multi-cluster companion to `import`.

```bash
cub-scout import-cluster-aggregator [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--suggest` | Generate unified Hub/App Space proposal |
| `--json` | Output as JSON |
| `-h, --help` | Help for fleet |

### Examples

```bash
# Full workflow: scan clusters, generate unified proposal, apply
for ctx in cluster-a cluster-b; do
  kubectl config use-context $ctx
  cub-scout import --json > ${ctx}.json
done
cub-scout import-cluster-aggregator cluster-*.json --suggest --json | cub-scout apply -

# Generate unified proposal
cub-scout import-cluster-aggregator cluster1.json cluster2.json --suggest

# Just aggregate (no proposal)
cub-scout import-cluster-aggregator cluster1.json cluster2.json cluster3.json
```

---

## hierarchy

Launch an interactive TUI to explore your ConfigHub hierarchy.

```bash
cub-scout hierarchy [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-h, --help` | Help for hierarchy |

### Navigation

| Key | Action |
|-----|--------|
| `↑/k, ↓/j` | Move up/down |
| `←/h` | Collapse node or go to parent |
| `→/l, Enter` | Expand node |
| `/` | Filter - type to filter |
| `f` | Toggle filter on/off |
| `n/N` | Jump to next/previous match |
| `i` | Import workloads from Kubernetes |
| `Esc` | Clear filter |
| `r` | Refresh data |
| `q` | Quit |

---

## import

Import your cluster workloads into ConfigHub. One command does everything.

```bash
cub-scout import [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace to import (discovers all if not specified) |
| `--dry-run` | Preview without making changes |
| `-y, --yes` | Skip confirmation |
| `--json` | Output as JSON (for GUI/scripting) |
| `--no-log` | Disable logging to file |
| `-h, --help` | Help for import |

### Examples

```bash
# Import everything (discovers all namespaces)
cub-scout import

# Import one namespace
cub-scout import -n argocd

# Preview what would be created
cub-scout import --dry-run

# Skip confirmation
cub-scout import -y

# JSON output (for GUI integration)
cub-scout import --json
```

---

## import-argocd

Import an ArgoCD Application's managed resources into ConfigHub as a Unit.

```bash
cub-scout import-argocd [application-name] [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--argocd-namespace` | `argocd` | Namespace where ArgoCD is installed |
| `--space` | auto-inferred | ConfigHub space to import into |
| `--list` | | List available ArgoCD Applications |
| `--dry-run` | | Preview what would be imported |
| `--show-yaml` | | Show YAML content (implies --dry-run) |
| `--raw` | | Keep raw YAML with all runtime fields |
| `--disable-sync` | | Disable auto-sync on Application after import |
| `--delete-app` | | Delete the ArgoCD Application after import |
| `--test-rollout` | | Test pipeline by triggering rollout restart |
| `--test-update` | | Test pipeline by adding annotation |
| `-y, --yes` | | Skip confirmation prompts |
| `-h, --help` | | Help for import-argocd |

### Examples

```bash
# List available ArgoCD Applications
cub-scout import-argocd --list

# Import a specific ArgoCD Application
cub-scout import-argocd guestbook

# Preview what would be imported (dry-run)
cub-scout import-argocd guestbook --dry-run

# Show YAML content that would be imported
cub-scout import-argocd guestbook --show-yaml

# Import and disable ArgoCD auto-sync
cub-scout import-argocd guestbook --disable-sync

# Import and delete the ArgoCD Application
cub-scout import-argocd guestbook --delete-app
```

---

## map

Query and explore Kubernetes resources, their ownership, and relationships.

```bash
cub-scout map [command] [flags]
```

### Global Flags

| Flag | Description |
|------|-------------|
| `--confighub-url` | ConfigHub API URL |
| `--token` | Agent authentication token |
| `--json` | Output in JSON format |
| `--verbose` | Show additional details |
| `-h, --help` | Help for map |

### Modes

- **Interactive** (default): Launch TUI dashboard
- **Plain text** (`map list`): Read from current Kubernetes context, scriptable output
- **Fleet** (`map fleet`): Query ConfigHub via cub CLI for fleet-wide visibility

### Subcommands

#### map list

List resources and their ownership.

```bash
cub-scout map list [flags]
```

| Flag | Description |
|------|-------------|
| `--namespace` | Filter by namespace |
| `--kind` | Filter by resource kind |
| `--owner` | Filter by owner (Flux, ArgoCD, Helm, Terraform, ConfigHub, Native) |
| `-q, --query` | Query expression |
| `--json` | Output as JSON |

**Query Syntax:**

| Pattern | Description |
|---------|-------------|
| `field=value` | Exact match (case-insensitive) |
| `field!=value` | Not equal |
| `field~=pattern` | Regex match |
| `field=val1,val2` | IN list (comma-separated) |
| `field=prefix*` | Wildcard match |
| `AND` | Both conditions must match |
| `OR` | Either condition must match |

**Available Fields:** `kind`, `namespace`, `name`, `owner`, `cluster`, `labels[key]`

**Examples:**

```bash
# List all resources from current cluster
cub-scout map list

# Filter by namespace and kind
cub-scout map list --namespace default --kind Deployment

# Query: GitOps-managed deployments
cub-scout map list -q "kind=Deployment AND owner!=Native"

# Query: Resources in production namespaces
cub-scout map list -q "namespace=prod*"

# Query: By label
cub-scout map list -q "labels[app]=nginx"
```

#### map fleet

Display units across spaces grouped by app and variant labels.

```bash
cub-scout map fleet [flags]
```

| Flag | Description |
|------|-------------|
| `--app` | Filter by app label |
| `--space` | Filter by space (App Space) |

**Examples:**

```bash
# View all apps across spaces
cub-scout map fleet

# Filter to specific app
cub-scout map fleet --app payment-api

# Filter to specific space
cub-scout map fleet --space payments-team
```

#### map queries

List and manage saved queries for filtering resources.

```bash
cub-scout map queries [command] [flags]
```

| Flag | Description |
|------|-------------|
| `--json` | Output in JSON format |

**Subcommands:**

| Command | Description |
|---------|-------------|
| `save <name> <query>` | Save a new user query |
| `delete <name>` | Delete a user query |
| `connect` | Check ConfigHub connection status |

**Examples:**

```bash
# List all saved queries
cub-scout map queries

# Save a new query
cub-scout map queries save my-apps "labels[team]=payments"

# Run a saved query
cub-scout map list -q unmanaged
```

---

## parse-repo

Parse a GitOps repository and show its structure.

```bash
cub-scout parse-repo [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--url` | Git repository URL to clone and parse |
| `--path` | Local path to parse |
| `--json` | Output as JSON |
| `-h, --help` | Help for parse-repo |

### Supported Patterns

- Single-repo (flux2-kustomize-helm-example style)
- D2 Fleet (clusters + tenants)
- D2 Infra (cluster add-ons)
- D2 Apps (namespace-scoped apps)

### Examples

```bash
# Parse a remote repo
cub-scout parse-repo --url https://github.com/fluxcd/flux2-kustomize-helm-example

# Parse a local directory
cub-scout parse-repo --path ./my-gitops-repo

# JSON output
cub-scout parse-repo --url https://github.com/org/repo --json
```

---

## scan

Scan the cluster for CCVEs including stuck states and Kyverno violations.

```bash
cub-scout scan [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace to scan (default: all namespaces) |
| `--state` | State scan only (stuck reconciliations) |
| `--kyverno` | Kyverno scan only (PolicyReports) |
| `--dangling` | Scan for dangling/orphan resources (HPA, Service, Ingress, NetworkPolicy) |
| `--json` | Output as JSON |
| `--list` | List all KPOL policies in database |
| `--verbose` | Show detailed output |
| `-h, --help` | Help for scan |

### Scan Types

1. **State scan** (`--state`) — Detect stuck HelmReleases, Kustomizations, and Applications
2. **Kyverno scan** (`--kyverno`) — Map PolicyReport violations to KPOL database
3. **Dangling scan** (`--dangling`) — Find orphan resources pointing to non-existent targets (CCVE-2025-0687 to 0690)

### Output

For stuck resources:
- **CCVE ID** — Reference to pattern database
- **Duration** — How long resource has been stuck
- **Remediation** — What to do
- **FIX command** — Copy-paste kubectl/flux/argocd command

For Kyverno violations:
- **Policy name and KPOL ID** — Mapped to policy database
- **Severity** — Critical, Warning, Info
- **Message** — What rule was violated

### Examples

```bash
# Full scan (Kyverno + state)
cub-scout scan

# Scan specific namespace
cub-scout scan -n production

# State scan only (stuck reconciliations)
cub-scout scan --state

# Kyverno scan only
cub-scout scan --kyverno

# Dangling/orphan resource scan
cub-scout scan --dangling

# Output as JSON
cub-scout scan --json

# List all KPOL policies in database
cub-scout scan --list
```

---

## snapshot

Take a snapshot of the current cluster state and output as GitOps State Format (GSF) JSON.

```bash
cub-scout snapshot [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-o, --output` | Output file (default: stdout, use `-` for explicit stdout) |
| `-n, --namespace` | Filter by namespace |
| `-k, --kind` | Filter by kind |
| `-h, --help` | Help for snapshot |

### Examples

```bash
# Output to stdout
cub-scout snapshot

# Output to file
cub-scout snapshot -o state.json

# Pipe to jq
cub-scout snapshot | jq '.entries[] | select(.owner.type == "flux")'

# Filter by namespace
cub-scout snapshot --namespace prod

# Filter by kind
cub-scout snapshot --kind Deployment
```

---

## trace

Trace the full ownership chain from Git source to deployed resource.

```bash
cub-scout trace <kind/name> or <kind> <name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace of the resource (default: flux-system) |
| `--app` | Trace Argo CD application by name |
| `--json` | Output as JSON |
| `-h, --help` | Help for trace |

### Output Shows

- Full chain from GitRepository -> Kustomization/HelmRelease -> Resource
- Status and revision at each level
- Where in the chain something is broken (if applicable)

### Examples

```bash
# Trace a deployment
cub-scout trace deployment/nginx -n demo

# Trace with kind and name separately
cub-scout trace Deployment nginx -n demo

# Trace an Argo CD application directly
cub-scout trace --app frontend-app

# Output as JSON
cub-scout trace deployment/nginx -n demo --json
```

---

## version

Print version information.

```bash
cub-scout version
```

---

## Test Tools

The `test/atk/` directory contains test tools for development and demos.

### ./test/atk/map

Display cluster ownership map via TUI.

```bash
./test/atk/map [flags]
```

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `--mode=admin` | Admin view: Org → Space → Unit hierarchy |
| `--namespace=NS` | Filter by Kubernetes namespace |
| `--space=SPACE` | Filter by ConfigHub space |
| `--group-by=LABEL` | Group variants by label (default: app) |

### ./test/atk/scan

Scan for CCVEs via TUI.

```bash
./test/atk/scan [flags]
```

| Flag | Description |
|------|-------------|
| `--severity SEV` | Filter by severity: critical, warning, info |
| `--category CAT` | Filter by category |
| `--ccve-dir DIR` | Additional CCVE definitions |

### ./test/atk/demo

Run interactive demos.

```bash
./test/atk/demo [scenario]
```

| Scenario | Description |
|----------|-------------|
| `quick` | 30-second overview |
| `full` | Complete walkthrough |
| `ccve` | Focus on CCVE scanning |

### ./test/atk/verify

Verify test fixtures are set up correctly.

```bash
./test/atk/verify
```

---

## Configuration Files

### Ownership Detection

```yaml
# config/ownership.yaml
detectors:
  - name: mydeployer
    priority: 50
    labels:
      - key: mycompany.io/deployed-by
        ref_field: true
    annotations:
      - key: mycompany.io/pipeline-id
```

### Custom Resources

```yaml
# config/resources.yaml
resources:
  - group: mycompany.io
    version: v1
    resource: widgets
    status:
      healthPath: .status.phase
      healthyValues: ["Succeeded", "Running"]
      degradedValues: ["Failed"]
```

### Webhooks

```yaml
# config/webhooks.yaml
webhooks:
  - name: slack-alerts
    url: https://hooks.slack.com/services/xxx
    events:
      - finding.created
      - finding.resolved
    filter:
      severity: [critical, warning]
```

---

## Troubleshooting

### CCVEs not detecting patterns

1. Validate CCVE definition:
   ```bash
   cub-scout scan --validate ./my-ccves/CCVE-2025-9001.yaml
   ```

2. Test against specific resource:
   ```bash
   cub-scout scan --test-ccve CCVE-2025-9001 --resource my-namespace/Deployment/my-app
   ```

---

## Connected Mode: cub-scout vs cub CLI

When you connect to ConfigHub (`cub auth login`), you gain access to richer capabilities via the `cub` CLI that complement what `cub-scout` provides.

### Architecture

```
OUTSIDE (Sources)              INSIDE (Hub + App Spaces)              TARGETS
─────────────────────         ─────────────────────────────         ──────────
• Git repos                    HUB                                   • K8s clusters
• Config generators              • Base Units (templates)            • Live state
• Programmatic patterns          • Patterns
                                 • Workers (Hub owns lifecycle)
                                   ├─ worker-east ──────────────────▶ prod-east
                                   └─ worker-west ──────────────────▶ prod-west

                               APP SPACES (select worker for deploy)
                                 • Fully rendered config as data
                                 • Units deploy via selected worker
                                 • Refresh pulls live state back
```

- **Sources** — Git repositories with templates, generators, and programmatic config
- **Hub** — Base Units, patterns, templates; **owns Worker lifecycle**
- **App Spaces** — Fully rendered config as data; **selects which Worker for deploy**
- **Targets** — Kubernetes clusters; Workers refresh live state back to Units

### Import Commands

| Command | Mode | What It Does |
|---------|------|--------------|
| `cub-scout import` | Standalone/TUI | Discover workloads, suggest structure, create Units |
| `cub unit import` | Connected (Worker) | Import with filters + suggestions; adjust names/labels after |
| `cub unit refresh` | Connected (Worker) | Pull live cluster state back into existing Unit |

**Key insight:** Once connected, names and labels can be adjusted. The initial import suggests structure, but you're not locked in.

### When to Use Each

**`cub-scout import`** — First-time discovery and onboarding:
```bash
# Discover what's running, propose App Space structure
cub-scout import -n my-namespace --dry-run

# Create Units in ConfigHub
cub-scout import -n my-namespace
```

**`cub unit import`** — After Units exist, import specific resources:
```bash
# Import ConfigMaps matching criteria
cub unit import myunit --where "kind = 'ConfigMap' AND metadata.namespace = 'prod'"

# Import with custom resources included
cub unit import myunit --where "import.include_custom = true"
```

**`cub unit refresh`** — Sync existing Unit from live cluster:
```bash
# Refresh single Unit from target
cub unit refresh myunit

# Bulk refresh Units by label
cub unit refresh --where "Labels.Environment = 'prod'"

# Preview what would be refreshed
cub unit refresh --where "Labels.Tier = 'backend'" --dry-run
```

### Live Data Commands

| Command | What It Shows |
|---------|---------------|
| `cub unit livedata` | Actual K8s resources on target (includes inventory ConfigMap) |
| `cub unit livestate` | Computed live state for diff/reconciliation |

```bash
# See what's actually running on the target
cub unit livedata myunit

# Output to file for analysis
cub unit livedata myunit -o livedata.yaml
```

### The Flow

```
1. STANDALONE (cub-scout)
   └─ cub-scout import → discovers workloads → creates Units

2. CONNECTED (cub + worker)
   └─ Worker runs on target cluster
   └─ cub unit refresh → pulls live state
   └─ cub unit import → imports specific resources
   └─ cub unit livedata → shows what's running
```

After connecting, ConfigHub can:
- Pull state from Targets (via Worker) with `refresh` / `import`
- Pull config from Sources (Git) with templates and generators
- Render fully-qualified config inside App Spaces
- Deploy rendered config to Targets

---

## See Also

- [ARCHITECTURE.md](ARCHITECTURE.md) — GSF protocol, API contracts
- [Scan Guide](../SCAN-GUIDE.md) — CCVE detection and remediation
- [EXTENDING.md](EXTENDING.md) — Extension points and customization
- [IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) — Import cluster workloads
- [TUI-SCAN.md](TUI-SCAN.md) — Cluster scanning documentation
- [TUI-TRACE.md](TUI-TRACE.md) — Trace resource ownership
