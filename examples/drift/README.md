# Drift Detection Examples

Detect when your live cluster state doesn't match what's in Git.

> **Current scope (v0.14.3):** `cub-scout drift` currently compares `spec.replicas`
> and container images only. The examples below illustrate the *target* drift model
> (env vars, imagePullPolicy, resource requests/limits) which is not yet implemented.
> See [#332](https://github.com/confighub/cub-scout/issues/332) for tracking.

## The Problem

Someone runs `kubectl edit` or `kubectl set env` in production during an incident.
The fix works, but nobody updates Git. ArgoCD still says "Synced" because it synced
the *old* state successfully. Your cluster has silently drifted.

**Target: cub-scout will compare desired manifests against live state:**

```
desired.yaml (Git)          Live Cluster           Result
──────────────────          ──────────────         ─────────────
LOG_LEVEL: info    ──→  ✗   LOG_LEVEL: debug       WARNING: config drift
imagePullPolicy:   ──→  ✗   imagePullPolicy:       WARNING: rollout drift
  Always                      IfNotPresent
requests.cpu:      ──→  ✗   requests.cpu:          CRITICAL: invalid config
  100m                        500m (> limits!)
```

## How cub-scout Sees Drift

```
DRIFT REPORT
════════════════════════════════════════════════════════════════════

Flux (3 drifted resources)
────────────────────────────────────────────────────────────────────
  ├── Deployment:prod/api         [Config]    WARNING
  │   └── env LOG_LEVEL: info → debug
  ├── Deployment:prod/worker      [Rollout]   WARNING
  │   └── imagePullPolicy: Always → IfNotPresent
  └── Deployment:prod/web         [Capacity]  CRITICAL
      └── requests.cpu: 100m → 500m  ⚠ exceeds limits (200m)

════════════════════════════════════════════════════════════════════
Summary: 3 findings │ 2 warning │ 1 critical │ 3 affected objects

  Config     █████████████░░░░░░░░░░░░░░░░░░  33%
  Rollout    █████████████░░░░░░░░░░░░░░░░░░  33%
  Capacity   █████████████░░░░░░░░░░░░░░░░░░  33%  ← includes invalid state

→ Fix critical first: cub-scout drift --file desired.yaml --fail-on critical
```

## Examples

| Example | Drift Type | Severity | Why It Matters |
|---------|-----------|----------|----------------|
| [env-var-drift/](env-var-drift/) | `LOG_LEVEL` changed | Warning | Running with wrong config |
| [image-policy-drift/](image-policy-drift/) | `imagePullPolicy` changed | Warning | Silently running stale images |
| [resource-drift/](resource-drift/) | `requests.cpu` exceeds limits | Critical | Next pod restart will fail |
| [mutation-cause-attribution/](mutation-cause-attribution/) | Cause classification | n/a | Distinguish controller-drift vs `kubectl edit` |

## Quick Start

```bash
# Run any example
./cub-scout drift --file examples/drift/env-var-drift/desired.yaml
./cub-scout drift --file examples/drift/image-policy-drift/desired.yaml
./cub-scout drift --file examples/drift/resource-drift/desired.yaml

# JSON output for CI pipelines
./cub-scout drift --file desired.yaml --format json

# CI gate: fail on warnings or critical
./cub-scout drift --file desired.yaml --fail-on warning
```

## Drift Classifications

| Classification | Meaning | Example |
|---------------|---------|---------|
| `config` | Application configuration differs | Environment variables |
| `rollout` | Deployment behavior differs | Image pull policy, strategy |
| `capacity` | Resource allocation differs | CPU/memory requests and limits |

## Severity Levels

| Severity | Meaning |
|----------|---------|
| `info` | Cosmetic difference (labels, annotations) |
| `warning` | Functional difference (wrong config, wrong policy) |
| `critical` | Invalid state (limits < requests, missing required fields) |

## See Also

- [Apptique Drift Scenario](../apptique-examples/scenarios/drift-detection/) — Working drift demo with real deployments
- [Platform Example](../platform-example/) — Clobbering demo (Flux reverts manual changes)
