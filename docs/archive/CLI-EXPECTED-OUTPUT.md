# Expected Output Reference

This guide shows what to expect from cub-scout commands - both healthy and unhealthy states.

## Quick Reference

| State | Map Output | Scan Output |
|-------|------------|-------------|
| **Healthy** | `✓ ALL HEALTHY` | `✓ No Config CVEs detected` |
| **Problems** | `🔥 N FAILURE(S)` | `CRITICAL: N, WARNING: N` |
| **Suspended** | `⏸ suspended` | N/A |

---

## Map Command Output

### Healthy Cluster

```bash
$ ./test/atk/map
```

```
  ✓ ALL HEALTHY   atk

  Deployers  3/3
  Workloads  16/16

  OWNERSHIP
  ────────────────────────────────────────────────
  Argo(1) ConfigHub(2) Flux(2) Helm(1) Native(10)
  ██████░░░░░░░░░░

  PIPELINES
  ────────────────────────────────────────────────
✓ company/infrastructure@main  →  monitoring-stack  →  3 resources
✓ company/frontend/k8s@HEAD  →  frontend-app  →  demo-payments
```

**What to look for:**
- Green `✓ ALL HEALTHY` banner
- Deployers and Workloads show `N/N` (all running)
- Pipelines show `✓` prefix (healthy)

---

### Unhealthy Cluster (Problems)

```bash
$ ./test/atk/map
```

```
  🔥 5 FAILURE(S)   atk

  Deployers  0/3
  Workloads  13/16

  PROBLEMS
  ────────────────────────────────────────────────
✗ HelmRelease/redis-cache  SourceNotReady
✗ Application/frontend-app  null
⏸ Kustomization/monitoring-stack  suspended
✗ demo-orders/order-processor  0/2 pods
✗ demo-payments/frontend  0/2 pods
✗ demo-payments/payment-api  0/3 pods

  PIPELINES
  ────────────────────────────────────────────────
⏸ company/infrastructure@main  →  monitoring-stack  →  0 resources
✗ company/frontend/k8s@HEAD  →  frontend-app  →  demo-payments

  OWNERSHIP
  ────────────────────────────────────────────────
  Argo(1) ConfigHub(2) Helm(1) Native(12)
  ████░░░░░░░░░░░░
```

**Problem indicators:**
| Symbol | Meaning |
|--------|---------|
| `🔥` | Critical failures present |
| `✗` | Failed resource |
| `⏸` | Suspended/paused |
| `0/N pods` | Pod not running |
| `SourceNotReady` | Git source unavailable |
| `null` | Argo CD sync status unknown |

---

### Map Subcommands

#### `map status` - One-liner Health Check

**Healthy:**
```
✓ 3 deployers, 16 workloads, 0 problems
```

**Unhealthy:**
```
✗ 0/3 deployers, 13/16 workloads, 5 problems
```

---

#### `map problems` - Problem Details

```bash
$ ./test/atk/map problems
```

**No problems:**
```
No problems detected
```

**With problems:**
```
PROBLEMS (5)
────────────────────────────────────────────────

Deployer Problems:
  ✗ HelmRelease/redis-cache
    Status: SourceNotReady
    Message: failed to fetch source: connection refused

  ✗ Application/frontend-app
    Status: Unknown
    Message: sync status is null (Argo CD not responding)

  ⏸ Kustomization/monitoring-stack
    Status: Suspended
    Message: reconciliation paused by user

Workload Problems:
  ✗ demo-orders/order-processor
    Ready: 0/2 pods
    Message: ImagePullBackOff - registry.example.com/order:v2

  ✗ demo-payments/frontend
    Ready: 0/2 pods
    Message: CrashLoopBackOff - container exited with code 1

  ✗ demo-payments/payment-api
    Ready: 0/3 pods
    Message: Pending - insufficient memory
```

---

#### `map suspended` - Paused Resources

```bash
$ ./test/atk/map suspended
```

**None suspended:**
```
No suspended resources
```

**With suspended:**
```
SUSPENDED RESOURCES (2)
────────────────────────────────────────────────

  ⏸ Kustomization/monitoring-stack
    Suspended since: 2026-01-03T10:30:00Z
    Reason: Manual pause for maintenance

  ⏸ HelmRelease/database
    Suspended since: 2026-01-02T15:00:00Z
    Reason: spec.suspend=true
```

