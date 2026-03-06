# Integration Scripts

**Status: Working** — Copy-paste scripts for integrating cub-scout into your workflow.

## Connected Demo Readiness

Fail-fast checks for `--live` demo runs (workers, targets, connected workloads):

```bash
# Argo demo space
./verify-connected-demo.sh --space argo-import-demo --renderer argocdrenderer

# Flux demo space
./verify-connected-demo.sh --space flux-import-demo --renderer fluxrenderer
```

## Demo Worker Lifecycle

Manage detached discovery workers for long-running `--live --keep` demos:

```bash
# Start detached discovery worker
./demo-worker-lifecycle.sh start \
  --space argo-import-demo \
  --worker discovery-worker \
  --target Kubernetes \
  --pid-file /tmp/cub-scout-demo-workers/argo-import-demo-discovery-worker.pid \
  --log-file /tmp/cub-scout-demo-workers/argo-import-demo-discovery-worker.log

# Keep running during cleanup
./demo-worker-lifecycle.sh cleanup --pid-file /tmp/cub-scout-demo-workers/argo-import-demo-discovery-worker.pid --keep

# Stop explicitly when done
./demo-worker-lifecycle.sh stop --pid-file /tmp/cub-scout-demo-workers/argo-import-demo-discovery-worker.pid
```

## Synthetic History Seed (Demo Only)

Create tagged synthetic ChangeSets for storytelling demos.

```bash
# Dry run (default)
./seed-connected-demo-history.sh --space argo-import-demo --allow-synthetic

# Apply (explicit)
./seed-connected-demo-history.sh --space argo-import-demo --allow-synthetic --apply
```

Safety contract:
- Requires `--allow-synthetic`.
- Synthetic records are tagged with:
  - `demo=true`
  - `synthetic=true`
  - `source=cub-scout-demo-seed`
- Never run against production spaces.

## k9s Plugin

Add map/scan commands to k9s:

```yaml
# ~/.k9s/plugin.yml
plugin:
  confighub-map:
    shortCut: Shift-M
    description: Show cluster map
    command: sh
    args: ["-c", "cub-scout map"]

  confighub-scan:
    shortCut: Shift-V
    description: Scan for risk issues
    command: sh
    args: ["-c", "cub-scout scan"]

  confighub-problems:
    shortCut: Shift-P
    description: Show problems only
    command: sh
    args: ["-c", "cub-scout map issues"]
```

## Slack Alerting

Alert on drift or risk issues:

```bash
#!/bin/bash
# slack-alerting.sh

cub-scout snapshot -o - | jq -e '.entries[] | select(.drift)' > /dev/null
if [ $? -eq 0 ]; then
  DRIFTED=$(cub-scout snapshot -o - | jq '[.entries[] | select(.drift)] | length')
  curl -X POST "$SLACK_WEBHOOK" \
    -H 'Content-type: application/json' \
    -d "{\"text\": \"Warning: $DRIFTED resources drifted in cluster\"}"
fi
```

## GitHub Actions

CI/CD gate for risk issues:

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

      - name: Scan for risk issues
        run: |
          cub-scout scan --json > scan-results.json
          CRITICAL=$(jq '[.findings[] | select(.severity == "critical")] | length' scan-results.json)
          if [ "$CRITICAL" -gt 0 ]; then
            echo "Found $CRITICAL critical risk issues"
            exit 1
          fi
```

## Prometheus Metrics

Export metrics from agent:

```bash
#!/bin/bash
# prometheus-metrics.sh

# Start agent with metrics endpoint
cub-scout serve --port 9876 --metrics-port 9877

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

cub-scout snapshot -o - | jq -r '
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

cub-scout snapshot -o - | jq -r '
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

cub-scout snapshot -o - | jq -r '
  .entries[]
  | select(.drift)
  | "DRIFT: \(.namespace)/\(.kind)/\(.name) - \(.drift.summary)"
'
```
