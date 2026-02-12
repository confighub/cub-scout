# Drift Detection

**Version:** v0.14.4
**Purpose:** Detect differences between desired state and live cluster state

---

## Overview

Drift detection compares what *should* exist (desired state from files/git) against what *actually* exists (live state in cluster) and reports any differences as structured findings.

```bash
cub-scout drift --file manifests/deployment.yaml
```

---

## Quick Start

### Basic Usage

```bash
# Compare a YAML file against the cluster
cub-scout drift --file manifests/deployment.yaml

# Compare a directory of manifests
cub-scout drift --file manifests/

# Filter by namespace
cub-scout drift --file manifests/ -n production
```

### Output Formats

```bash
# Human-readable (default)
cub-scout drift --file manifests/

# JSON for automation
cub-scout drift --file manifests/ --format json

# Fail CI on warnings or critical
cub-scout drift --file manifests/ --fail-on warning
```

---

## What Drift Detection Covers

### Replicas (v0.14.3)

Detects when live replica count differs from desired:

```
[Capacity] Capacity Drift
└─ [WARNING] Deployment:prod/api
      path: spec.replicas
      desired: 3
      live:    5
```

**Severity:**
- `warning` if scaled down (`live < desired`)
- `info` if scaled up (`live > desired`, often intentional via HPA)

### Container Images (v0.14.3)

Detects when container images differ:

```
[Image] Image Drift
└─ [CRITICAL] Deployment:prod/web
      path: spec.template.spec.containers[0].image
      desired: nginx:1.19
      live:    apache:2.4
```

**Severity:**
- `critical` if different repository (completely different workload)
- `warning` if different tag (version mismatch)

### Environment Variables (v0.14.4)

Detects added, removed, or changed environment variables:

```
[Config] Configuration Drift
└─ [WARNING] Deployment:prod/api
      path: spec.template.spec.containers[name=app].env[name=LOG_LEVEL]
      desired: info
      live:    debug
```

**Severity:** Always `warning`

### Resource Requests/Limits (v0.14.4)

Detects changes to CPU and memory requests/limits:

```
[Capacity] Capacity Drift
└─ [WARNING] Deployment:prod/api
      path: spec.template.spec.containers[name=app].resources.requests.cpu
      desired: 100m
      live:    200m
```

**Severity:**
- `critical` if invalid config (limits < requests)
- `warning` for normal drift

### Image Pull Policy (v0.14.4)

Detects changes to imagePullPolicy:

```
[Rollout] Rollout Drift
└─ [WARNING] Deployment:prod/api
      path: spec.template.spec.containers[name=app].imagePullPolicy
      desired: Always
      live:    IfNotPresent
```

**Severity:** Always `warning`

---

## CI Integration

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No failure (or no `--fail-on` specified) |
| 1 | Operational error |
| 2 | Findings met threshold |

### Examples

```bash
# Fail on any warning or critical
cub-scout drift --file manifests/ --fail-on warning
echo $?  # 2 if warnings found, 0 otherwise

# Fail only on critical
cub-scout drift --file manifests/ --fail-on critical

# Pure reporting (never fails on findings)
cub-scout drift --file manifests/
```

### GitHub Actions Example

```yaml
jobs:
  drift-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Check for drift
        run: |
          cub-scout drift --file manifests/ --fail-on warning --format json > drift.json
        continue-on-error: true

      - name: Upload drift report
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: drift-report
          path: drift.json
```

---

## JSON Output

For automation, use `--format json`:

```json
{
  "command": "drift",
  "context": {
    "cluster": "prod-east",
    "namespace": null,
    "desiredSource": { "type": "file", "ref": "manifests/" },
    "liveSource": { "type": "cluster", "ref": "prod-east" }
  },
  "findings": [
    {
      "id": "drift:Deployment:prod/api:spec.replicas",
      "object_id": "Deployment:prod/api",
      "object": { "kind": "Deployment", "namespace": "prod", "name": "api" },
      "path": "spec.replicas",
      "desired": 3,
      "live": 5,
      "classification": "capacity",
      "severity": "warning"
    }
  ],
  "summary": {
    "totalFindings": 1,
    "bySeverity": [{ "severity": "warning", "count": 1 }],
    "byClassification": [{ "classification": "capacity", "count": 1 }],
    "affectedObjects": 1
  }
}
```

For current JSON contract entry points, see [reference/json-contracts.md](reference/json-contracts.md).
The historical v0.14 schema doc is archived at [archive/v0.14-json-schema.md](archive/v0.14-json-schema.md).

---

## Path Formats

Drift findings use stable path formats:

| Drift Type | Path Format |
|------------|-------------|
| Replicas | `spec.replicas` |
| Image | `spec.template.spec.containers[<idx>].image` |
| Env var | `spec.template.spec.containers[name=<container>].env[name=<var>]` |
| Resources | `spec.template.spec.containers[name=<container>].resources.<type>.<resource>` |
| Pull policy | `spec.template.spec.containers[name=<container>].imagePullPolicy` |

Container name references (`[name=app]`) are preferred over indices for stability.

---

## Classification Reference

| Classification | Description |
|----------------|-------------|
| `capacity` | Scaling and resources (replicas, requests/limits) |
| `image` | Container images |
| `config` | Configuration (env vars) |
| `rollout` | Deployment behavior (imagePullPolicy) |

See `docs/reference/severity-taxonomy.md` for the full classification and severity reference.

---

## Limitations

**v0.14.4 scope:**
- Compares Deployments, StatefulSets, DaemonSets, ReplicaSets, Jobs, CronJobs
- Requires cluster access for live state
- Does not detect missing resources (desired exists, live doesn't)

**Related documentation:**
- [Debug Bundles](debug-bundle.md) — Portable snapshots for offline inspection
- [Semantic Contract](semantic-contract.md) — f(JSON) + g model
