# Copyable Prompts

## 1. Orient Me First

Read this example and do not mutate anything yet.

Explain:

- what fleet aggregation does (merge per-cluster JSONs)
- what the two fixture clusters contain
- what `--suggest` adds to the output
- what success looks like

Then run only:

```bash
../../cub-scout import-cluster-aggregator cluster-dev.json cluster-prod.json --json
```

## 2. Safe Walkthrough

Guide me through `fleet-import` step by step.

Before each command:

- explain what it does
- confirm it is read-only
- tell me what grouping logic applies (apps across clusters)
- tell me what variant inference rules apply

Use this path:

```bash
# Summary only
../../cub-scout import-cluster-aggregator cluster-dev.json cluster-prod.json

# With proposal
../../cub-scout import-cluster-aggregator cluster-dev.json cluster-prod.json --suggest
```

## 3. Verify The Fleet View

After the aggregator runs, verify:

- 3 apps span both clusters
- 1 app (cache-warmer) is prod-only and Native
- ownership counts: 4 Flux, 2 Helm, 1 Native
- JSON output matches expected fixture

```bash
diff <(../../cub-scout import-cluster-aggregator cluster-dev.json cluster-prod.json --suggest --json | jq -S .) \
     <(jq -S . expected-output/fleet-summary.json)
```

## 4. Call Out The Remaining Gap

Evaluate this example honestly.

Say whether:

- the aggregation correctly groups apps across clusters by component name
- the variant inference from environment labels is deterministic
- the reconciliation rule suggestions are presented as non-enforced defaults