---

#### `map workloads` - Workload Table

```bash
$ ./test/atk/map workloads
```

```
STATUS  NAMESPACE       NAME              OWNER      MANAGED-BY
✓       demo-orders     order-processor   Flux       Kustomization/order-app
✓       demo-orders     order-queue       Native     —
✗       demo-payments   frontend          ConfigHub  Unit/frontend
✗       demo-payments   payment-api       Argo       Application/frontend-app
✓       monitoring      prometheus        Helm       Release/prometheus-stack
```

**Status indicators:**
- `✓` = Running (all pods ready)
- `✗` = Not running (pods not ready)
- `⏸` = Suspended

---

#### `map deployers` - Deployer Status

```bash
$ ./test/atk/map deployers
```

**Healthy:**
```
TYPE            NAME              STATUS    RESOURCES
Kustomization   monitoring-stack  ✓ Ready   3
Kustomization   order-app         ✓ Ready   5
HelmRelease     redis-cache       ✓ Ready   2
Application     frontend-app      ✓ Synced  4
```

**Unhealthy:**
```
TYPE            NAME              STATUS           RESOURCES
Kustomization   monitoring-stack  ⏸ Suspended      0
Kustomization   order-app         ✓ Ready          5
HelmRelease     redis-cache       ✗ SourceNotReady 0
Application     frontend-app      ✗ Unknown        0
```

---

## Scan Command Output

### Clean Scan (No Issues)

```bash
$ ./test/atk/scan
```

```
✓ No Config CVEs detected

Scanned: 16 resources
Patterns: 1,700+ risk issues
```

---

### Scan with Findings

```bash
$ ./test/atk/scan
```

```
CRITICAL  RISK-2025-0027  Grafana sidecar whitespace bug
          ConfigMap/grafana-dashboards (monitoring)
          Fix: Remove leading/trailing whitespace from dashboard JSON keys

WARNING   RISK-2025-0043  Thanos sidecar not uploading
          StatefulSet/prometheus (monitoring)
          Fix: Check objstore.yml bucket configuration

WARNING   RISK-2025-0066  SSL redirect blocking ACME
          Ingress/api-gateway (default)
          Fix: Add annotation kubernetes.io/ingress.allow-http: "true"

INFO      RISK-2025-0084  PDB allows zero available
          PodDisruptionBudget/redis-pdb (cache)
          Fix: Set minAvailable to at least 1

────────────────────────────────────────────────
Summary: 1 CRITICAL, 2 WARNING, 1 INFO

Scanned: 16 resources
Patterns: 1,700+ risk issues
```

**Severity levels:**
| Severity | Meaning | Action |
|----------|---------|--------|
| `CRITICAL` | Will cause outage | Fix immediately |
| `WARNING` | May cause issues | Fix soon |
| `INFO` | Best practice | Consider fixing |

---

### Scan JSON Output

```bash
$ ./test/atk/scan --json
```

**No findings:**
```json
{
  "findings": [],
  "summary": {
    "critical": 0,
    "warning": 0,
    "info": 0,
    "total": 0
  },
  "scanned": 16,
  "patterns": 337
}
```

**With findings:**
```json
{
  "findings": [
    {
      "id": "RISK-2025-0027",
      "severity": "critical",
      "name": "Grafana sidecar whitespace bug",
      "resource": {
        "kind": "ConfigMap",
        "name": "grafana-dashboards",
        "namespace": "monitoring"
      },
      "fix": "Remove leading/trailing whitespace from dashboard JSON keys"
    }
  ],
  "summary": {
    "critical": 1,
    "warning": 0,
    "info": 0,
    "total": 1
  },
  "scanned": 16,
  "patterns": 337
}
```

---

## Demo Command Output

### Quick Demo

```bash
$ ./test/atk/demo quick
```

```
cub-scout - Quick Demo
════════════════════════════

Creating demo resources...
  ✓ namespace/demo-quick created
  ✓ deployment/nginx created
  ✓ service/nginx created

Running map...

  ✓ ALL HEALTHY   demo-quick

  Deployers  0/0
  Workloads  1/1

  OWNERSHIP
  ────────────────────────────────────────────────
  Native(1)
  ██████████████████

Demo complete!

Cleanup: ./test/atk/demo quick --cleanup
```

