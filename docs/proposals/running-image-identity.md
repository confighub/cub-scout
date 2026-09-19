# Running-Image Identity Check

Status: **UNRELEASED source-branch implementation in progress.** The latest
published `v2.12.0` binary has the earlier opt-in running-image slice and its
documented per-Pod completeness gaps. For current public usage, start with
[Is This Image Deployed?](../howto/is-this-image-deployed.md).

## User Value

The exact configuration release check can show that a literal OCI bundle,
controller binding, authored configuration, and workload status agree. It does
not by itself answer:

> Is the intended image actually running in every replica?

This tier keeps three identities separate:

1. **Configuration-bundle digest:** identity of the OCI configuration artifact.
2. **Intended container-image reference:** the image declared by the workload,
   preferably pinned as `repository@sha256:...`.
3. **Pod-reported container-image digest:** the runtime identity in
   `.status.containerStatuses[].imageID`.

The bundle digest is never compared with either image identity. A mutable tag
cannot be resolved from cluster reads and remains unknown.

## Contract

The tier is opt-in and off by default: `release check --check-running-image`,
MCP `check_running_image: true`, or the equivalent interactive TUI option.
`--max-pods` defaults to 50 and accepts 1..200. The CLI, plugin, MCP, and TUI
share one provider; watch and bot do not schedule the check.

The source-branch implementation currently proves complete image rollout only
for `apps/v1` Deployments:

1. Run one bounded structured-selector Pod LIST.
2. Read each distinct controlling ReplicaSet exactly once.
3. Re-read the exact Deployment after the Pod and ReplicaSet reads.
4. Verify each Pod's controlling owner reference and UID, the ReplicaSet's
   identity and controlling Deployment UID, and the current ReplicaSet template
   against the Deployment template.
5. Require a positive desired replica count, `generation ==
   status.observedGeneration`, and `replicas`, `updatedReplicas`,
   `readyReplicas`, and `availableReplicas` all equal to desired.
6. Require an exact complete Pod set: no duplicate, terminating, old, foreign,
   or ambiguous Pods; exact count; `phase=Running`; exactly one
   `PodReady=True` condition.
7. Require every intended regular container in every Pod status to have the
   exact digest, `state.running`, and `ready=true`.

The report's `runningImage.workloads[]` contains optional `observedAt` at the
Pod LIST time. Deployment evidence contains `complete`, `reason`, `uid`,
`generation`, `observedGeneration`, `desiredReplicas`, `replicas`,
`updatedReplicas`, `readyReplicas`, `availableReplicas`, and `ownedPods`.
Per-Pod evidence contains `name`, `uid`, `replicaSetName`, `replicaSetUID`, and
per-container results. `deployment.complete` means ownership and coverage were
proved; a workload `verdict: match` additionally requires all intended regular
container statuses in every Pod to match.
The match also requires the exact intended digest, `state.running`, and
`ready=true` for every intended regular container status.

Direct Pods retain exact-object evidence. StatefulSets, DaemonSets, and Jobs do
not receive a fabricated ownership/completion proof; this tier reports
`unknown` with `workload-ownership-unsupported` for them.
Direct Pods still require no deletion timestamp, `phase=Running`,
`PodReady=True`, and every intended regular container running and ready; they
receive no Deployment-completion claim.

## Verdicts And Degradation

| Result | Meaning |
|---|---|
| `match` | The intended regular-container digest, running state, readiness, and, for a Deployment, complete ownership/replica evidence were observed. |
| `mismatch` | Legacy vocabulary retained for compatibility; current builders do not emit `BLOCK` for an unresolved digest difference. |
| `unknown` | Required identity or completeness evidence is unavailable or ambiguous. |

Missing or ambiguous data, capped coverage, RBAC denial, stale generation,
observation races, zero replicas, terminating Pods, duplicate or foreign Pods,
old ReplicaSets, and unsupported ownership all remain `unknown` and map to
`INCONCLUSIVE`. Mutable tags are unknown without a Pod read. An index digest
versus a platform manifest digest is also unknown with
`digest-form-unresolved`; no registry image resolution is performed.

Only regular `containers` are included. `initContainers`, ephemeral containers,
application behavior, traffic, and SLOs are outside this tier.

## Boundaries

- The full report's `startedAt` and `finishedAt` bound sequential reads. The
  result is not an atomic snapshot and is not a continuing health guarantee.
- Each Kubernetes read has a 10-second deadline and 2 MiB response cap; the
  full check has a 90-second deadline. The Pod LIST is bounded by `--max-pods`;
  there is no pagination or hidden retry.
- The image check does not render, publish, authenticate to a registry, write
  credentials, deploy, repair, or claim application success.
- Connected OCI source correlation requires the active ConfigHub registry to
  agree with the configured registry and uses the reported source revision
  (`Release.ManifestDigest`). This exact registry-match behavior is
  **UNRELEASED** source-branch behavior. A desired URL pin is not observed
  source evidence, `Release.Digest` is bundle content, and a target-only row is
  scope context rather than proof of release execution.

## Success Criteria

Deterministic, offline fixtures must cover:

- complete Argo and Flux release checks through CLI, plugin, stdio MCP, and TUI
  provider paths;
- a synthetic non-pullable digest Deployment fixture with every required owner,
  status, Pod, ReplicaSet, and container field;
- incomplete Pod count, duplicate/terminating/old/foreign Pod, bad owner UID,
  missing ReplicaSet, stale generation, zero replicas, changed Deployment,
  read-denied, Pod LIST cap, and malformed/ambiguous status cases;
- mutable tags, digest-form-unresolved, init/ephemeral exclusion, direct Pod
  evidence, and unsupported StatefulSet/DaemonSet/Job ownership;
- exact request accounting, including 11 Kubernetes requests for the strict
  Argo fixture with no ConfigMap in the fixture; and
- identical structured semantics across ASCII, JSON, Markdown, plugin, MCP,
  and TUI output.

Reproduce the public integration proof with:

```bash
go test ./cmd/cub-scout -run 'TestReleaseCheck(RunningImage|CLIAndMCP)$' -count=1 -v
```

Positive results are fixture-backed read-only evidence, not authenticated
production authority or an application-success test.
