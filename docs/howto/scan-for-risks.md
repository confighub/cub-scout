# Risk Scanning: Find Configuration Problems Early

Risk scanning helps teams catch misconfiguration patterns before they become outages.

## Quick Start

```bash
# Build locally
go build ./cmd/cub-scout

# Scan current cluster context
./cub-scout scan
```

Common variants:

```bash
# State-only checks (stuck reconciliations + runtime pod failures)
./cub-scout scan --state

# Kyverno-only checks
./cub-scout scan --kyverno

# JSON output for CI pipelines
./cub-scout scan --json
```

## What the Scanner Detects

The scanner evaluates live cluster state against a maintained risk-pattern catalog. Typical findings include:

- stuck GitOps reconciliation
- pod runtime failures (ImagePullBackOff, ErrImagePull, CrashLoopBackOff, Pending, Evicted)
- invalid or missing dependencies
- drift and ownership gaps
- fragile lifecycle sequencing patterns
- common configuration anti-patterns learned from incidents

## Example Output

```text
CONFIG RISK SCAN: prod-east
════════════════════════════════════════════════════════════════════

CRITICAL (1)
────────────────────────────────────────────────────────────────────
[RISK-2025-0027] Grafana sidecar namespace whitespace error
  Resource: monitoring/ConfigMap/grafana-sidecar
  Message:  NAMESPACE has whitespace after commas
  Fix:      Use NAMESPACE="monitoring,grafana"

WARNING (2)
────────────────────────────────────────────────────────────────────
[RISK-2025-0014] Unit pending changes
  Resource: payments/Deployment/payment-api
  Message:  Desired revision is ahead of live revision
  Fix:      Apply pending changes or rollback

════════════════════════════════════════════════════════════════════
Summary: 1 critical, 2 warning, 0 info
```

## Useful Flags

```bash
# Scope
./cub-scout scan --namespace production
./cub-scout scan --exclude-ns kube-system,flux-system

# Severity filter
./cub-scout scan --severity critical,warning

# Catalog and machine output
./cub-scout scan --list
./cub-scout scan --json

# Optional packs
./cub-scout scan --timing-bombs
./cub-scout scan --include-unresolved
./cub-scout scan --lifecycle-hazards
```

## Risk Categories

The catalog spans categories such as:

- `SOURCE` (fetch/auth/source readiness)
- `RENDER` (template/render failures)
- `APPLY` (apply/sync failures)
- `DRIFT` (live differs from desired)
- `DEPEND` (missing service/secret/issuer/target)
- `STATE` (stuck or unhealthy runtime state)
- `ORPHAN` (unmanaged resources)
- `TIMING` (future-failure signals)

## Typical Workflow

1. Run `./cub-scout scan` on the affected cluster.
2. Triage `critical` findings first.
3. Use the specific resource path from each finding to inspect root cause.
4. Apply the suggested fix.
5. Re-run the scan and verify the summary is clean.

## Standalone vs Connected

| Capability | Standalone | Connected |
|---|---|---|
| Single-cluster risk scan | yes | yes |
| Fleet-wide risk visibility | no | yes |
| Trend/history across environments | no | yes |
| Shared governance context | no | yes |

## Next Steps

- Run demos: `./cub-scout demo --help`
- Explore command details: `./CLI-GUIDE.md`
- Review testing coverage: `docs/testing/README.md`
