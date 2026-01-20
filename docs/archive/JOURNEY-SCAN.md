# Journey: Scanning for Issues

**Time:** 5 minutes
**Goal:** Find configuration anti-patterns (CCVEs) in your cluster

**Prerequisites:** Have a Kubernetes cluster running.

---

## What is Scanning?

Scanning detects **CCVEs** (Cloud Configuration Vulnerabilities and Exposures) — configuration anti-patterns that cause real problems.

Examples:
- Missing resource limits
- Orphan resources (no GitOps owner)
- Deprecated API versions
- Stuck reconciliation loops
- Drift between Git and cluster

---

## Step 1: Run a Scan

```bash
./test/atk/scan
```

**Expected output:**

```
┌─ ⚡ SCAN ────────────────────────────────────────────────────────────────────┐
│                                                                              │
│  Cluster: kind-atk                                                           │
│  Scanned: 47 resources                                                       │
│  Findings: 12 CCVEs                                                          │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

┌─ FINDINGS BY SEVERITY ───────────────────────────────────────────────────────┐
│                                                                              │
│  ████ Critical   2                                                           │
│  ████████ High   4                                                           │
│  ████████████ Medium   6                                                     │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

┌─ TOP ISSUES ─────────────────────────────────────────────────────────────────┐
│                                                                              │
│  CRITICAL                                                                    │
│  ✗ CCVE-2025-0027  Grafana sidecar with whitespace in data sources          │
│    payments-prod/grafana                                                     │
│                                                                              │
│  ✗ CCVE-2025-0089  Flux HelmRelease stuck in pending-upgrade                │
│    payments-prod/redis                                                       │
│                                                                              │
│  HIGH                                                                        │
│  ⚠ CCVE-2025-0001  Deployment missing resource limits                       │
│    default/mystery-app                                                       │
│                                                                              │
│  ⚠ CCVE-2025-0042  Orphan resource - no GitOps owner                        │
│    default/legacy-service                                                    │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

Press [Enter] on a finding for details, [q] to quit
```

---

## Step 2: View Finding Details

Press **Enter** on a CCVE to see details:

```
┌─ CCVE-2025-0027: Grafana sidecar whitespace bug ─────────────────────────────┐
│                                                                              │
│  Severity: Critical                                                          │
│  Category: CONFIG                                                            │
│                                                                              │
│  Affected:                                                                   │
│    Deployment: payments-prod/grafana                                         │
│                                                                              │
│  Problem:                                                                    │
│    Grafana sidecar with LABEL_VALUE containing whitespace causes            │
│    dashboards to fail silently. The sidecar matches ConfigMaps by           │
│    label but whitespace in values breaks the match.                         │
│                                                                              │
│  Detection:                                                                  │
│    Found LABEL_VALUE=" dashboard" (leading space)                           │
│                                                                              │
│  Remediation:                                                                │
│    Remove whitespace from LABEL_VALUE environment variable:                 │
│                                                                              │
│    - name: LABEL_VALUE                                                       │
│      value: "dashboard"     # Was: " dashboard"                              │
│                                                                              │
│  References:                                                                 │
│    - https://github.com/grafana/grafana/issues/12345                        │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

Press [Esc] to go back, [f] to fix (if remediation available)
```

---

## Step 3: Filter by Category

CCVEs are categorized:

| Category | What It Detects |
|----------|-----------------|
| **ORPHAN** | Resources with no GitOps owner |
| **DRIFT** | Live state differs from Git |
| **CONFIG** | Misconfiguration (limits, probes, etc.) |
| **STATE** | Controller stuck/failed |
| **DEPEND** | Missing dependencies |
| **SOURCE** | Git/repo issues |
| **RENDER** | Template/overlay problems |
| **APPLY** | Deployment failures |

Filter by category:

```bash
./test/atk/scan -q "category=ORPHAN"
./test/atk/scan -q "category=DRIFT"
./test/atk/scan -q "category=CONFIG"
```

---

## Step 4: Filter by Severity

```bash
# Only critical and high
./test/atk/scan -q "severity=Critical OR severity=High"

# Exclude low severity
./test/atk/scan -q "severity!=Low"
```

---

## Step 5: Scan Specific Namespaces

