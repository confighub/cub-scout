# Extending the cub-scout and TUI

This document describes how to add custom risk issues, ownership detectors, resource watchers, webhooks, and output plugins.

---

## Extension Points

| Extension | What You Can Do |
|-----------|-----------------|
| **Custom risk issues** | Add your own config anti-pattern detectors |
| **Custom Ownership** | Detect ownership from custom labels/annotations |
| **Custom Resources** | Watch additional CRDs |
| **Webhooks** | Receive real-time events |
| **Output Plugins** | Send GSF to custom destinations |

---

## 1. Custom risk issues

Add your own configuration anti-pattern detectors.

### Risk Definition Format

Create a YAML file in `cve/`:

```yaml
id: RISK-2025-9001
name: Custom Redis MaxMemory Not Set
version: "1.0"
severity: warning
category: CONFIG
description: |
  Redis deployments should have maxmemory configured to prevent OOM kills.
  Without this setting, Redis will use all available memory.

detection:
  # Match resources
  resources:
    - apiVersion: apps/v1
      kind: Deployment
      labelSelector:
        matchLabels:
          app.kubernetes.io/name: redis

  # Check conditions (all must be true)
  conditions:
    - path: spec.template.spec.containers[*].env[?(@.name=="MAXMEMORY")]
      operator: not_exists

    - path: spec.template.spec.containers[*].args
      operator: not_contains
      value: "--maxmemory"

remediation: |
  Add maxmemory configuration to your Redis deployment:

  ```yaml
  env:
    - name: MAXMEMORY
      value: "256mb"
  ```

  Or use command args:
  ```yaml
  args: ["--maxmemory", "256mb", "--maxmemory-policy", "allkeys-lru"]
  ```

references:
  - https://redis.io/docs/reference/eviction/
  - https://your-internal-wiki/redis-standards

tags:
  - redis
  - memory
  - production
```

### Detection Operators

| Operator | Description |
|----------|-------------|
| `exists` | Path must exist |
| `not_exists` | Path must not exist |
| `equals` | Path value equals given value |
| `not_equals` | Path value does not equal given value |
| `contains` | Path value contains substring |
| `not_contains` | Path value does not contain substring |
| `matches` | Path value matches regex |
| `greater_than` | Numeric comparison |
| `less_than` | Numeric comparison |

### Testing Custom risk issues

```bash
# Validate risk definition
cub-scout scan --validate ./my-risks/RISK-2025-9001.yaml

# Test against a specific resource
cub-scout scan --test-risk RISK-2025-9001 --resource my-namespace/Deployment/redis

# Run with custom risk directory (flag name remains legacy for compatibility)
cub-scout scan --risk-dir ./my-risks
```

### Sharing risk issues

1. **Community contribution**: Open a PR to add to the main database
2. **Private risk issues**: Use `--risk-dir` for organization-specific patterns
3. **ConfigHub risk issues**: Upload via ConfigHub for fleet-wide scanning

---

## 2. Custom Ownership Detection

Add detection for custom deployment tools.

### Configuration File

Create one of these files:

- `~/.cub-scout/detectors.yaml` (default)
- or set `CUB_SCOUT_OWNERSHIP_DETECTORS=/path/to/detectors.yaml`

Example:

```yaml
detectors:
  - name: internal-platform
    labels:
      - key: platform.company.com/managed-by
        value: "platform-controller"
    owner_name: "Internal Platform"
    owner_type: "custom"

  - name: pulumi
    annotations:
      - key: pulumi.com/stack
    owner_name: "Pulumi"
    owner_type: "custom"
```

### Matching Rules

1. Built-in detectors run first (Flux, ArgoCD, Helm, Terraform, ConfigHub, Crossplane).
2. Custom detectors run after built-ins, in YAML order.
3. A detector matches when all listed label/annotation rules match.
4. `value` is optional: when omitted, key existence is enough.
5. First matching custom detector wins.

### Behavior on Errors

- Missing config file: silently ignored (default behavior only).
- Invalid config file: warning is printed once to stderr, then built-ins continue.
- Invalid detector entry (missing `name`, no usable rules): that entry is skipped.

### Where It Appears

Custom owners appear in:

- `cub-scout map list` owner column
- `cub-scout explain` owner field
- `cub-scout trace` warning text for unsupported non-GitOps trace chains

---

## 3. Custom Resources

Watch additional CRDs beyond the defaults.

### Configuration

Create one of these files:

