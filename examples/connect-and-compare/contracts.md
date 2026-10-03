# Contracts

This file documents the stable inspection paths for `connect-and-compare`.

## Read-Only Contracts

### `./demo.sh`

- mutates: no
- output shape: terminal text (doctor, compare, history views)
- proves:
  - `cub-scout doctor` can produce cluster signal from fixture input
  - `cub-scout compare` can show intent-vs-observed alignment from fixtures
  - `cub-scout history` can show ChangeSet timeline from fixtures
- notes: all data comes from `testdata/` fixtures, not a live cluster

### `./demo.sh --verify`

- mutates: no
- output shape: pass/fail per snapshot
- proves:
  - generated output matches committed expected snapshots
  - the demo flow is deterministic and reproducible
- expected anchors:
  - exit code 0 means all snapshots match
  - exit code non-zero means at least one snapshot diverged

## Evidence Boundary

This example proves only fixture-driven evidence:

- doctor signal from `testdata/doctor_input.json`
- compare alignment from offline fixture data
- history timeline from `testdata/history_changesets.json`

It does not prove live cluster connectivity, ConfigHub import success,
or runtime reconciliation state.

## Explicit-context three-way invocation

`compare three-way --kube-context NAME` captures one Kubernetes selection for
resource, namespace, cluster and View scope. The scoped `--tui` pane and the
local map Trace picker `c` entry share the same collector/ASCII report. JSON,
ASCII and Markdown retain context labels and omissions; denied discovery or
source/Pod enrichment cannot establish agreement or empty inventory. View
membership requires UnitID or SpaceID plus slug, with separate ConfigHub
server/space authority. Local rendered DRY is distinct from controller intent.

The [live-delivery example](../live-delivery-observability/README.md#captured-three-way-comparison-contracts-755)
links reproducible TLS/MCP/TUI contract tests. Its synthetic fixtures prove
binding/interpretation behavior, not live acceptance, real ConfigHub governance
or agent savings. The interpretation scaffold in
`evals/compare-three-way-context/` has no model execution.
