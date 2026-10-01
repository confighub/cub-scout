# Owned-kind Trace context proof

The owned before/after capture **passed on attempt three** on 2026-10-01,
after independent helper and product review. The [derived report](report.json)
records CLI observations, per-action TUI requests, source/binary pins and cleanup.
The original receipt is retained at `/tmp/scout-trace-context-proof-3/receipt.json`;
its digest links the summary to that local record. This proves scoped context
binding against a real Kubernetes API and RBAC, not Argo reconciliation.

The old source pin is `8cdb27b0bc9db17f5c0d1b59628b67cd3936ed6c`; the fixed
source pin is `24074d858e8e2411ce2ba3e94340ec5646395eed`. The helper requires a
clean checkout containing the fixed pin as an ancestor. It compiles both CLI
binaries and compiles the fixed-source TUI probe **before** it creates a
cluster. The fixed source build and full Go suite were independently reported
passing before this capture packet was prepared; this packet adds no product
code.

## What acceptance means

The observations all use one invocation-owned kind cluster and namespace. A
synthetic `argoproj.io/v1alpha1` CRD and Application object plus an
Argo-labeled, zero-replica Deployment exercise normal Argo trace without
installing an Argo controller. The Application source is the inert
`example.invalid` URL. A second unlabelled, zero-replica Deployment is the
Native control. No Pod is created and no workload image is pulled. Fixture
inventory records each object’s exact kind, namespace, name, and API UID.

The CLI proof runs both normal and `--reverse` Trace against the Argo-labeled
Deployment:

| Source and selection | Required result |
|---|---|
| Old source, ambient private context | Exact target and Argo identity; fixed-source fields are not assumed. |
| Old source, explicit denied selector | Fails with `unknown flag: --kube-context`; it is not treated as a cluster observation. |
| Fixed source, explicit allowed context | Exact target, selected context label, Argo identity, and synthetic source URL. |
| Fixed source, explicit denied context | Normal trace must fail with the exact denied service-account identity; reverse trace must retain its forbidden error and target identity. Neither is accepted as Native or clean output. |

Both sources use the legacy `--json` alias for reverse observations because the
old source ignored `--format json` on that route. This proof tests context
binding; the normal/reverse format contract has separate deterministic tests.

The first attempt at `/tmp/scout-trace-context-proof-1` failed: the old-source
CLI shim rejected its availability check, and the old reverse route returned
ASCII with `--format json`. Its failed receipt is retained. The owned cluster,
private credentials and source worktrees were removed, with shared kubeconfig
unchanged. The corrected helper has 16 passing offline tests.

The second attempt at `/tmp/scout-trace-context-proof-2` passed the CLI
acceptance checks but failed the TUI target-identity assertion: the selected
Deployment was replaced by its Argo Application in the human result. Its
cleanup and shared/private config integrity checks passed. The product fix
retains the selected workload and preserves the Application provenance chain;
a new observer/rendering regression fails before that fix. Independent review,
focused tests, and the full Go suite pass at the new fixed source pin above.
Both failed attempts remain failed and retained.

The additional fixed-source reverse control requires the unlabelled Deployment
to report `native` under the allowed context, while its denied read must retain
the RBAC error and must not report `native`. This keeps “unmanaged” separate
from “not observable.” Context labels are selection names, not stable cluster
identities.

The fixed-source TUI probe sends actual `T`, `Enter`, close, reopen, and `Enter`
messages through `LocalClusterModel.Update`. It wraps the API transport and
rejects any method other than GET. After the first allowed action, it retargets
only the disposable TUI kubeconfig so its allowed context points at denied
credentials. Reopening the same model must still make a successful read with
the original captured binding. A newly bound denied model must receive HTTP
403 for the exact Deployment GET and display the denial. This exercises model
events, binding, requests, and rendering; it is not a terminal-emulator or
manual-navigation proof.

The successful TUI actions made six GETs on initial open, six on reopen after
private config retargeting, and one denied GET (403) in the newly bound denied
model. The owned cluster and source worktrees were removed; private kubeconfigs
were absent after cleanup. Shared kubeconfig and CLI private config integrity
passed. No model evaluation or cost-saving result follows from this capture.

## Boundaries and cleanup

- The shared kubeconfig path is only hashed before and after; its contents are
  not parsed or used to authenticate. All cluster operations use a newly
  written private kubeconfig under a mode-0700 temporary directory.
- The helper strips ambient provider, proxy, and ConfigHub variables from child
  environments. CLI/TUI processes receive a private `HOME` and a restricted
  `PATH`: external `argocd`, Flux, Helm, and `cub` commands are blocked. The
  old Argo CLI is a fail-closed shim that exercises its existing Kubernetes
  fallback. That `kubectl` shim permits only the exact Application-list read
  against the owned private config and pins it to the allowed owned context.
- The denied service-account token is minted only inside the owned cluster,
  written directly to the private config, never passed in process arguments,
  and redacted from receipts. The TUI kubeconfig is a separate private copy;
  only that copy is deliberately retargeted during the test.
- No controller, chart, external source checkout, image download, install,
  model call, paid run, or provider authentication is part of this proof.
  `--execute` requires existing kind `v0.31.0`, a local Unix Docker socket, and
  the already cached digest-pinned Kubernetes node image. Go module and toolchain
  downloads are disabled.
- Each command has a deadline and 2 MiB output cap; the serial attempt has a
  600-second deadline plus a separate 120-second cleanup budget. Failure output
  is retained. Cleanup deletes only after a successful kind creation; an
  uncertain partial create is inspected by its exact Docker label and left for
  ownership review. Temporary source worktrees and private kubeconfigs are
  removed in the overall cleanup path, including pre-cluster build failures.
  Any cleanup or shared-config integrity failure makes the receipt fail.

The proof demonstrates context binding and denied coverage for these owned
fixture reads. It does not demonstrate a real Argo reconciliation, general
cluster identity, ConfigHub behavior, controller writes, or all credential
provider types. Only the scoped attempt-three result above is claimed.

## Offline acceptance tests

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s evals/trace-context-live -p 'test_capture.py' -v
```

These tests use only synthetic records and short local Python children. They
reject missing/wrong target or context evidence, a native/empty result passed
off as denied coverage, non-GET TUI traffic, cleanup deletion after uncertain
creation, and failure to remove a source worktree left by a pre-cluster build
failure. They do not compile product code or contact a cluster.

## Future invocation

After the independent helper review, use a fresh output directory and provide
an existing kubeconfig path solely for the before/after hash:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/trace-context-live/capture.py \
  --execute \
  --integrity-only-shared-kubeconfig /explicit/path/to/kubeconfig-to-hash-only \
  --output-dir /tmp/scout-trace-context-proof
```

Inspect `receipt.json`, including `cleanupErrors`, `sharedKubeconfigUnchanged`,
`privateKubeconfigUnchangedDuringCLI`, and `clusterCreationSucceeded` before
accepting any result. Keep failed receipts; use a new output directory for any
later authorized attempt.
