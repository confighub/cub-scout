# Image Pull Policy Drift

## The Problem

Your Git manifest says `imagePullPolicy: Always` (to ensure you always get the latest build),
but someone changed it to `IfNotPresent` on the cluster. Now your pods use a cached image
from 3 weeks ago instead of the latest fix.

Deployments succeed. Pods run. But you're running stale code and don't know it.

**cub-scout detects the divergence:**

```
$ ./cub-scout drift --file desired.yaml

  Drift Report
  ════════════
  Cluster: prod-cluster
  Source:  desired.yaml (file)

  [Rollout] Rollout Drift
  └─ [WARNING] Deployment:prod/worker
        path: spec.template.spec.containers[name=app].imagePullPolicy
        desired: Always
        live:    IfNotPresent

  Summary: 1 finding, 1 affected object
```

## How It Works

```
desired.yaml (Git)                Live Cluster
──────────────────────            ──────────────────────
imagePullPolicy: Always   ──→  ✗  imagePullPolicy: IfNotPresent  ← DRIFTED
image: myworker:latest    ──→  ✓  image: myworker:latest         ← matches
```

## Why This Matters

| Policy | Behavior | Risk of Drift |
|--------|----------|---------------|
| `Always` | Pull on every pod start | Desired: ensures latest code |
| `IfNotPresent` | Use cached if available | Risk: stale code in production |
| `Never` | Only local images | Risk: pod fails if image evicted |

Drifting from `Always` → `IfNotPresent` means your pods silently stop getting updates.

## Ownership Context

```
./cub-scout trace deploy/worker -n prod

TRACE: Deployment:prod/worker
════════════════════════════════════════════════════════════════════
  GitRepository/platform-config (flux-system)
  └── Kustomization/apps (flux-system)
      └── Deployment/worker (prod)                     ← Owner: Flux
          ├── imagePullPolicy: Always (Git)
          │                    IfNotPresent (Live)      ← DRIFT
          └── image: myworker:latest (Git = Live)       ✓ OK

  Impact:
  ├── Pod worker-8f7d6c-abc12  Running  image cached 21d ago
  ├── Pod worker-8f7d6c-def34  Running  image cached 21d ago
  └── Pod worker-8f7d6c-ghi56  Running  image cached 21d ago
      └── ⚠ All 3 pods using stale image (3 weeks old)
```

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
    spec:
      containers:
      - name: app
        image: myworker:latest
        imagePullPolicy: Always
```

## Quick Start

```bash
# Detect drift
./cub-scout drift --file desired.yaml

# CI gate: fail if pull policy drifts
./cub-scout drift --file desired.yaml --fail-on warning
```

## See Also

- [Env Var Drift](../env-var-drift/) — Configuration value divergence
- [Resource Drift](../resource-drift/) — CPU/memory request drift (with invalid config detection)
