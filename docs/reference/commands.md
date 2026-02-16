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
| `map deployers` | List GitOps deployers (Flux, ArgoCD, core Deployments) |
| `map cronjobs` | List CronJobs with schedule/run state |
| `map jobs` | List Jobs with CronJob linkage and run state |
| `map actions` | Read-only operator action preview (runbook output) |
| `map activity` | Unified activity timeline from Flux/Argo/Helm/events |
| `map previews` | Detect PR preview environments |
| `trace` | Show GitOps ownership chain |
| `scan` | Scan for misconfigurations |
| `tree` | Hierarchical resource views |
| `import` | Import workloads into ConfigHub |
| `discover` | Scout-style workload discovery |
| `health` | Scout-style health check |
| `connect` | Quickly configure kube context from server URL or kubeconfig |
| `setup` | Set up shell completions |
| `graph export` | Export resource graph as JSON (v0.6) |
| `graph explain` | Explain a resource's graph relationships (v0.6) |
| `patterns list` | List registered patterns (v0.7) |
| `patterns detect` | Run pattern detection (v0.7) |
| `patterns explain` | Explain a specific pattern (v0.7) |
| `gitops status` | Show GitOps pipeline health (v0.14) |

For JSON contract navigation, start with [JSON Contracts and Output Model](json-contracts.md).

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

### Pipeline Source Semantics

In the pipelines view (`p`):
- Flux `Kustomization`: source from `spec.sourceRef.name`
- Flux `HelmRelease`: source from `spec.chart.spec.chart`
- Argo CD `Application`: source from `spec.source.repoURL`
- `unknown`: source field missing/unreadable

See `docs/reference/pipeline-source-resolution.md` for details.

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
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |
| `--json` | Output as JSON (shorthand for `--format json`) |
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
cub-scout map list --format json

# Output as Markdown
cub-scout map list --format md
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

Shows resources with unhealthy conditions.

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

List canonical workloads grouped by owner.

Canonical workload scope (v1.0):
- `Deployment`
- `StatefulSet`

Argo lineage semantics:
- `MANAGED-BY` shows the owning Application.
- If lineage is detected, it is appended as:
  `application-name <- applicationset/<name>` or
  `application-name <- applicationset/<name> <- application/<parent>`

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

Canonical deployer scope (v1.0):
- Flux `Kustomization`
- Flux `HelmRelease`
- Argo CD `Application`
- Core Kubernetes `Deployment` (fallback where GitOps CRDs are absent)

---

## map cronjobs

List CronJobs with schedule, active jobs, last schedule time, and owner.

```bash
cub-scout map cronjobs [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--namespace` | Filter by namespace |
| `--owner` | Filter by owner (for example `Flux`, `Native`) |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |

### Examples

```bash
cub-scout map cronjobs
cub-scout map cronjobs --namespace operations
cub-scout map cronjobs --owner Flux --format json
```

---

## map jobs

List Jobs with owning CronJob and run status.

```bash
cub-scout map jobs [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--namespace` | Filter by namespace |
| `--owner` | Filter by owner (for example `Flux`, `Native`) |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |

### Examples

```bash
cub-scout map jobs
cub-scout map jobs --namespace operations
cub-scout map jobs --format json
```

---

## map actions

Generate read-only operator action previews for a target resource.

```bash
cub-scout map actions <kind/name> -n <namespace> [flags]
```

No mutation is performed. Output is intended as runbook guidance.

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace of the target resource |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |

### Examples

```bash
cub-scout map actions deployment/payment-api -n ecommerce
cub-scout map actions cronjob/nightly-backup -n operations --format json
```

---

## map activity

Show normalized activity from Flux, ArgoCD, Helm release history, and Kubernetes Events.

```bash
cub-scout map activity [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--namespace` | Filter by namespace |
| `--owner` | Filter by owner (`Flux`, `ArgoCD`, `Helm`) |
| `--since` | Time filter (for example `24h`, `7d`) |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |

### Examples

```bash
cub-scout map activity
cub-scout map activity --owner Flux --since 24h
cub-scout map activity --namespace prod --format json
```

---

## map previews

Detect ephemeral preview environments using deterministic label/annotation heuristics
including PR metadata and Forgejo/Gitea hints.

```bash
cub-scout map previews [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--namespace` | Filter by namespace |
| `--stale-after` | Mark previews stale after duration (for example `72h`) |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |

### Examples

```bash
cub-scout map previews
cub-scout map previews --stale-after 72h
cub-scout map previews --format json
```

---

## trace

Show the full GitOps ownership chain for a resource. Works with **Flux, ArgoCD, or standalone Helm**.

Owner detection for `trace` uses the same deterministic precedence as `map list`.
After owner detection, `trace` resolves with an owner-specific chain resolver (Flux, ArgoCD, or Helm).
For detailed rules, see `docs/reference/ownership-precedence.md` and `docs/howto/trace-ownership.md`.

