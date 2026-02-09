# GitOps Trace: Follow the Delivery Chain

Trace any Kubernetes resource back to its Git source. See the full ownership chain from source → deployer → resource, and find exactly where in the pipeline something is broken.

## Quick Start

```bash
# Trace a specific resource
cub-scout trace deployment/nginx -n demo

# Trace an Argo CD application by name
cub-scout trace --app frontend-app

# Interactive mode: pick from a list
./test/atk/map trace

# Batch trace all deployers
./test/atk/map pipelines --trace
```

## What Trace Shows

The trace command calls `flux trace` or `argocd app get` (auto-detected from ownership labels) to show:

1. **Full ownership chain** — Git source → Deployer → Resource
2. **Status at each level** — Which links are healthy, which are broken
3. **Revision tracking** — Which commit is deployed at each level
4. **Error messages** — Why something failed

## Color Coding

The trace output uses colors to help identify issues at a glance:

| Color | Element | Meaning |
|-------|---------|---------|
| 🟢 **Green** | `✓` | Healthy, ready, synced |
| 🔴 **Red** | `✗` | Failed, not ready, error |
| 🟡 **Yellow** | `⚠` | Warning, stale, degraded |
| 🟣 **Purple** | GitRepository, HelmRepository | Source resources |
| 🔵 **Cyan** | Kustomization, HelmRelease | Deployer resources |
| 🔷 **Blue** | Application, URLs | Argo CD resources |
| ⬜ **Dim** | `│`, labels | Structural elements |

## Example Output

### ✅ Healthy Chain (Flux)

```
┌─────────────────────────────────────────────────────────────────────┐
│ TRACE: Deployment/nginx                                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   🟢 ✓ 🟣 GitRepository/infra-repo                                  │
│       │ URL: https://github.com/your-org/infra.git                  │
│       │ Revision: main@sha1:abc123f                                 │
│       │ Status: 🟢 Artifact is up to date                           │
│       │                                                             │
│       └─▶ 🟢 ✓ 🔵 Kustomization/apps                                │
│               │ Namespace: flux-system                              │
│               │ Path: ./clusters/prod/apps                          │
│               │ Revision: main@sha1:abc123f                         │
│               │ Status: 🟢 Applied revision main@sha1:abc123f       │
│               │                                                     │
│               └─▶ 🟢 ✓ Deployment/nginx                             │
│                       Namespace: demo                               │
│                       Status: 🟢 Synced / Healthy                   │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ 🟢 ✓ All levels in sync. Managed by flux.                          │
└─────────────────────────────────────────────────────────────────────┘
```

### ❌ Broken Chain (Kustomization Failed)

```
┌─────────────────────────────────────────────────────────────────────┐
│ TRACE: Deployment/broken-app                                        │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   🟢 ✓ 🟣 GitRepository/infra-repo                                  │
│       │ URL: https://github.com/your-org/infra.git                  │
│       │ Revision: main@sha1:def456                                  │
│       │ Status: 🟢 Artifact is up to date                           │
│       │                                                             │
│       └─▶ 🔴 ✗ 🔵 Kustomization/apps        ◀── PROBLEM HERE        │
│               │ Status: 🟡 Reconciliation failed                    │
│               │ 🔴 Error: path './clusters/prod/apps' not found     │
│               │                                                     │
│               └─▶ Deployment/broken-app                             │
│                       Status: Running stale revision abc123         │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ 🟡 ⚠ Chain broken at Kustomization/apps                            │
│     path './clusters/prod/apps' not found in repository             │
└─────────────────────────────────────────────────────────────────────┘
```

### 🔷 Argo CD Application

```
┌─────────────────────────────────────────────────────────────────────┐
│ TRACE: Application/frontend-app                                     │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   🟢 ✓ 🟣 Source/your-org/frontend                                  │
│       │ URL: https://github.com/your-org/frontend.git               │
│       │ Revision: v2.1.0                                            │
│       │                                                             │
│       └─▶ 🟢 ✓ 🔷 Application/frontend-app                          │
│               │ Namespace: argocd                                   │
│               │ Status: 🟢 Synced / Healthy                         │
│               │ Revision: abc123def456                              │
│               │                                                     │
│               ├─▶ 🟢 ✓ Deployment/frontend                          │
│               │       Status: 🟢 Synced / Healthy                   │
│               │                                                     │
│               ├─▶ 🟢 ✓ Service/frontend                             │
│               │       Status: 🟢 Synced / Healthy                   │
│               │                                                     │
│               └─▶ 🟢 ✓ ConfigMap/frontend-config                    │
│                       Status: 🟢 Synced / Healthy                   │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ 🟢 ✓ All levels in sync. Managed by argocd.                        │
└─────────────────────────────────────────────────────────────────────┘
```

