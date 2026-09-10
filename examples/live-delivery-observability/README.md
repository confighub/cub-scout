# Live Delivery Observability Fixture

This fixture demonstrates the live-delivery diagnostic questions in one small
recorded object set:

- aggregate delivery status from first-class controller resources
- audited user-action events recorded as Kubernetes Events
- desired-vs-observed drift shape
- current-generation rollout evidence, including stale generation and runtime
  pod symptoms
- bounded ConfigHub delivery evidence shape for release history, unit events,
  event-consumer health, and live-status writeback

The files are review fixtures, not a guaranteed `kubectl apply` recipe. Some
objects include `status` fields that are normally written by controllers or the
API server.

For the operator workflow this fixture supports, see
[`docs/howto/delivery-readiness-decision.md`](../../docs/howto/delivery-readiness-decision.md).

## Files

| File | Purpose |
|---|---|
| `desired.yaml` | Intended Deployment shape used as the comparison baseline. |
| `observed.yaml` | Recorded live objects: aggregate resource, workload, pod symptom, and audited action event. |
| `confighub-delivery-evidence.json` | Example `gitops status --with-confighub --format json` evidence envelope. |

## Review Commands

```bash
# Inspect the objects and fields a reviewer should expect cub-scout to parse.
grep -n "kind:\\|event.toolkit.fluxcd.io\\|observedGeneration\\|CrashLoopBackOff" \
  examples/live-delivery-observability/observed.yaml

# Against a cluster with equivalent objects:
./cub-scout map activity --owner Flux --format json
./cub-scout gitops status --format json
./cub-scout gitops status --with-confighub --confighub-space prod --confighub-since 24h --format json
./cub-scout explain deployment/api -n prod --format json
./cub-scout doctor -n prod --format json

# With desired.yaml as the intended object set and equivalent live objects:
./cub-scout receipt verify \
  --file examples/live-delivery-observability/desired.yaml \
  --scope namespace/prod \
  --predicate workloads-converged \
  --format json
```

## Expected Evidence

- `map activity` should surface the `WebAction` event as `source=k8s.action`
  with `actor`, `subject`, and raw action metadata.
- `gitops status`, `trace`, and map deployer surfaces should treat the
  aggregate delivery resource as a first-class controller object.
- `gitops status --with-confighub` should keep release history, unit events,
  live-status writeback, and event-consumer workload evidence separate under
  `deliveryEvidence`.
- `explain` and `doctor` should report the Deployment current change as
  non-PASS because `status.observedGeneration` is behind
  `metadata.generation` and the related Pod has `CrashLoopBackOff` evidence.
- `receipt verify --predicate workloads-converged` should use the same rollout
  decision model as the diagnostic surfaces.
