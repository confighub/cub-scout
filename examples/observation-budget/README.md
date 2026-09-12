# Observation Request Baseline

User question: "What does keeping observation running actually cost?"

This is a measurement of the polling collector. It established the first
baseline (the recorded 48-request cycle below) and now also records the first
efficiency improvement against it. Tracked in
[#539](https://github.com/confighub/cub-scout/issues/539) (observation
efficiency); design in
[docs/proposals/observation-efficiency.md](../../docs/proposals/observation-efficiency.md).

## Repeat The Measurement

From the repository root:

```sh
go test ./cmd/cub-scout -run '^TestWatchObservationBudget$' -count=1 -v
go test ./cmd/cub-scout -run '^$' -bench '^BenchmarkWatchObservation$' \
  -benchmem -benchtime=5x -count=3
```

The test calls the production `collectWatchState` and `buildWatchEvents`
functions against an HTTP loopback fixture. It supplies 100 or 1,000 ready
Deployments in one namespace, followed by an unchanged collection and a
collection with one ownership-label change. Other requested resource lists are
empty. Custom-resource configuration and cluster-name inputs are isolated.
The server counts actual requests and written JSON response-body bytes per
path, rejects writes and requests outside the fixture namespace, and checks
inventory and event results. No real cluster, credentials or connected account
is used. The cold event count is the diff against an empty state; long-running
watch primes that baseline without emitting the initial discovery events.

## Recorded Baseline

Measured 2026-09-11 against the collector at `bc68a9a`, unchanged by the
v2.10.1 release-preparation patch. Per cycle:

| Objects | Phase | Requests | Response-body bytes | Events |
|---|---|---|---|---|
| 100 | Cold | 48 | 46,791 | 100 |
| 100 | Unchanged | 48 | 46,791 | 0 |
| 100 | One ownership change | 48 | 46,840 | 1 |
| 1,000 | Cold | 48 | 433,791 | 1,000 |
| 1,000 | Unchanged | 48 | 433,791 | 0 |
| 1,000 | One ownership change | 48 | 433,840 | 1 |

There are 43 distinct request paths. In this original baseline the inventory and
scanner read the application list twice, and each of the release and
reconciliation lists three times, for 48 requests. Full path-level counts appear
in the test output.

### After within-cycle coalescing (2026-09-12, #539)

Slice 1 shares one memoized read of each type between the inventory sweep and the
state scanner within a single cycle, so each distinct path is fetched once per
cycle. Per cycle, idle and cold identical:

| Objects | Requests | Response-body bytes |
|---|---|---|
| 100 | 43 | 46,396 |
| 1,000 | 43 | 433,396 |

The duplicated list requests are gone (48 → 43), and their bytes are removed too
(larger on real Flux/Argo clusters, where those lists are non-empty). The fixture
ceiling is now 43; it is not a global product limit. This does **not** yet make
idle cycles cheap — an unchanged cycle still re-reads full inventory. That is the
watch-backed Slice 2 in
[docs/proposals/observation-efficiency.md](../../docs/proposals/observation-efficiency.md).

Three local benchmark runs on darwin/arm64, five collections per run:

| Objects | Time per collection | Allocated bytes per collection |
|---|---|---|
| 100 | 4.99-5.61 ms | 2.09-2.13 MB |
| 1,000 | 17.34-18.13 ms | 12.91-12.95 MB |

These timings include the loopback server and client allocations. Fixture
generation is outside the timer and client-side throttling is disabled for
this measurement only. Allocation volume is not retained memory or process
RSS. These are not production latency, bandwidth or fleet-size guarantees.

## What This Does Not Prove

- Response bytes exclude HTTP/TLS headers, compression and authentication.
  Startup health probes, sink delivery and optional receipt reads are outside
  this collector measurement; their costs must be added for a complete run.
- The fixture acknowledges requested namespace paths; it is not a discovery,
  CRD-scope, RBAC or API-version compatibility emulator. Missing APIs, denial,
  deletion and interrupted watches need separate fixtures.
- This is one namespace and one controller-free workload set, not mixed
  controller, authenticated connected, fleet or live-cluster proof.
- No continuous-watch update-delay, steady-state RSS, interface task timing
  or competing-tool result is claimed. Those remain planned measurements.
- Existing [bounded exact-object reads](../bounded-resource-read/) answer a
  different, narrower question. Their two cold requests and zero-request cache
  hits must not be compared to a full inventory as if the tasks were equal.

## Next Decision

Slice 1 (within-cycle coalescing) has shipped, lowering each cycle to 43
requests and removing duplicated list bytes. The next step (Slice 2, tracked in
[#539](https://github.com/confighub/cub-scout/issues/539)) is to eliminate
unchanged full-inventory polling in long-running observation. Before
implementation, define watched resource/scope selection, finite storage,
coverage, refresh, deletion, reconnect/relist, cancellation and stale evidence
semantics. Measure scanner and optional receipt costs too; changing inventory
transport alone does not eliminate their queries. One-shot commands remain
daemon-free. Delivery and status-writeback ownership stay outside the observer.
