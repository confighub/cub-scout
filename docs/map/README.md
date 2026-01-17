# Map: See What's Running, Who Owns It, Is It Configured Correctly

**cub-agent map** is a read-only Kubernetes observer that answers three questions in 30 seconds:
1. What's running in my cluster?
2. Who owns each resource? (Flux, ArgoCD, Helm, ConfigHub, or Native)
3. Is it configured correctly?

## The Problem

You have a Kubernetes cluster. Multiple tools deploy to it:
- Flux syncs from Git
- ArgoCD manages applications
- Helm installs charts
- Someone used `kubectl apply` directly

**Without map:** You check each tool's UI. You grep labels. You ask team members. You update spreadsheets. 30-45 minutes later, maybe you have an answer.

**With map:** One command shows everything. Ownership detected automatically. Problems highlighted. 30 seconds.

```
$ cub-agent map list
NAME            NAMESPACE    OWNER      STATUS
payment-api     prod         Flux       ✓ Synced
frontend        prod         ArgoCD     ✓ Synced
redis-cache     prod         Helm       ✓ Deployed
debug-pod       prod         Native     ⚠ Orphan   <- Who did this?
```

## Two Modes

### 1. Local Cluster (Standalone)

Works immediately, no setup required:

```bash
cub-agent map                 # Interactive TUI
cub-agent map list            # Plain text output
cub-agent map orphans         # Show unmanaged resources
cub-agent map trace deploy/x  # Trace ownership chain
```

**What you get:**
- Ownership detection for all resources
- Trace from workload → deployer → source
- CCVE scanning (46 configuration anti-patterns)
- Query and filter resources
- No account, no setup, just run it

### 2. ConfigHub Hierarchy (Connected)

Connect to ConfigHub for fleet-wide visibility:

```bash
cub-agent map --hub           # ConfigHub hierarchy TUI
cub-agent map fleet           # Fleet view (Hub/AppSpace model)
```

**What you get:**
- DRY → WET → Live visibility across all clusters
- Fleet-wide queries: "Which clusters run payment-api v2.1.0?"
- Import wizard: bring existing Flux/Argo workloads into ConfigHub
- Platform team collaboration (Hub/AppSpace model)

## The Value Ladder

| Stage | What You Get | Commands |
|-------|--------------|----------|
| **OSS (Free)** | Ownership, trace, scan on single cluster | `map`, `map list`, `map trace` |
| **Connected (Free tier)** | Multi-cluster view, ConfigHub hierarchy | `map --hub`, `map fleet` |
| **Paid** | Make changes, AI trace, Apps/Actions | `import`, `apply`, AI features |

## Quick Start

```bash
# Build
go build ./cmd/cub-agent

# Run the TUI
./cub-agent map

# Press ? for help, q to quit
```

## Keyboard Shortcuts (TUI)

| Key | Action |
|-----|--------|
| `s` | Status/Dashboard |
| `w` | Workloads view |
| `p` | Pipelines (GitOps deployers) |
| `o` | Orphans (Native resources) |
| `T` | Trace ownership chain |
| `S` | Scan for CCVEs |
| `/` | Search |
| `?` | Help |
| `q` | Quit |

## Documentation

| Document | Purpose |
|----------|---------|
| [QUICKSTART.md](QUICKSTART.md) | 5-minute getting started |
| [PRD.md](PRD.md) | Full specification |
| **How-To Guides** | |
| [howto/ownership-detection.md](howto/ownership-detection.md) | Understand who owns what |
| [howto/find-orphans.md](howto/find-orphans.md) | Find unmanaged resources |
| [howto/trace-ownership.md](howto/trace-ownership.md) | Trace from resource to source |
| [howto/scan-for-ccves.md](howto/scan-for-ccves.md) | Detect configuration issues |
| [howto/query-resources.md](howto/query-resources.md) | Filter and query |
| [howto/import-to-confighub.md](howto/import-to-confighub.md) | Bring workloads to ConfigHub |
| **Reference** | |
| [reference/commands.md](reference/commands.md) | All 12 subcommands |
| [reference/views.md](reference/views.md) | 9+ TUI views |
| [reference/keybindings.md](reference/keybindings.md) | All keyboard shortcuts |
| [reference/query-syntax.md](reference/query-syntax.md) | Query language |
| [reference/gitops-repo-structures.md](reference/gitops-repo-structures.md) | GitOps repo patterns |
| [reference/hub-appspace-examples.md](reference/hub-appspace-examples.md) | Hub/AppSpace view examples |

## Ownership Detection

Map automatically detects who manages each resource:

| Owner | Detection Method |
|-------|------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| **ArgoCD** | Both `app.kubernetes.io/instance` AND `argocd.argoproj.io/instance` labels |
| **Helm** | `app.kubernetes.io/managed-by: Helm` label |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | No GitOps ownership detected (the "shadow IT" bucket) |

## Example Use Cases

### Find orphan resources
```bash
cub-agent map orphans
# Shows all Native (unmanaged) resources - who kubectl'd something?
```

### Trace a deployment's ownership
```bash
cub-agent map trace deploy/payment-api -n prod
# Shows: Deployment → HelmRelease → GitRepository → git@github.com:...
```

### Query across the fleet
```bash
cub-agent map list -q "owner=Flux AND namespace=prod*"
# Shows all Flux-managed resources in production namespaces
```

### Scan for configuration issues
```bash
cub-agent map scan
# Checks 46 CCVE patterns, highlights misconfigurations
```

## Zero-Friction Adoption

Map works with any existing Flux, ArgoCD, or Helm deployment:

```bash
# Step 1: You have existing GitOps
kubectl get kustomizations,applications -A

# Step 2: Just run map
cub-agent map    # Instant ownership visibility, no setup

# Step 3: Optionally import to ConfigHub
cub-agent import # Wizard guides through import
```

**Reference architectures tested:**

| Pattern | Description | Docs |
|---------|-------------|------|
| **Banko** (Flux) | Cluster-per-directory, versioned platform | [examples](reference/hub-appspace-examples.md#example-6-banko-pattern-real-world-flux-production) |
| **Arnie** (ArgoCD) | Folders-per-environment, promotion=cp | [examples](reference/hub-appspace-examples.md#example-7-arnie-pattern-argocd-folders-per-environment) |
| **TraderX** | Multi-region with base/infra Hub | [examples](reference/hub-appspace-examples.md#example-2-traderx-multi-region) |
| **KubeCon Demo** | Platform + app teams | [examples](reference/hub-appspace-examples.md#example-1-kubecon-demo-online-boutique) |
| **curious-cub** | Standard dev/staging/prod | [examples](reference/hub-appspace-examples.md#example-3-curious-cub-full-pattern) |
| **IITS/Jesper** | Per-deployer workspaces | [examples](reference/hub-appspace-examples.md#example-4-iits-pattern-jesper-examples) |

See [GitOps Repo Structures](reference/gitops-repo-structures.md) for Git layout patterns and [Hub/AppSpace Examples](reference/hub-appspace-examples.md) for how these render in the TUI.

## Related

- [Business Outcomes](../outcomes/README.md) - Why this matters
- [Demo Suite](../demos/README.md) - Try it yourself
- [CCVE Guide](../CCVE-GUIDE.md) - Configuration vulnerability scanning
