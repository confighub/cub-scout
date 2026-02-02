# CLI Contract Reference

This document defines the stable CLI behavior for cub-scout v0.5.
These commands, flags, and output formats are considered stable and
breaking changes will be avoided.

**This is the authoritative reference for v0.5.**

---

## Contract Freeze Notice

> **v0.5 CLI surfaces are frozen.**
>
> The commands, output formats, and exit codes documented here are locked.
> Changes to contracted behavior require a new minor version (v0.6+).
>
> If documentation and behavior ever diverge, **golden tests under
> `test/golden/` are the source of truth**.

---

## Stable Commands

The following commands are stable for v0.5:

| Command | Purpose | Stable Since |
|---------|---------|--------------|
| `cub-scout map` | Interactive TUI dashboard | v0.5 |
| `cub-scout map list` | List resources (scriptable) | v0.5 |
| `cub-scout map status` | One-line health check | v0.5 |
| `cub-scout map deployers` | List GitOps deployers | v0.5 |
| `cub-scout trace` | Trace resource to Git source | v0.5 |
| `cub-scout scan` | Scan for CCVEs and issues | v0.5 |

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
| `--json` | bool | false | JSON output |
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

---

## cub-scout map deployers

List deployers (Kubernetes Deployments) in the cluster.

> **v0.5 contract:** Deployers are Kubernetes Deployments. Flux Kustomizations,
> HelmReleases, and Argo Applications are out of scope for v0.5 and may be
> added in a future release.

```bash
cub-scout map deployers [flags]
```

### JSON Output (`--json`)

```json
[
  {
    "kind": "Deployment",
    "name": "app-alpha",
    "namespace": "default",
    "status": "Ready",
    "ready": true,
    "revision": "-"
  }
]
```

**Stable JSON fields:**
| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Always `Deployment` in v0.5 |
| `name` | string | Deployment name |
| `namespace` | string | Deployment namespace |
| `status` | string | Status string (e.g., `Ready`) |
| `ready` | bool | Whether deployment is ready |
| `revision` | string | Revision string (or `-` if unavailable) |

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
| `--json` | bool | false | JSON output |
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
  "resource": {
    "kind": "Deployment",
    "name": "nginx",
    "namespace": "demo"
  },
  "owner": {
    "type": "flux",
    "subType": "kustomization",
    "name": "apps",
    "namespace": "flux-system"
  },
  "chain": [...]
}
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Trace successful |
| 1 | Resource not found, trace failed, or not managed |

---

## cub-scout scan

Scan for CCVEs and stuck states.

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
[W] Probe timeout exceeds period [CCVE-2025-0244]
  Resource: default/Deployment/misconfigured-app
  Message: livenessProbe timeout (10s) > period (5s) - probe may never succeed
  → Remediation: Ensure probe timeoutSeconds <= periodSeconds

════════════════════════════════════════════════════════════════════
Summary: 0 critical, <N> warning, 0 info
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No misconfigurations found |
| 1 | Misconfigurations found or error |

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

### v0.5 Guarantees

1. **Flag names are stable** - existing flags will not be renamed or removed
2. **JSON schema is stable** - fields will not be removed, only added
3. **Exit codes are stable** - 0 = success, 1 = error/issues
4. **Query syntax is stable** - existing operators work as documented

### Future Changes (v0.6+)

These may change in future versions:
- Plain text output formatting (column widths, alignment)
- TUI appearance and keybindings
- New flags may be added
- New JSON fields may be added

---

## Related Documentation

- [CLI Guide](../../CLI-GUIDE.md) - Full CLI reference with examples
- [Reference: Commands](commands.md) - Command matrix
- [Reference: Query Syntax](query-syntax.md) - Query language details
