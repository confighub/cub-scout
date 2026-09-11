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
- `map activity --with-confighub` should keep ConfigHub activity rows separate,
  and may attach live-status `deliveryEvidence` to matching Argo Application
  rows only when a non-wildcard ConfigHub space, observed Application Space ID,
  and unique Application name match. The Application must carry
  `confighub.com/space-id` or `confighub.com/SpaceID` metadata; scope selection
  and naming convention alone do not prove a join. Missing/conflicting IDs,
  duplicate names across namespaces, and duplicate statuses produce omissions.
- `explain` and `doctor` should report the Deployment current change as
  non-PASS because `status.observedGeneration` is behind
  `metadata.generation` and the related Pod has `CrashLoopBackOff` evidence.
- `receipt verify --predicate workloads-converged` should use the same rollout
  decision model as the diagnostic surfaces.

Run the offline correlation and read-budget proof from the repository root:

```bash
go test ./cmd/cub-scout -run 'TestActivityDeliveryIdentityAndReadBudget' -count=1
```

The fixture preserves an Argo failure even when separate ConfigHub evidence
reports PASS. It also proves namespace filtering cannot hide a name collision
and the join performs no additional Kubernetes reads beyond one Application list.

## Trusting Feedback Freshness

**Unreleased correction after v2.10.0.** The question is: "Does this report tell
me what is true now, or only what was last reported?"

[`freshness-cases.json`](freshness-cases.json) fixes the observer clock at
`2026-09-11T12:00:00Z` and the threshold at `15m`. Each case starts with reported
`Synced` / `Healthy` / `Succeeded`, source `argobot`, app `api`, space `team-a`
(`space-a`) and revision `sha256:abc`. `failed: true` changes the three reported
states to `OutOfSync` / `Degraded` / `Failed`; omitted `observedAt` stays absent.

| Report timestamp | Freshness | Positive report | Failure report |
|---|---|---|---|
| Valid, not future, at most 15 minutes old | `fresh` | `PASS` | `BLOCK` |
| More than 15 minutes old | `stale` | `WATCH` | `WATCH` |
| Missing, malformed, zero, or future | `unknown` | `INCONCLUSIVE` | `INCONCLUSIVE` |

Empty status remains `INCONCLUSIVE`. Exact-now and exact-threshold timestamps
are valid. Even one nanosecond in the future is unknown: there is no hidden
clock-skew allowance. Offsets/fractions and the existing legacy UTC format are
covered. Reported fields and timestamps remain available; non-fresh reports
include an explanatory `confighub.liveStatus.freshness` omission.

```sh
# Offline: parser, collector, MCP, doctor, activity, trace, receipt and scope proof.
go test ./cmd/cub-scout -run 'TestLiveStatusFreshness|TestActivityDeliveryIdentity' -count=2
```

The tests assert one connected read per MCP call and the collector's existing
three ConfigHub reads plus two label-selected Deployment lists. They exercise
ASCII/Markdown, exact resource correlation, original activity timestamps, and
fingerprinted receipt evidence. Altering a stored delivery verdict invalidates
the fingerprint; the independent receipt predicate verdict is unchanged.

An unchanged producer report naturally gets older. Re-reading it must not turn
it fresh. Read current controller/workload status when needed; do not conclude
that stale feedback means the application or event consumer has failed.
Likewise, this does not prove that an Application still exists, detect every
out-of-order update, or prove that the expected release was delivered.
Command exit codes are unchanged: successful execution is not a successful
delivery verdict, and absence of `BLOCK` is not equivalent to `PASS`.

No live authentication, production cursor or cluster writes are used by this
proof. Authenticated intended-state mapping remains separate. The existing
standalone/companion bounded TUI panels do not consume connected writeback, so
this fix does not add that TUI capability or change their cache contracts.