### 🔴 Source Not Fetching

```
┌─────────────────────────────────────────────────────────────────────┐
│ TRACE: Deployment/nginx                                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   🔴 ✗ 🟣 GitRepository/infra-repo       ◀── PROBLEM HERE           │
│       │ URL: https://github.com/your-org/private-repo.git           │
│       │ Status: 🔴 Failed to clone                                  │
│       │ 🔴 Error: authentication required                           │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ 🟡 ⚠ Chain broken at GitRepository/infra-repo                      │
│     authentication required                                         │
└─────────────────────────────────────────────────────────────────────┘
```

### 🟡 Orphan Resource (No GitOps Owner)

```
┌─────────────────────────────────────────────────────────────────────┐
│ TRACE: Deployment/mystery-app                                       │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   🟡 ⚠ No GitOps owner detected                                     │
│       │ Labels: app=mystery-app                                     │
│       │ Created: 2025-12-15 via kubectl                             │
│       │ Last modified: 2026-01-05                                   │
│       │                                                             │
│       └─▶ Deployment/mystery-app                                    │
│               Status: Running (no sync tracking)                    │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ 🟡 ⚠ Resource not managed by GitOps                                │
│     Consider adding to a Kustomization or Argo Application          │
└─────────────────────────────────────────────────────────────────────┘
```

## CLI Options

```
cub-scout trace <kind/name> [flags]

Flags:
  -n, --namespace string   Namespace of the resource
      --app string         Trace Argo CD application by name directly
      --json               Output as JSON (for scripting)
  -h, --help               Help for trace
```

## TUI Integration

### Interactive Trace (`t` key)

Press `t` in the TUI dashboard to open the interactive trace picker:

1. Shows list of traceable resources (deployers + managed workloads)
2. Use arrow keys or type to filter
3. Select resource to trace
4. View full ownership chain

### Batch Trace (`pipelines --trace`)

```bash
./test/atk/map pipelines --trace
```

Traces all deployers (Kustomizations, HelmReleases, Applications) and shows their full chains.

**Performance Note:** Tracing calls external CLI tools (~500ms per resource). Use batch tracing sparingly on large clusters.

## Use Cases

### 1. "Why isn't my change deployed?"

```bash
cub-scout trace deployment/my-app -n production
```

The trace shows:
- Is the GitRepository fetching the latest commit?
- Did the Kustomization/HelmRelease apply successfully?
- Is the deployment running the expected revision?

### 2. "What manages this resource?"

```bash
cub-scout trace configmap/my-config -n default
```

Shows the full chain from source to resource, identifying the owning Flux/Argo deployer.

### 3. "Find the broken link"

When something is wrong, trace immediately shows which level in the chain has the problem:

- **Source level**: Git credentials, URL, branch issues
- **Deployer level**: Kustomize errors, Helm values problems, sync failures
- **Resource level**: Pod failures, missing dependencies

### 4. Debugging CI/CD Pipelines

```bash
# Get structured output for automation
cub-scout trace deployment/api -n prod --json | jq '.chain[] | select(.ready == false)'
```

## Related risk issues

Trace-based detection enables these risk issue patterns:

| risk issue | Category | Description |
|------|----------|-------------|
| RISK-2025-0638 | ORPHAN | Resource not in any GitOps trace |
| RISK-2025-0639 | DRIFT | Trace shows stale revision |
| RISK-2025-0640 | APPLY | Trace chain broken at intermediate level |
| RISK-2025-0641 | STATE | Trace shows reconciliation stuck |
| RISK-2025-0642 | SOURCE | Trace source not fetching |

## Requirements

- **Flux**: `flux` CLI installed and working (`flux version`)
- **Argo CD**: `argocd` CLI installed and logged in (`argocd login <server>`)

The trace command auto-detects which tool manages the resource and uses the appropriate CLI.

## See Also

- [README.md](../README.md) - Main documentation
- [Scan Guide](../SCAN-GUIDE.md) - Configuration vulnerability detection
