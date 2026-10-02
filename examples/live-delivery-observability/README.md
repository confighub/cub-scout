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

The revision-correlation values in `confighub-delivery-evidence.json` are
synthetic illustrative values, not a captured live ConfigHub result or a claim
about historical release delivery.

For the operator workflow this fixture supports, see
[`docs/howto/delivery-readiness-decision.md`](../../docs/howto/delivery-readiness-decision.md).

The rollout reader's client-binding contract is exercised offline by
`TestFetchRolloutDecisionFromCapturedClient` in
`cmd/cub-scout/observe_rollout_client_test.go`. Its synthetic Deployment and
label-selected Pod responses validate the same runtime-symptom evidence shape
against two distinct TLS endpoints, including denial and cancellation. This is
an internal shared-reader foundation for #755; three-way comparison context
selection is still pending. Missing workload evidence remains unavailable;
denied Pod reads preserve the existing workload-only decision behavior.

```bash
go test ./cmd/cub-scout -run TestFetchRolloutDecisionFrom -count=1
```

`TestCompareLiveSnapshot` in `cmd/cub-scout/compare_resource_session_test.go`
also checks an internal captured-session LIVE reader: workload, Argo source,
ConfigHub link discovery and a bound Flux adapter use the selected endpoint
after the private source kubeconfig is retargeted. Denied, ambiguous or
multi-source evidence stays incomplete; credential cleanup failure discards
the anchor. These recorded TLS contracts require no live cluster or ConfigHub
server. This reader is not yet wired into three-way comparison, and its future
adapters must retain errors accompanying partial LIVE summaries.

```bash
go test ./cmd/cub-scout ./pkg/agent -run 'TestCompareLiveSnapshot|TestCompareGitSourceUnavailableInput|TestGitSourceAnchorFromTrace' -count=1
```

## Files

| File | Purpose |
|---|---|
| `desired.yaml` | Intended Deployment shape used as the comparison baseline. |
| `observed.yaml` | Recorded live objects: aggregate resource, workload, pod symptom, and audited action event. |
| `confighub-delivery-evidence.json` | Example `gitops status --with-confighub --format json` evidence envelope. |
| `revision-correlation-cases.json` | Deterministic strict revision-to-manifest-digest correlation cases, including unknown and ambiguous evidence. |
| `trace-unit-events.md` | Reproducible trace Markdown rendering for correlated unit events. |

## Review Commands

```bash
# Inspect the objects and fields a reviewer should expect cub-scout to parse.
grep -n "kind:\\|event.toolkit.fluxcd.io\\|observedGeneration\\|CrashLoopBackOff" \
  examples/live-delivery-observability/observed.yaml

# Against a cluster with equivalent objects:
./cub-scout map activity --owner Flux --format json
./cub-scout gitops status --format json
./cub-scout gitops status --kube-context production --format json
./cub-scout gitops status --with-confighub --confighub-space prod --confighub-since 24h --format json
./cub-scout gitops status --with-confighub --confighub-space prod --tui
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
- `gitops status --kube-context production` binds every Kubernetes read to
  that named context. The returned `context` is a kubeconfig label, not a
  stable cluster identity; ConfigHub service/auth selection is separate.
- Argo `runtimeOmission` records a denied or unscoped destination-Pod read
  separately from Argo's reported health; it never means zero Pods.
- `gitops status --with-confighub` should keep release history, unit events,
  live-status writeback, and event-consumer workload evidence separate under
  `deliveryEvidence`. Its versioned `revisionCorrelation` compares the complete
  reported SHA-256 revision string only with `manifestDigest` values from the
  same exact non-empty ConfigHub SpaceID among rows returned by the existing
  bounded release query. `coverage` describes those returned rows only; it does
  not assert complete server history or pagination.
  A match is string correlation only; it does not prove fetch, application,
  execution, or gate acceptance. Missing or incomplete evidence stays unknown,
  and duplicate matching release digests remain ambiguous. Report freshness is
  still separate from this comparison.
- `gitops status --tui` displays one collected status snapshot in a scrollable
  viewport. Add `--with-confighub` to opt in to connected evidence; the TUI does
  not poll or make additional reads.
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
go test ./cmd/cub-scout -run 'TestActivityDeliveryIdentityAndReadBudget|TestConfigHubReportedRevisionCorrelation|TestConfigHubRevisionCorrelation|TestGitOpsStatusTUI' -count=1
```

The fixture preserves an Argo failure even when separate ConfigHub evidence
reports PASS. It also proves namespace filtering cannot hide a name collision
and the join performs no additional Kubernetes reads beyond one Application list.

## Trusting Feedback Freshness

**v2.10.1 correction.** The question is: "Does this report tell
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
standalone/companion bounded panels still do not consume connected writeback.
The separate `gitops status --tui` option is a single collected status snapshot,
including `--with-confighub` only when explicitly requested; it does not poll
or change those panels' cache contracts.

### Packaged-Binary Smoke

After extracting a checksum-verified native Unix archive, test the actual
standalone and plugin entry points from the repository root:

```sh
CUB_SCOUT_TEST_BINARY=/absolute/archive/cub-scout \
CUB_SCOUT_TEST_PLUGIN_BINARY=/absolute/archive/main \
go test ./cmd/cub-scout -run '^TestLiveStatusFreshnessPackaged$' -count=1 -v
```

This opt-in test starts each binary's real stdio MCP server. An isolated fake
`cub` returns eight timestamp cases in one explicit all-spaces fixture query;
the test checks verdicts, original fields, six omissions and exactly one data
read. It uses a temporary home and synthetic credentials, never user auth or
a cluster. The provider's existing public startup health HEAD still requires
network reachability; this is not an authenticated connected end-to-end test.
Without an explicit binary path the test skips. Exact clock boundaries remain
covered by the fixed-clock source tests above, not this wall-clock smoke.
