# Command Reference

Curated reference for high-value cub-scout commands with usage examples.

For the **complete alphabetical command index**, see [Complete CLI Reference (A-Z)](cli-reference.md).
For the **exhaustive stable surface** (all contracted commands, flags, exit codes, and output schemas), see [CLI Contract Reference](cli-contract.md).

## Overview

| Command | Purpose | Stable Since |
|---------|---------|--------------|
| `map` | Interactive cluster explorer (TUI) | v0.5 |
| `map list` | List resources by ownership | v0.5 |
| `map meaning` | Experimental meaning-first grouped browse | v1.7 |
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
| `quickstart demo` | Fixture-backed demo runner | v1.0 |
| `doctor` | One-command cluster health summary | v1.4 |
| `explain` | Plain-English ownership and lineage for one resource | v1.4 |
| `impact` | Connected blast-radius preview for one unit | v1.6 |
| `fleet outliers` | Connected cluster-drift outlier report | v1.6 |
| `trace` | Show GitOps ownership chain | v0.5 |
| `watch --webhook <url>` | Stream observation events to webhook/file sinks | v1.7 |
| `scan` | Scan for misconfigurations | v0.5 |
| `scan --lifecycle-hazards` | Detect Helm hook risks under ArgoCD | v0.19 |
| `tree` | Hierarchical resource views | v0.5 |
| `compare drift` | Compare desired manifests to live cluster state | v0.14.3 |
| `import` | Import workloads into ConfigHub | v1.0 |
| `import apply` | Apply an App model proposal JSON | v1.0 |
| `import argocd` | Import a single ArgoCD Application into ConfigHub | v1.0 |
| `import cluster-aggregator` | Aggregate multiple import proposals into a fleet view | v1.0 |
| `import parse-repo` | Parse GitOps repository structure for import preview | v1.0 |
| `compare` (alias: `combined`) | Compare Git and cluster/bundle structures, or show LIVE snapshot for one resource | v1.0 |
| `compare three-way` | Connected intent/render/observed comparison for resource/namespace/cluster scopes | v1.6 |
| `context-pack` | Export deterministic AI context JSON bundle | v1.8 |
| `debug` | Guided GitOps debugging wizard | v0.14.2 |
| `discover` | Scout-style workload discovery | v0.5 |
| `health` | Scout-style health check | v0.5 |
| `app` | Manage ConfigHub Apps | v1.0 |
| `remedy` | Execute remediation for auto-fixable risk findings | v0.20 |
| `snapshot` | Export cluster state as GSF JSON | v0.20 |
| `status` | Show connection status and cluster info | v1.0 |
| `history` | Show connected change history from ConfigHub ChangeSets | v1.4 |
| `audit list` | Show connected break-glass accept/reject audit trail | v1.6 |
| `summary list` | Query persisted connected drift/sync/risk snapshots | v1.7 |
| `summary slack` | Publish connected summary digest to Slack webhook | v1.7 |
| `setup` | Set up shell completions and quick cluster connect helpers | v0.19 |
| `setup completion` | Generate shell completion script | v0.19 |
| `setup connect` | Quickly configure kube context from server URL or kubeconfig | v1.0 |
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
| `version` | Print version/build information | v0.5 |

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

# Filter by custom owner name from detectors.yaml
cub-scout map list -q "owner=Internal Platform"

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

Custom CRDs can be appended to this inventory using `~/.cub-scout/resources.yaml`
or `CUB_SCOUT_RESOURCE_CONFIG` (see `docs/howto/extending.md`).

---

## map meaning

Experimental meaning-first browse groups with deterministic comparative labels.

