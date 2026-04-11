# AI Start Here

Use this page when you want to drive `fleet-import` safely with
Codex, Claude, Cursor, or another AI assistant.

## What This Example Is For

This example shows multi-cluster aggregation: merging per-cluster import
JSON from two clusters (dev + prod) into a unified fleet view and App
proposal using `cub-scout import cluster-aggregator`.

This example is deterministic and does not require a live cluster.
All data comes from pre-generated JSON fixtures.

## Read-Only First

Everything in this example is read-only. No commands mutate cluster state
or ConfigHub.

```bash
cd examples/fleet-import

# Fleet summary
../../cub-scout import cluster-aggregator cluster-dev.json cluster-prod.json

# With unified App proposal
../../cub-scout import cluster-aggregator cluster-dev.json cluster-prod.json --suggest

# JSON output for scripting
../../cub-scout import cluster-aggregator cluster-dev.json cluster-prod.json --suggest --json
```

## Important Boundaries

- `import cluster-aggregator` merges existing JSON — it never connects to clusters
- No commands in this example write to ConfigHub or modify cluster state
- The `--suggest` flag produces a proposal, not an action
- Reconciliation rules in the proposal are suggested defaults, not applied policy

## What To Verify

After running the aggregator:

- 7 workloads across 2 clusters
- 3 apps span both clusters (payment-api, payment-worker, redis)
- 1 app runs only on prod (cache-warmer, Native/orphan)
- Ownership: 4 Flux, 2 Helm, 1 Native
- JSON output matches `expected-output/fleet-summary.json`

```bash
diff <(../../cub-scout import cluster-aggregator cluster-dev.json cluster-prod.json --suggest --json | jq -S .) \
     <(jq -S . expected-output/fleet-summary.json)
```

## Artifacts

| File | Purpose |
|------|---------|
| `cluster-dev.json` | ImportResult from dev cluster (3 workloads) |
| `cluster-prod.json` | ImportResult from prod cluster (4 workloads) |
| `expected-output/fleet-summary.json` | Expected JSON output with `--suggest` |

## Related Files

- [README.md](./README.md)
- [prompts.md](./prompts.md)
- [contracts.md](./contracts.md)
