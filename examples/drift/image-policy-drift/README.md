# Image Pull Policy Drift Example

This example shows drift detection for imagePullPolicy.

## Scenario

The desired manifest specifies `imagePullPolicy: Always`, but the live cluster has `imagePullPolicy: IfNotPresent`.

## Desired State

```yaml
# desired.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: worker
  namespace: prod
spec:
  replicas: 3
  selector:
    matchLabels:
      app: worker
  template:
    metadata:
      labels:
        app: worker
    spec:
      containers:
      - name: app
        image: myworker:latest
        imagePullPolicy: Always
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

[Rollout] Rollout Drift
└─ [WARNING] Deployment:prod/worker
      path: spec.template.spec.containers[name=app].imagePullPolicy
      desired: Always
      live:    IfNotPresent

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
  "findings": [
    {
      "id": "drift:Deployment:prod/worker:spec.template.spec.containers[name=app].imagePullPolicy",
      "object_id": "Deployment:prod/worker",
      "path": "spec.template.spec.containers[name=app].imagePullPolicy",
      "desired": "Always",
      "live": "IfNotPresent",
      "classification": "rollout",
      "severity": "warning"
    }
  ],
  "summary": {
    "totalFindings": 1,
    "bySeverity": [{ "severity": "warning", "count": 1 }],
    "byClassification": [{ "classification": "rollout", "count": 1 }],
    "affectedObjects": 1
  }
}
```

## Why This Matters

- `Always`: Ensures the latest image is always pulled (good for `:latest` tags)
- `IfNotPresent`: Uses cached image if available (faster but may run stale code)
- `Never`: Only uses local images

Drift in pull policy can cause unexpected behavior during deployments.

## CI Usage

```bash
# Fail CI if pull policy drifts
cub-scout drift --file desired.yaml --fail-on warning
```