```bash
cub-scout map meaning [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Filter by namespace |
| `--owner` | Filter by owner |
| `--kind` | Filter by kind |
| `--max-groups` | Limit number of groups (default: `12`) |
| `--max-members` | Limit members per group (default: `10`) |
| `--format` | Output format: `ascii`, `json` (default: ascii) |

### Examples

```bash
cub-scout map meaning
cub-scout map meaning --namespace prod --owner Flux
cub-scout map meaning --format json
```

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

## quickstart demo

Run fixture-backed demos that show cub-scout features without needing to invent your own scenario first.

```bash
cub-scout quickstart demo [name] [flags]
```

### Examples

```bash
cub-scout quickstart demo list
cub-scout quickstart demo quick
cub-scout quickstart demo ccve
cub-scout quickstart demo scenario bigbank-incident
cub-scout quickstart demo quick --cleanup
```

### Flags

| Flag | Description |
|------|-------------|
| `--cleanup` | Remove demo resources |
| `--no-pods` | Apply without running pods for a faster demo cycle |

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
cub-scout doctor --presentation ai
cub-scout doctor --hint-mode operator
cub-scout doctor --format json
```

### Flags

| Flag | Description |
|------|-------------|
| `--format` | Output format: `ascii`, `json` |
| `-n, --namespace` | Namespace scope (default: all namespaces) |
| `--top` | Number of top issues to include (default: `3`) |
| `--presentation` | Narrative framing for ASCII output: `human`, `ai`, `paired`. Omit the flag to keep the legacy/default render path. JSON is unchanged. |
| `--hint-mode` | Recommendation ranking for `TRY NEXT`: `default`, `beginner`, `operator`. JSON is unchanged. |

---

## explain

Plain-English ownership and lineage for a single resource.

```bash
cub-scout explain <kind/name> [flags]
```

### Examples

```bash
cub-scout explain deploy/payments-api -n prod
cub-scout explain deploy/payments-api -n prod --presentation ai
cub-scout explain deploy/payments-api -n prod --hint-mode operator
cub-scout explain deployment/payments-api -n prod --format md
```

### Flags

| Flag | Description |
|------|-------------|
| `--format` | Output format: `text`, `json`, `md` |
| `-n, --namespace` | Namespace of target resource |
| `--presentation` | Narrative framing for text/Markdown output: `human`, `ai`, `paired`. Omit the flag to keep the legacy/default render path. JSON is unchanged. |
| `--hint-mode` | Recommendation ranking for next-step hints: `default`, `beginner`, `operator`. JSON is unchanged. |

---

## map orphans

Find orphan resources, including unmanaged resources and explicit Argo ApplicationSet-link orphans.

```bash
cub-scout map orphans [flags]
```

Shows:
- Resources where `owner=Native` (not managed by Flux, ArgoCD, Helm, ConfigHub, Crossplane, or kro)
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

Show resources with problems — unhealthy conditions and missing/unreadable secrets.

```bash
cub-scout map issues [flags]
```

Shows:
- Resources with unhealthy conditions (not Ready)
- Secret issues: missing or unreadable secrets referenced by workloads and Flux resources

### Secret Issues

`map issues` detects secret problems across your scope:

**Supported resource kinds:**
- Workloads: Deployment, StatefulSet, DaemonSet
- Flux deployers: Kustomization, HelmRelease
- Flux sources: GitRepository, HelmRepository, Bucket

**Secret issue types:**

| Status | Meaning |
|--------|---------|
| `missing` | Secret does not exist (NotFound) |
| `unreadable` | Secret exists but RBAC denies read access (Forbidden) |

**Note:** Optional secrets (marked `optional: true` in the manifest) are excluded from issues — they are expected to potentially not exist.

### Examples

```bash
# Show all issues including secret problems
cub-scout map issues

# Scope to a namespace
cub-scout map issues -n production
```

### Example Output

