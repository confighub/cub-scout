# Command Reference

Curated reference for common cub-scout commands with usage examples.

For the **exhaustive stable surface** (all contracted commands, flags, exit codes, and output schemas), see [CLI Contract Reference](cli-contract.md).

## Overview

| Command | Purpose | Stable Since |
|---------|---------|--------------|
| `map` | Interactive cluster explorer (TUI) | v0.5 |
| `map list` | List resources by ownership | v0.5 |
| `map status` | One-line cluster health check | v0.5 |
| `map orphans` | Find resources without GitOps owner | v0.5 |
| `map issues` | Show resources with problems | v0.5 |
| `map crashes` | Show crashing pods | v0.5 |
| `map workloads` | List workloads by owner | v0.5 |
| `map deployers` | List GitOps deployers (Flux, ArgoCD, core Deployments) | v0.5 |
| `map hooks` | List lifecycle hooks (Helm/ArgoCD) | v0.19 |
| `map cronjobs` | List CronJobs with schedule/run state | v0.20 |
| `map jobs` | List Jobs with CronJob linkage and run state | v0.20 |
| `map actions` | Read-only operator action preview (runbook output) | v0.20 |
| `map activity` | Unified activity timeline from Flux/Argo/Helm/events | v0.20 |
| `map previews` | Detect PR preview environments | v0.20 |
| `quickstart` | Guided first-run walkthrough | v1.4 |
| `doctor` | One-command cluster health summary | v1.4 |
| `explain` | Plain-English ownership and lineage for one resource | v1.4 |
| `impact` | Connected blast-radius preview for one unit | v1.6 |
| `fleet outliers` | Connected cluster-drift outlier report | v1.6 |
| `trace` | Show GitOps ownership chain | v0.5 |
| `scan` | Scan for misconfigurations | v0.5 |
| `scan --lifecycle-hazards` | Detect Helm hook risks under ArgoCD | v0.19 |
| `tree` | Hierarchical resource views | v0.5 |
| `import` | Import workloads into ConfigHub | v1.0 |
| `combined` (`compare`) | Compare Git and cluster/bundle structures, or show LIVE snapshot for one resource | v1.0 |
| `compare three-way` | Connected intent/render/observed comparison for resource/namespace/cluster scopes | v1.6 |
| `discover` | Scout-style workload discovery | v0.5 |
| `health` | Scout-style health check | v0.5 |
| `status` | Show connection status and cluster info | v1.0 |
| `history` | Show connected change history from ConfigHub ChangeSets | v1.4 |
| `audit list` | Show connected break-glass accept/reject audit trail | v1.6 |
| `summary list` | Query persisted connected drift/sync/risk snapshots | v1.7 |
| `connect` | Quickly configure kube context from server URL or kubeconfig | v1.0 |
| `setup` | Set up shell completions | v0.19 |
| `mcp serve` | Serve read-only MCP tools over stdio | v1.4 |
| `graph export` | Export resource graph as JSON/DOT/SVG/HTML | v0.6 |
| `graph explain` | Explain a resource's graph relationships | v0.6 |
| `patterns list` | List registered patterns | v0.7 |
| `patterns detect` | Run pattern detection | v0.7 |
| `patterns explain` | Explain a specific pattern | v0.7 |
| `bundle inspect` | Show bundle metadata and contents | v0.15 |
| `bundle summarize` | Generate evidence summaries from debug bundles | v0.19 |
| `bundle replay` | Re-render bundle contents | v0.15 |
| `bundle diff` | Compare two bundles | v0.15 |
| `bundle timeline` | Time-series view across catalog | v0.15 |
| `catalog list` | List bundles in a catalog | v0.15 |
| `gitops status` | Show GitOps pipeline health | v0.14.1 |
| `completion` | Generate shell completion script | v0.19 |

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
| `M` | Open Three Maps view |
| `H` | Hub view (ConfigHub hierarchy) |
| `j/k` | Navigate up/down |
| `Enter` | Select/expand |
| `h` | Open history timeline panel (connected ChangeSets) |
| `T` | Trace selected resource |
| `e` | Export graph as HTML (Maps view) |
| `E` | Export graph as SVG (Maps view) |
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

