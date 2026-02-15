# Environment Variable Drift

## The Problem

Your Git manifest says `LOG_LEVEL=info`, but someone `kubectl set env`'d it to `debug`
in production during an incident — and forgot to revert it.

The deployment is running. ArgoCD says "Synced." But the live state doesn't match Git.

**cub-scout catches it:**

```
$ ./cub-scout drift --file desired.yaml

  Drift Report
  ════════════
  Cluster: prod-cluster
  Source:  desired.yaml (file)

  [Config] Configuration Drift
  └─ [WARNING] Deployment:prod/api
        path: spec.template.spec.containers[name=app].env[name=LOG_LEVEL]
        desired: info
        live:    debug

  Summary: 1 finding, 1 affected object
```

## How It Works

```
desired.yaml (Git)          Live Cluster
─────────────────           ─────────────────
LOG_LEVEL: info    ──→  ✗  LOG_LEVEL: debug   ← DRIFTED
PORT: 8080         ──→  ✓  PORT: 8080         ← matches
```

cub-scout compares the desired manifest against what's actually running.
Any mismatch is a drift finding with a severity and classification.

## Ownership Context

```
./cub-scout trace deploy/api -n prod

TRACE: Deployment:prod/api
════════════════════════════════════════════════════════════════════
  GitRepository/platform-config (flux-system)
  └── Kustomization/apps (flux-system)
      └── Deployment/api (prod)               ← Owner: Flux
          ├── env LOG_LEVEL: info (Git)
          │                  debug (Live)      ← DRIFT
          └── env PORT: 8080 (Git = Live)      ✓ OK
```

The drift is in a Flux-managed resource — someone bypassed GitOps with `kubectl set env`.

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

## Quick Start

```bash
# Detect drift
./cub-scout drift --file desired.yaml

# JSON output for tooling
./cub-scout drift --file desired.yaml --format json

# CI gate: fail if any drift detected
./cub-scout drift --file desired.yaml --fail-on warning
echo $?  # Returns 2 if drift found
```

## JSON Output

```json
{
  "command": "drift",
  "findings": [
    {
      "id": "drift:Deployment:prod/api:spec.template.spec.containers[name=app].env[name=LOG_LEVEL]",
      "object_id": "Deployment:prod/api",
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

## See Also

- [Image Policy Drift](../image-policy-drift/) — imagePullPolicy divergence
- [Resource Drift](../resource-drift/) — CPU/memory request drift (with invalid config detection)
