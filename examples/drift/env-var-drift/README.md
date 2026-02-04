# Environment Variable Drift Example

This example shows drift detection for environment variables.

## Scenario

The desired manifest specifies `LOG_LEVEL=info`, but the live cluster has `LOG_LEVEL=debug`.

## Desired State

```yaml
# desired.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
      - name: app
        image: myapp:v1.0
        env:
        - name: LOG_LEVEL
          value: "info"
        - name: PORT
          value: "8080"
```

## Running Drift Detection

```bash
cub-scout drift --file desired.yaml
```

## ASCII Output

```
Drift Report
============

Cluster: prod-cluster
Source:  desired.yaml (file)

[Config] Configuration Drift
└─ [WARNING] Deployment:prod/api
      path: spec.template.spec.containers[name=app].env[name=LOG_LEVEL]
      desired: info
      live:    debug

Summary
-------
Total findings: 1
Affected objects: 1
By severity: warning=1
```

## JSON Output

```bash
cub-scout drift --file desired.yaml --format json
```

```json
{
  "command": "drift",
  "context": {
    "cluster": "prod-cluster",
    "namespace": null,
    "desiredSource": { "type": "file", "ref": "desired.yaml" },
    "liveSource": { "type": "cluster", "ref": "prod-cluster" }
  },
  "findings": [
    {
      "id": "drift:Deployment:prod/api:spec.template.spec.containers[name=app].env[name=LOG_LEVEL]",
      "object_id": "Deployment:prod/api",
      "object": { "kind": "Deployment", "namespace": "prod", "name": "api" },
      "path": "spec.template.spec.containers[name=app].env[name=LOG_LEVEL]",
      "desired": "info",
      "live": "debug",
      "classification": "config",
      "severity": "warning"
    }
  ],
  "summary": {
    "totalFindings": 1,
    "bySeverity": [{ "severity": "warning", "count": 1 }],
    "byClassification": [{ "classification": "config", "count": 1 }],
    "affectedObjects": 1
  }
}
```

## CI Usage

```bash
# Fail CI if any env var drift is detected
cub-scout drift --file desired.yaml --fail-on warning
echo $?  # Returns 2 if drift found
```