- `~/.cub-scout/resources.yaml` (default)
- or set `CUB_SCOUT_RESOURCE_CONFIG=/path/to/resources.yaml`

```yaml
# ~/.cub-scout/resources.yaml
resources:
  # Standard format
  - group: mycompany.io
    version: v1
    resource: widgets

  # With custom state extraction
  - group: mycompany.io
    version: v1
    resource: pipelines
    status:
      healthPath: .status.phase
      healthyValues: ["Succeeded", "Running"]
      degradedValues: ["Failed"]
      progressingValues: ["Pending", "Building"]
```

### Where It Applies

Configured resources are appended to built-in defaults for:

- `cub-scout map list`
- `cub-scout watch`

### Matching Rules

1. Built-in resources are always included first.
2. Configured resources are appended in YAML order.
3. Duplicate Group/Version/Resource entries are deduplicated.
4. Invalid entries (missing group/version/resource) are skipped.

### Behavior on Errors

- Missing config file: silently ignored.
- Invalid config file: warning printed once to stderr, then defaults continue.

### Status Extraction Fields

`status.*` keys are accepted for forward compatibility, but this slice does not
override status rendering with those fields yet.

### Programmatic Registration

```go
import "github.com/confighub/agent/pkg/watcher"

watcher.RegisterResource(watcher.ResourceConfig{
    Group:    "mycompany.io",
    Version:  "v1",
    Resource: "widgets",
    StatusExtractor: func(obj *unstructured.Unstructured) string {
        phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
        return phase
    },
})
```

---

## 4. Webhooks (Available)

Receive read-only observation events from `cub-scout` using `watch`.

### Command Surface

```bash
cub-scout watch --webhook <url> [flags]
```

At least one destination is required:
- `--webhook <url>`
- and/or `--output-file <path>`

### Event Types

| Event Type | Description |
|------------|-------------|
| `resource.discovered` | New or changed resource discovered |
| `ownership.changed` | Ownership classification changed |
| `drift.detected` | Drift signal detected |
| `scan.finding` | Risk/issue finding emitted |

### Event Payload (JSON)

```json
{
  "type": "scan.finding",
  "timestamp": "2026-03-07T17:00:00Z",
  "resource": {
    "kind": "Deployment",
    "name": "api",
    "namespace": "prod"
  },
  "owner": {
    "type": "Flux",
    "name": "frontend-app"
  },
  "severity": "warning",
  "details": {
    "category": "STATE",
    "message": "out of sync"
  }
}
```

### Usage Examples

```bash
# One deterministic collection cycle
cub-scout watch --webhook http://127.0.0.1:8787/events --once

# Continuous stream with filters
cub-scout watch --webhook https://hooks.example.com/cub-scout --interval 30s --namespace prod --severity warning,critical

# File sink only (JSONL)
cub-scout watch --output-file /tmp/cub-scout-events.jsonl --once
```

See `docs/reference/commands.md` (`watch`) and `examples/watch-webhook/`.

---

## 5. Output Destinations (Current + Future)

Current output and sink options are command-specific and deterministic.

### Available Today

| Surface | Output/Sink | Command |
|---------|-------------|---------|
| Core command output | `ascii`, `json`, `md` | Most commands (`--format`) |
| Watch stream to webhook | HTTP POST events | `cub-scout watch --webhook ...` |
| Watch stream to file | JSONL file sink | `cub-scout watch --output-file ...` |
| Connected summary digest | Slack webhook | `cub-scout summary slack --webhook-url ...` |

### Notes

- `cub-scout` remains read-only for cluster state.
- Connected workflows may write summary records to configured local summary storage.
- File sink and webhook routing are available now; broader pluggable sink architecture remains follow-up work.

See `docs/reference/commands.md` (`watch`, `summary list`, `summary slack`) and `examples/connected-summary-storage/`.

---

## 6. GraphQL API (Future)

`cub-scout` does not currently expose a GraphQL server.

For automation today, use deterministic JSON from existing command surfaces:

```bash
cub-scout map list --json
cub-scout trace deploy/api -n prod --format json
cub-scout scan --json
```

Then filter/transform with your normal tooling (`jq`, scripts, pipelines).

---

## Plugin Directory

Community plugins are listed at:
- https://github.com/confighub/agent-plugins

Submit your plugin via PR to be listed.

---

## See Also

- [Architecture](../concepts/architecture.md) — GSF protocol and API contracts
- [CLI Guide](../../CLI-GUIDE.md) — Workflow-first CLI tour
- [Scan for risk issues](scan-for-risks.md) — Risk detection and remediation
