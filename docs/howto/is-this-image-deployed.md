# Is This Image Deployed?

This guide separates the image evidence available in the shipped `v2.12.0`
binary from the stricter implementation currently on the source branch.

`v2.12.0` has an opt-in `release check --check-running-image` slice. It compares
digest-pinned intended container images with pod-reported image IDs, but its
label-selected observations do not prove complete per-pod execution or
ownership. The stricter Deployment proof described below is **UNRELEASED**:
it is source-branch behavior and must not be attributed to `v2.12.0`.

This is image-identity evidence for a known configuration release, not an
unrestricted image search, an application-health check, or a deployment command.

## Three Questions To Ask

| Question | What the shipped v2.12.0 check can say | What the UNRELEASED source branch adds |
|---|---|---|
| **Do I have the exact release identity?** | Only when given a digest-pinned OCI configuration bundle and an explicit controller and target. The bundle digest and image digest are separate. | No change to the identity boundary. A desired URL pin is input; it is not an observed source revision. |
| **Are all replicas running that image?** | No. A bounded label-selected pod read can compare reported IDs, but missing status, ownership, and running-state gaps can be masked. | For a Deployment, verifies the complete current pod set, owner UID chain, current ReplicaSet template, running/ready containers, and replica/status counts. |
| **What does an incomplete proof mean?** | Some missing evidence is `unknown` / `INCONCLUSIVE`, but the gaps above can still produce a match. | Missing, ambiguous, capped, denied, stale, racing, zero-replica, or unsupported evidence cannot produce a complete Deployment match. |

## Five Different Questions

| User question | What cub scout checks | What that does not prove |
|---|---|---|
| Did the expected configuration release reach this target? | Bundle content digests and supported controller/source/target evidence. | Publication authority, controller liveness, execution history, or application success. |
| Does live configuration specify the intended image? | Authored manifest fields, including container image references, against live configuration. | That pods started with those images. |
| Do observed pods report the intended image digest? | With `--check-running-image`, intended regular-container digests against pod `status.containerStatuses[].imageID`. | Registry identity resolution, init/ephemeral containers, process reload, or application success. |
| Has the workload converged? | A separate workload-controller convergence assessment. | Application-specific functional success. |
| Is the application successfully serving users? | Not assessed by this command. | Functional tests, traffic checks, and SLOs remain with application-health tools. |

## Start With What You Have

- **An exact image identity:** use a digest-pinned image reference in the
  supplied configuration bundle, such as `registry.example/payments@sha256:...`.
  A mutable tag such as `payments:v2` remains `unknown` because cluster reads
  cannot prove which digest the tag meant.
- **An image reference only:** there is no dedicated image-reference search that
  finds every deployment across a cluster or fleet. Use `./cub-scout map` and
  `./cub-scout trace deploy/payments -n payments` to identify ownership and
  source, then obtain the exact configuration bundle before claiming identity.
- **A Git repo, Helm chart, or rendered YAML only:** these are not direct inputs
  to this release check. Scout does not render or publish the OCI bundle.

## Why OCI Is Involved

The bundle is an OCI artifact containing literal Kubernetes YAML/JSON, not a
Helm chart. Keep these identities separate:

| Identity | Example | Role |
|---|---|---|
| Configuration-bundle digest | `oci://registry.example/config/payments@sha256:<config-digest>` | The intended manifest bundle supplied through `--bundle`. |
| Intended container-image reference | `registry.example/payments@sha256:<image-digest>` | The image declared by a workload in that bundle. |
| Pod-reported image digest | `status.containerStatuses[].imageID` | The runtime identity observed from pod status. |

The configuration-bundle digest is never compared with a container-image
digest. A configuration-only change can therefore be checked even when the
application image is unchanged.

