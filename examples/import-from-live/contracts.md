# Contracts

This file documents the stable inspection paths for `import-from-live`.

## Read-Only Contracts

### `../../cub-scout import --dry-run --from-bundle fixtures/`

- mutates: no
- output shape: ASCII table (import preview)
- proves:
  - ownership detection classifies resources by actual labels
  - ArgoCD resources are detected via `argocd.argoproj.io/instance`
  - Helm resources are detected via `app.kubernetes.io/managed-by: Helm`
  - Native resources have no ownership labels
  - variant inference uses ArgoCD Application `spec.source.path`
  - App structure groups by component name across variants
  - Native resources are reported but excluded from the proposal

### `../../cub-scout import --dry-run --from-bundle fixtures/ --json`

- mutates: no
- output shape: JSON object
- proves: same as above, in machine-readable form
- expected anchors:
  - `.appSpace` is present
  - `.units` array has 9 entries (3 components x 3 variants)
  - each unit has `app`, `variant`, and `workloads` fields
  - no unit references the Native ConfigMap

### `./demo.sh`

- mutates: no (runs dry-run path)
- output shape: narrated terminal output
- proves: the full human walkthrough produces expected output

## Evidence Boundary

This example proves only fixture-driven discovery evidence:

- ownership detection from label parsing
- variant inference from ArgoCD Application paths
- App structure proposal from workload grouping

It does not prove live cluster connectivity, ConfigHub import success,
or runtime reconciliation state. The `--from-bundle` flag means no
cluster connection is attempted.
