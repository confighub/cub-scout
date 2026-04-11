# CLI Contract Reference

This document defines the stable CLI behavior for cub-scout.
Commands, flags, and output formats documented here are considered stable and
breaking changes will be avoided within the same contract version.

**Current Version:** v1.0

**Output model:** JSON is the canonical contract. ASCII/Markdown output is a rendering of the same fields.

Human-friendly JSON contract index: [json-contracts.md](json-contracts.md)
Alphabetical command index: [cli-reference.md](cli-reference.md)

---

## Contract Evolution

| Version | Key Additions |
|---------|---------------|
| v0.5 | Core commands: map, trace, scan |
| v0.6 | graph export, graph explain |
| v0.7 | patterns list/detect/explain |
| v0.14 | `--format` flag (ascii/json/md), JSON schema guarantees |
| v0.14.1 | gitops status command |
| v0.15 | bundle replay, bundle diff, bundle timeline, catalog |
| v0.16 | Attribution graph/report, Crossplane lineage, Kustomize overlay |
| v0.19 | Shell completion, map hooks, scan --lifecycle-hazards, bundle summarize |
| v0.20 | Flux operator interop read-only slice (`map cronjobs/jobs/actions/activity/previews`, `trace --artifacts`) |
| v1.0 | Contract freeze, connected mode auth, comprehensive test coverage |

> If documentation and behavior ever diverge, **golden tests under
> `test/golden/` are the source of truth**.

---

## Stable Commands

| Command | Purpose | Stable Since |
|---------|---------|--------------|
| `cub-scout map` | Interactive TUI dashboard | v0.5 |
| `cub-scout map list` | List resources (scriptable) | v0.5 |
| `cub-scout map status` | One-line health check | v0.5 |
| `cub-scout map deployers` | List deployers (Flux/ArgoCD + core Deployments) | v0.5 |
| `cub-scout map hooks` | List lifecycle hooks (Helm/ArgoCD) | v0.19 |
| `cub-scout map cronjobs` | List CronJobs with schedule/run state | v0.20 |
| `cub-scout map jobs` | List Jobs with CronJob linkage and status | v0.20 |
| `cub-scout map actions` | Read-only action/runbook previews | v0.20 |
| `cub-scout map activity` | Unified operational timeline | v0.20 |
| `cub-scout map previews` | Preview environment detection heuristics | v0.20 |
| `cub-scout doctor` | Single-command cluster health summary | v1.4 |
| `cub-scout explain` | Plain-English ownership and lineage for one resource | v1.4 |
| `cub-scout trace` | Trace resource to Git source | v0.5 |
| `cub-scout scan` | Scan for risk issues and issues | v0.5 |
| `cub-scout scan --lifecycle-hazards` | Detect Helm hook risks under ArgoCD | v0.19 |
| `cub-scout graph export` | Export resource graph as JSON/DOT/SVG/HTML | v0.6 |
| `cub-scout graph explain` | Explain resource relationships | v0.6 |
| `cub-scout patterns list` | List registered patterns | v0.7 |
| `cub-scout patterns detect` | Run pattern detection | v0.7 |
| `cub-scout patterns explain` | Explain specific pattern | v0.7 |
| `cub-scout gitops status` | GitOps pipeline health | v0.14.1 |
| `cub-scout bundle inspect` | Show bundle metadata and contents | v0.15 |
| `cub-scout bundle replay` | Re-render bundle contents | v0.15 |
| `cub-scout bundle diff` | Compare two bundles | v0.15 |
| `cub-scout bundle timeline` | Time-series view across catalog | v0.15 |
| `cub-scout bundle summarize` | Generate summary for tickets/PRs | v0.19 |
| `cub-scout catalog list` | List bundles in a catalog | v0.15 |
| `cub-scout completion` | Generate shell completion script | v0.19 |
| `cub-scout connect` | Configure kube context from server URL or kubeconfig import | v1.0 |
| `cub-scout status` | Show connection status and cluster info | v1.0 |
| `cub-scout history` | Connected change timeline from ConfigHub ChangeSets | v1.4 |
| `cub-scout compare three-way` | Connected DRY/WET/LIVE comparison with conformance and agreement summary | v1.6 |
| `cub-scout mcp serve` | Serve read-only MCP observation tools over stdio | v1.4 |

