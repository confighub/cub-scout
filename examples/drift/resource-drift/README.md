# Resource Requests/Limits Drift Example

This example shows drift detection for CPU and memory requests/limits.

## Scenario

The desired manifest specifies `cpu: 100m`, but the live cluster has `cpu: 500m` with limits below requests (invalid config).

## Desired State

```yaml
# desired.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: prod
spec:
  replicas: 2
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
      - name: app
        image: nginx:1.19
        resources:
          requests:
            cpu: "100m"
            memory: "256Mi"
          limits:
            cpu: "200m"
            memory: "512Mi"
```

## Running Drift Detection

```bash
cub-scout drift --file desired.yaml
```

## ASCII Output (Normal Drift)

```
Drift Report
============

Cluster: prod-cluster
Source:  desired.yaml (file)

[Capacity] Capacity Drift
└─ [WARNING] Deployment:prod/web
      path: spec.template.spec.containers[name=app].resources.requests.cpu
      desired: 100m
      live:    200m

Summary
-------
Total findings: 1
Affected objects: 1
By severity: warning=1
```

## ASCII Output (Invalid Config — Critical)

If live has `requests.cpu: 500m` but `limits.cpu: 200m` (limits < requests):

```
Drift Report
============

Cluster: prod-cluster
Source:  desired.yaml (file)

[Capacity] Capacity Drift
└─ [CRITICAL] Deployment:prod/web
      path: spec.template.spec.containers[name=app].resources.requests.cpu
      desired: 100m
      live:    500m

Summary
-------
Total findings: 1
Affected objects: 1
By severity: critical=1
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
      "id": "drift:Deployment:prod/web:spec.template.spec.containers[name=app].resources.requests.cpu",
      "object_id": "Deployment:prod/web",
      "path": "spec.template.spec.containers[name=app].resources.requests.cpu",
      "desired": "100m",
      "live": "500m",
      "classification": "capacity",
      "severity": "critical"
    }
  ],
  "summary": {
    "totalFindings": 1,
    "bySeverity": [{ "severity": "critical", "count": 1 }],
    "byClassification": [{ "classification": "capacity", "count": 1 }],
    "affectedObjects": 1
  }
}
```

## Why Critical?

When `limits < requests`, Kubernetes will reject the pod. This is flagged as `critical` because it indicates an invalid configuration that will cause deployment failures.

## CI Usage

```bash
# Fail CI on critical issues (invalid configs)
cub-scout drift --file desired.yaml --fail-on critical
```
