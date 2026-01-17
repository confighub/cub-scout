# Extending the ConfigHub Agent and TUI

This document describes how to add custom CCVEs, ownership detectors, resource watchers, webhooks, and output plugins.

---

## Extension Points

| Extension | What You Can Do |
|-----------|-----------------|
| **Custom CCVEs** | Add your own config anti-pattern detectors |
| **Custom Ownership** | Detect ownership from custom labels/annotations |
| **Custom Resources** | Watch additional CRDs |
| **Webhooks** | Receive real-time events |
| **Output Plugins** | Send GSF to custom destinations |

---

## 1. Custom CCVEs

Add your own configuration anti-pattern detectors.

### CCVE Definition Format

Create a YAML file in `cve/`:

```yaml
id: CCVE-2025-9001
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

### Testing Custom CCVEs

```bash
# Validate CCVE definition
cub-scout scan --validate cve/ccve/CCVE-2025-9001.yaml

# Test against a specific resource
cub-scout scan --test-ccve CCVE-2025-9001 --resource my-namespace/Deployment/redis

# Run with custom CCVE directory
cub-scout scan --ccve-dir ./my-ccves
```

### Sharing CCVEs

1. **Community contribution**: Open a PR to add to the main database
2. **Private CCVEs**: Use `--ccve-dir` for organization-specific patterns
3. **ConfigHub CCVEs**: Upload via ConfigHub for fleet-wide scanning

---

## 2. Custom Ownership Detection

Add detection for custom deployment tools.

### Ownership Detector Interface

```go
// pkg/ownership/detector.go

type Detector interface {
    // Name returns the owner type name (e.g., "mydeployer")
    Name() string

    // Priority returns detection priority (lower = higher priority)
    Priority() int

    // Detect checks if this detector owns the resource
    // Returns (owner, confidence) where confidence is 0.0-1.0
    Detect(obj *unstructured.Unstructured) (*Owner, float64)
}

type Owner struct {
    Type   string            // e.g., "mydeployer"
    Ref    string            // e.g., "pipeline/prod-deploy"
    Labels map[string]string // Additional ownership metadata
}
```

### Example: Custom Detector

```go
// pkg/ownership/mydeployer.go

type MyDeployerDetector struct{}

func (d *MyDeployerDetector) Name() string {
    return "mydeployer"
}

func (d *MyDeployerDetector) Priority() int {
    return 50  // Between Flux (20) and Unknown (100)
}

func (d *MyDeployerDetector) Detect(obj *unstructured.Unstructured) (*Owner, float64) {
    labels := obj.GetLabels()

    // Check for our custom label
    if pipeline, ok := labels["mycompany.io/deployed-by"]; ok {
        return &Owner{
            Type: "mydeployer",
            Ref:  pipeline,
            Labels: map[string]string{
                "pipeline": pipeline,
            },
        }, 1.0  // High confidence
    }

    return nil, 0
}
```

### Registering Custom Detectors

```go
// In your main.go or init

import "github.com/confighub/agent/pkg/ownership"

func init() {
    ownership.Register(&MyDeployerDetector{})
}
```

### Configuration-Based Detectors

For simple label/annotation detection, use config:

```yaml
# config/ownership.yaml
detectors:
  - name: mydeployer
    priority: 50
    labels:
      - key: mycompany.io/deployed-by
        ref_field: true  # Use this label's value as the owner ref
    annotations:
      - key: mycompany.io/pipeline-id
```

**Note:** Configuration-based detectors are not yet implemented. Currently, custom detectors must be added in Go code.

---

## 3. Custom Resources

Watch additional CRDs beyond the defaults.

### Configuration

```yaml
# config/resources.yaml
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

**Note:** Configuration-based resource watching is not yet implemented. Currently, custom resources must be registered in Go code.

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
    "id": "CCVE-2025-0027",
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

- [ARCHITECTURE.md](ARCHITECTURE.md) — GSF protocol and API contracts
- [CLI-REFERENCE.md](CLI-REFERENCE.md) — CLI reference and configuration
- [CCVE-GUIDE.md](CCVE-GUIDE.md) — CCVE detection and remediation
