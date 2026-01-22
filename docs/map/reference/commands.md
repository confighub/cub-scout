# CLI Commands Reference

Complete reference for all `cub-scout` commands.

## Top-Level Commands

| Command | Description |
|---------|-------------|
| `map` | Interactive TUI explorer |
| `tree` | Hierarchical views (runtime, git, config) |
| `discover` | Find workloads (alias for map workloads) |
| `health` | Check for issues (alias for map issues) |
| `trace` | Show GitOps ownership chain |
| `scan` | Scan for CCVEs |

---

## `tree` Command

Show hierarchical views of cluster resources, Git repos, or ConfigHub units.

```bash
cub-scout tree [type] [flags]
```

**Views:**
| Type | Description |
|------|-------------|
| `runtime` | Deployment → ReplicaSet → Pod trees (default) |
| `ownership` | Resources grouped by GitOps owner |
| `git` | Git repository structure |
| `patterns` | Detected GitOps patterns (D2, Arnie, Banko, Fluxy) |
| `config` | ConfigHub Unit relationships (wraps `cub unit tree`) |
| `suggest` | Suggested Hub/AppSpace organization |

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Filter by namespace |
| `--all` | `-A` | Include system namespaces |
| `--space` | | ConfigHub space for config view |
| `--edge` | | Edge type: clone or link (default: clone) |
| `--json` | | JSON output |

**Examples:**
```bash
# Runtime hierarchy
cub-scout tree
cub-scout tree runtime

# Resources by owner
cub-scout tree ownership

# Git sources
cub-scout tree git

# ConfigHub relationships
cub-scout tree config --space my-space
cub-scout tree config --space "*" --edge link

# Suggested organization
cub-scout tree suggest
```

---

## `discover` Command (Scout Alias)

Find workloads in your cluster. Alias for `map workloads`.

```bash
cub-scout discover
```

---

## `health` Command (Scout Alias)

Check cluster health and issues. Alias for `map issues`.

```bash
cub-scout health
```

---

## `map` Subcommands

### Synopsis

```bash
cub-scout map [subcommand] [flags]
```

### Subcommands

### map (no subcommand)

Launch the interactive TUI.

```bash
cub-scout map
cub-scout map --hub    # ConfigHub hierarchy mode
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--hub` | Launch ConfigHub hierarchy TUI (requires auth) |

---

### map list

Scriptable resource listing with query support.

```bash
cub-scout map list [flags]
```

**Flags:**
| Flag | Short | Description |
|------|-------|-------------|
| `--namespace` | `-n` | Filter by namespace |
| `--kind` | `-k` | Filter by resource kind |
| `--owner` | `-o` | Filter by owner (Flux, ArgoCD, Helm, ConfigHub, Native) |
| `--query` | `-q` | Query expression |
| `--since` | | Time filter (1h, 24h, 7d) |
| `--json` | | JSON output |
| `--verbose` | `-v` | Show additional details |

**Examples:**
```bash
# All resources
cub-scout map list

# Filter by namespace
cub-scout map list -n production

# Filter by owner
cub-scout map list -o Flux

# Query expression
cub-scout map list -q "owner=Flux AND namespace=prod*"

# JSON output
cub-scout map list --json
```

---

### map status

One-line cluster health summary.

```bash
cub-scout map status
```

**Output:**
```
Cluster: prod-east | Resources: 142 | Healthy: 138 | Issues: 4
```

---

### map issues

Show resources with problems.

```bash
cub-scout map issues [flags]
```

Alias: `map problems`

**Flags:**
| Flag | Description |
|------|-------------|
| `--namespace` | `-n` | Filter by namespace |
| `--json` | JSON output |

---

### map deployers

Show GitOps deployers (Flux + ArgoCD controllers).

```bash
cub-scout map deployers [flags]
```

**Output:**
```
DEPLOYER                    TYPE            STATUS    MANAGED
flux-system/main-app        Kustomization   Applied   12
flux-system/monitoring      HelmRelease     Applied   8
argocd/frontend             Application     Synced    5
```

---

### map workloads

Show workloads grouped by owner.

```bash
cub-scout map workloads [flags]
```

**Output:**
```
Flux (15 resources)
  deploy/api-gateway       prod    ✓ Ready
  deploy/payment-service   prod    ✓ Ready
  ...

ArgoCD (8 resources)
  deploy/frontend          web     ✓ Ready
  ...

Native (3 resources)
  deploy/debug-pod         prod    ⚠ Orphan
```

---

### map drift

Show resources diverged from desired state.

```bash
cub-scout map drift [flags]
```

**Output:**
```
RESOURCE                NAMESPACE    OWNER    DRIFT
deploy/api-gateway      prod         Flux     Image tag differs
cm/app-config           prod         ArgoCD   Missing key 'timeout'
```

---

### map sprawl

Configuration sprawl analysis.

```bash
cub-scout map sprawl [flags]
```

Shows how configuration is distributed across namespaces and owners.

---

### map bypass

Factory bypass detection.

```bash
cub-scout map bypass [flags]
```

Identifies resources deployed outside the standard GitOps pipeline.

---

### map crashes

Show crashing or failing pods/deployments.

```bash
cub-scout map crashes [flags]
```

**Output:**
```
RESOURCE                NAMESPACE    RESTARTS    STATUS
pod/api-worker-xyz      prod         5           CrashLoopBackOff
deploy/payment-api      prod         0           ImagePullBackOff
```

---

### map orphans

Show unmanaged (Native) resources.

```bash
cub-scout map orphans [flags]
```

Aliases: `map native`, `map unmanaged`

**Flags:**
| Flag | Description |
|------|-------------|
| `--namespace` | `-n` | Filter by namespace |

**Output:**
```
RESOURCE            NAMESPACE    CREATED              ANNOTATIONS
deploy/debug-pod    prod         2026-01-10 14:30     kubectl applied
cm/temp-config      staging      2026-01-08 09:15     kubectl applied
```

---

### map hub

ConfigHub hierarchy explorer.

```bash
cub-scout map hub
# Same as: cub-scout map --hub
```

**Requires:** `cub` CLI authenticated

Shows the ConfigHub hierarchy:
- Organization
  - Spaces
    - Units
      - Targets
      - Workers

---

### map fleet

Hub/AppSpace model view.

```bash
cub-scout map fleet [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--app` | Filter by app label |
| `--space` | Filter by App Space |

Shows resources organized by the Hub/AppSpace model (platform + app team separation).

---

## Global Flags

These flags work with all subcommands:

| Flag | Description |
|------|-------------|
| `--json` | JSON output for scripting |
| `--verbose` | Additional details |
| `--help` | Show help |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error |
| 2 | Invalid arguments |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `KUBECONFIG` | Path to kubeconfig file |
| `CUB_CONTEXT` | ConfigHub context (for connected mode) |

## See Also

- [Views Reference](views.md) - TUI views
- [Keybindings](keybindings.md) - Keyboard shortcuts
- [Query Syntax](query-syntax.md) - Query language
