# Live Validation Harness

This is the acceptance harness for the existing
`release check --check-running-image` read path. The caller supplies the
expected OCI configuration bundle digest, exact controller identity, and
Kubernetes context(s). The harness never recovers an intended digest from a
live object, renders a bundle, changes a cluster, or treats a fixture as
production evidence.

## Acceptance Contract

The contract was defined before the harness implementation:

1. Required input is explicit: a digest-pinned `oci://...@sha256:<64 lowercase
   hex digits>` bundle reference, `Kind/name`, controller API version,
   controller namespace, target kube context, and optionally a separate
   controller context.
2. Argument and dependency failures happen before a release-check process is
   started. Mutable tags, malformed identities, embedded registry credentials,
   and out-of-range read limits are rejected.
3. Standalone always runs. If `cub` is installed and can load the selected
   binary as a local plugin in an isolated temporary `CUB_CONFIG`, plugin
   mode runs with that same isolated configuration and the same inputs.
   Otherwise the skip and reason are recorded.
4. Every mode uses `--check-running-image`, `--format json`,
   `--fail-on any-non-pass`, a new `--out` path, and stdin from
   `/dev/null`. JSON stdout, saved report, exit code, redacted stderr, and
   command arguments are retained.
5. A report is valid only when it is JSON `version: "v1"`, has an aggregate
   verdict in PASS, WATCH, BLOCK, INCONCLUSIVE, an explicit `runningImage`
   object with a match/mismatch/unknown verdict, an explicit `running-image`
   stage, and a workloads array. Saved JSON and stdout must be semantically
   identical. PASS requires exit 0; other contract verdicts require exit 2.
6. A PASS report must also have `runningImage.verdict == "match"`, at least
   one supported workload, and supported workload identities. A Deployment
   workload must have `deployment.complete == true`; a direct Pod may use its
   exact-object evidence without Deployment coverage. This prevents an older
   binary with incomplete image proof from being accepted as live PASS.
7. `--expect-verdict` is optional. When supplied, every executed mode must
   have that exact verdict. This lets an existing wrong bundle or other
   approved negative input be accepted as a negative test without changing the
   cluster. Without it, non-PASS returns harness exit 2.
8. The run directory records binary version, binary SHA-256, source commit,
   dirty-worktree state, explicit inputs, plugin availability, and exit codes.
   It uses restrictive permissions. Credentials and credential-bearing
   environment values are not captured; stderr redaction is best-effort.

This accepts one bounded observation, not a continuous guarantee. Kubernetes
reads remain sequential and the release check's own read and freshness limits
apply.

## Prerequisites

Required:

- Bash, this checkout, and an executable `./cub-scout`. Build one with
  `go build -o ./cub-scout ./cmd/cub-scout`, or select an already-built
  executable with `--binary /path/to/cub-scout`. The harness never
  overwrites a binary.
- Kubeconfig read access to the target objects and, when different, the
  controller context. Use unattended credentials for automation.
- Existing registry access and credentials for the OCI client. The harness
  does not log in, write credential files, or print the environment.
- `jq` and `sha256sum` or `shasum`.

Optional:

- `cub` on `PATH`. The harness stages the selected binary in a
  temporary plugin directory and does not alter the user's installed plugin.
- A local OCI layout for a real scratch-cluster check. Pass the same explicit
  digest-pinned `--bundle` plus `--oci-layout /path/to/layout`.
  The layout is not derived from Kubernetes.

## Run A Live Check

Run from the repository root. The bundle reference must be the configuration
bundle's published manifest digest obtained independently from the approved
release record. It is not a container image digest, `Release.Digest`,
release number, or value read from the controller.

```bash
./examples/oci-release-check/validate-live.sh \
  --bundle 'oci://registry.example/config/payments@sha256:<config-manifest-digest>' \
  --controller Application/payments \
  --api-version argoproj.io/v1alpha1 \
  --controller-namespace argocd \
  --kube-context production \
  --controller-context management \
  --expect-verdict PASS \
  --out-dir ./live-evidence/argo-payments
```

For a Flux OCI-backed Kustomization:

```bash
./examples/oci-release-check/validate-live.sh \
  --bundle 'oci://registry.example/config/payments@sha256:<config-manifest-digest>' \
  --controller Kustomization/payments \
  --api-version kustomize.toolkit.fluxcd.io/v1 \
  --controller-namespace flux-system \
  --kube-context production \
  --expect-verdict PASS \
  --out-dir ./live-evidence/flux-payments
```

The default `--controller-context` is the target context. Context names are
opaque and may contain slashes, such as an EKS ARN. A name is not evidence that
two contexts point to the same cluster; the adapter must establish the target
binding from its supported evidence.