```
ISSUES
  Deployment/api-server in prod: not Ready (0/3 replicas)
  StatefulSet/redis in cache: not Ready (1/3 replicas)

SECRET ISSUES
  ✗ Deployment/api-server in prod: missing secret "db-credentials" (envFrom.secretRef)
  ✗ HelmRelease/monitoring in flux-system: missing secret "grafana-admin" (spec.valuesFrom)
  ✗ GitRepository/private-repo in flux-system: unreadable (RBAC) secret "git-credentials" (spec.secretRef)
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
| `--owner` | Filter by owner (`Flux`, `ArgoCD`, `Helm`, `Crossplane`, `kro`, `ConfigHub`, `Native`) |
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

Show the full GitOps ownership chain for a resource. Works with **Flux, ArgoCD, and standalone Helm**. Platform composition lineage (Crossplane/kro) has experimental support.

Owner detection for `trace` uses the same deterministic precedence as `map list`.
After owner detection, `trace` resolves with an owner-specific chain resolver (Flux, ArgoCD, Helm).
When a resource matches a custom ownership detector, `trace` prints the configured owner name and explains that chain resolution is currently limited to built-in owner resolvers.
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

### Secret Evidence

For workloads (Deployment, StatefulSet, DaemonSet, Pod) and Flux sources/deployers, `trace` includes a `Secrets` section showing referenced secrets and their status.

**Supported resource kinds:**
- Workloads: Deployment, StatefulSet, DaemonSet, Pod
- Flux sources: GitRepository, HelmRepository, Bucket
- Flux deployers: Kustomization, HelmRelease

**Secret status values:**

| Status | Meaning |
|--------|---------|
| `present` | Secret exists and is readable |
| `missing` | Secret does not exist (NotFound) |
| `unreadable` | Secret exists but RBAC denies read access (Forbidden) |
| `unresolved` | Reference could not be resolved (e.g., optional secret) |

**Reference types (`refType`):**

| Value | Description |
|-------|-------------|
| `envFrom.secretRef` | secretRef in envFrom |
| `env.valueFrom.secretKeyRef` | secretKeyRef in env variables |
| `volume.secret` | Secret volume |
| `volume.projected.secret` | Projected secret volume |
| `imagePullSecrets` | Image pull secrets |
| `spec.secretRef` | Flux source/deployer secretRef (also for decryption, valuesFrom) |
| `spec.credentials.secretRef` | Crossplane ProviderConfig credential secretRef |

**Example output (ASCII):**

```
✓ Secret evidence:
  ✓ Secret/db-credentials [present]
      referenced via: envFrom.secretRef
      type: Opaque
  ✓ Secret/api-keys [present]
      referenced via: env.valueFrom.secretKeyRef
      type: Opaque
  ✗ Secret/missing-secret [missing]
      referenced via: volume.secret
      secret not found
```

**Safety:** Secret evidence exposes only safe metadata (name, namespace, type, status). The `.data` and `.stringData` fields are never read or exposed.

---

## watch

Stream observation events to webhook/file sinks.

```bash
cub-scout watch [--webhook <url>] [--output-file <path>] [flags]
```

At least one destination is required: `--webhook` and/or `--output-file`.

### Flags

| Flag | Description |
|------|-------------|
| `--webhook` | Webhook URL to receive events |
| `--output-file` | Append JSONL events to a local file |
| `--interval` | Polling interval (default: `20s`) |
| `-n, --namespace` | Namespace filter |
| `--owner` | Owner display-name filter |
| `--severity` | Finding severity filter (`critical,warning,info`) |
| `--once` | Run one collection cycle and exit |
| `--max-queued-events` | Max buffered events while webhook is unreachable |

### Event Types

- `resource.discovered`
- `ownership.changed`
- `drift.detected`
- `scan.finding`

### Event Payload

```json
{
  "type": "drift.detected",
  "timestamp": "2026-03-07T11:00:00Z",
  "resource": {"kind": "Deployment", "name": "api", "namespace": "prod"},
  "owner": {"type": "ArgoCD", "name": "api-app"},
  "severity": "warning",
  "details": {"category": "STATE", "message": "out of sync"}
}
```

### Example

```bash
cub-scout watch --webhook https://hooks.example.com/cub-scout --interval 30s
```

```bash
cub-scout watch --output-file /tmp/cub-scout-events.jsonl --once
```

See [`examples/watch-webhook/`](../../examples/watch-webhook/) for a local receiver and end-to-end walkthrough.
Custom CRDs in `~/.cub-scout/resources.yaml` (or `CUB_SCOUT_RESOURCE_CONFIG`)
are included in watch resource discovery.

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
| `--git-path` | Preview/import from a local GitOps repository path |
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

# Proposal JSON from a local GitOps repo
cub-scout import --git-path ./repo --dry-run --json
```

