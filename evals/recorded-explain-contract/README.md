# Recorded explain contract (prepared only)

This is a prepared, independently gradable product-contract case over the full
recorded Deployments List already used by the main eval fixtures. The case-local
copy is byte-for-byte identical to `evals/fixtures/cluster/deployments.yaml`:
SHA-256 `305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8`,
67,696 bytes, one YAML document, 14 objects. `recording.json` records those
facts, and the fixture test verifies both source and scaffold bytes.

The prompt asks for exact recorded-object facts for `apps/v1 Deployment
shop/checkout`; the complete raw List, including its `managedFields`, is the
evidence. No data has been fabricated or regenerated for this case. It is not
mapped into `benchmark-v1`, is not an Experiment A run, and has not been run
through a model. Its configured tool surface allows file reading/search only,
so it checks evidence interpretation and the requested response contract; it
does not invoke or prove the new CLI, TUI, or recorded MCP mode. The deterministic
offline Go contract tests exercise those product surfaces against fixed bytes.

There is no trusted capture time, desired-state export, or independent live
confirmation in this case. The grader therefore checks only the fixed recorded
facts and requires `resourceRead: null`; it makes no freshness, rollout-time,
delivery, or human-actor claim.
