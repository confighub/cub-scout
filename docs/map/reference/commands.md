# CLI Commands Reference

Complete reference for all `cub-agent map` subcommands.

## Synopsis

```bash
cub-agent map [subcommand] [flags]
```

## Subcommands

### map (no subcommand)

Launch the interactive TUI.

```bash
cub-agent map
cub-agent map --hub    # ConfigHub hierarchy mode
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--hub` | Launch ConfigHub hierarchy TUI (requires auth) |

---

### map list

Scriptable resource listing with query support.

```bash
cub-agent map list [flags]
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
cub-agent map list

# Filter by namespace
cub-agent map list -n production

# Filter by owner
cub-agent map list -o Flux

# Query expression
cub-agent map list -q "owner=Flux AND namespace=prod*"

# JSON output
cub-agent map list --json
```

---

### map status

One-line cluster health summary.

```bash
cub-agent map status
```

**Output:**
```
Cluster: prod-east | Resources: 142 | Healthy: 138 | Issues: 4
```

---

### map issues

Show resources with problems.

```bash
cub-agent map issues [flags]
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
cub-agent map deployers [flags]
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
cub-agent map workloads [flags]
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
cub-agent map drift [flags]
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
cub-agent map sprawl [flags]
```

Shows how configuration is distributed across namespaces and owners.

---

### map bypass

Factory bypass detection.

```bash
cub-agent map bypass [flags]
```

Identifies resources deployed outside the standard GitOps pipeline.

---

### map crashes

Show crashing or failing pods/deployments.

```bash
cub-agent map crashes [flags]
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
cub-agent map orphans [flags]
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
cub-agent map hub
# Same as: cub-agent map --hub
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
cub-agent map fleet [flags]
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