`import --json` output includes an `evidence` block:
- `evidence.source`: `cluster`, `bundle`, or `git`
- `evidence.bundlePath`: set when source is `bundle`
- `evidence.gitPath`: set when source is `git`
- `workloads[].connected`: true when the workload is already linked by `confighub.com/UnitSlug` or when a proposed unit slug already exists in the target App Space.

`import --git-path` notes:
- this is a local structure/import-preview flow, not a manifest renderer
- parser support includes ArgoCD `ApplicationSet` git generators, matrix-contained git generators, exclude patterns, and duplicate-basename-safe proposal slugs
- if you need controller-faithful rendering/import, that remains the `cub gitops discover` + `cub gitops import` path

Connected audit note:
- `--audit-reason` writes a break-glass ChangeSet entry so `cub-scout audit list` can show who/when/why/what.

---

## import apply

Apply an App model proposal to create resources in ConfigHub.

```bash
cub-scout import apply [proposal.json] [flags]
```

### Examples

```bash
cub-scout import --json > proposal.json
cub-scout import apply proposal.json
cub-scout import apply proposal.json --dry-run
cub-scout import cluster-aggregator cluster-*.json --suggest --json | cub-scout import apply -
```

### Flags

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview what would be created without making changes |
| `--no-log` | Disable local apply logging |

---

## import argocd

Import a single ArgoCD Application's managed resources into ConfigHub.

```bash
cub-scout import argocd [application-name] [flags]
```

### Examples

```bash
cub-scout import argocd --list
cub-scout import argocd guestbook --dry-run
cub-scout import argocd guestbook --show-yaml
cub-scout import argocd guestbook --disable-sync
```

### Flags

| Flag | Description |
|------|-------------|
| `--list` | List available ArgoCD Applications |
| `--dry-run` | Preview without importing |
| `--show-yaml` | Show YAML that would be imported (implies dry-run) |
| `--disable-sync` | Disable ArgoCD auto-sync after import |
| `--delete-app` | Delete the ArgoCD Application after import |
| `--space` | ConfigHub space override |

---

## import cluster-aggregator

Aggregate multiple import proposal JSON files into a single fleet view or suggestion.

```bash
cub-scout import cluster-aggregator [files...] [flags]
```

### Examples

```bash
cub-scout import cluster-aggregator cluster1.json cluster2.json
cub-scout import cluster-aggregator cluster1.json cluster2.json --suggest
cub-scout import cluster-aggregator cluster-*.json --suggest --json | cub-scout import apply -
```

### Flags

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `--suggest` | Generate a unified App model proposal |

---

## import parse-repo

Parse a GitOps repository and show its structure for import-preview workflows.

```bash
cub-scout import parse-repo [flags]
```

### Examples

```bash
cub-scout import parse-repo --url https://github.com/fluxcd/flux2-kustomize-helm-example
cub-scout import parse-repo --path ./my-gitops-repo
cub-scout import parse-repo --path ./my-gitops-repo --json
```

### Flags

| Flag | Description |
|------|-------------|
| `--path` | Local Git repository path |
| `--url` | Remote Git repository URL to clone and parse |
| `--json` | Output as JSON |

---

## compare

Show alignment across Git, live cluster, bundle snapshots, and Git↔Git compare.

Canonical path: `compare`

Hidden top-level alias retained for one release: `combined`

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
- `--fail-on info|warning`

Examples:

```bash
# Resource scope
cub-scout compare three-way --scope deploy/payment-api -n prod

# Namespace scope
cub-scout compare three-way --scope namespace/prod --format json

# Cluster scope
cub-scout compare three-way --scope cluster --format md

# CI / automation
cub-scout compare three-way --scope namespace/prod --fail-on warning
```

