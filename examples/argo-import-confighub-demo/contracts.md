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
  - connected readiness is checked when the live worker pid file is present
  - `cub-scout` status and ownership surfaces produce output
- note: `./verify.sh` is a Slice 1 contract; it does not include `cub-scout scan`

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

## Connected Contracts

### `../scripts/verify-connected-demo.sh --space argo-import-demo --renderer argocdrenderer`

- mutates: no
- output shape: plain text PASS/FAIL summary
- proves:
  - at least one ready worker exists
  - at least one Kubernetes target exists
  - at least one renderer target exists
  - at least one connected workload appears in `cub-scout import --dry-run --json`

### `cub target list --space argo-import-demo`

- mutates: no
- output shape: terminal table view
- proves: the worker-registered discovery and renderer targets are visible

### `cub unit list --space argo-import-demo`

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

Post-import `cub-scout scan` evidence is explicitly out of scope for this Slice 1
contract.
