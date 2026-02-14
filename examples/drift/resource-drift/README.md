# Resource Requests/Limits Drift

## The Problem

Your Git manifest says `cpu: 100m` requests and `cpu: 200m` limits.
Someone scaled up the requests to `500m` directly — but didn't touch the limits.

Now `requests > limits`, which is an **invalid configuration** that Kubernetes will reject
on the next pod restart. The current pods keep running (they were created before the edit),
but new pods will fail to schedule.

**cub-scout catches this as critical:**

```
$ ./cub-scout drift --file desired.yaml

  Drift Report
  ════════════
  Cluster: prod-cluster
  Source:  desired.yaml (file)

  [Capacity] Capacity Drift
  └─ [CRITICAL] Deployment:prod/web
        path: spec.template.spec.containers[name=app].resources.requests.cpu
        desired: 100m
        live:    500m

  Summary: 1 finding (critical=1), 1 affected object
```

## How It Works

```
desired.yaml (Git)             Live Cluster
──────────────────────         ──────────────────────
requests.cpu:  100m    ──→  ✗  requests.cpu:  500m   ← DRIFTED (critical!)
limits.cpu:    200m    ──→  ✓  limits.cpu:    200m   ← matches
requests.memory: 256Mi ──→  ✓  requests.memory: 256Mi← matches
limits.memory:   512Mi ──→  ✓  limits.memory:   512Mi← matches

⚠ Live state: requests (500m) > limits (200m) = INVALID CONFIG
  Next pod restart will fail with: Insufficient cpu
```

## Why Critical?

| Scenario | Severity | What happens |
|----------|----------|-------------|
| `requests: 100m → 200m` (within limits) | **warning** | Pods work but use more resources than planned |
| `requests: 100m → 500m` (exceeds limits) | **critical** | Invalid config — next pod creation fails |

When `limits < requests`, Kubernetes rejects the pod spec. Existing pods keep running,
but any scale-up, rollout, or node migration will fail.

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

## Quick Start

```bash
# Detect drift
./cub-scout drift --file desired.yaml

# CI gate: fail on critical (invalid configs only)
./cub-scout drift --file desired.yaml --fail-on critical

# JSON output for programmatic use
./cub-scout drift --file desired.yaml --format json
```

## See Also

- [Env Var Drift](../env-var-drift/) — Configuration value divergence
- [Image Policy Drift](../image-policy-drift/) — imagePullPolicy divergence