Output notes:
- ASCII and Markdown now include an explicit agreement/convergence line
- JSON includes `summary.agreement.{state,summary,reasons,sources}`
- agreement states are `agreed`, `converging`, `diverged`, and `partial`
- conformance exit codes still depend only on JSON facts + `--fail-on`

---

## compare drift

Detect differences between desired manifests and live cluster state.

```bash
cub-scout compare drift --file <path> [flags]
```

### Examples

```bash
cub-scout compare drift --file manifests/deployment.yaml
cub-scout compare drift --file manifests/ -n production
cub-scout compare drift --file manifests/ --format json
cub-scout compare drift --file manifests/ --fail-on warning
```

### Flags

| Flag | Description |
|------|-------------|
| `--file` | YAML file or directory containing desired state |
| `-n, --namespace` | Namespace to compare |
| `--format` | Output format: `ascii`, `json` |
| `--fail-on` | Exit non-zero when max severity meets the threshold |

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

Deprecated top-level alias retained for one release. Prefer `cub-scout setup connect`.

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

## summary slack

Build a connected digest from persisted summary records and post to Slack Incoming Webhook.

```bash
cub-scout summary slack [flags]
```

### Flags

| Flag | Description |
|------|-------------|
| `--webhook-url` | Slack incoming webhook URL (or `CUB_SCOUT_SLACK_WEBHOOK_URL`) |
| `--since` | Digest lookback window (examples: `24h`, `7d`, `2w`) |
| `--type` | Filter by type (`scan`, `gitops-status`) |
| `--cluster` | Filter by cluster/context name |
| `-n, --namespace` | Filter by namespace |
| `--batch-size` | Max entries included in digest body |
| `--dedupe-window` | Skip duplicate digest signatures within this window |
| `--force` | Bypass dedupe and always post |
| `--dry-run` | Print webhook payload JSON without posting |

### Examples

```bash
# Post the last 24h digest to Slack
cub-scout summary slack --webhook-url https://hooks.slack.com/services/...

# Scope to one cluster and namespace
cub-scout summary slack --since 7d --cluster kind-dev --namespace prod

# Render payload only (no webhook post)
cub-scout summary slack --dry-run --since 24h
```

### Digest Contract

Each digest includes:
- Severity breakdown (risk critical/warning/info)
- Affected scope (clusters + namespaces)
- Sync/drift summary (`sync failed/out-of-sync`, `drift total`)
- Next-action command (`cub-scout summary list ... --format md`)

Noise controls:
- Windowing: `--since`
- Batching: `--batch-size`
- Deduping: signature state file + `--dedupe-window` (`--force` bypass)

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
| `--include-synthetic` | Include synthetic/demo seeded ChangeSets |

### Examples

```bash
# Default lookback (7d), ASCII output
cub-scout history deploy/my-app -n prod

# JSON for automation
cub-scout history deploy/my-app -n prod --since 24h --format json

# Markdown timeline for tickets/PRs
cub-scout history deploy/my-app --since 2w --format md

# Include synthetic/demo seeded records when needed
cub-scout history deploy/my-app -n prod --include-synthetic
```

### Connected Mode Notes

- Requires ConfigHub authentication (`cub auth login`).
- Uses read-only ChangeSet queries under the hood.
- Synthetic/demo seeded ChangeSets are excluded by default; use `--include-synthetic` to include them.
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
| `--include-synthetic` | Include synthetic/demo seeded ChangeSets |
| `--json` | Output as JSON (shorthand for `--format json`) |

### Examples

```bash
# Show recent break-glass decisions
cub-scout audit list

# Filter by namespace and window
cub-scout audit list -n prod --since 7d

# Export machine-readable output
cub-scout audit list --format json

# Include synthetic/demo seeded break-glass entries
cub-scout audit list --include-synthetic
```

### Connected Mode Notes

- Requires ConfigHub authentication (`cub auth login`).
- Uses read-only ChangeSet queries filtered to break-glass records.
- Synthetic/demo seeded ChangeSets are excluded by default; use `--include-synthetic` to include them.
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

## setup connect

Canonical path for quick kubeconfig setup and optional immediate launch into the TUI.