---

## cub-scout doctor

Single-command cluster health summary.

```bash
cub-scout doctor [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n, --namespace` | string | all | Namespace scope |
| `--format` | string | ascii | Output format: `ascii`, `json` |
| `--top` | int | 3 | Number of top issues to include |
| `--presentation` | string | legacy/default render path | Narrative framing for ASCII output: `human`, `ai`, `paired`. Omitting the flag keeps the legacy/default render path. JSON is unchanged. |
| `--hint-mode` | string | default | Recommendation ranking for `TRY NEXT`: `default`, `beginner`, `operator`. JSON is unchanged. |

### Stable Output Rules

- JSON is the canonical contract for `doctor`.
- `--presentation` affects ASCII framing only.
- `--hint-mode` affects recommendation ranking only.
- Omitting `--presentation` preserves the legacy/default text render path.

---

## cub-scout explain

Plain-English ownership and lineage for a single resource.

```bash
cub-scout explain <kind/name> [flags]
cub-scout explain <kind> <name> [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n, --namespace` | string | - | Namespace of target resource |
| `--format` | string | text | Output format: `text`, `json`, `md` |
| `--presentation` | string | legacy/default render path | Narrative framing for text/Markdown output: `human`, `ai`, `paired`. Omitting the flag keeps the legacy/default render path. JSON is unchanged. |
| `--hint-mode` | string | default | Recommendation ranking for next-step hints: `default`, `beginner`, `operator`. JSON is unchanged. |

### Stable Output Rules

- JSON is the canonical contract for `explain`.
- `--presentation` affects text and Markdown framing only.
- `--hint-mode` affects next-step recommendation ranking only.
- Omitting `--presentation` preserves the legacy/default text/Markdown render path.

---

## cub-scout compare three-way

Connected DRY/WET/LIVE comparison for a resource, namespace, or cluster scope.

```bash
cub-scout compare three-way --scope <scope> [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--scope` | string | required | `<kind/name>`, `resource:<kind/name>`, `namespace/<ns>`, or `cluster` |
| `-n, --namespace` | string | - | Namespace override for resource scope |
| `--format` | string | ascii | Output format: `ascii`, `json`, `md` |
| `--json` | bool | false | Shorthand for `--format json` |
| `--fail-on` | string | - | Exit non-zero when max severity meets `info` or `warning` |

### Stable Output Rules

- JSON is the canonical contract for `compare three-way`.
- JSON includes `summary.conformance` and `summary.agreement`.
- `summary.agreement.state` is one of `agreed`, `converging`, `diverged`, `partial`.
- `summary.agreement.summary`, `summary.agreement.reasons[]`, and `summary.agreement.sources` are additive facts, not presentation-only text.
- Exit behavior depends only on JSON facts plus `--fail-on`, not ASCII/Markdown formatting.

---

## cub-scout mcp serve

Read-only MCP gateway over stdio.

```bash
cub-scout mcp serve
```

### Stable Behavior

- The MCP gateway remains read-only.
- Standalone tool set includes:
  - `doctor`
  - `explain`
  - `map`
  - `scan`
  - `trace`
- Connected mode adds read-only connected comparison plus ConfigHub query tools.
- MCP tool responses are backed by the existing CLI JSON surfaces rather than a separate hand-built fact model.

### Stable `doctor` MCP Surface

- Tool name: `doctor`
- Parameters:
  - `namespace` (optional string)
  - `top` (optional integer)
- Backed by `cub-scout doctor --format json`

### Stable `compare_three_way` MCP Surface

- Tool name: `compare_three_way`
- Availability: connected mode only
- Parameters:
  - `scope` (required string)
  - `namespace` (optional string)
