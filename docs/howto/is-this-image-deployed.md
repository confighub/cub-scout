# Is This Image Deployed?

**Available in cub scout v2.11.0.** Use `release check --check-running-image`
to compare the container images declared in an OCI configuration bundle with
the image IDs reported by pods in a selected cluster. It is read-only and
opt-in, and does not require a ConfigHub account.

This is **image-identity evidence for a known release**, not an unrestricted
image search or proof that every replica is currently running successfully.
This guide describes the v2.11.0 implementation at tag commit `89f0bf2`.

## Five Different Questions

| User question | What cub scout checks | What that does not prove |
|---|---|---|
| Did the expected configuration release reach this target? | Bundle content digests and the supported controller's repository, revision, destination and inventory evidence. | Release publication authority, controller liveness or execution history. |
| Does live configuration specify the intended image? | Authored manifest fields, including container image references, against live configuration. | That pods have started with those images. |
| Do observed pods report the intended image digest? | With `--check-running-image`, intended container image digests against pod `status.containerStatuses[].imageID`, grouped by container name. | Complete per-pod coverage, current container execution or traffic success; see the limitations below. |
| Has the workload converged? | A separate workload-controller convergence assessment using current-generation status. | Application-specific functional success. |
| Is the application successfully serving users? | Not assessed by this command. | Functional tests, traffic checks and SLOs remain with application-health tools. |

## Start With What You Have

- **An image reference only:** v2.11.0 has no dedicated image-reference search
  command that finds every deployment across a cluster or fleet. Identify the
  workload and its intended configuration first. `./cub-scout map` and
  `./cub-scout trace deploy/payments -n payments` can help investigate ownership
  and source; they do not replace digest verification.
- **A literal OCI configuration bundle, controller and target:** use the check
  below. Both the configuration bundle and the container image need immutable
  digests for the respective identity checks to confirm a match.
- **A Git repo, Helm chart or rendered YAML only:** those are not direct inputs
  to `release check`. Existing comparison workflows can inspect desired/live
  configuration, but this running-image check requires the OCI bundle. Scout
  does not render or publish it for you.

## Why OCI Is Involved

An OCI registry can hold a configuration artifact as well as application images.
Here the artifact contains **literal Kubernetes YAML/JSON**, not a Helm chart.
Keep these three identities separate:

| Identity | Example | Role |
|---|---|---|
| Configuration-bundle digest | `oci://registry.example/config/payments@sha256:<config-digest>` | Supplied through `--bundle`; identifies the intended manifest bundle. |
| Intended container-image reference | `registry.example/payments@sha256:<image-digest>` | Declared in a workload inside that bundle. |
| Pod-reported image digest | `status.containerStatuses[].imageID` | Compared with the intended container-image digest. |

The **configuration-bundle digest is never compared to a container-image
digest**. Configuration-only changes, such as a changed ConfigMap, can still be
checked when the application image has not changed.

## Run The Check

Use a v2.11.0 or newer binary with this flag. These examples use the local
`./cub-scout` executable; the equivalent plugin command is
`cub scout release check`.

You need read access to the selected controller/source, desired live objects and
pods. Registry reads use existing Docker credentials when needed; Scout does
not log in or change credentials. Replace the example repository, application,
namespaces and context with your own. `<config-digest>` is the bundle's actual
64-character lowercase SHA-256 value, not the image digest.

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
  --out release-check.json
