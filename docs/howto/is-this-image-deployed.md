# Is This Image Deployed?

Use `release check --check-running-image` when you have a digest-pinned OCI
configuration bundle and want bounded, read-only evidence from a selected
controller and target. The check does not search a cluster for an image, render
or publish configuration, deploy changes, or test application behavior.

The complete Deployment proof is **v2.12.1 (release pending)**. The published
`v2.12.0` check compares pod-reported image IDs, but its label-selected reads do
not prove complete per-pod execution or ownership.

## Start With The Identity

Supply the literal configuration bundle consumed by the selected controller.
Keep these identities separate:

| Identity | Example | Meaning |
|---|---|---|
| Configuration bundle | `oci://registry.example/config/payments@sha256:<config-digest>` | Literal Kubernetes YAML/JSON supplied through `--bundle`. |
| Intended image | `registry.example/payments@sha256:<image-digest>` | Image declared by the bundle. |
| Observed image | `status.containerStatuses[].imageID` | Runtime identity reported by a Pod. |

For a ConfigHub release, use the publisher's registry, space slug, and
`Release.ManifestDigest`:
`oci://<registry>/space/<space-slug>@<Release.ManifestDigest>`.
Do not substitute `Release.Digest`, a container-image digest, a mutable tag, or
a release number. A desired URL pin is input, not observed source evidence.

If you have only an image reference, there is no dedicated cluster or fleet
search. Use `./cub-scout map list` and
`./cub-scout trace deploy/payments -n payments` to identify the workload, then
obtain the exact bundle. Git, Helm, and rendered YAML are not direct inputs;
Scout does not render the OCI bundle.

## Run The Check

Registry and Kubernetes credentials must already be available. Required scope
flags are `--bundle`, `--controller`, `--api-version`,
`--controller-namespace`, and `--kube-context`. Use
`--controller-context <management-context>` when the controller is in another
context. The controller namespace is not the workload namespace.

Kubernetes access needs discovery and GET permissions for the controller,
source, intended objects and ReplicaSet owners, plus namespace-scoped Pod LIST
permission. Missing access produces incomplete evidence, not a successful check.

### Argo CLI

```bash
./cub-scout release check \
  --bundle 'oci://registry.example/config/payments@sha256:<config-digest>' \
  --controller Application/payments \
  --api-version argoproj.io/v1alpha1 \
  --controller-namespace argocd \
  --kube-context production \
  --check-running-image \
  --max-pods 50 \
  --format json \
  --out release-check.json \
  --fail-on any-non-pass
```

### Flux CLI

```bash
./cub-scout release check \
  --bundle 'oci://registry.example/config/payments@sha256:<config-digest>' \
  --controller Kustomization/payments \
  --api-version kustomize.toolkit.fluxcd.io/v1 \
  --controller-namespace flux-system \
  --kube-context production \
  --check-running-image \
  --max-pods 50 \
  --format json \
  --out release-check.json \
  --fail-on any-non-pass
```

Supported adapters are a single-source Argo CD Application using a native OCI
source, or a Flux v1 Kustomization with a v1 OCIRepository. General
HelmRelease support, chart rendering, and other plugin source shapes are not
inputs to this check.

## Interpret The Result

Inspect `runningImage.workloads[]` for per-workload, per-Pod, and per-container
evidence. The overall verdict uses `BLOCK > INCONCLUSIVE > WATCH > PASS`.

| Image result | Stage verdict | Meaning |
|---|---|---|
| `match` | `PASS` | **v2.12.1 (release pending):** required image, running, readiness, and supported Deployment ownership/replica evidence matched. |
| `mismatch` | `BLOCK` | Compatibility vocabulary. An unresolved digest form is not treated as a wrong image. |
| `unknown` | `INCONCLUSIVE` | Identity or completeness was not confirmed. Inspect `reason`; do not treat it as a match. |

With `--check-running-image`, no supported workloads is `unknown` /
`INCONCLUSIVE`, not a configuration-only positive result. Without the flag, a
passing report says nothing about Pod image identity. An image match cannot
override failed configuration or controller checks.

