# Scan: Find Stuck Resources and Misconfigurations

Scan your cluster for stuck reconciliation states and Kyverno policy violations. Find issues before they cause outages.

**Two scan modes:**
1. **State scan** — Detect stuck HelmReleases, Kustomizations, and Applications
2. **Kyverno scan** — Map policy violations to our KPOL database

## Quick Start

```bash
# Full scan (state + Kyverno)
cub-agent scan

# State scan only (stuck reconciliations)
cub-agent scan --state

# Kyverno scan only (policy violations)
cub-agent scan --kyverno

# Scan specific namespace
cub-agent scan -n production

# JSON output for scripting
cub-agent scan --json

# List all KPOL policies in database
cub-agent scan --list
```

## What Scan Shows

### State Scan (Stuck Detection)

Detects stuck GitOps reconciliation loops:

1. **Stuck HelmReleases** — Ready=False or Stalled for >5 minutes
2. **Stuck Kustomizations** — BuildFailed, ArtifactFailed, etc.
3. **Stuck Applications** — Degraded, OutOfSync, sync stuck
4. **Copy-paste remediation commands** — One-liner fixes

### Kyverno Scan (Policy Violations)

Reads Kyverno PolicyReports and maps to our KPOL database:

1. **Policy violations** — What rules are being violated
2. **Severity levels** — Critical, Warning, Info
3. **KPOL mapping** — Link to our documented policy patterns
4. **Affected resources** — Which resources need attention

## Color Coding

| Color | Severity | Meaning |
|-------|----------|---------|
| **Red** | Critical | Security risk or major misconfiguration |
| **Yellow** | Warning | Best practice violation |
| **Dim** | Info | Informational finding |

## Example Output

### Stuck Resources Found

```
STUCK RECONCILIATION SCAN
Scanned at: 2026-01-09 14:30:00

CRITICAL (1)
────────────────────────────────────────────────────────────────────
[C] HelmRelease/redis-cluster [CCVE-2025-0166]
  Namespace: production
  Condition: Ready=False (2h30m)
  Reason: UpgradeFailed
  Message: Helm upgrade failed: timed out waiting for condition
  → Remediation: Check Helm release history; rollback if needed; verify chart values
  FIX: flux suspend hr redis-cluster -n production && flux resume hr redis-cluster -n production

WARNING (2)
────────────────────────────────────────────────────────────────────
[W] Kustomization/monitoring [CCVE-2025-0012]
  Namespace: flux-system
  Condition: Ready=False (15m)
  Reason: BuildFailed
  Message: kustomization.yaml not found in ./overlays/prod
  → Remediation: Check kustomization.yaml syntax; verify paths exist in source
  FIX: flux reconcile ks monitoring -n flux-system --with-source

[W] Application/frontend [CCVE-2025-0169]
  Namespace: argocd
  Condition: health=Degraded, sync=OutOfSync
  Reason: Degraded
  Message: Application unhealthy or out of sync for 18m
  → Remediation: Resources unhealthy; check pod status and logs
  FIX: argocd app sync frontend --force

════════════════════════════════════════════════════════════════════
State Summary: 1 HelmRelease, 1 Kustomization, 1 Application stuck
```

### Policy Violations Found

```
KYVERNO POLICY SCAN
Scanned at: 2026-01-08 14:23:45

CRITICAL (2)
────────────────────────────────────────────────────────────────────
[C] disallow-privileged[KPOL-0042]
  Resource: prod/debug-pod
  Message:  Privileged container not allowed

[C] require-run-as-non-root[KPOL-0015]
  Resource: prod/legacy-app
  Message:  Container must run as non-root

WARNING (3)
────────────────────────────────────────────────────────────────────
[W] require-labels[KPOL-0001]
  Resource: default/nginx
  Message:  Missing required label 'team'

[W] require-resources[KPOL-0023]
  Resource: default/api
  Message:  Resource limits not set

[W] require-probes[KPOL-0031]
  Resource: default/worker
  Message:  Liveness probe not configured

════════════════════════════════════════════════════════════════════
Summary: 2 critical, 3 warning, 0 info

🔗 Track violations in ConfigHub: cub-agent scan --confighub
```

### No Violations (Healthy)

```
KYVERNO POLICY SCAN
Scanned at: 2026-01-08 14:23:45

✓ No policy violations found
```

### Kyverno Not Installed

```
⚠ Kyverno not installed
  PolicyReport CRD not found in cluster.
  Install Kyverno: https://kyverno.io/docs/installation/
```