```

The bundle must declare the intended image, for example
`image: registry.example/payments@sha256:<image-digest>`, with its actual digest.
A mutable tag such as `payments:v2` cannot confirm image identity in this check.
Every namespaced manifest must include `metadata.namespace`.
`--controller-namespace` locates the delivery controller object; it does not
set the workload namespace.

The supported release bindings are a **single-source Argo CD Application with
a literal whole-bundle directory source**, or a **Flux v1 Kustomization with a
v1 OCIRepository**. For Flux, use `--controller Kustomization/payments` and
`--api-version kustomize.toolkit.fluxcd.io/v1`; controller and target must use
the same context. For Argo, `--controller-context` can select a management
cluster, but the Application destination server must bind to the target.
Named destinations, rendering/transforms and other unsupported shapes remain
inconclusive. [Full adapter boundaries](../../examples/oci-release-check/#input-and-adapter-boundaries).

## Read The Result

The report keeps `bundle`, `controller`, `configuration`, `workloads` and the
optional `running-image` stage separate. Inspect `runningImage.workloads[]` for
container names, intended references, reported digests, reasons and coverage.

| Image result | Stage verdict | Meaning and next step |
|---|---|---|
| `match` | `PASS` | The compared image IDs match the intended digests within the coverage below. Also inspect the configuration and convergence stages. |
| `mismatch` | `BLOCK` | A comparable reported digest differs. Inspect the reported images and registry manifests; a multi-architecture index/platform difference can be legitimate. |
| `unknown` | `INCONCLUSIVE` | Identity could not be confirmed, for example a mutable tag, denied pod read, unsupported selector or capped coverage. Inspect the reason; do not treat it as a match. |

The overall verdict uses `BLOCK > INCONCLUSIVE > WATCH > PASS`; an image-stage
match cannot override another stage's failure. Without `--check-running-image`,
a passing release report says nothing about pod image identity. With no supported
workloads, the image stage is `NOT_ASSESSED`, not a positive result.

For a script gate, add `--fail-on any-non-pass`: a matching non-pass verdict
exits 2 after preserving the report. Without it, exit 0 means the report was
produced, not that it passed. The outer JSON report is not signed or an immutable
receipt; its nested configuration/workload receipts have their own fingerprints.

## Ways To Run It

| Surface | Availability |
|---|---|
| Standalone client | The command above, with `--format ascii`, `json` or `md`. |
| ConfigHub plugin | Same arguments after `cub scout release check`. |
| MCP | Tool `release_check` with `check_running_image: true`; required keys are `bundle`, `controller`, `api_version`, `controller_namespace`, `context`. Optional `max_pods` sets the cap. |
| Interactive TUI | Keep the scope and image-check flags; replace `--format`/`--out` with `--interactive`. Press `r` for a fresh check. Do not combine interactive mode with `--fail-on`. |
| Watch and in-cluster bot | Do not schedule release/image checks automatically in v2.11.0. |

## Limits Before Trusting A Match

- **Not an all-replicas-running guarantee.** The v2.11.0 collector pools image
  IDs by container name across selected pods. It skips missing status arrays
  and does not check `state.running`. A missing container status on one pod can
  be masked by a matching status on another. Workload convergence is separate.
- **Pod selection is not ownership proof.** Non-Pod workloads select pods with
  `spec.selector.matchLabels`; this tier does not verify the ownerReference/UID
  chain. Overlapping labels can include unrelated pods. `matchExpressions`
  are not evaluated; selectors with only expressions are unsupported.
- **Multi-architecture mismatch is not necessarily the wrong image.** The
  registry's index digest and a platform manifest digest can legitimately
  differ. v2.11.0 still reports `mismatch`/`BLOCK`, and its headline can say the
  intended image is not running. Read the caveat and resolve the relationship
  at the registry before interpreting that as a wrong-image finding.
- **Bounded coverage.** At most 100 desired objects; one selector-scoped pod
  list per eligible workload, default 50 pods, maximum 200. There is no
  pagination beyond that page. A continuation marks coverage as capped and
  yields `unknown` unless a mismatch has already been observed. Direct Pods
  reuse their existing live read; tag-only workloads skip
  the extra pod read.
- **Limited container/workload coverage.** Only ordinary `containers` are
  compared, not initContainers or ephemeral containers. Workload kinds are
  Deployment, StatefulSet, DaemonSet, Job and Pod; CronJobs and custom workload
  kinds are not assessed by this tier.
- **A snapshot, not continuing assurance.** Reads are sequential, not atomic.
  The check does not prove future health, process-level configuration reload,
  release authority or application success. It never deploys or repairs.

These are current implementation limits, not problems fixed by this guide.
The [collector](../../pkg/agent/running_image.go),
[pod-read integration](../../cmd/cub-scout/release_check.go) and
[worked example](../../examples/oci-release-check/) provide reviewable detail.
See the [JSON contract](../reference/json-contracts.md#configuration-release-check)
for automation and the [design history](../proposals/running-image-identity.md)
for planned extensions.