## map status

One-line cluster health summary.

```bash
cub-scout map status [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Filter by namespace |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |

### Examples

```bash
cub-scout map status
cub-scout map status -n production
cub-scout map status --format json
```

See [CLI Contract Reference](cli-contract.md) for exit codes and JSON schema.

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

### Navigation Hints

In default ASCII mode, `map list` includes a `TRY NEXT` section with contextual follow-up commands (for example `map orphans`, `explain`, and `doctor`).

Machine-readable formats (`--format json` and `--format md`) do not include these hints.

---

## quickstart

Guided first-run walkthrough across map, doctor, explain, ownership, and risk signals.

```bash
cub-scout quickstart [flags]
```

### Examples

```bash
cub-scout quickstart
cub-scout quickstart --yes
cub-scout quickstart -n production --yes
```

---

## doctor

Single-command cluster health summary (ownership, health, risk, drift, top issues).

```bash
cub-scout doctor [flags]
```

### Examples

```bash
cub-scout doctor
cub-scout doctor -n production
cub-scout doctor --format json
```

---

## explain

Plain-English ownership and lineage for a single resource.

```bash
cub-scout explain <kind/name> [flags]
```

### Examples

```bash
cub-scout explain deploy/payments-api -n prod
cub-scout explain deployment/payments-api -n prod --format md
```

---

## map orphans

Find orphan resources, including unmanaged resources and explicit Argo ApplicationSet-link orphans.

```bash
cub-scout map orphans [flags]
```

Shows:
- Resources where `owner=Native` (not managed by Flux, ArgoCD, Helm, or ConfigHub)
- ArgoCD `Application` resources with explicit `ApplicationSet` lineage that points to a missing generator

### Examples

```bash
cub-scout map orphans
cub-scout map orphans -n default
cub-scout map orphans --json
```

---

## impact

Preview connected blast radius for a ConfigHub unit.

```bash
cub-scout impact <unit> [flags]
```

Requires ConfigHub authentication.

### Examples

```bash
cub-scout impact unit/shared-db-config
cub-scout impact shared-db-config --format md
cub-scout impact shared-db-config --json
```

### Flags

| Flag | Description |
|------|-------------|
| `--format` | Output format: `ascii`, `json`, `md` |
| `--json` | Output as JSON (shorthand for `--format json`) |

---

## fleet outliers

Identify clusters that diverge from fleet norms (connected mode).

```bash
cub-scout fleet outliers [flags]
```

Requires ConfigHub authentication and at least two clusters with target data.

### Examples

```bash
cub-scout fleet outliers
cub-scout fleet outliers --format md
cub-scout fleet outliers --json
```

### Flags

| Flag | Description |
|------|-------------|
| `--format` | Output format: `ascii`, `json`, `md` |
| `--json` | Output as JSON (shorthand for `--format json`) |

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

## map hooks

List lifecycle hooks (Helm pre/post-install, ArgoCD sync hooks).

```bash
cub-scout map hooks [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Filter by namespace |
| `--format` | Output format: `ascii`, `json`, `md` (default: ascii) |

### Examples

```bash
cub-scout map hooks
cub-scout map hooks -n production
```

See [CLI Contract Reference](cli-contract.md) for full output details.

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
| ConfigHub OCI (OCI target path) | Flux |
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
| `--dangling` | Scan for dangling resources (includes Argo ApplicationSet-link orphans and generator errors) |
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

Provider behavior:
- `scan --file`: uses `confighub-scan` / `cub-scan` when available, otherwise legacy scanner.
- Cluster `scan`: preserves legacy runtime/state checks and augments static findings through
  `cluster export -> cub-scan` when available.
- Fallback is safe: if export or `cub-scan` fails, output falls back to legacy provider results.

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
- `argoApplications[]` (Argo Applications with optional `generatedByApplicationSet`, `applicationSetLinkStatus`, and `parentApplication`)
- `applicationSets[]` (Argo ApplicationSets with `generatorTypes[]` and `generatedApplications[]`)

`applicationSetLinkStatus` values:
- `resolved`: referenced ApplicationSet exists in-cluster
- `orphan`: explicit ownerReference points to a missing ApplicationSet
- `unknown`: inferred label/annotation points to a missing ApplicationSet

For JSON contract details and schema ownership by surface, see [JSON Contracts and Output Model](json-contracts.md).

---

## import

Import workloads into ConfigHub.

> **Terminology:** The CLI currently uses Space/Unit commands. These map to App/Deployment
> in the new model. See [Glossary](glossary.md#confighub-model-app-centric).

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
| `--connect` | After import, start worker and set targets |
| `--no-connect` | Do not auto-start worker/targets after import |
| `--from-bundle` | Import proposal/apply from a debug bundle directory (offline path, no cluster discovery) |
| `--audit-reason` | Record break-glass decision reason in connected audit history (max 512 chars) |

### Canonical Migration Path

Use the canonical Argo/Helm migration flow:
`docs/howto/import-to-confighub.md`

### Examples

```bash
# Preview one namespace migration
cub-scout import -n payments-prod --dry-run

# Execute namespace import
cub-scout import -n payments-prod

# Execute import with explicit break-glass audit reason
cub-scout import -n payments-prod --audit-reason "approved by sre lead for Q1 migration"

# Non-interactive import
cub-scout import -n payments-prod -y

# Proposal JSON for automation/GUI
cub-scout import -n payments-prod --json

# Proposal JSON from a debug bundle (offline)
cub-scout import --from-bundle ./debug-bundle --dry-run --json
```

`import --json` output includes an `evidence` block:
- `evidence.source`: `cluster` or `bundle`
- `evidence.bundlePath`: set when source is `bundle`
- `workloads[].connected`: true when the workload is already linked by `confighub.com/UnitSlug` or when a proposed unit slug already exists in the target App Space.

Connected audit note:
- `--audit-reason` writes a break-glass ChangeSet entry so `cub-scout audit list` can show who/when/why/what.

---

## combined

Show alignment across Git, live cluster, bundle snapshots, and Git↔Git compare.

Alias: `compare`

```bash
cub-scout combined [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--git-url` | Left-side Git repository URL |
| `--git-path` | Left-side local Git repository path |
| `--git-url-compare` | Right-side Git repository URL for Git↔Git compare |
| `--git-path-compare` | Right-side local Git repository path for Git↔Git compare |
| `-n, --namespace` | Namespace to scan from live cluster |
| `--bundle` | Use debug bundle directory as offline cluster snapshot |
| `--format` | Resource compare output format: `ascii`, `json`, `md` |
| `--suggest` | Generate App model proposal |
| `--apply` | Apply proposal to ConfigHub |
| `--dry-run` | Preview apply behavior without making changes |
| `--json` | Output JSON |

### Examples

```bash
# Resource compare mode (DRY/WET/LIVE when linked; LIVE-only otherwise)
cub-scout compare deploy/checkout -n prod --format ascii
cub-scout compare deploy/checkout -n prod --format json

# Git ↔ live cluster compare
cub-scout combined --git-path ./repo --namespace payments --json

# Git ↔ bundle (offline cluster snapshot) compare
cub-scout combined --git-path ./repo --bundle ./debug-bundle --json

# Git ↔ Git compare (left vs right)
cub-scout combined --git-path ./repo-a --git-path-compare ./repo-b --json
```

Resource compare mode behavior:
- If the live resource is linked to ConfigHub (`confighub.com/UnitSlug`), output includes DRY/WET/LIVE sections.
- When DRY/WET/LIVE values differ, output includes a mismatch highlight section (`Diff Highlights` in ASCII, `Mismatches` table in Markdown/JSON).
- If not linked (or not connected), output degrades to LIVE-only with explicit notes.

### compare three-way

Connected three-way comparison command for selected scopes.

```bash
cub-scout compare three-way --scope <scope> [flags]
```

Supported scopes:
- `<kind/name>` or `resource:<kind/name>` (single resource)
- `namespace/<ns>` (all workloads in namespace)
- `cluster` (all discovered namespaces/workloads)

Flags:
- `--scope` (required)
- `-n, --namespace` (resource scope namespace override)
- `--format ascii|json|md`
- `--json` (shorthand for `--format json`)

Examples:

```bash
# Resource scope
cub-scout compare three-way --scope deploy/payment-api -n prod

# Namespace scope
cub-scout compare three-way --scope namespace/prod --format json

# Cluster scope
cub-scout compare three-way --scope cluster --format md
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

## status

Show connection status, cluster info, and worker status.

```bash
cub-scout status [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |

### Examples

```bash
cub-scout status
cub-scout status --json
```

Displays ConfigHub connection mode (Offline/Online/Connected), current cluster
name, kubectl context, and optional worker/space info when the `cub` CLI is
available.

---

## summary list

List persisted connected-mode summaries generated by:
- `cub-scout scan` (risk snapshot)
- `cub-scout gitops status` (sync/drift snapshot)

```bash
cub-scout summary list [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--since` | Lookback window (examples: `24h`, `7d`, `2w`) |
| `--type` | Filter by type (`scan`, `gitops-status`) |
| `--cluster` | Filter by cluster/context name |
| `-n, --namespace` | Filter by namespace |
| `--format` | Output format: `ascii`, `json`, `md` |
| `--json` | Output as JSON (shorthand for `--format json`) |

### Examples

```bash
# Last 24 hours across all connected summary records
cub-scout summary list

# Last week, scan-only records in one cluster
cub-scout summary list --since 7d --type scan --cluster kind-dev

# JSON export for automation
cub-scout summary list --since 48h --namespace prod --json
```

### Storage Contract

- Schema version: `connected.summary.v1`
- Index dimensions: `cluster`, `scope.namespace`, `timestamp`, `type`
- Default retention: 30 days
- Optional overrides:
  - `CUB_SCOUT_SUMMARY_DIR` (storage path)
  - `CUB_SCOUT_SUMMARY_RETENTION_DAYS` (retention window)

---

## history

Show connected change history for one resource from ConfigHub ChangeSets.

```bash
cub-scout history <resource> [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Optional namespace scope |
| `--since` | Lookback window (examples: `24h`, `7d`, `2w`) |
| `--format` | Output format: `ascii`, `json`, `md` |

### Examples

```bash
# Default lookback (7d), ASCII output
cub-scout history deploy/my-app -n prod

# JSON for automation
cub-scout history deploy/my-app -n prod --since 24h --format json

# Markdown timeline for tickets/PRs
cub-scout history deploy/my-app --since 2w --format md
```

### Connected Mode Notes

- Requires ConfigHub authentication (`cub auth login`).
- Uses read-only ChangeSet queries under the hood.
- If no history is found, output clearly notes the resource may not be imported yet.

---

## audit list

Show connected break-glass accept/reject decisions from ConfigHub ChangeSets.

```bash
cub-scout audit list [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Optional namespace scope |
| `--since` | Lookback window (examples: `24h`, `7d`, `2w`) |
| `--format` | Output format: `ascii`, `json`, `md` |
| `--json` | Output as JSON (shorthand for `--format json`) |

### Examples

```bash
# Show recent break-glass decisions
cub-scout audit list

# Filter by namespace and window
cub-scout audit list -n prod --since 7d

# Export machine-readable output
cub-scout audit list --format json
```

### Connected Mode Notes

- Requires ConfigHub authentication (`cub auth login`).
- Uses read-only ChangeSet queries filtered to break-glass records.
- If no records are found, output reports: `No break-glass decisions recorded for this scope`.

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

## mcp

Read-only MCP gateway for AI agent tooling.

### mcp serve

Serve MCP over stdio and expose read-only tools.

```bash
cub-scout mcp serve
```

#### Notes

- Standalone tools: `map`, `trace`, `scan`, `explain` (via existing cub-scout JSON surfaces).
- Connected tools (when authenticated to ConfigHub): `confighub_changesets`, `confighub_units`, `confighub_unit_get`.
- Standalone and read-only: no cluster mutations and no ConfigHub write path.
- Protocol transport is stdio with `Content-Length` framed JSON-RPC messages.

#### Example

```bash
# Run as MCP server process (stdio)
cub-scout mcp serve
```

---

## graph (v0.6)

Resource graph operations for exploring cluster relationships.

> **v0.6 contract surface.** Does not modify v0.5 contracts.

### graph export

Export the resource graph as deterministic JSON, DOT, SVG, or HTML.

```bash
cub-scout graph export [flags]
```

#### Flags

| Flag | Description |
|------|-------------|
| `--format` | Output format: `json` (default), `dot`, `svg`, `html` |
| `-o, --output` | Write output to a file instead of stdout |
| `--max-nodes` | Max nodes for visual formats (`dot`/`svg`/`html`), `0` = unlimited |
| `--json` | Deprecated alias for `--format json` |
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

#### Visual Export Examples

```bash
# Graphviz DOT for docs or Graphviz tooling
cub-scout graph export --format dot > graph.dot

# Static SVG for docs/wiki embeds
cub-scout graph export --format svg --output graph.svg

# Self-contained interactive HTML for sharing
cub-scout graph export --format html --output graph.html
```

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

## bundle summarize (v0.19)

Generate evidence summaries from debug bundles for external systems (Jira, PRs, Slack).

```bash
cub-scout bundle summarize <bundle-path> [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `ascii` | Output format: `ticket`, `pr`, `slack`, `ascii`, `json` |
| `--out` | string | stdout | Output file path |

### Formats

| Format | Output | Use Case |
|--------|--------|----------|
| `ticket` | Markdown | Jira, ServiceNow incident tickets |
| `pr` | Markdown | Pull request descriptions/comments |
| `slack` | Slack Block Kit JSON | Channel notifications |
| `ascii` | Plain text | Human reading (default) |
| `json` | Structured JSON | Downstream tooling, CI/CD |

### Examples

```bash
# Jira/ServiceNow ticket
cub-scout bundle summarize ./bundle --format ticket --out incident.md

# PR description
cub-scout bundle summarize ./bundle --format pr

# Slack notification
cub-scout bundle summarize ./bundle --format slack --out notification.json

# Machine-readable JSON
cub-scout bundle summarize ./bundle --format json
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Summary generated successfully |
| 1 | Bundle read error or invalid format |

Output is deterministic: same bundle always produces identical output. All content derives from bundle facts only.

For the full evidence export schema, see [Evidence Export v1](evidence-export-v1.md).

---

## bundle inspect (v0.15)

Show bundle metadata, contents, and structure.

```bash
cub-scout bundle inspect <bundle-path>
```

---

## bundle replay (v0.15)

Re-render bundle contents through the current rendering pipeline.

```bash
cub-scout bundle replay <bundle-path> [flags]
```

---

## bundle diff (v0.15)

Compare two bundles and show differences.

```bash
cub-scout bundle diff <bundle-a> <bundle-b> [flags]
```

---

## bundle timeline (v0.15)

Time-series view of bundles in a catalog directory.

```bash
cub-scout bundle timeline <catalog-path> [flags]
```

---

## catalog list (v0.15)

List bundles in a catalog directory with metadata.

```bash
cub-scout catalog list <catalog-path> [flags]
```

See [CLI Contract Reference](cli-contract.md) for full flag and output documentation for all bundle and catalog commands.

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
