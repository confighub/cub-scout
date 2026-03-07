# Connected Summary Storage Example

This example shows the #209 workflow: persist connected scan/sync summaries and query them by time window.

## What It Demonstrates

- Connected `scan` writes risk summary snapshots.
- Connected `gitops status` writes sync/drift summary snapshots.
- `summary list` retrieves records by `--since`, `--type`, `--cluster`, and `--namespace`.

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

## Storage Contract

- Schema version: `connected.summary.v1`
- Index dimensions: `cluster`, `scope.namespace`, `timestamp`, `type`
- Default retention: 30 days
- Retention override: `CUB_SCOUT_SUMMARY_RETENTION_DAYS`
