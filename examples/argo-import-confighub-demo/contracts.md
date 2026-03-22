# Contracts

This file documents the safest stable inspection paths for
`argo-import-confighub-demo`.

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
  - `.example == "argo-import-confighub-demo"`
  - `.entrypoint == "./setup.sh"`
  - `.mutates == false`
  - `.clusterName == "argo-import-demo"`
  - `.configHubSpace == "argo-import-demo"`
  - `.executionCommand`

### `./verify.sh`

- mutates: no
- output shape: plain text
- proves:
  - the kind cluster is reachable
  - the `argocd` namespace exists
  - the expected ArgoCD Applications are present
  - connected readiness is checked when the live demo space exists, even if the
    local worker pid file is missing
  - `cub-scout` status and ownership surfaces produce output
  - `cub-scout scan --state --json` yields at least one finding or runtime finding
  - a scan summary and sample finding can be surfaced without overclaiming import success
  - note: scan evidence here is cluster and cub-scout evidence, not ConfigHub import/render proof

### `kubectl --context kind-argo-import-demo get applications -n argocd`

- mutates: no
- output shape: Kubernetes table output
- proves: ArgoCD `Application` resources are present in the local cluster

### `../../cub-scout gitops status`

- mutates: no
- output shape: terminal status view
- proves:
  - which GitOps objects appear healthy or unhealthy live
  - controller-side status and ownership context
- note: non-healthy objects are evidence, not necessarily script failure

### `../../cub-scout map list`

- mutates: no
- output shape: terminal table view
- proves:
  - how live resources are classified by ownership
  - whether a workload appears Argo-managed, Helm-managed, or Native

### `../../cub-scout scan --state --json`

- mutates: no
- output shape: JSON object
- proves:
  - the current Argo demo fixtures surface at least one `findings` or `runtimeFindings` entry
  - scan summaries can be reported in a stable way during `./verify.sh`
  - runtime failures stay distinct from ConfigHub import/render evidence
- expected anchors:
  - `.state.summary`
  - `.state.findings` or `.state.runtimeFindings`

## Connected Contracts

### `../scripts/verify-connected-demo.sh --space argo-import-demo --renderer argocdrenderer`

- mutates: no
- output shape: plain text PASS/FAIL summary
- proves:
  - at least one ready worker exists
  - at least one ready Kubernetes-backed target exists
  - at least one ready renderer target exists
  - at least one imported dry unit and one imported wet unit exist in the demo space
  - `cub-scout import --dry-run --json` connected workload counts are only used as a gate
    when the scout proposal App Space matches the demo space and the bounded preview
    returns before timeout

### `cub target list --space argo-import-demo`

- mutates: no
- output shape: terminal table view
- proves: the worker-registered discovery and renderer targets are visible

### `cub unit list --space argo-import-demo`

- mutates: no
- output shape: terminal table view
- proves: ConfigHub has imported units visible in the target space

## Command vs Watch

This example should be read in two modes:

- `command`
  `cub gitops discover`, `cub gitops import`, and other ConfigHub mutations
  write intended config or command intent into ConfigHub
- `watch`
  `kubectl`, ArgoCD APIs, `cub-scout gitops status`, and `cub-scout scan`
  observe runtime state and status from the live systems

Authority boundary:

- ConfigHub is authoritative for intended config and command intent
- live systems are authoritative for runtime state and status

Do not bypass a broken ConfigHub apply/import path by pulling config from
ConfigHub and applying it directly. Use watch-mode evidence to diagnose the
live problem instead.

## Evidence Boundary

This example can prove three different kinds of evidence:

- cluster evidence
- ConfigHub evidence
- cub-scout evidence

Import/render evidence does not, by itself, prove live workload reconciliation.

Connected readiness does not, by itself, prove scan/finding evidence.

Scan/finding evidence does not, by itself, prove ConfigHub import/render success.

This Argo Slice 2 contract now includes scan/finding evidence in `./verify.sh`,
but that evidence remains separate from connected readiness and import/render
proof.
