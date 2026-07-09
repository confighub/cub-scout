# How To: Decide Whether Delivery Is Done

Use this guide when the operational question is:

> Can I move to the next delivery task, wait, or retry/investigate?

cub-scout does not deploy, sync, restart, or roll back anything. It reads the
cluster and connected intent evidence, then gives you a decision frame:

- `PASS`: evidence supports proceeding.
- `WATCH`: delivery or rollout is still converging; wait or re-check.
- `BLOCK`: evidence shows a failure or mismatch; investigate before proceeding.
- `INCONCLUSIVE`: required evidence is missing or unreadable; do not treat this
  as success.

## Fast Path

Start broad, then narrow.

```bash
# 1. Cluster or namespace overview.
./cub-scout doctor -n prod --format json

# 2. One resource, with current-generation rollout evidence when available.
./cub-scout explain deployment/api -n prod --format json

# 3. Delivery/controller status.
./cub-scout gitops status -n prod --format json

# 4. Desired/rendered/live agreement when connected intent is available.
./cub-scout compare three-way --scope namespace/prod --format json
```

For an auditable gate over a rendered object set:

```bash
./cub-scout receipt verify \
  --file out/manifests \
  --scope namespace/prod \
  --predicate workloads-converged \
  --grace-window 5m \
  --fail-on any-non-pass \
  --format json \
  --out delivery-readiness.receipt.json
```

## How To Read The Result

| Evidence | Meaning | Decision |
|---|---|---|
| `currentChange.verdict=PASS` and no blocking drift | Workload is current-generation converged at observation time. | Proceed, while remembering this is a point-in-time observation. |
| `currentChange.verdict=WATCH` with `reason=stale_generation` | The API has not yet observed the desired generation. | Wait and re-check; do not call it done. |
| `currentChange.verdict=WATCH` with `reason=rollout_progressing` | Current generation is observed and still progressing. | Wait, or extend the grace window if your rollout is expected to be slow. |
| `currentChange.verdict=BLOCK` with `reason=runtime_failed` | Runtime evidence, such as pod/container failure, is present. | Investigate application/runtime symptoms before retrying delivery. |
| `currentChange.verdict=BLOCK` with `reason=runtime_failed`, `rollout_failed`, or `progress_stalled` | Runtime or rollout evidence is blocking the current-generation change. | Investigate workload/controller symptoms before retrying delivery. |
| `compare three-way` divergence or `compare source-truth` mismatch | Live state, rendered state, or controller source evidence disagrees with intent. | Investigate drift/source mismatch before proceeding. |
| `INCONCLUSIVE` or omitted `currentChange` | cub-scout could not collect enough evidence for this resource or kind. | Treat as a proof gap; inspect raw cluster/controller evidence. |

## What Cub-Scout Checks

For workload rollout evidence, cub-scout uses:

- Kubernetes status and kstatus classification.
- `metadata.generation` vs `status.observedGeneration`.
- progress clock fields where available.
- related pod waiting reasons such as `CrashLoopBackOff` or `ImagePullBackOff`.
- optional desired object sets for `receipt verify --predicate
  workloads-converged`.

Stale generation matters: if `status.observedGeneration` is behind
`metadata.generation`, cub-scout reports the current change as not complete
instead of treating an old ready condition as proof.

## Separate Delivery From Runtime Health

The same release can fail in different places:

| Symptom | Likely area | Start with |
|---|---|---|
| Controller not ready, missing source, failed apply | delegated delivery | `gitops status`, `trace`, `map activity` |
| Desired/rendered/live disagreement | drift or source mismatch | `compare three-way`, `compare source-truth` |
| Generation observed but pods fail | runtime/application | `explain`, `doctor`, `scan` |
| Action event exists before failure | operator or automation action context | `map activity --owner Flux --format json`, `trace`, `explain` |

cub-scout surfaces this evidence; it does not own the final application-success
policy.

## Keep A Receipt For The Decision

When a pipeline, incident, or review needs durable evidence, use a receipt.

```bash
./cub-scout receipt verify \
  --file out/manifests \
  --scope namespace/prod \
  --predicate workloads-converged \
  --save \
  --out delivery-readiness.receipt.json

./cub-scout receipt validate delivery-readiness.receipt.json
```

The receipt is an immutable record of what cub-scout observed at `verifiedAt`.
It does not prove the workload stays healthy forever; re-run the check for a new
decision.

## Example Fixture

See [`examples/live-delivery-observability`](../../examples/live-delivery-observability/)
for a review fixture that includes:

- desired vs observed Deployment shape
- aggregate delivery status
- audited action event metadata
- stale-generation rollout evidence
- runtime pod failure evidence