- Backed by `cub-scout compare three-way --format json`

---

## cub-scout map

### Interactive Mode (default)

```bash
cub-scout map              # Local cluster TUI
cub-scout map --hub        # ConfigHub hierarchy TUI
```

**Behavior:**
- Launches interactive terminal UI
- Reads from current kubectl context
- Exit with `q` or Ctrl+C

### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--hub` | bool | Launch ConfigHub hierarchy view |
| `--json` | bool | Output in JSON format (subcommands) |
| `--verbose` | bool | Show additional details |

---

## cub-scout map list

List resources and their ownership.

```bash
cub-scout map list [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n, --namespace` | string | all | Filter by namespace |
| `--kind` | string | all | Filter by resource kind |
| `--owner` | string | all | Filter by owner type |
| `-q, --query` | string | - | Query expression |
| `--since` | string | - | Time filter (1h, 24h, 7d) |
| `--format` | string | ascii | Output format: ascii, json, md (v0.14+) |
| `--json` | bool | false | JSON output (shorthand for --format json) |
| `--count` | bool | false | Count only |
| `--names-only` | bool | false | Names only (scripting) |

### Owner Values

Valid values for `--owner` and query `owner=`:

| Value | Description |
|-------|-------------|
| `Flux` | Managed by Flux CD |
| `ArgoCD` | Managed by Argo CD |
| `Helm` | Managed by Helm |
| `Terraform` | Managed by Terraform |
| `Crossplane` | Managed by Crossplane |
| `kro` | Managed by kro |
| `ConfigHub` | Managed by ConfigHub |
| `Native` | Not managed by any GitOps tool |

### Query Syntax

```
field=value           # Exact match (case-insensitive)
field!=value          # Not equal
field~=pattern        # Regex match
field=val1,val2       # IN list
field=prefix*         # Wildcard

condition AND condition  # Both must match
condition OR condition   # Either must match
```

**Available fields:** `kind`, `namespace`, `name`, `owner`, `status`, `cluster`, `labels[key]`

### Output Formats

**Plain text (default):**
```
NAMESPACE    KIND        NAME         OWNER
default      Deployment  nginx        Flux
default      Service     nginx-svc    Flux
```

