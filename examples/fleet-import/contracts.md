# Contracts

This file documents the stable inspection paths for `fleet-import`.

## Read-Only Contracts

### `../../cub-scout import-cluster-aggregator cluster-dev.json cluster-prod.json`

- mutates: no
- output shape: ASCII fleet summary
- proves:
  - per-cluster import JSONs can be merged into a unified fleet view
  - apps are grouped across clusters by component name
  - ownership counts are accurate (Flux, Helm, Native)

### `../../cub-scout import-cluster-aggregator ... --suggest`

- mutates: no
- output shape: ASCII fleet summary + App proposal
- proves:
  - unified App structure spans both clusters
  - variants are inferred from environment labels
  - reconciliation rules are suggested per-variant defaults
- notes: reconciliation rules are suggestions, not applied policy

### `../../cub-scout import-cluster-aggregator ... --suggest --json`

- mutates: no
- output shape: JSON object
- proves: same as above, in machine-readable form
- expected anchors:
  - `.summary.clusters` == 2
  - `.summary.workloads` == 7
  - `.summary.byApp` groups apps across clusters

## Evidence Boundary

This example proves only offline aggregation evidence:

- merging per-cluster import JSONs into a fleet view
- grouping apps across clusters by component name
- variant inference from environment labels

It does not prove live cluster connectivity, ConfigHub import success,
or runtime reconciliation state. The aggregator reads JSON files, not
live clusters.