The output directory is new, mode `0700`, and its evidence files are mode
`0600`. It contains `run-info.txt`, `summary.txt`, JSON report and
stdout files, exit-code files, redacted stderr, and command capture. A
`plugin.*` set is added when plugin mode runs.

Reports may contain cluster, object, controller, registry, or other metadata
emitted by the provider. Redaction is best-effort, not a secret scanner. The
harness does not collect kubeconfig files or raw secret data, but inspect every
report and log before sharing it. Do not publish credential files, shell
history, or unrelated environment captures with the evidence directory.

Inspect a report with:

```bash
jq '{verdict, headline, nextStep, stages,
     runningImage: {verdict: .runningImage.verdict,
                    workloads: .runningImage.workloads}}' \
  ./live-evidence/argo-payments/standalone.report.json
```

The underlying command exits 0 for PASS, 2 for WATCH/BLOCK/INCONCLUSIVE, and
1 for invalid input, setup, or output errors. An expected matching negative
verdict makes the harness itself exit 0; an unexpected verdict or invalid
evidence exits 1.

## Argo And Flux Success Evidence

For Argo, obtain the exact OCI bundle digest from the approved publication
record independently of the selected Application. The single-source
Application must report matching source and target evidence, and the target
context must expose every desired object. Running-image PASS additionally
requires the current Deployment ownership, replica, pod, readiness, and
regular-container digest evidence required by the source branch.

For Flux, use the exact digest for the literal bundle referenced by the v1
Kustomization and its v1 OCIRepository. Current-generation and Ready evidence,
source URL, artifact revision, target binding, desired objects, and
running-image evidence must all be readable in the supplied contexts. Do not
substitute an OCI artifact revision for the configuration manifest digest
unless the approved publication record makes them the same identity.

In both cases, PASS is a dated bounded observation. It does not prove
publication authority, application behavior, traffic success, or future
rollout state.

## Negative Evidence Without Cluster Changes

Use only an existing caller-approved negative input or existing read boundary:

- an older or otherwise wrong published configuration bundle digest;
- an existing bundle whose authored image digest is known not to be the
  selected workload's observed digest; or
- an existing context, namespace, or controller with an expected missing,
  denied, stale, or incomplete observation.

For example:

```bash
./examples/oci-release-check/validate-live.sh \
  --bundle 'oci://registry.example/config/payments@sha256:<existing-wrong-bundle-digest>' \
  --controller Application/payments \
  --api-version argoproj.io/v1alpha1 \
  --controller-namespace argocd \
  --kube-context production \
  --expect-verdict BLOCK \
  --out-dir ./live-evidence/argo-payments-wrong-bundle
```

Do not edit a Deployment, change a Pod image, suspend a controller, publish a
fake bundle, revoke access, or create a temporary production failure. A wrong
bundle may correctly produce BLOCK, WATCH, or INCONCLUSIVE. If the expected
result is not known in advance, omit `--expect-verdict`, retain the
report, and document the observed reason.

## Live Validation Versus Fixtures

`objects.yaml` and `image-deployment.yaml` are offline regression
material with synthetic or non-pullable image identities. Go tests use
recorded HTTP/Kubernetes fixtures and prove parser, adapter, JSON, exit-code,
request-budget, and standalone/plugin behavior. They do not prove that a real
Argo or Flux controller delivered a production release.

This harness accepts only a caller-supplied digest-pinned OCI reference,
reads the named live controller/source/target/Pod/ReplicaSet objects, and
retains the live report plus binary checksum and input identities. Its only
writes are local evidence files; the release check remains read-only.

## Clean-Machine Walkthrough

1. Check out the reviewed revision and record the worktree state.
2. Build and identify the selected binary:

```bash
go build -o ./cub-scout ./cmd/cub-scout
./cub-scout version
```

3. Install or verify `jq` and a SHA-256 utility. Configure kubeconfig
   contexts and non-interactive registry credentials through the normal secret
   store. Do not put credentials in command arguments or the evidence folder.
4. Run the script with the independent OCI digest and exact Argo or Flux
   inputs, using a new `--out-dir` for each observation. Use
   `--binary` for a reviewed binary outside the checkout.
5. Check `summary.txt`, all JSON files, every exit code, `run-info.txt`,
   and the complete `runningImage` section. A PASS is not a deployment
   action or continuing health guarantee.
6. For negative evidence, repeat with an existing wrong bundle or existing
   incomplete/denied scope and record why that input was approved. Never
   manufacture a production failure.

The accompanying `validate-live_test.sh` is a disposable fake-process
regression test for argument validation, verdict gates, plugin isolation,
headless stdin, local layouts, permissions, and legacy-PASS rejection. It is
not live-cluster evidence.