---

### risk issue Demo

```bash
$ ./test/atk/demo risk
```

```
cub-scout - risk issue Demo
════════════════════════════

Creating resources with known issues...
  ✓ namespace/demo-risk created
  ✓ configmap/grafana-dashboards created (with whitespace bug)
  ✓ deployment/grafana created

Running scan...

CRITICAL  RISK-2025-0027  Grafana sidecar whitespace bug
          ConfigMap/grafana-dashboards (demo-risk)
          Fix: Remove leading/trailing whitespace from dashboard JSON keys

────────────────────────────────────────────────
Summary: 1 CRITICAL

The scanner detected a real configuration issue!

Cleanup: ./test/atk/demo risk --cleanup
```

---

### Healthy Demo

```bash
$ ./test/atk/demo healthy
```

```
cub-scout - Healthy Enterprise Demo
══════════════════════════════════════════

Creating healthy enterprise pattern...
  ✓ namespace/demo-healthy created
  ✓ gitrepository/app-source created
  ✓ kustomization/app-deploy created
  ✓ deployment/frontend created
  ✓ deployment/backend created
  ✓ service/frontend created
  ✓ service/backend created

Running map...

  ✓ ALL HEALTHY   demo-healthy

  Deployers  1/1
  Workloads  2/2

  PIPELINES
  ────────────────────────────────────────────────
✓ app-source@main  →  app-deploy  →  2 resources

  OWNERSHIP
  ────────────────────────────────────────────────
  Flux(2)
  ██████████████████

Running scan...
✓ No Config CVEs detected

This is what a healthy GitOps deployment looks like!

Cleanup: ./test/atk/demo healthy --cleanup
```

---

### Unhealthy Demo

```bash
$ ./test/atk/demo unhealthy
```

```
cub-scout - Unhealthy Demo
═════════════════════════════════

Creating resources with common problems...
  ✓ namespace/demo-unhealthy created
  ✓ gitrepository/broken-source created (invalid URL)
  ✓ kustomization/broken-deploy created
  ✓ deployment/broken-app created (image doesn't exist)

Running map...

  🔥 3 FAILURE(S)   demo-unhealthy

  Deployers  0/1
  Workloads  0/1

  PROBLEMS
  ────────────────────────────────────────────────
✗ GitRepository/broken-source  GitOperationFailed
✗ Kustomization/broken-deploy  DependencyNotReady
✗ demo-unhealthy/broken-app  0/1 pods

This shows common failure patterns:
1. Invalid Git URL → GitOperationFailed
2. Dependency cascade → DependencyNotReady
3. Bad image → ImagePullBackOff

Cleanup: ./test/atk/demo unhealthy --cleanup
```

---

## Side-by-Side Comparison

### Map Output

| Aspect | Healthy | Unhealthy |
|--------|---------|-----------|
| Banner | `✓ ALL HEALTHY` | `🔥 N FAILURE(S)` |
| Deployers | `3/3` | `0/3` |
| Workloads | `16/16` | `13/16` |
| Problems section | Not shown | Lists all failures |
| Pipeline prefix | `✓` | `✗` or `⏸` |

### Scan Output

| Aspect | Clean | Has Issues |
|--------|-------|------------|
| Header | `✓ No Config CVEs detected` | Shows findings list |
| Summary | Not shown | `N CRITICAL, N WARNING, N INFO` |
| Exit code | `0` | `1` (if CRITICAL) |

---

## Error Messages

### Common Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `error: no kubeconfig` | kubectl not configured | Run `kubectl config use-context <name>` |
| `error: cannot list resources` | RBAC missing | Apply ClusterRole from install |
| `error: CRD not found` | Flux/Argo not installed | Install GitOps tool first |
| `null` status | Argo CD API unavailable | Check Argo CD server |
| `SourceNotReady` | Git URL invalid or auth failed | Check GitRepository |

### Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success (healthy or no critical findings) |
| `1` | Failures present or critical findings |
| `2` | Configuration error |

---

## See Also

- [Testing Guide](../TESTING-GUIDE.md) - Step-by-step testing walkthrough
- [Scan Guide](../SCAN-GUIDE.md) - risk issue scanner details
- [JOURNEY-QUERY.md](JOURNEY-QUERY.md) - Fleet query scenarios