```bash
cub-scout setup connect [server-url] [flags]
```

### Examples

```bash
cub-scout setup connect https://api.example.com:6443 --token "$K8S_BEARER_TOKEN" --context prod
cub-scout setup connect --from-kubeconfig ./artem.yaml --from-context ske-vcl-pro --map
```

See [`connect`](#connect) for the full flag set while the hidden top-level alias remains available for one release.

---

## setup completion

Generate shell completion scripts from the canonical `setup` command family.

```bash
cub-scout setup completion [bash|zsh|fish|powershell]
```

### Examples

```bash
cub-scout setup completion bash
cub-scout setup completion zsh
cub-scout setup completion fish
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

- Standalone tools: `doctor`, `explain`, `map`, `scan`, `trace` (via existing cub-scout JSON surfaces).
- `doctor` is intentionally first: it is the natural first troubleshooting command for AI and MCP clients, including when the problem may be local access uncertainty such as wrong context, stale kubeconfig, or API reachability.
- Connected tools (when authenticated to ConfigHub): `compare_three_way`, `confighub_changesets`, `confighub_units`, `confighub_unit_get`.
- Standalone and read-only: no cluster mutations and no ConfigHub write path.
- MCP tool descriptors mark every tool with `annotations.readOnlyHint=true`.
- Protocol transport is stdio with `Content-Length` framed JSON-RPC messages.

#### Tool Parameters

- `doctor`
  - `namespace` (optional)
  - `top` (optional integer)
- `compare_three_way`
  - `scope` (required)
  - `namespace` (optional)
- `explain`
  - `resource` (required)
  - `namespace` (optional)
- `map`
  - `namespace` (optional)
- `scan`
  - `namespace` (optional)
- `trace`
  - `resource` (required)
  - `namespace` (optional)

#### Example

```bash
# Run as MCP server process (stdio)
cub-scout mcp serve
```

---

## context-pack

Export a bounded AI handoff bundle as deterministic JSON.

```bash
cub-scout context-pack [flags]
```

For the full v2 contract and usage guidance, see [docs/howto/context-pack-v2.md](../howto/context-pack-v2.md).

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

## debug

Guided GitOps debugging wizard for diagnosing pipeline or workload issues.

```bash
cub-scout debug [resource] [flags]
```

### Examples

```bash
cub-scout debug
cub-scout debug deployment/api-server -n production
cub-scout debug deployment/api-server -n production --format json
```

### Flags

| Flag | Description |
|------|-------------|
| `--format` | Output format: `ascii`, `json`, `md` |
| `-n, --namespace` | Namespace of the target resource |
| `--non-interactive` | Run without interactive prompts when a resource is provided |
| `--save-bundle` | Save a debug bundle to a directory |

---

## remedy

Execute automated remediation for detected auto-fixable risk findings.

```bash
cub-scout remedy [RISK-ID] [flags]
```

### Examples

```bash
cub-scout remedy CCVE-2025-0687 --dry-run -n production
cub-scout remedy --all --dry-run -n production
cub-scout remedy --list
```

### Flags

| Flag | Description |
|------|-------------|
| `--all` | Fix all auto-fixable issues |
| `--dry-run` | Show what would change without applying it |
| `--list` | List auto-fixable risk issues |
| `-n, --namespace` | Namespace scope |
| `--json` | Output as JSON |

---

## snapshot

Export current cluster state as GitOps State Format (GSF) JSON.

```bash
cub-scout snapshot [flags]
```

### Examples

```bash
cub-scout snapshot
cub-scout snapshot -o state.json
cub-scout snapshot --namespace prod
cub-scout snapshot --relations
```

### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace filter |
| `-k, --kind` | Kind filter |
| `-o, --output` | Output file path |
| `--relations` | Include resource relations |

---

## app

Manage ConfigHub Apps from cub-scout's connected surface.

```bash
cub-scout app [command]
```

### Available Subcommands

- `app create`
- `app list`

---

## version

Print version information for the local cub-scout binary.

```bash
cub-scout version
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
