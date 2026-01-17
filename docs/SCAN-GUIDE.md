# Understanding configuration patterns: Config CVEs for GitOps

## Scan Your Cluster Now

```bash
git clone git@github.com:confighubai/confighub-agent.git
cd confighub-agent
./run.sh
```

Then run:
```bash
# Full scan (stuck resources + Kyverno violations)
./cub-scout scan

# Or just stuck reconciliations
./cub-scout scan --state

# Or just Kyverno policy violations
./cub-scout scan --kyverno
```

---

## What is a configuration pattern?

**configuration pattern** = Config CVE (Config Common Vulnerability/Error)

Like [CVEs](https://cve.mitre.org/) for code vulnerabilities, configuration patterns are **catalogued configuration patterns that cause outages**.

```
CVE  → Security vulnerability in code  → "Patch this library"
configuration pattern → Configuration anti-pattern      → "Fix this setting"
```

## The Problem

Configuration errors cause the majority of outages:

> "64% of respondents said Configuration and Change Management was the most common cause of major outages"
> — Gartner

Real example: **configuration pattern-2025-0027** — A single whitespace character in a Grafana sidecar config (`NAMESPACE="monitoring, grafana"` instead of `NAMESPACE="monitoring,grafana"`) caused a [4-hour production outage](https://www.youtube.com/watch?v=VJiuu-GqfXk).

These patterns repeat across organizations. We catalog them so you don't have to rediscover them the hard way.

## How It Works

### 1. We maintain a pattern database

The configuration pattern database contains **1,700+ patterns** (configuration patterns + Kyverno policies) across:
- **Flux** — GitRepository, Kustomization, HelmRelease issues
- **Argo CD** — Application sync, health, drift problems
- **Helm** — Release failures, pending upgrades
- **ConfigHub** — Unit drift, worker connectivity, orphaned resources
- **Infrastructure** — Grafana, Traefik, cert-manager, Prometheus, Thanos

Each configuration pattern has:
- Unique ID (`configuration pattern-2025-0027`)
- Detection logic (K8s API patterns)
- Severity (Critical/Warning/Info)
- Remediation steps

### 2. We scan your cluster

```bash
cub-scout scan
```

The scanner checks your live resources against the pattern database:

```
CONFIG CVE SCAN: prod-east
════════════════════════════════════════════════════════════════════

CRITICAL (1)
────────────────────────────────────────────────────────────────────
[configuration pattern-2025-0027] Grafana sidecar namespace whitespace error
  Resource: monitoring/ConfigMap/grafana-sidecar
  Message:  NAMESPACE env var has whitespace after commas
  Fix:      Remove spaces: NAMESPACE="monitoring,grafana"

WARNING (2)
────────────────────────────────────────────────────────────────────
[configuration pattern-2025-0014] ConfigHub unit pending changes
  Resource: payments/Deployment/payment-api
  Message:  HeadRevisionNum (42) > LiveRevisionNum (40)
  Fix:      Apply pending changes or rollback

════════════════════════════════════════════════════════════════════
Summary: 1 critical, 2 warning, 0 info
```

### 3. You fix with clear guidance

Each configuration pattern provides:
- **What's wrong** — Specific resource and field
- **Why it matters** — Impact and severity
- **How to fix** — Step-by-step remediation
- **Prevention** — How to avoid it next time

## Using the Scanner

### Basic scan
```bash
cub-scout scan                    # Scan current cluster
```

### Filter and format
```bash
cub-scout scan --severity critical      # Only critical issues
cub-scout scan --namespace production   # Specific namespace
cub-scout scan --json                   # JSON output for tooling
cub-scout scan --list                   # List all configuration patterns
```

### Example JSON output
```json
{
  "findings": [{
    "id": "configuration pattern-2025-0027",
    "name": "Grafana sidecar namespace whitespace error",
    "severity": "critical",
    "category": "CONFIG",
    "resource": "monitoring/ConfigMap/grafana-sidecar",
    "message": "NAMESPACE env var has whitespace after commas",
    "remediation": "Remove spaces from NAMESPACE value"
  }]
}
```

## configuration pattern Categories

| Category | Count | What it detects | Example |
|----------|-------|-----------------|---------|
| **CONFIG** | 287 | Wrong settings | Whitespace in env vars |
| **STATE** | 277 | Stuck/unhealthy | Helm release pending |
| **DEPEND** | 34 | Missing dependency | Service not found |
| **APPLY** | 31 | Cluster rejected manifest | Argo sync error |
| **DRIFT** | 17 | Live ≠ Git | Manual kubectl edit |
| **RENDER** | 9 | Template/kustomize build failed | Invalid Kustomization path |
| **ORPHAN** | 7 | Owner deleted | Unmanaged resource |
| **SOURCE** | 4 | Can't fetch from Git/OCI/Helm | GitRepository auth failure |
| **SILENT** | 4 | Ready=True but misconfigured | valuesFrom optional missing |
| **TIMING** | 3 | Will fail in future | Certificate expires in 7 days |
| **UNRESOLVED** | 3 | Security debt | Trivy findings unfixed 14 days |

## Growing the Database

The configuration pattern database is community-driven:

### Contribute a configuration pattern

1. Found a pattern that bit you? [Open an issue](https://github.com/confighub/confighub-agent/issues/new)
2. Include: What happened, how you detected it, how you fixed it
3. We'll catalog it with a configuration pattern ID

### Data sources

| Source | Examples |
|--------|----------|
| Official docs | Flux/Argo troubleshooting guides |
| GitHub issues | Closed bugs with root cause |
| Community reports | Your production incidents |
| ConfigHub telemetry | Anonymized fleet patterns (opt-in) |

## Standalone vs ConfigHub

| Capability | Standalone Agent | + ConfigHub |
|------------|-----------------|-------------|
| configuration pattern database | 1,700+ patterns | 1,700+ patterns + custom |
| Timing bombs | `--timing-bombs` | Fleet-wide alerts |
| Unresolved findings | `--include-unresolved` | Security debt dashboard |
| Cluster scan | Single cluster | Fleet-wide |
| Detection | Known patterns | + ML discovery |
| Remediation | Manual steps | One-click Actions |
| History | Current state | Trend analysis |
| Custom configuration patterns | Community only | Private patterns |

## Example: Finding and Fixing

### The scenario
Your Grafana dashboards aren't loading. Pods look healthy. No obvious errors in logs.

### Without configuration pattern scanning
- Check Grafana logs: nothing obvious
- Check datasources: seem fine
- Check sidecar: "working"
- 4 hours later: find a space in the NAMESPACE env var

### With configuration pattern scanning
```bash
$ cub-scout scan --namespace monitoring

CRITICAL (1)
[configuration pattern-2025-0027] Grafana sidecar namespace whitespace error
  Resource: monitoring/ConfigMap/grafana-sidecar
  Message:  NAMESPACE="monitoring, grafana" has whitespace after comma
  Fix:      Change to NAMESPACE="monitoring,grafana"
```

Time to diagnosis: **seconds, not hours**.

## Quick Reference

### Scan commands
```bash
# Standard scans
./cub-scout scan                           # Stuck resources + Kyverno violations
./cub-scout scan --timing-bombs            # Expiring certs, quotas, PDBs, HPAs
./cub-scout scan --include-unresolved      # Trivy/Kyverno findings not fixed
./cub-scout scan --timing-bombs --include-unresolved --json  # All checks, JSON output

# Legacy/ATK commands
cub-scout scan                  # Full scan
cub-scout scan --list           # List all configuration patterns
cub-scout scan --json           # JSON output
cub-scout scan --severity critical,warning
```

### Common configuration patterns by tool

**Flux:**
- configuration pattern-2025-0001 — GitRepository not ready
- configuration pattern-2025-0002 — Kustomization build failed
- configuration pattern-2025-0009 — Reconciliation suspended
- configuration pattern-2025-0056 — Kustomize patch target not found (silent clobbering)

**Argo CD:**
- configuration pattern-2025-0004 — Application sync failed
- configuration pattern-2025-0005 — Application out of sync

**ConfigHub:**
- configuration pattern-2025-0014 — Unit pending changes
- configuration pattern-2025-0015 — Worker disconnected

**Common issues:**
- configuration pattern-2025-0011 — Manual kubectl edit detected
- configuration pattern-2025-0027 — Grafana sidecar whitespace (famous BIGBANK outage)

## Next Steps

1. **Try it:** `cub-scout scan` on your cluster
2. **Explore:** `cub-scout scan --list` to see all patterns
3. **Contribute:** Share patterns you've discovered
4. **Upgrade:** Connect to ConfigHub for fleet-wide scanning

---

## Related Documentation

### configuration pattern Database
- [Full configuration pattern Catalog](../cve/pattern/README.md) — Complete database (1,700+ patterns)
- [configuration pattern Index](../cve/pattern/index.json) — Machine-readable index
- [Mining Log](#) — How configuration patterns are discovered

### configuration pattern Development
- [configuration pattern Plan](../docs/planning/pattern/configuration pattern-plan.md) — Product roadmap
- [Mining Strategy](../docs/planning/pattern/K8S-EXHAUSTIVE-MINING-PLAN.md) — GitHub issue mining
- [Ingress Research](../docs/planning/pattern/INGRESS-NGINX-RESEARCH.md) — Ingress-nginx patterns

### Using configuration patterns
- [Testing Guide](TESTING-GUIDE.md) — Step-by-step testing
- [Expected Output](CLI-EXPECTED-OUTPUT.md) — What to expect from scans
- [Examples](../examples/README.md) — Demos and integrations