```bash
cub-scout trace <kind/name> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace of the resource |
| `--app` | Trace ArgoCD Application by name |
| `-r, --reverse` | Reverse trace (walk up ownerReferences, show orphan metadata) |
| `-d, --diff` | Show diff between live and Git state |
| `--artifacts` | Include source artifact provenance (`url`, `revision`, `digest`, `lastUpdateTime`) |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |
| `--json` | Output as JSON (shorthand for `--format json`) |
| `--explain` | Show explanatory content |

### Examples

```bash
# Trace a Flux-managed deployment
cub-scout trace deployment/nginx -n demo

# Trace ArgoCD app
cub-scout trace --app frontend

# Trace standalone Helm release (not Flux-managed)
cub-scout trace deployment/prometheus -n monitoring

# Reverse trace (from Pod up)
cub-scout trace pod/nginx-abc123 -n prod --reverse

# Reverse trace shows orphan metadata for native resources
cub-scout trace deployment/debug-nginx -n default --reverse

# Show what would change on reconciliation
cub-scout trace deployment/nginx -n demo --diff

# Show source artifact provenance (read-only)
cub-scout trace deployment/nginx -n demo --artifacts

# Output as JSON or Markdown
cub-scout trace deployment/nginx -n demo --format json
cub-scout trace deployment/nginx -n demo --format md
```

### Argo Context Troubleshooting

If trace fails due to stale/invalid Argo endpoint context, run:

```bash
argocd context
argocd app list
argocd logout <server>
argocd login <server>
cub-scout trace --app <app-name>
```

See `docs/howto/trace-context-troubleshooting.md` for the full flow.

### Supported Sources

| Source Type | Owner |
|-------------|-------|
| GitRepository | Flux |
| OCIRepository | Flux |
| HelmRepository | Flux |
| Bucket | Flux |
| Repository (Git/Helm) | ArgoCD |
| Helm secrets | Standalone Helm |

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
| `ownership` | Resources by GitOps owner (+ ownerRef lineage) |
| `git` | Git source structure |
| `patterns` | Detected GitOps patterns |
| `config` | ConfigHub relationships |
| `suggest` | Recommended ConfigHub structure |
| `workloads` | Alias for map workloads |

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Filter by namespace |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |

### Examples

```bash
cub-scout tree                  # Runtime hierarchy
cub-scout tree ownership        # By owner
cub-scout tree suggest          # Suggested ConfigHub structure
cub-scout tree ownership --format json   # JSON output
cub-scout tree runtime --format md       # Markdown output
```

`tree ownership` includes owner references for managed resources.
For ArgoCD resources, this includes optional lineage to parent `Application` and/or `ApplicationSet` when discoverable.

`tree git --format json` emits:
- `gitRepositories[]` (Flux GitRepository sources)
- `argoApplications[]` (Argo Applications with optional `generatedByApplicationSet` and `parentApplication`)
- `applicationSets[]` (Argo ApplicationSets with `generatorTypes[]` and `generatedApplications[]`)

For JSON contract details and schema ownership by surface, see [JSON Contracts and Output Model](json-contracts.md).

---

## import

Import workloads into ConfigHub.

```bash
cub-scout import [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Import one namespace (default: auto-discover all non-system namespaces with workloads) |
| `--dry-run` | Preview without creating anything |
| `-y, --yes` | Skip confirmation prompt |
| `--json` | Output proposal JSON (implies dry-run) |
| `--no-log` | Disable local import log file |
| `-w, --wizard` | Launch interactive import wizard |

### Canonical Migration Path

Use the canonical Argo/Helm migration flow:
`docs/howto/import-to-confighub.md`

### Examples

```bash
# Preview one namespace migration
cub-scout import -n payments-prod --dry-run

# Execute namespace import
cub-scout import -n payments-prod

# Non-interactive import
cub-scout import -n payments-prod -y

# Proposal JSON for automation/GUI
cub-scout import -n payments-prod --json
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

## connect

Quickly create or import a kubeconfig context for Cub Scout.

```bash
cub-scout connect [server-url] [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--server` | Kubernetes API server URL (alternative to positional arg) |
| `--token` | Bearer token authentication (or use `K8S_BEARER_TOKEN`) |
| `--username`, `--password` | Basic auth credentials |
| `--client-cert`, `--client-key` | TLS client cert authentication |
| `--from-kubeconfig` | Import context from an existing kubeconfig file |
| `--from-context` | Context name inside source kubeconfig |
| `--context` | Destination context name |
| `--kubeconfig` | Destination kubeconfig path |
| `--skip-verify` | Skip API connectivity check |
| `--map` | Launch `cub-scout map` immediately |

### Examples

```bash
# Direct API server connection with token
cub-scout connect https://api.example.com:6443 --token "$K8S_BEARER_TOKEN" --context prod

# Import from shared kubeconfig and launch map
cub-scout connect --from-kubeconfig ./artem.yaml --from-context ske-vcl-pro --map
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

## graph (v0.6)

Resource graph operations for exploring cluster relationships.

> **v0.6 contract surface.** Does not modify v0.5 contracts.

### graph export

Export the resource graph as deterministic JSON.

```bash
cub-scout graph export [flags]
```

#### Flags

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON (default, only format supported) |
| `-n, --namespace` | Namespace to collect (empty = all namespaces) |
| `--empty` | Output empty graph (skip cluster collection) |

#### Output Schema (graph.v1)

```json
{
  "schema_version": "graph.v1",
  "generated_at": "2026-01-01T00:00:00Z",
  "cluster": "cluster-name",
  "nodes": [...],
  "edges": [...]
}
```

See [Graph Contract Reference](graph-contract.md) for full schema documentation.

### graph explain

Explain a resource's relationships in the graph.

```bash
cub-scout graph explain <kind>/<name> -n <namespace>
```

Shows the resource's details and all incoming/outgoing edges with evidence.

#### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace of the resource (required) |

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (missing args / missing namespace) |
| 3 | Target not found |

#### Example Output

```
GRAPH EXPLAIN
Target: cluster/default/Deployment/nginx
Schema: graph.v1

Node:
  kind: Deployment
  name: nginx
  namespace: default
  api_version: apps/v1
  id: cluster/default/Deployment/nginx
  labels:
    app=nginx

Relationships:
  outgoing (1):
    - owns -> cluster/default/ReplicaSet/nginx-abc123
      evidence (1):
        - field: metadata.ownerReferences
          reason: ReplicaSet nginx-abc123 has ownerReference to Deployment nginx
  incoming (0):

Hint: Use 'cub-scout graph export --json' for the full graph.
```

See [Graph Explain Contract](graph-explain-contract.md) for full output format documentation.

---

## patterns (v0.7)

Pattern detection engine for analyzing resource graphs.

> **v0.7 contract surface.** Does not modify v0.5 or v0.6 contracts.

### patterns list

List all registered patterns.

```bash
cub-scout patterns list
```

Output is deterministic: patterns are sorted by ID.

### patterns detect

Run all patterns against the resource graph.

```bash
cub-scout patterns detect [flags]
```

#### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace to collect (empty = all namespaces) |
| `--empty` | Use empty graph (skip cluster collection) |
| `--json` | Output as JSON |

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All patterns passed |
| 4 | One or more patterns failed |

### patterns explain

Explain a specific pattern with detection results.

```bash
cub-scout patterns explain <pattern-id> [flags]
```

#### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace to collect (empty = all namespaces) |
| `--empty` | Use empty graph (skip cluster collection) |

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (missing argument) |
| 3 | Unknown pattern ID |
| 4 | Pattern failed |

See [Patterns Contract Reference](patterns-contract.md) for full documentation.

---

## gitops (v0.14)

GitOps pipeline status and diagnostics.

> **v0.14 contract surface.** Provides visibility into delegated apply health.

### gitops status

Show the health of GitOps deployers and sources.

```bash
cub-scout gitops status [flags]
```

#### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace to scan (empty = all namespaces) |
| `--json` | Output as JSON |

#### Detected Backends

| Backend | Detection |
|---------|-----------|
| `flux` | Kustomization or HelmRelease CRDs present |
| `argocd` | Application CRD present |
| `worker` | ConfigHub worker labels |
| `none` | No GitOps backend detected |

#### Failure Stages

| Stage | Description |
|-------|-------------|
| `source` | OCI/Git/Helm fetch failed |
| `build` | Kustomize/Helm rendering failed |
| `apply` | Kubernetes apply failed |
| `sync` | ArgoCD sync failed |
| `healthy` | All stages passed |

#### Example Output

```
GITOPS STATUS
════════════════════════════════════════════════════════════════════

Backend:    flux
Transport:  oci

SOURCES (1)
────────────────────────────────────────────────────────────────────
  ✗ OCIRepository/manifests (flux-system)    failing
    Reason:  AuthenticationFailed
    Message: failed to login to OCI registry: UNAUTHORIZED

DEPLOYERS (1)
────────────────────────────────────────────────────────────────────
  ✗ Kustomization/app (flux-system)          failing at source

Summary: 0 healthy, 1 failing

NEXT STEPS
────────────────────────────────────────────────────────────────────
• Source failure: Check OCI registry credentials and network access
```

#### JSON Output

```json
{
  "backend": "flux",
  "transport": "oci",
  "deployers": [...],
  "sources": [...],
  "healthyCount": 0,
  "failedCount": 1
}
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
