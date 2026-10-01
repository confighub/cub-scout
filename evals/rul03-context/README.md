# RUL-03 explicit-context recording

This case contains one reviewed, bounded capture with two explicit Kubernetes
contexts. The denied context returned HTTP 403 to a namespaced Deployment List
request. The readable context returned one Deployment with its captured UID.
The explicitly requested contexts mapped to distinct local API servers while
the observer kubeconfig's default stayed `rul03-readable` before and after both
requests. A 403 means the denied inventory is unknown; it is not an empty list.

The two response bodies are copied byte-for-byte from the reviewed capture.
`observer-context-map.json` is likewise copied exactly. `capture-scope.json` is
derived capture metadata that records the source/helper pins, request context,
path, status, timestamps, and response hashes; it is not a Kubernetes object.
The scaffold stages exactly these four files identically for both benchmark
arms. It excludes setup manifests and apply logs, admin-side object checks,
private kubeconfigs, credentials, and the full capture provenance.

## Reviewed capture

The accepted run completed 2026-10-01 04:54:23.759–04:55:29.053 UTC in 65.29
seconds (wrapper exit 0, 65.36 seconds). It used capture source revision
`e61506e0335ab8033635f477da8f2c4ad43dc060`, helper SHA-256
`501daae70b0f0a7ef52c7e873356e984b24d6f17594431444adbbf35987a10b0`, kind
`v0.31.0`, and pinned node image
`kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661`.
The two raw response hashes and context binding are pinned in `capture-scope.json`
and [`benchmark-v1.json`](../benchmark-v1.json); complete non-secret factual
run details are in the [RUL-03 report](../reports/2026-10-01-rul03-context.json).

Shared kubeconfig hashes matched before the run, before cleanup, and after it.
The observer configuration hash and default context remained unchanged. The
private admin config hashes matched their initial values before cleanup and
changed during cleanup; the report distinguishes these phases. Both newly owned
clusters were deleted and verified absent. The earlier failed preflight stopped
before cluster creation, is retained separately, and did not contribute model
input.

These are two sequential observations, not an atomic snapshot, fleet result, or
health/ownership claim. In particular, the readable result must not be used to
fill the denied context's inventory. The frozen benchmark remains
non-executable and no model or paid evaluation was run.

## Capture helper and offline guards

`capture.py` is a bounded source-preparation helper, not needed to consume this
recording. It requires `--execute`, explicit local tool paths, explicit shared
kubeconfig path for hash-only integrity checks, and a new output path. It uses
two uniquely named owned kind clusters, private per-cluster kubeconfigs, and a
private observer-only kubeconfig; every query names its context explicitly.
It uses the pinned kind/node versions inherited from INV-04, limits the overall
run to 240 seconds plus a 90-second cleanup window, caps command output at
2 MiB and API bodies at 4 MiB, retains partial-operation evidence, and only
deletes clusters covered by the run's ownership marker. Cleanup fails closed if
a per-cluster private kubeconfig is absent; shared `KUBECONFIG` is never used as
a fallback.

The offline tests exercise explicit context/server bindings, strict forbidden
versus list responses, output limits, ownership cleanup, and interrupted
partial creation. They do not call a cluster, network, product CLI, or model:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/rul03-context -v
```

The prompt, grader, and recorded files prepare a correctness contract only.
Tool permissions, equal ordinary-tool admission, benchmark protocol review, and
any future evaluation remain separate gates.
