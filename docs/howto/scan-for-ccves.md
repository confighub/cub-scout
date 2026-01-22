# How To: Scan for Configuration Vulnerabilities (CCVEs)

CCVE scanning detects configuration anti-patterns before they cause production incidents. This guide shows how to use the scanner.

## The Problem

Configuration mistakes hide in your cluster:
- Grafana sidecar with whitespace in namespace annotation (CCVE-2025-0027)
- Traefik IngressRoute pointing to non-existent service
- Missing cert-manager Issuer reference
- Resource limits missing on critical workloads

**Question:** What configuration issues exist in my cluster?

## The Solution

### CLI: Run scan

```bash
cub-scout scan
```

Output:
```
CCVE-2025-0027  Grafana Namespace Whitespace    deploy/grafana      prod     HIGH
CCVE-2025-0028  Service Reference Missing       ingressroute/api    web      MEDIUM
CCVE-2025-0034  Issuer Not Found               cert/api-tls        web      HIGH

Found 3 issues (2 HIGH, 1 MEDIUM)
```

### TUI: Press 'S'

In the interactive TUI, press `S` to run a scan. Results appear in the details pane.

## Understanding CCVEs

### What is a CCVE?

CCVE = **C**onfig**C**VE (Configuration Common Vulnerabilities and Exposures)

Like CVEs for security vulnerabilities, CCVEs identify common configuration problems.

### Format

```
CCVE-2025-XXXX
     │    │
     │    └── Sequential number
     └── Year discovered
```

### Categories

| Category | Meaning |
|----------|---------|
| SOURCE | Git repository issues |
| RENDER | Template rendering problems |
| APPLY | Deployment/sync issues |
| DRIFT | Configuration drift |
| DEPEND | Missing dependencies |
| STATE | Resource state problems |
| ORPHAN | Unmanaged resources |
| CONFIG | Configuration anti-patterns |

## Example CCVEs

### CCVE-2025-0027: Grafana Namespace Whitespace

**The BIGBANK incident:** 4-hour outage caused by a trailing space in a Grafana sidecar annotation.

```yaml
# BAD - trailing whitespace
annotations:
  k8s-sidecar-target-namespace: "monitoring "  # <-- space at end

# GOOD
annotations:
  k8s-sidecar-target-namespace: "monitoring"
```

**Scan detects:** Whitespace in namespace-related annotations.

### CCVE-2025-0028: Service Reference Missing

IngressRoute or Ingress pointing to a service that doesn't exist.

```yaml
# IngressRoute references "api-service"
routes:
  - match: Host(`api.example.com`)
    services:
      - name: api-service  # Does this exist?
        port: 80
```

**Scan detects:** Cross-reference validation between routes and services.

### CCVE-2025-0034: Issuer Not Found

Certificate references an Issuer that doesn't exist.

```yaml
# Certificate references "letsencrypt-prod"
spec:
  issuerRef:
    name: letsencrypt-prod  # Is this Issuer created?
    kind: ClusterIssuer
```

**Scan detects:** Missing Issuer/ClusterIssuer references.

## Scanning Options

### Scan entire cluster

```bash
cub-scout scan
```

### Scan specific namespace

```bash
cub-scout scan -n production
```

### Scan specific file (static analysis)

```bash
cub-scout scan --file ./manifests/deployment.yaml
```

### JSON output for CI/CD

```bash
cub-scout scan --json
```

```json
{
  "issues": [
    {
      "ccve": "CCVE-2025-0027",
      "severity": "HIGH",
      "resource": "deploy/grafana",
      "namespace": "monitoring",
      "message": "Trailing whitespace in namespace annotation"
    }
  ],
  "summary": {
    "total": 1,
    "high": 1,
    "medium": 0,
    "low": 0
  }
}
```

## Coverage

The scanner includes:
- **46 active patterns** for common misconfigurations
- **4,500+ reference database** for research
- **79% xBOW benchmark accuracy**

## Integrating with CI/CD

### Fail on HIGH issues

```bash
cub-scout scan --exit-code
# Returns non-zero if HIGH severity issues found
```

### In GitHub Actions

```yaml
- name: Scan for CCVEs
  run: |
    ./cub-scout scan --json > ccve-results.json
    if jq -e '.summary.high > 0' ccve-results.json; then
      echo "HIGH severity CCVEs found!"
      exit 1
    fi
```

## Try It

```bash
# Scan your cluster
cub-scout scan

# Scan a specific namespace
cub-scout scan -n production

# JSON output for CI/CD integration
cub-scout scan --json
```

## Next Steps

- [Scan Guide](../../SCAN-GUIDE.md) - Full CCVE scanning documentation
- [Query Resources](query-resources.md) - Filter before scanning