When connected delivery history is shown elsewhere, an exact release row joins
only when the active ConfigHub registry agrees with the configured registry and
the reported source manifest revision (`Release.ManifestDigest`). This exact
registry-match behavior is **UNRELEASED** source-branch behavior. Recognizing a
host alone is not enough. A desired URL pin is not an observation, and
`Release.Digest` is bundle content, not an OCI manifest or image digest. A
target-only row is scope context, not proof that this release executed.
For `trace --with-confighub` and `explain --with-confighub`, this exact source
join is also **UNRELEASED** and requires one bounded current source read. Argo
checks the current Application spec source against `status.sync.comparedTo`
and requires the reported digest to stay unchanged. Flux checks the current
generation against `observedGeneration`, a current Ready condition, and the
current OCIRepository URL and artifact revision. The additive
`ociSourceVerified`/`ociSourceRead` evidence records this read; if it is
missing, stale, denied, or inconsistent, no exact release row is attached.
Registry verification uses a fresh configured-registry lookup; the
parser recognition cache may last 30 seconds but is never authority.
These connected history joins are exposed in CLI trace/explain and single-resource
receipts; the map TUI does not yet display the connected release rows. The
interactive release check described below is a separate, supported TUI surface.

## Run The Check

The command below is available in `v2.12.0` for the shipped, bounded image
slice. It also shows the opt-in flag used by the **UNRELEASED** source branch.
The standalone command, `cub scout release check`, MCP `release_check`, and the
interactive TUI use one provider; watch and bot do not schedule release checks.

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

Required scope flags are `--bundle`, `--controller`, `--api-version`,
`--controller-namespace`, and `--kube-context`. `--controller-namespace`
locates the delivery controller; it does not set the workload namespace.
Registry reads use existing credentials when needed. Scout does not log in,
write credentials, render, deploy, or repair.

Supported release adapters are a single-source Argo CD Application using a
native OCI source, or a Flux v1 Kustomization with a v1 OCIRepository. The
whole literal bundle is required. General HelmRelease support, chart rendering,
and other render/plugin source shapes are not inputs to this check.

## Read The Result

The report keeps `bundle`, `controller`, `configuration`, `workloads`, and the
optional `running-image` stage separate. Inspect `runningImage.workloads[]` for
per-workload, per-pod, and per-container evidence.

| Image result | Stage verdict | Meaning |
|---|---|---|
| `match` | `PASS` | **UNRELEASED only:** the intended regular-container digest, running state, readiness, and, for a Deployment, complete ownership/replica proof were observed. The shipped v2.12.0 match has the documented per-Pod gaps. |
| `mismatch` | `BLOCK` | Legacy vocabulary retained for compatibility. The current builders emit no mismatch/BLOCK for an unresolved digest difference; v2.12.0 could report an index/platform difference as a false-alarm mismatch. |
| `unknown` | `INCONCLUSIVE` | Identity or completeness could not be confirmed. Inspect `reason`; do not treat it as a match. |

Mutable tags, missing or ambiguous status, RBAC failures, capped coverage,
unsupported ownership, stale generation, observation races, and zero replicas
are unknown. An index digest versus a platform manifest digest is
`unknown` with `digest-form-unresolved`; no registry image resolution is
performed.

The overall verdict uses `BLOCK > INCONCLUSIVE > WATCH > PASS`. With
`--check-running-image`, a request that finds no supported workloads is
`unknown` / `INCONCLUSIVE`, not a configuration-only positive result. Without
`--check-running-image`, a passing report says nothing about pod image identity.
`--fail-on any-non-pass` gates the result after preserving the report; exit 0
without it means that a report was produced, not that it passed.
The outer JSON report is not signed or an immutable receipt. Nested configuration
and workload receipts may carry their own fingerprints.

## UNRELEASED Deployment Proof

For each digest-pinned Deployment, the source-branch implementation performs a
bounded structured-selector Pod LIST, one exact GET for each distinct
ReplicaSet owner, and a final exact Deployment re-read.

The structured selector preserves both `matchLabels` and `matchExpressions`; an
empty or invalid selector never widens the read to a namespace-wide list.

The proof requires all of the following in the same bounded observation:

