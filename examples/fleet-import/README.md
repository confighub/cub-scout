# Fleet Import: Multi-Cluster Aggregation

This example shows `cub-scout import-cluster-aggregator` merging import data from
two clusters into a unified App proposal.

## The Scenario

You've already run `cub-scout import --json` against two separate clusters -- a dev
cluster and a prod cluster. Now you want a single view of what's running across
both, grouped by app rather than by cluster.

## 1. You've Imported Two Clusters Separately

Each cluster was scanned independently:

**Dev cluster** (`cluster-dev.json`) -- 3 workloads:

| Namespace    | Workload       | Kind        | Owner | Replicas |
|--------------|----------------|-------------|-------|----------|
| payment-dev  | payment-api    | Deployment  | Flux  | 1        |
| payment-dev  | payment-worker | Deployment  | Flux  | 1        |
| payment-dev  | redis          | StatefulSet | Helm  | 1        |

**Prod cluster** (`cluster-prod.json`) -- 4 workloads:

| Namespace    | Workload       | Kind        | Owner  | Replicas |
|--------------|----------------|-------------|--------|----------|
| payment-prod | payment-api    | Deployment  | Flux   | 3        |
| payment-prod | payment-worker | Deployment  | Flux   | 2        |
| payment-prod | redis          | StatefulSet | Helm   | 3        |
| payment-prod | cache-warmer   | Deployment  | Native | 1        |

The prod cluster has one extra workload (`cache-warmer`) that doesn't exist in dev.

## 2. Here's the Fleet View

```bash
./cub-scout import-cluster-aggregator \
  examples/fleet-import/cluster-dev.json \
  examples/fleet-import/cluster-prod.json
```

The aggregator produces a summary:

```
FLEET SUMMARY
  Clusters: 2
  Workloads: 7

  Ownership:
    Flux: 4
    Helm: 2
    Native: 1

  Apps across clusters:
    payment-api (2 clusters)
    payment-worker (2 clusters)
    redis (2 clusters)
    cache-warmer
```

Three apps run on both clusters. One (`cache-warmer`) runs only on prod.

## 3. Here's the Unified Proposal

Add `--suggest` to generate a single App structure that spans both clusters:

```bash
./cub-scout import-cluster-aggregator \
  examples/fleet-import/cluster-dev.json \
  examples/fleet-import/cluster-prod.json \
  --suggest
```

```
PROPOSED HUB/APP SPACE MODEL

  APP SPACE: fleet-team
    Deployer: Flux
    Reconciliation Rules:
      variant=prod  -> drift:revert, approval:required
      variant=dev   -> drift:accept, approval:none

    cache-warmer
      cache-warmer (variant=default)

    payment-api
      payment-api-dev  (variant=dev)   cluster-dev.json:payment-dev/payment-api
      payment-api-prod (variant=prod)  cluster-prod.json:payment-prod/payment-api

    payment-worker
      payment-worker-dev  (variant=dev)   cluster-dev.json:payment-dev/payment-worker
      payment-worker-prod (variant=prod)  cluster-prod.json:payment-prod/payment-worker

    redis
      redis-dev  (variant=dev)   cluster-dev.json:payment-dev/redis
      redis-prod (variant=prod)  cluster-prod.json:payment-prod/redis
```

Workload references include the source cluster so you can trace where each
instance lives: `cluster-prod.json:payment-prod/payment-api` means the
`payment-api` Deployment in the `payment-prod` namespace of the prod cluster.

## Key Points

- **Fleet aggregation is read-only.** It merges existing import JSONs. It doesn't
  connect to clusters or modify anything.

- **Apps are grouped across clusters.** `payment-api` appears once in the proposal
  with two variants (`dev` and `prod`), not as two separate entries.

- **Variant inference uses the same logic.** The `environment` label on each
  workload determines the variant. Dev workloads become `variant=dev`, prod
  workloads become `variant=prod`.

- **Reconciliation rules are per-variant.** Prod gets `drift:revert` +
  `approval:required`. Dev gets `drift:accept` + `approval:none`. These are
  suggestions, not enforced policy.

- **The input format is flexible.** The aggregator accepts raw `ImportResult` JSON
  (from `cub-scout import --json`) or `CombinedResult` JSON (from
  `cub-scout combined --json`).

## Files in This Example

| File | Purpose |
|------|---------|
| `cluster-dev.json` | ImportResult from the dev cluster (3 workloads) |
| `cluster-prod.json` | ImportResult from the prod cluster (4 workloads) |
| `expected-output/fleet-summary.json` | Full JSON output from the aggregator with `--suggest` |

## Try It

```bash
# Plain text summary
./cub-scout import-cluster-aggregator \
  examples/fleet-import/cluster-dev.json \
  examples/fleet-import/cluster-prod.json

# With unified proposal
./cub-scout import-cluster-aggregator \
  examples/fleet-import/cluster-dev.json \
  examples/fleet-import/cluster-prod.json \
  --suggest

# JSON output for scripting
./cub-scout import-cluster-aggregator \
  examples/fleet-import/cluster-dev.json \
  examples/fleet-import/cluster-prod.json \
  --suggest --json

# Pipe to jq to see which apps run on multiple clusters
./cub-scout import-cluster-aggregator \
  examples/fleet-import/cluster-dev.json \
  examples/fleet-import/cluster-prod.json \
  --json | jq '.summary.byApp | to_entries[] | select(.value | length > 1)'
```

## Full Workflow

In practice, you'd generate the per-cluster JSONs first, then aggregate:

```bash
# Step 1: Scan each cluster
for ctx in dev-cluster prod-cluster; do
  kubectl config use-context $ctx
  ./cub-scout import --json > ${ctx}.json
done

# Step 2: Aggregate into fleet view
./cub-scout import-cluster-aggregator dev-cluster.json prod-cluster.json --suggest --json
```

## See Also

- [Combined Git + Live](../combined-git-live/) -- Aligning Git intent with cluster state
- [CLI-GUIDE.md](../../CLI-GUIDE.md) -- Workflow-first CLI guide