```bash
# Single namespace
./test/atk/scan -n payments-prod

# Multiple namespaces
./test/atk/scan -n payments-prod,payments-staging

# Exclude system namespaces
./test/atk/scan --exclude-ns "kube-system,flux-system"
```

---

## Step 6: JSON Output for CI/CD

```bash
./test/atk/scan --json > scan-results.json
```

**Example JSON:**

```json
{
  "cluster": "kind-atk",
  "scanned_at": "2026-01-09T15:30:00Z",
  "total_resources": 47,
  "findings": [
    {
      "ccve_id": "CCVE-2025-0027",
      "severity": "Critical",
      "category": "CONFIG",
      "resource": {
        "kind": "Deployment",
        "namespace": "payments-prod",
        "name": "grafana"
      },
      "message": "Grafana sidecar with whitespace in LABEL_VALUE",
      "remediation": "Remove whitespace from LABEL_VALUE environment variable"
    }
  ],
  "summary": {
    "critical": 2,
    "high": 4,
    "medium": 6,
    "low": 0
  }
}
```

Use in CI/CD:

```bash
# Fail if any critical findings
./test/atk/scan --json | jq -e '.summary.critical == 0'
```

---

## Step 7: Common CCVEs

### CCVE-2025-0001: Missing Resource Limits

```yaml
# Problem: No limits set
containers:
- name: app
  image: myapp:latest
  # No resources section

# Fix: Add resource limits
containers:
- name: app
  image: myapp:latest
  resources:
    requests:
      memory: "64Mi"
      cpu: "250m"
    limits:
      memory: "128Mi"
      cpu: "500m"
```

### CCVE-2025-0042: Orphan Resource

```bash
# Problem: Resource created via kubectl, not tracked by GitOps
kubectl get deploy mystery-app -o yaml | grep -A5 labels
# No flux or argocd labels

# Fix: Add to Git and let GitOps manage it
# Or delete if no longer needed
```

### CCVE-2025-0089: Flux HelmRelease Stuck

```bash
# Problem: HelmRelease stuck in pending-upgrade
kubectl get helmrelease -A

# Fix: Check helm history, possibly rollback
helm history <release> -n <namespace>
helm rollback <release> <revision> -n <namespace>
```

---

## CCVE Database

cub-scout includes **46 active scanner patterns** plus **4,500+ reference patterns** covering:

- Kubernetes core
- Flux CD
- Argo CD
- Helm
- Prometheus
- cert-manager
- Traefik
- And more...

The CCVE database is maintained in [confighubai/confighub-scan](https://github.com/confighubai/confighub-scan).

---

## Scan vs Map

| Feature | Map | Scan |
|---------|-----|------|
| Shows resources | ✓ | ✓ (affected only) |
| Shows ownership | ✓ | — |
| Detects anti-patterns | — | ✓ |
| Provides remediation | — | ✓ |
| Severity levels | — | ✓ |

Use Map to **see** your cluster. Use Scan to **validate** it.

---

## Next Steps

| Journey | What You'll Learn |
|---------|-------------------|
| [**JOURNEY-QUERY.md**](JOURNEY-QUERY.md) | Query across fleet |
| [**SCAN-GUIDE.md**](../SCAN-GUIDE.md) | Full CCVE reference |
| [**EXTENDING.md**](../EXTENDING.md) | Add custom CCVEs |

---

## Troubleshooting

### "No findings"

Your cluster is healthy! Or check if resources exist:
```bash
kubectl get deploy,sts,ds --all-namespaces
```

### "Too many findings"

Filter to actionable items:
```bash
./test/atk/scan -q "severity=Critical OR severity=High"
```

### "Unknown CCVE"

Update the CCVE database:
```bash
git pull  # Get latest CCVE definitions
```

---

**Previous:** [JOURNEY-MAP.md](JOURNEY-MAP.md) — Navigate the map | **Next:** [JOURNEY-QUERY.md](JOURNEY-QUERY.md) — Query across fleet

---

## See Also

- [Scan Guide](../SCAN-GUIDE.md) — Full CCVE scanning documentation
- [TUI-SCAN.md](TUI-SCAN.md) — Kyverno policy scanning
- [CLI Guide](../../CLI-GUIDE.md) — Full command reference