1. Every selected Pod has a unique name and UID, is not terminating, and has
   the Deployment selector labels.
2. Each Pod has one unambiguous controlling `apps/v1` ReplicaSet owner. The
   Pod owner UID must match the exact ReplicaSet GET, whose controlling
   Deployment owner UID must match the live Deployment UID.
3. The current ReplicaSet template matches the live Deployment template after
   removing only the controller's `pod-template-hash` label. Old, foreign, or
   ambiguous ownership is unknown.
4. Deployment `generation == status.observedGeneration`, desired replicas are
   positive, and `replicas`, `updatedReplicas`, `readyReplicas`, and
   `availableReplicas` each equal desired replicas.
5. The number of Pods is exactly the desired replica count. Every Pod is
   `phase=Running` with exactly one `PodReady=True` condition.
6. Every intended regular container appears in every Pod's status with an exact
   digest, `state.running`, and `ready=true`.

The JSON evidence includes a `deployment` object with `complete`, `reason`, `uid`,
`generation`, `observedGeneration`, `desiredReplicas`, `replicas`,
`updatedReplicas`, `readyReplicas`, `availableReplicas`, and `ownedPods`.
`deployment.complete` means ownership and replica coverage only. A workload
`verdict: match` additionally requires every intended regular container status
in every observed Pod to have the exact digest, `state.running`, and
`ready=true`.
Each workload also includes `pods[]` with Pod `name`, `uid`, `replicaSetName`,
`replicaSetUID`, and per-container results. `observedAt` is an optional
timestamp for the Pod LIST. The report's `startedAt` and `finishedAt` bound
sequential reads; this is not an atomic snapshot or a continuing health
guarantee.

Full ownership and completion proof is supported only for Deployments. Direct
Pods retain exact-object evidence, but still require no deletion timestamp,
`phase=Running`, `PodReady=True`, and every intended regular container in
their own status to be running, ready, and at the exact digest. Direct Pods do
not receive a Deployment-completion claim. StatefulSets, DaemonSets, and Jobs
are `unknown` with `workload-ownership-unsupported` for this tier, not
universal positive claims.

## Ways To Run It

| Surface | Availability |
|---|---|
| Standalone client | `./cub-scout release check --format ascii|json|md`. |
| ConfigHub plugin | The same arguments after `cub scout release check`. |
| MCP | `release_check` with `check_running_image: true`; `max_pods` is optional. |
| Interactive TUI | Use `--interactive` and refresh with `r`; do not combine it with `--fail-on`. |
| Watch and in-cluster bot | No automatic release/image checks. |

## Limits Before Trusting A Match

- **Shipped v2.12.0 gap:** label-selected pod evidence can pool image IDs by
  container name. Missing status arrays, `state.running`, and owner UID chains
  are not fully verified, so a match is not an all-replicas guarantee.
- **UNRELEASED scope:** complete proof is Deployment-only. Missing, ambiguous,
  capped, denied, stale, racing, terminating, foreign, old, duplicate, or
  zero-replica evidence stays `unknown` / `INCONCLUSIVE`.
- **Digest forms:** registry image resolution is not performed. Index/platform
  differences remain `digest-form-unresolved`; do not call them a wrong image.
- **Container scope:** only regular `containers` are assessed. `initContainers`
  and `ephemeralContainers` are excluded. Application behavior and traffic are
  excluded.
- **Bounds:** `--max-pods` defaults to 50 and accepts 1..200. The Pod LIST is
  bounded, responses are capped at 2 MiB, each read has a 10-second deadline,
  and the full check has a 90-second deadline. No pagination or hidden retry.
- **Observation window:** reads are sequential. The final Deployment re-read
  detects a race; it cannot turn the report into an atomic or continuous claim.

These limits describe evidence, not a deployment action. The [JSON contract](../reference/json-contracts.md#configuration-release-check)
and [worked example](../../examples/oci-release-check/) provide machine-readable
fields, adapters, and request budgets.
