# Grafana Integration Note

> Status: Proposed (Design Note)
> Last reviewed: 2026-05-17
> Concepts index: [README.md](README.md)

This note captures the safest first path if we want Grafana to display
Flux- and Argo-oriented cub-scout state.

## TL;DR

Use existing cub-scout JSON outputs as the contract.

Start with a small collector or Grafana backend data source that:

- runs near the observed cluster
- invokes `cub-scout` on a schedule
- caches structured results
- serves Grafana from cached data instead of live shell-outs

Do not start by inventing a new Prometheus exporter, a browser-only Grafana
plugin, or a separate Flux/Argo parser that duplicates cub-scout logic.

## Why This Exists

The recurring question is: can Grafana display cub-scout information for
Flux and Argo state?

The answer is yes, but the first useful integration should build on the
machine-readable outputs cub-scout already has:

- `gitops status --json`
- `summary list --json`
- `watch` webhook or JSONL output
- `snapshot`
- `map list --json`
- `compare three-way --format json`
- `scan --normalized-json`

That keeps the contract aligned with current CLI behavior and avoids adding a
new product surface before the existing one is proven.

## Recommended First Shape

Prefer a small in-cluster collector near Flux or Argo.

Why this is the safer default:

- it keeps Kubernetes access local to the cluster
- it avoids placing kubeconfig or ConfigHub credentials inside Grafana
- it keeps Grafana reads fast by serving cached results
- it reuses cub-scout as the observation engine rather than reimplementing it

For multi-cluster views, run one collector per cluster and let Grafana read
aggregated results from a central store.

An alternative is a Grafana backend data source plugin. That can work, but it
should still query a cached result store or a local helper service rather than
shelling out to `cub-scout` on every panel refresh.

## Existing Inputs To Reuse

| Input | First use in Grafana | Notes |
|------|-----------------------|-------|
| `cub-scout gitops status --json` | Current Flux/Argo health | Best primary feed for backend, transport, source/deployer state, failing stage, reason, and message |
| `cub-scout summary list --json --type gitops-status` | Trend/history panels | Best source for "how long has this been failing?" |
| `cub-scout watch --webhook` or `--output-file` | Event timelines | Good for drift, ownership, and scan findings over time |
| `cub-scout snapshot` | Inventory and relation panels | Good for cluster-scope state snapshots |
| `cub-scout map list --json` | Ownership and scope panels | Good lightweight inventory feed |
| `cub-scout compare three-way --format json` | Connected drift panels | Optional; best when ConfigHub-backed desired/live comparison matters |
| `cub-scout scan --normalized-json` | Risk overlays | Optional; useful when GitOps state and risk findings should appear together |

## Suggested Dashboard Primitives

The first useful Grafana experience should include:

- cluster, namespace, backend, and severity filters
- overview cards for backend, transport, healthy count, and failed count
- source status table with reason and message
- deployer status table with failing stage
- event timeline sourced from `watch`
- trend panel sourced from persisted `gitops-status` summaries
- optional drift panel sourced from `compare three-way`
- optional risk overlay sourced from normalized scan results

If topology becomes important later, `graph export --format json` is the next
candidate for node-link or relationship-oriented panels.

## What Runs Where

### Preferred placement

Run the collector inside the observed cluster as a Deployment or CronJob with
read-only RBAC.

### Alternative placement

Run a Grafana backend data source where Grafana itself runs, but only if
credential handling and network access are acceptable there.

### What should not run in the browser

A browser-only plugin should not be responsible for invoking `cub-scout`,
holding kubeconfig, or reaching cluster-private endpoints.

## Scope Boundaries

This note does **not** imply:

- a native Prometheus or OpenMetrics surface in v1
- a new ConfigHub worker role for cub-scout
- real-time shell execution on every Grafana panel refresh
- a rewrite of cub-scout internals into a dedicated SDK

The first implementation should be mostly new glue around the existing
`cub-scout` CLI, not a second implementation of Flux/Argo inspection logic.

## Decision Heuristic

If the goal is:

- current GitOps health: start with `gitops status --json`
- health over time: add `summary list --json --type gitops-status`
- event correlation: add `watch`
- connected desired-vs-live drift: add `compare three-way --format json`
- risk correlation: add `scan --normalized-json`

## Related Docs

- [architecture.md](architecture.md)
- [state-and-snapshots.md](state-and-snapshots.md)
- [commands.md](../reference/commands.md)
- [cli-contract.md](../reference/cli-contract.md)
- [json-contracts.md](../reference/json-contracts.md)
- [gsf-schema.md](../reference/gsf-schema.md)
- [graph-contract.md](../reference/graph-contract.md)
