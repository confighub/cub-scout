# Integration Scripts

**Status: Working** — Copy-paste scripts for integrating cub-scout into your workflow.

> **Maintainer note:** When updating this file, also update [docs/EXAMPLES-OVERVIEW.md](../../docs/EXAMPLES-OVERVIEW.md).

## k9s Plugin

Add map/scan commands to k9s:

```yaml
# ~/.k9s/plugin.yml
plugin:
  confighub-map:
    shortCut: Shift-M
    description: Show cluster map
    command: sh
    args: ["-c", "./test/atk/map"]

  confighub-scan:
    shortCut: Shift-V
    description: Scan for CCVEs
    command: sh
    args: ["-c", "./test/atk/scan"]

  confighub-problems:
    shortCut: Shift-P
    description: Show problems only
    command: sh
    args: ["-c", "./test/atk/map problems"]
```

## Slack Alerting

Alert on drift or CCVEs:

```bash
#!/bin/bash
# slack-alerting.sh

./cub-scout snapshot -o - | jq -e '.entries[] | select(.drift)' > /dev/null
if [ $? -eq 0 ]; then
  DRIFTED=$(./cub-scout snapshot -o - | jq '[.entries[] | select(.drift)] | length')
  curl -X POST "$SLACK_WEBHOOK" \
    -H 'Content-type: application/json' \
    -d "{\"text\": \"Warning: $DRIFTED resources drifted in cluster\"}"
fi
```

## GitHub Actions

CI/CD gate for CCVEs:

```yaml
# .github/workflows/check-cluster.yml
name: Check Cluster Health

on:
  schedule:
    - cron: '0 * * * *'  # hourly

jobs:
  scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Scan for CCVEs
        run: |
          ./test/atk/scan --json > scan-results.json
          CRITICAL=$(jq '[.findings[] | select(.severity == "critical")] | length' scan-results.json)
          if [ "$CRITICAL" -gt 0 ]; then
            echo "Found $CRITICAL critical CCVEs"
            exit 1
          fi
```

## Prometheus Metrics

Export metrics from agent:

```bash
#!/bin/bash
# prometheus-metrics.sh

# Start agent with metrics endpoint
./cub-scout serve --port 9876 --metrics-port 9877

# Metrics exposed at localhost:9877/metrics:
# gsf_entries_total{cluster="prod",kind="Deployment",owner="flux"} 45
# gsf_entries_drifted{cluster="prod"} 3
# gsf_entries_unowned{cluster="prod"} 12
```

## Image Audit

Find all image versions across cluster:

```bash
#!/bin/bash
# audit-images.sh

./cub-scout snapshot -o - | jq -r '
  .entries[]
  | select(.kind == "Deployment")
  | "\(.namespace)/\(.name): \(.state.image // "unknown")"
' | sort
```

Output:
```
cache/redis: redis:7.2.1
monitoring/grafana: grafana/grafana:10.2.3
prod/backend: myapp:v1.2.3
staging/backend: myapp:v1.2.2  # <- older version
```

## Find Orphans

Resources with no GitOps owner:

```bash
#!/bin/bash
# find-orphans.sh

./cub-scout snapshot -o - | jq -r '
  .entries[]
  | select(.owner == null or .owner.type == "unknown")
  | "\(.namespace)/\(.kind)/\(.name)"
'
```

## Drift Report

Generate drift report:

```bash
#!/bin/bash
# drift-report.sh

./cub-scout snapshot -o - | jq -r '
  .entries[]
  | select(.drift)
  | "DRIFT: \(.namespace)/\(.kind)/\(.name) - \(.drift.summary)"
'
```