**JSON (`--json`):**
```json
[
  {
    "id": "default/default//Deployment/nginx",
    "clusterName": "default",
    "namespace": "default",
    "kind": "Deployment",
    "name": "nginx",
    "apiVersion": "apps/v1",
    "owner": "Flux",
    "status": "Ready",
    "createdAt": "2025-01-15T10:30:00Z",
    "updatedAt": "2025-01-15T10:32:00Z"
  }
]
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (cluster unreachable, invalid query, etc.) |

---

## cub-scout map status

One-line health summary.

```bash
cub-scout map status
```

**Output format (healthy):**
```
✓ healthy: <N>/<N> deployers, <N>/<N> workloads
```

**Output format (problems detected):**
```
✗ <N> problem(s): <N>/<N> deployers, <N>/<N> workloads
```

**Exit codes:**
| Code | Meaning |
|------|---------|
| 0 | All healthy |
| 1 | Problems detected or error |

Canonical workload scope counted by `map status` (v1.0):
- `Deployment`
- `StatefulSet`

---

## cub-scout map deployers

List deployers in the cluster.

Canonical deployer scope (v1.0):
- Flux `Kustomization`
- Flux `HelmRelease`
- Argo CD `Application`
- Core Kubernetes `Deployment` (fallback where GitOps CRDs are absent)

```bash
cub-scout map deployers [flags]
```

### JSON Output (`--json`)

```json
[
  {
    "kind": "Application",
    "name": "frontend",
    "namespace": "argocd",
    "status": "Healthy",
    "ready": true,
    "revision": "HEAD",
    "resources": 12
  }
]
```

**Stable JSON fields:**
| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Deployer kind (`Kustomization`, `HelmRelease`, `Application`, `Deployment`) |
| `name` | string | Deployer name |
| `namespace` | string | Deployer namespace |
| `status` | string | Status string (for example `Ready`, `Healthy`, `NotReady`, `Unhealthy`) |
| `ready` | bool | Whether the deployer is healthy/ready |
| `revision` | string | Revision string (or `-` if unavailable) |
| `resources` | number | Number of managed resources when available |

---

## cub-scout map cronjobs

List CronJobs with deterministic ordering by `(namespace, name)`.

```bash
cub-scout map cronjobs [--namespace <ns>] [--owner <owner>] [--format ascii|json|md]
```

Stable JSON fields per entry:
- `name`
- `namespace`
- `schedule`
- `suspend`
- `activeJobs`
- `lastScheduleTime`
- `lastRunStatus`
- `owner`
- `lineage` (optional)

---

## cub-scout map jobs

List Jobs with CronJob owner linkage (when ownerReferences are present).

```bash
cub-scout map jobs [--namespace <ns>] [--owner <owner>] [--format ascii|json|md]
```

Stable JSON fields per entry:
- `name`
- `namespace`
- `cronJob` (optional)
- `active`
- `succeeded`
- `failed`
- `lastRunStatus`
- `startTime` (optional)
- `completionTime` (optional)
- `owner`
- `lineage` (optional)

---

## cub-scout map actions

Show read-only operator action previews for a specific resource.

```bash
cub-scout map actions <kind/name> -n <namespace> [--format ascii|json|md]
```

No mutation path is provided by this command.

Stable JSON fields per action:
- `actionType`
- `commandPreview`
- `riskLevel`
- `requiresConfirmation`
- `whySuggested`
- `expectedImpact`

---

## cub-scout map activity

Show normalized activity from Flux/Argo/Helm/events, sorted descending by time.

```bash
cub-scout map activity [--namespace <ns>] [--owner Flux|ArgoCD|Helm] [--since 24h] [--format ascii|json|md]
```

Stable JSON fields per event:
- `time`
- `source`
- `resource`
- `action`
- `result`
- `message` (optional)
- `suggestedNextStep` (optional)
- `owner` (optional)

---

## cub-scout map previews

Detect preview environments using deterministic label/annotation heuristics.

```bash
cub-scout map previews [--namespace <ns>] [--stale-after 72h] [--format ascii|json|md]
```

Stable JSON fields per preview:
- `previewID`
- `repo`
- `prNumber`
- `namespaceOrTarget`
- `age`
- `stale`
- `cleanupSuggestion`
- `matchReason` (optional)

---

## cub-scout trace

Trace a resource to its Git source.

```bash
cub-scout trace <kind/name> -n <namespace> [flags]
cub-scout trace <kind> <name> -n <namespace> [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n, --namespace` | string | flux-system | Resource namespace |
| `--app` | string | - | Trace Argo CD app by name |
| `-r, --reverse` | bool | false | Walk ownerRefs up |
| `-d, --diff` | bool | false | Show Git vs live diff |
| `--history` | bool | false | Show deployment history |
| `--artifacts` | bool | false | Include source artifact provenance |
| `--format` | string | ascii | Output format: ascii, json, md |
| `--json` | bool | false | Deprecated shorthand for `--format json` |
| `--limit` | int | 10 | History entry limit |
| `--explain` | bool | false | Show learning content |

### Behavior by Owner Type

| Owner | Underlying Tool | Behavior |
|-------|-----------------|----------|
| Flux | `flux trace` | Shows GitRepo -> Kustomization/HelmRelease -> Resource |
| ArgoCD | `argocd app get` | Shows Application -> Resource |
| Helm | Release metadata | Shows chart, version, values |
| Native | N/A | Shows "not managed by GitOps" |

### Output (Plain Text)

**Flux-managed resource:**
```
TRACE: Deployment/nginx in demo

  ✓ GitRepository flux-system/flux-system [main@sha1:abc123]
  ✓ Kustomization flux-system/apps [Applied revision: abc123]
  ✓ Deployment demo/nginx [Ready]
```

**Native resource:**
```
TRACE: Deployment/coredns in kube-system

  [warning] resource not managed by Flux
```

### Output (JSON, `--json`)

```json
{
  "command": "trace",
  "target": {
    "kind": "Deployment",
    "name": "nginx",
    "namespace": "demo"
  },
  "chain": [],
  "summary": {
    "ownerType": "Flux",
    "source": {
      "kind": "GitRepository",
      "namespace": "flux-system",
      "name": "apps",
      "url": "https://github.com/acme/apps"
    },
    "deployer": {
      "kind": "Kustomization",
      "namespace": "flux-system",
      "name": "apps"
    }
  }
}
```

When `--artifacts` is enabled and source provenance is available, JSON adds:

```json
"artifact": {
  "url": "http://source-controller...tar.gz",
  "revision": "main@sha1:abc123",
  "digest": "sha256:...",
  "lastUpdateTime": "2026-02-11T15:31:07Z",
  "sourceKind": "GitRepository"
}
```

If artifact metadata is missing/unreadable, the artifact fields are present with `unknown` values.

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Trace successful |
| 1 | Resource not found, trace failed, or not managed |

---

## cub-scout scan

Scan for risk issues and stuck states.

```bash
cub-scout scan [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n, --namespace` | string | all | Namespace to scan |
| `--state` | bool | false | State scan only |
| `--kyverno` | bool | false | Kyverno scan only |
| `--dangling` | bool | false | Scan for orphan resources |
| `--timing-bombs` | bool | false | Scan for expiring certs |
| `--file` | string | - | Scan YAML file (static) |
| `--threshold` | string | 5m | Stuck detection threshold |
| `--json` | bool | false | JSON output |
| `--list` | bool | false | List all KPOL policies |
| `--verbose` | bool | false | Detailed output |
| `--include-unresolved` | bool | false | Include Trivy/Kyverno unresolved |
| `--fail-on` | string | - | Exit 1 when findings at or above threshold: info, warning, critical |

### Static File Scan (`--file`)

Scans a YAML file without cluster access.

**No issues:**
```
STATIC FILE SCAN
File: <FILE>
Resources: <N>

✓ No misconfigurations found
```

**Issues found:**
```
STATIC FILE SCAN
File: <FILE>
Resources: <N>

WARNING (<N>)
────────────────────────────────────────────────────────────────────
[W] Probe timeout exceeds period [RISK-2025-0244]
  Resource: default/Deployment/misconfigured-app
  Message: livenessProbe timeout (10s) > period (5s) - probe may never succeed
  → Remediation: Ensure probe timeoutSeconds <= periodSeconds

════════════════════════════════════════════════════════════════════
Summary: 0 critical, <N> warning, 0 info
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Scan completed successfully (default, even when findings are present) |
| 1 | Error, or `--fail-on` threshold met |

By default, scan exits 0 for a successful scan regardless of findings.
Use `--fail-on <severity>` to exit 1 when findings at or above the threshold
exist (values: `info`, `warning`, `critical`).

---

## Error Behavior

### Resource Not Found

```
Error: trace failed: flux trace failed: exit status 1:
deployments.apps "does-not-exist" not found
```

**Exit code:** 1

### Cluster Unreachable

```
Error: unable to connect to cluster: connection refused
```

**Exit code:** 1

### Invalid Query

```
Error: invalid query: unknown field "foo"
```

**Exit code:** 1

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CLUSTER_NAME` | `default` | Name for this cluster |
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig |

---

## Compatibility Notes

### v1.0 Guarantees

1. **Flag names are stable** — existing flags will not be renamed or removed
2. **JSON schema is stable** — fields will not be removed, only added
3. **Exit codes are stable** — 0 = success, 1 = error/issues
4. **Query syntax is stable** — existing operators work as documented
5. **Sealed schemas** — `attribution-graph.v1` and `attribution-report.v1` are locked

### What May Change (v1.x)

These may change in minor versions without being considered breaking:
- Plain text output formatting (column widths, alignment)
- TUI appearance and keybindings
- New flags may be added
- New JSON fields may be added (additive only)
- Connected mode features may expand

### Breaking Change Policy

Breaking changes require a new major version (v2.0+). We define breaking as:
- Removing a command or flag
- Changing exit code semantics
- Removing JSON fields
- Changing query syntax in incompatible ways

---

## cub-scout gitops status (v0.14.1)

Show GitOps pipeline health and failure details.

```bash
cub-scout gitops status [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n, --namespace` | string | all | Namespace to scan |
| `--json` | bool | false | JSON output |

### Output (Plain Text)

```
GITOPS STATUS
════════════════════════════════════════════════════════════════════

Backend:    flux
Transport:  oci

SOURCES (1)
────────────────────────────────────────────────────────────────────
  ✗ OCIRepository/manifests (flux-system)    failing
    Reason:  AuthenticationFailed

DEPLOYERS (1)
────────────────────────────────────────────────────────────────────
  ✗ Kustomization/app (flux-system)          failing at source

Summary: 0 healthy, 1 failing

NEXT STEPS
────────────────────────────────────────────────────────────────────
• Source failure: Check OCI registry credentials
```

### Output (JSON)

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

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (even if failures shown) |
| 1 | Error (cluster unreachable, etc.) |

---

## cub-scout scan --lifecycle-hazards (v0.19)

Detect Helm lifecycle hook risks when running under ArgoCD.

```bash
cub-scout scan --lifecycle-hazards [flags]
```

Helm hooks (pre-install, post-install, etc.) behave differently under ArgoCD
sync waves. This scanner identifies potential issues.

### Output

```
LIFECYCLE HAZARDS
════════════════════════════════════════════════════════════════════

Found 2 hazard(s)

WARNING
────────────────────────────────────────────────────────────────────
[W] Helm pre-install hook may conflict with ArgoCD PreSync
    Resource: default/Job/db-migrate
    Helm Phase: pre-install
    ArgoCD Phase: PreSync
    Risk: Hook may run twice or in unexpected order
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No hazards found |
| 1 | Hazards found or error |

---

## cub-scout map hooks (v0.19)

List all resources with lifecycle hook annotations (Helm or ArgoCD).

```bash
cub-scout map hooks [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n, --namespace` | string | all | Filter by namespace |
| `--json` | bool | false | JSON output |

### Output

```
LIFECYCLE HOOKS
════════════════════════════════════════════════════════════════════

NAMESPACE   KIND   NAME         HOOK TYPE    PHASE
default     Job    db-migrate   helm         pre-install
default     Job    cleanup      argocd       PostSync
```

---

## cub-scout bundle summarize (v0.19)

Generate summaries from debug bundles for external systems (Jira, PRs, Slack).

```bash
cub-scout bundle summarize <bundle-path> [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | ascii | Output format: ticket, pr, slack, ascii, json |
| `--out` | string | stdout | Output file path |

### Formats

| Format | Output | Use Case |
|--------|--------|----------|
| `ticket` | Markdown | Jira, ServiceNow incident tickets |
| `pr` | Markdown | Pull request descriptions/comments |
| `slack` | Slack Block Kit JSON | Channel notifications |
| `ascii` | Plain text | Human reading (default) |
| `json` | Structured JSON | Downstream tooling |

### Examples

```bash
# Jira/ServiceNow ticket
cub-scout bundle summarize ./bundle --format ticket --out incident.md

# PR description
cub-scout bundle summarize ./bundle --format pr

# Slack notification
cub-scout bundle summarize ./bundle --format slack --out notification.json
```

### Output

Produces a deterministic summary of the bundle's findings. Same bundle
always produces identical output. All content derives from bundle facts.

For the full evidence export schema, field reference, and rendering mappings, see
[Evidence Export v1](evidence-export-v1.md).

---

## cub-scout connect (v1.0)

Configure a kube context for cub-scout using either direct server credentials
or by importing an existing kubeconfig context.

```bash
cub-scout connect [server-url] [flags]
```

### Core Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server` | string | - | Kubernetes API server URL (alternative to positional arg) |
| `--from-kubeconfig` | string | - | Import from existing kubeconfig file |
| `--from-context` | string | - | Source context in imported kubeconfig |
| `--context` | string | derived | Destination context name |
| `--kubeconfig` | string | first `KUBECONFIG` entry or `~/.kube/config` | Destination kubeconfig path |
| `--skip-verify` | bool | false | Skip Kubernetes API connectivity check |
| `--map` | bool | false | Launch `cub-scout map` immediately after connect |

### Authentication Flags

| Flag | Type | Description |
|------|------|-------------|
| `--token` | string | Bearer token auth (or `K8S_BEARER_TOKEN`) |
| `--username`, `--password` | string | Basic auth credentials |
| `--client-cert`, `--client-key` | string | TLS client certificate auth |

Exactly one auth mode is accepted when using direct server mode.

---

## cub-scout completion (v0.19)

Generate shell completion scripts.

```bash
cub-scout completion [bash|zsh|fish|powershell]
```

### Usage

```bash
# Bash (add to ~/.bashrc)
source <(cub-scout completion bash)

# Zsh (add to ~/.zshrc)
source <(cub-scout completion zsh)

# Fish
cub-scout completion fish | source
```

---

## cub-scout status (v1.0)

Show connection status and cluster info.

```bash
cub-scout status [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | false | JSON output |

### Behavior

The status command uses `pkg/hub` for connectivity and authentication state:

- **Offline**: No network connectivity or `CUB_SCOUT_OFFLINE=true`
- **Online**: Network available but not authenticated
- **Connected**: Authenticated with ConfigHub

**Optional dependency:** If the `cub` CLI is installed, status provides richer
information including workspace, auth token validation, and worker status.
Without `cub`, basic mode detection still works via `pkg/hub`.

### Output (Plain Text)

```
ConfigHub:  ● Connected (user@example.com)
Cluster:    prod-east
Context:    eks-prod-east
Worker:     ● bridge-prod (connected)
```

### Output (JSON)

```json
{
  "mode": "connected",
  "email": "user@example.com",
  "cluster_name": "prod-east",
  "context": "eks-prod-east",
  "space": "platform-prod",
  "worker": {
    "name": "bridge-prod",
    "status": "connected"
  }
}
```

---

## cub-scout history (v1.4)

Show connected resource history from ConfigHub ChangeSets.

```bash
cub-scout history <resource> [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-n, --namespace` | string | empty | Optional namespace scope |
| `--since` | string | `7d` | Lookback window (`24h`, `7d`, `2w`) |
| `--format` | string | `ascii` | Output format: `ascii`, `json`, `md` |

### Connected Behavior

- Requires ConfigHub authentication (`cub auth login`).
- Uses read-only ChangeSet queries from ConfigHub.
- Returns clear empty-history messaging when no matching tracked changes exist.

### Output (JSON)

```json
{
  "resource": "deploy/my-app",
  "namespace": "prod",
  "since": "7d",
  "entries": [
    {
      "timestamp": "2026-03-03T14:22:00Z",
      "actor": "ci-bot",
      "change": "image: v1.4.2 -> v1.4.3",
      "changeset": "CS-4821"
    }
  ]
}
```

---

## Environment Variables (v1.0)

| Variable | Default | Description |
|----------|---------|-------------|
| `CLUSTER_NAME` | `default` | Name for this cluster |
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig |
| `CUB_SCOUT_OFFLINE` | unset | Set to `true` to force offline mode |
| `CUB_SCOUT_TELEMETRY` | unset | Set to `false` to disable telemetry |

---

## Related Documentation

- [CLI Guide](../../CLI-GUIDE.md) - Full CLI reference with examples
- [Reference: Commands](commands.md) - Command matrix
- [Reference: Query Syntax](query-syntax.md) - Query language details
