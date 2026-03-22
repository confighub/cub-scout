# Contracts

This file documents the safest stable inspection paths for
`flux-import-confighub-demo`.

## Read-Only Contracts

### `./setup.sh --explain-json`

- mutates: no
- output shape: JSON object
- proves:
  - which execution command `setup.sh` will delegate to
  - whether `--with-worker` and `--seed-history` are enabled
  - which cluster and ConfigHub space names are in play
  - which evidence surfaces the example expects afterwards
- expected anchors:
  - `.example == "flux-import-confighub-demo"`
  - `.entrypoint == "./setup.sh"`
  - `.mutates == false`
  - `.clusterName == "flux-import-demo"`
  - `.configHubSpace == "flux-import-demo"`
  - `.executionCommand`

### `./verify.sh`

- mutates: no
- output shape: plain text
- proves:
  - the kind cluster is reachable
  - the `flux-system` namespace exists
  - the expected Flux sources and deployers are present
  - connected readiness is checked when the live worker pid file is present
  - `cub-scout` status and ownership surfaces produce output
  - `cub-scout scan --state --json` yields a summary during verification
  - scan evidence is reported either as findings-present or no-findings-observed
  - scan evidence here stays separate from ConfigHub import/render proof

### `flux --context kind-flux-import-demo get all -A`

- mutates: no
- output shape: Flux table output
- proves: Flux controllers, sources, and deployers are visible from the cluster API

### `kubectl --context kind-flux-import-demo get gitrepositories,kustomizations,helmreleases -A`

- mutates: no
- output shape: Kubernetes table output
- proves: Flux source and deployer objects are present in the local cluster

### `../../cub-scout gitops status`

- mutates: no
- output shape: terminal status view
- proves:
  - which Flux objects appear healthy or unhealthy live
  - controller-side status and ownership context

### `../../cub-scout tree ownership`

- mutates: no
- output shape: terminal tree view
- proves:
  - how live resources are grouped into Flux ownership chains
  - whether infrastructure and application paths are visible separately

### `../../cub-scout scan --state --json`

- mutates: no
- output shape: JSON object
- proves:
  - scan summary data is available during `./verify.sh`
  - Flux scan evidence can be surfaced as either a concrete finding or an explicit
    no-findings contract
  - runtime and controller findings remain separate from ConfigHub import/render proof
- expected anchors:
  - `.state.summary`
  - `.state.findings` or `.state.runtimeFindings`

## Connected Contracts

### `../scripts/verify-connected-demo.sh --space flux-import-demo --renderer fluxrenderer`

- mutates: no
- output shape: plain text PASS/FAIL summary
- proves:
  - at least one ready worker exists
  - at least one ready Kubernetes-backed target exists
  - at least one ready renderer target exists
  - at least one imported dry unit and one imported wet unit exist in the demo space
  - `cub-scout import --dry-run --json` connected workload counts are only used as a gate
    when the scout proposal App Space matches the demo space

### `cub target list --space flux-import-demo`

- mutates: no
- output shape: terminal table view
- proves: the worker-registered discovery and renderer targets are visible

### `cub unit list --space flux-import-demo`

- mutates: no
- output shape: terminal table view
- proves: ConfigHub has imported units visible in the target space

## Evidence Boundary

This example can prove three different kinds of evidence:

- cluster evidence
- ConfigHub evidence
- cub-scout evidence

Import/render evidence does not, by itself, prove live workload reconciliation.

Connected readiness does not, by itself, prove scan/finding evidence.

Scan/finding evidence does not, by itself, prove ConfigHub import/render success.

This Flux Slice 2 contract includes scan/finding evidence in `./verify.sh`,
with an explicit findings-or-no-findings boundary.