For the complete field contract, see the
[configuration release check JSON contract](../reference/json-contracts.md#configuration-release-check)
and the [worked example](../../examples/oci-release-check/).

### Scripts And CI

With `--fail-on any-non-pass`, exit 0 means the overall report passed, exit 2
means a non-passing report was preserved, and exit 1 means argument, setup, or
output failure. Without the gate, exit 0 only means a report was produced.
Keep JSON stdout separate from stderr and check the current exit status before
using an existing report file. The CLI and plugin produce the same report.

```bash
jq '{verdict, headline, nextStep, stages, requestCounts,
     runningImage: {verdict: .runningImage.verdict,
                    workloads: .runningImage.workloads}}' release-check.json
```

No TTY is required. The interactive TUI is optional: use `--interactive` and
omit `--format`, `--out`, and `--fail-on`. MCP uses
`release_check` with `check_running_image: true`; `max_pods` is optional.
Watch and in-cluster bot do not schedule release/image checks.

## Ways To Run It

| Interface | Command |
|---|---|
| Standalone | `./cub-scout release check ...` |
| cub plugin | `cub scout release check ...` with the same flags |
| MCP | `release_check` with `check_running_image: true` |
| Optional TUI | `./cub-scout release check ... --interactive` |

Watch and bot do not run this check automatically.

## What Complete Deployment Proof Checks

For each digest-pinned Deployment, the bounded check performs a structured-
selector Pod LIST, one exact GET for each distinct ReplicaSet owner, and a final
Deployment re-read. The selector preserves `matchLabels` and
`matchExpressions`; an empty or invalid selector never widens to a
namespace-wide list.

The proof requires:

1. Every selected Pod has a unique name and UID, is not terminating, and has
   the Deployment selector labels.
2. Each Pod has one controlling `apps/v1` ReplicaSet owner. The Pod owner UID,
   ReplicaSet UID, and controlling Deployment UID agree.
3. The current ReplicaSet template matches the Deployment template after
   removing only `pod-template-hash`.
4. `generation == status.observedGeneration`; desired replicas are positive;
   `replicas`, `updatedReplicas`, `readyReplicas`, and `availableReplicas` equal
   desired replicas.
5. Pod count equals desired replicas; every Pod is `phase=Running` with exactly
   one `PodReady=True` condition.
6. Every intended regular container appears in every Pod status with the exact
   digest, `state.running`, and `ready=true`.

Evidence includes `deployment.complete`, replica counts, `ownedPods`, `pods[]`,
`replicaSetName`, `replicaSetUID`, per-container results, and optional
`observedAt`. `deployment.complete` describes ownership and replica coverage;
it does not by itself prove image equality. Sequential reads are bounded by
`startedAt` and `finishedAt`; this is not an atomic snapshot or continuous
health guarantee.

Full ownership proof is Deployment-only. Direct Pods retain exact-object
evidence but receive no Deployment-completion claim. StatefulSets, DaemonSets,
and Jobs are `unknown` with `workload-ownership-unsupported`.

## Limits And Read Boundaries

- `v2.12.0` does not prove all replicas: label-selected image IDs can hide
  missing Pod status, running-state, and owner-UID evidence.
- In **v2.12.1 (release pending)**, incomplete, ambiguous, capped, denied,
  stale, racing, terminating, foreign, duplicate, or zero-replica evidence
  remains `unknown` / `INCONCLUSIVE`.
- Malformed entries, missing container names/images, and duplicate intended
  names produce `intended-containers-malformed`; entries are not dropped.
- Only regular `containers` are assessed. `initContainers` and
  `ephemeralContainers` are excluded. Application behavior and traffic are not
  assessed.
- `--max-pods` defaults to 50 and accepts 1..200. Pod LIST responses are capped
  at 2 MiB; each read has a 10-second deadline and the full check has a
  90-second deadline. No pagination or hidden retry is used.
- Index/platform differences remain `digest-form-unresolved`; no registry image
  resolution is performed.

Without image checks, N desired objects cost at most `2N + 4` Kubernetes
requests for Argo or `2N + 8` for Flux. Image proof adds at most `2R + 3` per
Deployment, where R is the number of distinct ReplicaSet owners. The
one-Deployment/two-Pod fixtures require exactly **11 Argo / 15 Flux** requests;
capped or denied Pod lists use **7 / 11**. These are request budgets, not
latency SLAs. See the [full budgets](../../examples/oci-release-check/#read-budget).

For unattended use, provide non-interactive credentials and an outer timeout.
The v2.12.1 reader (release pending) cancels exec-auth helpers, caps
credential output at 1 MiB, suppresses helper stderr, and caps HTTP error
bodies before client-go buffers them. Linux/macOS also terminate the helper
process group; Windows descendant cleanup still needs the enclosing CI
job/process timeout. Failed reads never become positive evidence.

## Connected Evidence

An exact connected release-history row requires the active ConfigHub registry,
source/status binding, `Release.ManifestDigest`, and current controller
evidence to agree. `Release.Digest` is bundle content, not an image digest.
Argo checks `status.sync.comparedTo`; Flux checks the current generation,
`observedGeneration`, Ready condition, and `OCIRepository URL`. Missing,
stale, denied, or inconsistent reads produce an omission. The map TUI does not
display these connected release rows.

Authenticated local ConfigHub through an HTTPS proxy to Argo and Flux has passed
in standalone and plugin modes. Missing or invalid credentials and denied TLS
remain `INCONCLUSIVE`. See the [image verification readiness record](../releases/image-verification-readiness.md)
and the [read-only live acceptance runner](../../examples/oci-release-check/LIVE-VALIDATION.md).
