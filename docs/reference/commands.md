# Command Reference

Complete reference for all cub-scout commands.

## Overview

| Command | Purpose |
|---------|---------|
| `map` | Interactive cluster explorer (TUI) |
| `map list` | List resources by ownership |
| `map orphans` | Find resources without GitOps owner |
| `map issues` | Show resources with problems |
| `map crashes` | Show crashing pods |
| `map workloads` | List workloads by owner |
| `map deployers` | List GitOps deployers |
| `trace` | Show GitOps ownership chain |
| `scan` | Scan for misconfigurations |
| `tree` | Hierarchical resource views |
| `discover` | Scout-style workload discovery |
| `health` | Scout-style health check |
| `setup` | Set up shell completions |

---

## map

Interactive TUI for exploring your cluster.

```bash
cub-scout map [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--hub` | ConfigHub hierarchy view |
| `-n, --namespace` | Filter by namespace |
| `-q, --query` | Resource query filter |

### TUI Keys

| Key | Action |
|-----|--------|
| `1-5` | Switch tabs |
| `H` | Hub view (ConfigHub hierarchy) |
| `j/k` | Navigate up/down |
| `Enter` | Select/expand |
| `t` | Trace selected resource |
| `?` | Help |
| `q` | Quit |

---

## map list

List all resources with ownership information.

```bash
cub-scout map list [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Filter by namespace |
| `-q, --query` | Filter by query |
| `--json` | Output as JSON |
| `--count` | Show count only |
| `--names-only` | Show names only |
| `--explain` | Show explanatory content |

### Examples

```bash
# List all resources
cub-scout map list

# Filter by namespace
cub-scout map list -n production

# Filter by owner
cub-scout map list -q "owner=Flux"

# Filter by multiple criteria
cub-scout map list -q "owner!=Native AND kind=Deployment"

# Output as JSON
cub-scout map list --json
```

---

## map orphans

Find resources not managed by GitOps.

```bash
cub-scout map orphans [flags]
```

Shows resources where `owner=Native` - not managed by Flux, ArgoCD, Helm, or ConfigHub.

### Examples

```bash
cub-scout map orphans
cub-scout map orphans -n default
cub-scout map orphans --json
```

---

## map issues

Show resources with problems (not Ready).

```bash
cub-scout map issues [flags]
```

Shows both deployer issues (Kustomization, HelmRelease, Application) and workload issues (Deployment, StatefulSet).

### Examples

```bash
cub-scout map issues
cub-scout map issues -n production
```

---

## map crashes

Show pods with crash/error states.

```bash
cub-scout map crashes [flags]
```

Shows pods with:
- CrashLoopBackOff
- ImagePullBackOff
- OOMKilled
- Error
- High restart counts (>= 5)

---

## map workloads

List workloads grouped by owner.

```bash
cub-scout map workloads [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-q, --query` | Filter query |

---

## map deployers

List GitOps deployers.

```bash
cub-scout map deployers [flags]
```

Shows Kustomizations, HelmReleases, and Applications.

---

## trace

Show the full GitOps ownership chain for a resource.

```bash
cub-scout trace <kind/name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace of the resource |
| `--app` | Trace ArgoCD Application by name |
| `-r, --reverse` | Reverse trace (walk up ownerReferences) |
| `-d, --diff` | Show diff between live and Git state |
| `--json` | Output as JSON |
| `--explain` | Show explanatory content |

### Examples

```bash
# Trace a deployment
cub-scout trace deployment/nginx -n demo

# Trace ArgoCD app
cub-scout trace --app frontend

# Reverse trace (from Pod up)
cub-scout trace pod/nginx-abc123 -n prod --reverse

# Show what would change on reconciliation
cub-scout trace deployment/nginx -n demo --diff
```

---

## scan

Scan for misconfigurations and risks.

```bash
cub-scout scan [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace to scan |
| `--state` | State scan only (stuck reconciliations) |
| `--kyverno` | Kyverno scan only |
| `--timing-bombs` | Scan for expiring certs/quotas |
| `--dangling` | Scan for dangling resources |
| `--file` | Scan a YAML file (static analysis) |
| `--list` | List all known patterns |
| `--json` | Output as JSON |
| `--explain` | Show explanatory content |

### Examples

```bash
# Full cluster scan
cub-scout scan

# Scan specific namespace
cub-scout scan -n production

# Scan a manifest file
cub-scout scan --file deployment.yaml

# List all known risk patterns
cub-scout scan --list
```

---

## tree

Hierarchical views of cluster resources.

```bash
cub-scout tree [view] [flags]
```

### Views

| View | Description |
|------|-------------|
| `runtime` | Deployment → ReplicaSet → Pod (default) |
| `ownership` | Resources by GitOps owner |
| `git` | Git source structure |
| `patterns` | Detected GitOps patterns |
| `config` | ConfigHub relationships |
| `suggest` | Recommended ConfigHub structure |
| `workloads` | Alias for map workloads |

### Examples

```bash
cub-scout tree                  # Runtime hierarchy
cub-scout tree ownership        # By owner
cub-scout tree suggest          # Suggested ConfigHub structure
```

---

## discover

Scout-style workload discovery (alias for `map workloads`).

```bash
cub-scout discover [flags]
```

---

## health

Scout-style health check (alias for `map issues`).

```bash
cub-scout health [flags]
```

---

## setup

Set up shell completions and configuration.

```bash
cub-scout setup [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--shell` | Shell to configure (bash, zsh, fish). Auto-detects if not specified |
| `--dry-run` | Show what would be done without making changes |

### Examples

```bash
# Auto-detect shell and install completions
cub-scout setup

# Install for specific shell
cub-scout setup --shell zsh

# Preview without installing
cub-scout setup --dry-run
```

---

## Global Flags

These flags work with all commands:

| Flag | Description |
|------|-------------|
| `--kubeconfig` | Path to kubeconfig file |
| `--context` | Kubernetes context to use |
| `-v, --verbose` | Verbose output |
| `--help` | Help for the command |

---

## Query Syntax

See [Query Syntax Reference](query-syntax.md) for full query language documentation.

```bash
# Basic filters
cub-scout map list -q "owner=Flux"
cub-scout map list -q "namespace=prod*"
cub-scout map list -q "kind=Deployment"

# Compound filters
cub-scout map list -q "owner=Flux AND namespace=production"
cub-scout map list -q "owner!=Native OR kind=ConfigMap"

# Label filters
cub-scout map list -q "labels[app]=frontend"
```