## CLI Options

```
cub-agent scan [flags]

Flags:
  -n, --namespace string   Namespace to scan (default: all namespaces)
      --state              State scan only (stuck reconciliations)
      --kyverno            Kyverno scan only (PolicyReports)
      --json               Output as JSON (for scripting)
      --list               List all KPOL policies in database
      --verbose            Show detailed output including rules
  -h, --help               Help for scan
```

### Scan Mode Combinations

| Flags | What it scans |
|-------|---------------|
| (none) | Both state + Kyverno |
| `--state` | Only stuck resources |
| `--kyverno` | Only Kyverno violations |
| `--state --kyverno` | Both (same as default) |

## TUI Integration

### Interactive Scan (`c` key)

Press `c` in the TUI dashboard to run a Kyverno policy scan:

1. Shows current cluster context
2. Runs scan across all namespaces (or filtered namespace)
3. Displays findings grouped by severity
4. Press any key to return to dashboard

### Command Line

```bash
# Run scan directly
./test/atk/map scan

# Or
./test/atk/map ccve
```

## KPOL Policy Database

We maintain a database of 460+ Kyverno policy patterns (KPOL-*) in `cve/ccve/kyverno/`.

### List All Policies

```bash
cub-agent scan --list
```

```
KYVERNO POLICY CATALOG
460 policies available

ID           SEV      NAME                                          CATEGORY
----         ---      ----                                          --------
KPOL-0001    warning  Application Field Validation                  Best Practices
KPOL-0002    warning  Add Certificates as a Volume                  Best Practices
KPOL-0003    warning  Add Default Resources                         Best Practices
...
KPOL-0042    critical Disallow Privileged Containers                Pod Security
KPOL-0043    critical Disallow Host Namespaces                      Pod Security
...
```

### Policy Categories

| Category | What it covers |
|----------|----------------|
| **Pod Security** | Privileged containers, host access, capabilities |
| **Best Practices** | Labels, resources, probes, annotations |
| **Security** | Image policies, network policies, RBAC |
| **Multi-Tenancy** | Namespace isolation, quotas |
| **Other** | Custom policies, compliance checks |

## JSON Output

```bash
cub-agent scan --json
```

```json
{
  "clusterName": "prod-east",
  "scannedAt": "2026-01-08T14:23:45Z",
  "summary": {
    "critical": 2,
    "warning": 3,
    "info": 0
  },
  "findings": [
    {
      "id": "disallow-privileged/check-privileged",
      "policyId": "KPOL-0042",
      "policyName": "disallow-privileged",
      "category": "Pod Security",
      "severity": "critical",
      "resource": "Pod/debug-pod",
      "namespace": "prod",
      "message": "Privileged container not allowed",
      "result": "fail",
      "rule": "check-privileged"
    }
  ]
}
```

## Use Cases

### 1. "Are we following security best practices?"

```bash
cub-agent scan | grep -i critical
```

Quickly see if any pods are running privileged or violating security policies.

### 2. "What's missing from our deployments?"

```bash
cub-agent scan --verbose
```

Find deployments missing resource limits, probes, or required labels.

### 3. "CI/CD Gate"

```bash
# Fail pipeline if critical violations exist
cub-agent scan --json | jq -e '.summary.critical == 0'
```

### 4. "Namespace Audit"

```bash
cub-agent scan -n production --json > prod-audit.json
```

Generate audit reports for specific namespaces.

## Requirements

- **Kyverno** installed in cluster with PolicyReports enabled
- PolicyReport CRD (`wgpolicyk8s.io/v1alpha2`)

Install Kyverno:
```bash
kubectl create -f https://github.com/kyverno/kyverno/releases/download/v1.11.0/install.yaml
```

## ConfigHub Integration

When resources have ConfigHub labels, scan findings include ConfigHub context:

```json
{
  "id": "require-resources/check-limits",
  "resource": "Deployment/payment-api",
  "confighub": {
    "unitSlug": "payment-api",
    "spaceName": "production",
    "spaceId": "space_abc123"
  }
}
```

This allows tracking violations back to ConfigHub units for fleet-wide remediation.

## Related

- [CCVE-GUIDE.md](CCVE-GUIDE.md) — Full CCVE scanning (1,700+ patterns)
- [TUI-TRACE.md](TUI-TRACE.md) — Trace resource ownership chains
- [README.md](../README.md) — Main documentation
- [cve/ccve/kyverno/](../cve/ccve/kyverno/) — KPOL policy database
