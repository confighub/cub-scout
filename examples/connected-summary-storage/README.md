# Connected Summary Storage Example

This example shows the #209 workflow: persist connected scan/sync summaries and query them by time window.

## What It Demonstrates

- Connected `scan` writes risk summary snapshots.
- Connected `gitops status` writes sync/drift summary snapshots.
- `summary list` retrieves records by `--since`, `--type`, `--cluster`, and `--namespace`.
- JSON output carries `observation` metadata so agents can tell the answer came
  from stored summaries, not a fresh Kubernetes API read.

## Quick Run

```bash
# Optional: isolate storage for a demo run
export CUB_SCOUT_SUMMARY_DIR="$(mktemp -d)/cub-scout-summaries"

# Generate connected summaries
cub-scout scan -n prod
cub-scout gitops status -n prod

# Query the last 24h
cub-scout summary list --since 24h

# JSON for automation
cub-scout summary list --since 24h --json
```

See [`summary-list.json`](./summary-list.json) for the JSON shape.

```bash

# Build/post Slack digest (webhook required)
cub-scout summary slack --webhook-url https://hooks.slack.com/services/... --since 24h

# Preview Slack payload only
cub-scout summary slack --dry-run --since 24h
```

## Storage Contract

- Schema version: `connected.summary.v1`
- Index dimensions: `cluster`, `scope.namespace`, `timestamp`, `type`
- Observation source: `summary-store`
- Observation mode: `summary-list`
- Default retention: 30 days
- Retention override: `CUB_SCOUT_SUMMARY_RETENTION_DAYS`

## End-to-End Demo Script

```bash
./examples/connected-summary-storage/slack-demo.sh
```

The script posts only when `CUB_SCOUT_SLACK_WEBHOOK_URL` is set, and always prints
the generated payload in dry-run mode first.
