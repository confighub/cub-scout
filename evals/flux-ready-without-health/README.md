# HLT-02 — Flux Ready without workload health evidence

This directory prepares a real raw-object capture for benchmark case HLT-02.
It is **not run yet**, contains no captured result fixture, and is not admitted
to `benchmark-v1`. No paid evaluation was run.

## Success contract

The case must establish all of these from captured objects: Flux Kustomization
`Ready=True` for its current generation; `.spec.wait: false`; `healthChecks` and
`healthCheckExprs` absent or empty; the applied revision equals the pinned
GitRepository artifact revision; and the independently observed Deployment is
unavailable for its current generation. This explicit `wait: false` matters:
absence of `healthChecks` alone is insufficient because Flux `wait: true`
checks all reconciled resources. See the [Flux Kustomization reference](https://fluxcd.io/flux/components/kustomize/kustomizations/#wait).

Expected answer: Ready reports reconciliation/apply status, not workload
health. With `wait: false` and no explicit health checks/expressions, this
Kustomization does not establish workload health. The captured Deployment and
Pod status independently show the workload unavailable. Do not infer that
Ready means healthy, or that the workload is generally healthy/unhealthy beyond
the captured observation.

Negative grading case: a response claiming “Ready means healthy” is wrong. A
second design negative is important: do not generalize from omitted
`healthChecks` when `.spec.wait` is true, because that setting checks all
resources.

## Prepared source and fixture

The capture pins the public cub-scout repository at
`7732dde28be8cf8c42c096d94efbd8ce4a9d0a19` and path
`./examples/combined-git-live/git-repo/apps/payment-worker/base`, which
contains the reviewed Deployment source. The committed
[`gitrepository.yaml`](gitrepository.yaml) and [`kustomization.yaml`](kustomization.yaml)
are applied unchanged. The Kustomization deliberately has `wait: false`, omits
both explicit health-check fields, and patches the workload to
`registry.k8s.io/pause:3.9` with a nonexistent command. The command failure is
independent of a bad image name, but the node still needs to pull the public
pause image; only the exact kind node image is checked for a local cache. An
image-pull failure is not accepted as the intended workload evidence.

## Capture and safety

After the lead authorizes the live proof, run from the repository root:

```bash
bash evals/flux-ready-without-health/capture.sh --execute /tmp/scout-hlt02-capture
```

The output path must not already exist. The script requires kind v0.31.0,
Flux CLI 2.8.6, Python 3, jq, and the exact cached kind node image shown in the
script. It refuses to reuse only its fixed cluster name (other kind clusters
are allowed), uses a private temporary kubeconfig for creation, every
Kubernetes request, and deletion, and deletes only that named cluster
(including cleanup after a partial create). It does not use or modify the
caller's kubeconfig. Do not run it until the serial live proof is authorized.

The capture contains raw GitRepository, Kustomization (start and end),
Deployment, ReplicaSet, and fixture Pod objects with managed fields. A
validator requires the actual controller owner chain Deployment → ReplicaSet →
Pod, a current-generation unavailable Deployment, and a selected unready Pod
with evidence of the intentional command failure. A compact controller image
and image-ID record and the Kubernetes API server version are also saved.
Provenance records object hashes, Flux/kubectl binary hashes, source and
install-manifest pins, capture-script/validator hashes, timestamps, and stable
Kustomization UID/generation/applied revision across the capture interval. The
observations are sequential, not an atomic snapshot. Secrets, kubeconfig, and
credentials are excluded. The temporary private kubeconfig is removed on exit.
Review the resulting data for sensitive metadata before checking any capture
into the repository.

No expected object/status fixture is synthesized here. The real outputs must
be inspected and approved before adding them to an eval case. Capture failure,
missing status, revision mismatch, or controller/workload evidence mismatch
must remain a failed or unknown capture rather than being hand-filled.
