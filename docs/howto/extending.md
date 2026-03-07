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

## 4. Webhooks (Planned)

> **Not Yet Implemented:** This feature is planned for a future release.

Receive real-time events from the Agent.

### Configuration

```yaml
# config/webhooks.yaml
webhooks:
  - name: slack-alerts
    url: https://hooks.slack.com/services/xxx
    events:
      - finding.created
      - finding.resolved
    filter:
      severity: [critical, warning]

  - name: custom-system
    url: https://my-system.internal/webhook
    events:
      - entry.created
      - entry.updated
      - entry.deleted
      - drift.detected
      - drift.resolved
    headers:
      Authorization: Bearer ${WEBHOOK_TOKEN}
    retry:
      maxAttempts: 3
      backoffSeconds: [1, 5, 30]
```

### Event Types

| Event | Payload |
|-------|---------|
| `entry.created` | GSFEntry |
| `entry.updated` | GSFEntry (before/after) |
| `entry.deleted` | GSFEntry |
| `drift.detected` | GSFEntry with drift |
| `drift.resolved` | GSFEntry |
| `finding.created` | GSFFinding |
| `finding.resolved` | GSFFinding |
| `relation.created` | GSFRelation |
| `relation.deleted` | GSFRelation |

### Webhook Payload

```json
{
  "event": "finding.created",
  "timestamp": "2025-01-02T18:30:00Z",
  "cluster": "prod-east",
  "data": {
    "id": "RISK-2025-0027",
    "severity": "critical",
    "resource": "prod-east/monitoring/ConfigMap/grafana-sidecar",
    "message": "Namespace whitespace in sidecar config"
  }
}
```

### Using Webhooks

When implemented, webhooks will be configured via a YAML file.

---

## 5. Output Plugins (Planned)

> **Not Yet Implemented:** This feature is planned for a future release.

Send GSF output to custom destinations.

### Built-in Outputs

| Output | Flag | Description |
|--------|------|-------------|
| stdout | `--output=json` | JSON to stdout |
| stdout | `--output=jsonl` | JSON Lines to stdout |
| ConfigHub | `--output=confighub` | Stream to ConfigHub API |
| File | `--output=file:./out.json` | Write to file |
| Prometheus | `--metrics` | Expose /metrics endpoint |

### Custom Output Plugin

```go
// pkg/output/plugin.go

type OutputPlugin interface {
    Name() string
    Init(config map[string]interface{}) error
    Write(snapshot *GSFSnapshot) error
    WriteEvent(event *GSFEvent) error
    Close() error
}
```

### Example: Kafka Output

```go
type KafkaOutput struct {
    producer *kafka.Producer
    topic    string
}

func (k *KafkaOutput) Name() string {
    return "kafka"
}

func (k *KafkaOutput) Init(config map[string]interface{}) error {
    k.topic = config["topic"].(string)
    // Initialize Kafka producer
    return nil
}

func (k *KafkaOutput) Write(snapshot *GSFSnapshot) error {
    data, _ := json.Marshal(snapshot)
    return k.producer.Produce(&kafka.Message{
        TopicPartition: kafka.TopicPartition{Topic: &k.topic},
        Value:          data,
    }, nil)
}
```

### Using Custom Output

When implemented, custom output plugins will be configured via command-line flags.

---

## 6. GraphQL API (Planned)

A GraphQL API for flexible queries is planned:

```graphql
query {
  entries(
    cluster: "prod-east"
    owner: { type: "flux" }
    status: DEGRADED
  ) {
    id
    name
    owner { type ref }
    drift { detected fields { path desired live } }
    relations { to { id } type }
  }

  findings(severity: [CRITICAL, WARNING]) {
    id
    severity
    resource { id name namespace }
    remediation
  }
}
```

---

## Plugin Directory

Community plugins are listed at:
- https://github.com/confighub/agent-plugins

Submit your plugin via PR to be listed.

---

## See Also

- [Architecture](../concepts/architecture.md) — GSF protocol and API contracts
- [CLI Guide](../../CLI-GUIDE.md) — CLI reference and configuration
- [Scan for risk issues](scan-for-risks.md) — Risk detection and remediation
