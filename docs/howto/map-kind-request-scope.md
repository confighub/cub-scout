# `map list --kind` request scope

To inspect Deployments in one namespace, run:

```sh
./cub-scout map list --kind Deployment --namespace team-a --format json
```

Previously, the live collector listed every configured built-in and first-class
controller collection, then discarded rows whose Kind did not match. It now
uses explicit canonical Kind-to-GVR mappings to omit only collections known
not to contain the requested Kind. A custom GVR remains in the request set
because its configuration has no Kind field and it could contain a
`Deployment`. The separate ApplicationSet lookup remains for ownership
lineage. Empty and unrecognized Kind filters keep the unfiltered request set.

The deterministic HTTP test `TestMapListKindFilterLimitsLiveRequestsAndRetainsUnknownGVR`
checks the exact request paths and visible ownership result; its fixture issues
three GETs for this filtered example (the Deployment collection, an unknown
custom GVR, and the ApplicationSet dependency). This is a request-count test,
not a measurement of time, bytes, or cost on a live cluster.

## Captured comparison — 2026-10-01

The same restricted observer and owned kind cluster were used for sequential
old/new runs (capture source `35c9bf4`, old product `a99d7d4`, new product
`eec6d44`). All selected resource and ownership facts matched. The remaining
ApplicationSet omission was preserved in all scopes; the denied scope also
retained its Deployment omission. All collections remained explicitly partial.

| Deployment scope | Old / new JSON bytes | Old / new elapsed seconds | Old / new omissions |
|---|---:|---:|---:|
| Readable, two objects | 8,324 / 1,018 | 6.518 / 1.303 | 41 / 1 |
| Readable, empty | 7,463 / 317 | 6.424 / 0.031 | 41 / 1 |
| Forbidden | 7,287 / 461 | 6.425 / 0.041 | 42 / 2 |

The whole capture took 52.654 seconds; owned cleanup was verified and shared
and private kubeconfigs were unchanged before cleanup. kind's deletion of the
owned admin context is recorded separately. The before/after source, binary,
output and provenance hashes are in the
[proof report](../../evals/reports/2026-10-01-map-kind-scope.json).

This is one old-first/new-second capture, not a general latency benchmark.
The 42-to-two request count comes from deterministic HTTP tests, not a live
wire trace. No model, dollar, credit or MCP/TUI latency measurement was made.
The frozen benchmark and its original INV-04 raw fixtures are unchanged.

The reviewed capture driver is `evals/inv04-rbac/compare_kind_scope.py`:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/inv04-rbac/compare_kind_scope.py \
  --execute --shared-kubeconfig /absolute/path/to/operator-config \
  --old-binary /absolute/path/to/pinned-old-cub-scout \
  --new-binary /absolute/path/to/pinned-new-cub-scout \
  --output-dir /tmp/a-new-owned-comparison-directory
```

It requires the exact historical binary and node-image hashes embedded in the
driver; a different build environment needs separately reviewed pins. It uses
private cluster credentials, compares explicit namespaces, retains failures,
and deletes only its own cluster. Do not run it as part of ordinary tests.
