# RUL-04 raw StatefulSet image identity capture

This is a bounded evidence-preparation helper for the frozen RUL-04 question.
It captures an owned kind cluster's literal authored StatefulSet manifest and
the exact response bodies for one StatefulSet GET and one namespace Pod LIST.
It does not edit the benchmark manifest, run model evaluations, or claim a
controller applied the OCI configuration.

The intended image is deliberately `registry.k8s.io/pause:3.10`, with no
immutable digest. The captured Ready Pod's actual `imageID` is required and
preserved verbatim. UNKNOWN is independently justified because the authored
reference does not identify an immutable intended digest; the runtime digest
and matching tag/name do not establish that relationship. Missing readiness,
UID linkage, or runtime imageID aborts acceptance and is retained as a failed
capture, never rewritten as absent/empty evidence.

The local unsigned OCI layout pins authored bytes only. It is not an Argo/Flux
installation, applied-source receipt, or controller binding. If the optional
Scout command is supplied, its output is saved separately as derived diagnostic
output and is never raw model evidence. Current `release check` may report the
controller binding unsupported/inconclusive for StatefulSet; that result does
not turn the fixture into applied-source proof.

## Actual capture — 2026-10-01

The reviewed capture completed at 03:15:20–03:15:53 UTC in 33.78 seconds,
from helper source `fd4963872b9875bed01b9c4a2fdbab5c6db2e635`. See the
[full scoped proof](../reports/2026-10-01-rul04-image-identity.json) for binary,
source, raw response hashes and timestamps. Both raw API requests returned 200.
The Pod was Running and Ready, linked to StatefulSet UID
`c260243a-e06c-4529-b514-c977de02f5bc`, with runtime image ID
`sha256:afb61768ce381961ca0beff95337601f29dc70ff3ed14e5e4b3e5699057e6aa8`.
No immutable intended image digest is authored in the configuration.

The separate Scout run returned configuration PASS and workloads PASS,
controller INCONCLUSIVE, and running-image unknown with
`workload-ownership-unsupported`. It made no Pod reads for that unsupported
image tier; the raw Pod response was independently captured by the same
observer. This is not an applied-source, immutable-image, or savings claim.
The owned cluster was removed; shared config bytes and private configs before
cleanup were unchanged. The two raw reads are sequential, not atomic.

## Run after lead review

Prerequisites: local Docker engine, pinned kind `v0.31.0` and node image from
`evals/inv04-rbac/capture.py`, `kubectl`, a shared kubeconfig whose bytes are
recorded before/after but never used for cluster API calls, and explicit pinned
local binaries. Output must be a new path. The helper creates exactly one
uniquely named kind cluster, applies only its fixture, uses a temporary private
admin kubeconfig for setup/cleanup, creates a namespaced get/list-only observer
for StatefulSets and Pods, then deletes and verifies deletion of the owned
cluster. The observer never reads Secret resources. Temporary admin credentials
are used only for fixture setup and cleanup, then removed.

```sh
python3 evals/rul04-image-identity/capture.py \
  --execute \
  --shared-kubeconfig "$KUBECONFIG" \
  --expected-layout-helper-sha256 <reviewed-64-hex-sha256> \
  --layout-helper-source-revision <full-source-commit> \
  --oci-layout-helper /absolute/path/to/create-layout \
  --scout-binary /absolute/path/to/pinned-cub-scout \
  --expected-scout-sha256 <reviewed-64-hex-sha256> \
  --scout-source-revision <full-source-commit> \
  --output-dir /tmp/new-rul04-capture
```

The required local OCI helper must match the reviewed SHA-256; its source
revision is recorded. The capture checkout must be clean, and its commit and
helper script hash are recorded separately. `--scout-binary` plus its two pin
arguments are optional as a group. If present, the helper invokes `release check`
with explicit `StatefulSet/rul04-pause`, `apps/v1`, namespace and observer
context, writing only separate derived output. The OCI layout helper writes a
new layout under the output directory without registry/network publication.
Capture has bounded command/API outputs and deadlines, exact response-body
hashes, expected and observed executable hashes, source identifiers, non-atomic sequencing and cleanup
provenance. It rejects reuse of an output directory and never stores bearer
tokens or kubeconfig contents.

The capture is intentionally not run as part of implementation. Offline guards:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest evals/rul04-image-identity/test_capture.py
```

The case is only prepared after a lead-reviewed real capture. It does not admit
RUL-04 to a paid suite or change frozen question, answer, controls, or weights.
