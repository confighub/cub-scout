# Observation Efficiency: Cheaper Long-Running Watch/Bot Polling

Status: Slice 1 (within-cycle coalescing) and Slice 2 (opt-in watch-backed idle
observation) implemented; unreleased. Builds on the measured polling baseline
(#533).

Tracking: **#539** (dedicated observation-efficiency issue). This work was
previously described in the now-closed #532 (section B) and the roadmap, both of
which cite #519 — but the live #519 is a different topic ("integrate cub-scout
evidence into Commander and evaluate TUI convergence"), so #539 is the correct
home; do not overload #519. Related: #533 (baseline), #427 (kstatus watch),
#502/#505.

## User Value

Answer: *can I leave this watching without hammering the cluster?* The baseline
(#533, [examples/observation-budget](../../examples/observation-budget/)) makes
the cost concrete and is the scoping brief for this work:

- Every poll cycle costs a flat **48 requests** whether or not anything changed —
  cold, idle and one-change cycles are identical. An idle watcher re-pulls the
  full inventory forever (idle is asserted byte-identical to cold).
- Response bytes scale with inventory (≈47 KB at 100 objects, ≈434 KB at 1,000)
  and are paid every cycle.
- Within a single cycle the inventory sweep and the state scanner **re-list the
  same types**: the application list twice, the release and reconciliation lists
  three times each (43 distinct paths, 48 requests).

Two independent inefficiencies fall out: **redundant reads within a cycle**, and
**full re-reads across idle cycles**. They have very different risk profiles, so
this splits into two slices.

## Design principles (non-negotiable)

- **Correctness is never traded for efficiency.** A cheaper transport must never
  produce a false "nothing changed", hide a deletion, or convert a missing read
  into a healthy/unmanaged assertion. Ambiguous beats wrong.
- **Measure the change.** Every slice is proven against the #533 baseline test
  (request count and response bytes), not asserted.
- **Bounded and finite.** Any retained state is capped by object count with a
  defined overflow behavior; no unbounded growth.
- **Daemon-free one-shots.** `map`, `snapshot`, `explain`, `release check` and
  other one-shot commands gain no background loop. This is a watch/bot concern.
- **Graceful degradation.** When a cheaper path is unavailable (RBAC denial,
  unsupported verb, expired stream), fall back to the current full LIST and mark
  freshness/coverage explicitly — never silently drop a type.
- **Read-only, boundary intact.** Delivery, retry, reconciliation and
  status-writeback ownership stay outside the observer.

## Slice 1 (recommended first): within-cycle read coalescing

The poll loop runs two sweeps per cycle over one dynamic client: the inventory
sweep (`collectWatchEntries`) and the scanner (`collectWatchFindings` →
`state_scanner`). They list overlapping types independently, so the same GVR is
read 2–3 times per cycle.

Introduce a **per-cycle memoizing dynamic client**: a thin wrapper implementing
`dynamic.Interface` that caches each `List(gvr, namespace, options)` result for
the duration of one cycle and is discarded at cycle end. Pass it to both the
inventory sweep and the scanner. Duplicate lists within a cycle become cache
hits; the scanner needs no change.

Properties:

- **Correctness-neutral, and slightly more correct:** every sweep in a cycle sees
  one consistent point-in-time snapshot per type, removing intra-cycle skew. The
  emitted events and inventory are identical to today's for the same inputs.
- **Removes redundant requests** (application ×2→×1, release ×3→×1,
  reconciliation ×3→×1): the 48-request ceiling drops to roughly 43, and on real
  Flux/Argo clusters the duplicated list *bytes* drop too.
- **No new failure modes, no daemon semantics, no lifecycle concerns.** Idle
  cycles still re-read full inventory (that is Slice 2).

This is the safe down-payment: it lowers per-cycle cost immediately and is fully
provable, but it does not make idle cycles cheap.

## Slice 2 (implemented, opt-in): watch-backed idle observation

As shipped: opt-in `watch --watch-backed` / `bot --watch-backed`
(`CUB_SCOUT_BOT_WATCH_BACKED`). Inventory is served from client-go **dynamic
informers**, so the reflector machinery handles relist, `410 Gone`, and resync
rather than a hand-rolled watch. A discovery pass filters to served types; only
types whose informer syncs are read from cache, and any setup failure falls back
to per-cycle polling — an unsynced type is never reported empty. Deletions are
surfaced as a new `resource.deleted` event (previously silent, and now correct in
both modes), and watch-backed cycles stamp `observation.mode: watch-informer`.
The state scan's own reads (and full watch-backing of the scanner) remain a
follow-up. Long-running only; `--once` is unaffected.

The only way to make an unchanged cycle cheap is to stop re-LISTing it. A LIST
always returns the full collection; the list's `metadata.resourceVersion` is a
cluster-global counter, so it cannot be used as a per-type "changed since" probe.
Cheap idle observation therefore requires a **watch stream**, which is greenfield
here (no informers/watches exist today).

Design: for each watched GVR, seed with one LIST (capturing its
`resourceVersion`), then maintain a bounded in-memory store from a watch stream;
each tick flushes accumulated deltas instead of re-LISTing. Idle cost falls to
stream bookmarks/heartbeats. This must define, before implementation:

- **Watched-scope selection:** which GVRs are watch-backed (start with the core
  workload set; leave rarely-changing or high-cardinality types on LIST if a
  watch is not worth a connection). Per-GVR, not all-or-nothing.
- **Finite storage:** the store is capped by object count with explicit overflow
  → drop to a full relist for that type; never unbounded `prevState`.
- **Deletion:** watch DELETE events close today's gap (deletions currently emit
  no event because only `curr` is iterated). This adds a `resource.deleted`
  event — a change to the closed 4-event enum in
  [docs/reference/watch-events.md](../reference/watch-events.md) — and must be
  coordinated as a contract addition.
- **Reconnection / relist:** on `410 Gone` / expired `resourceVersion`, relist
  and resume; distinguish a transient reconnect from a full resync.
- **Freshness / staleness:** stamp `observedAt`, source resourceVersion and
  stream health; when a stream is down, fall back to LIST and mark the type
  stale rather than reporting "no change".
- **Cancellation:** ctx-scoped stream teardown on shutdown/interval change.
- **Permission / scope isolation:** a per-GVR watch RBAC denial falls back to
  LIST for that type and surfaces a coverage gap, instead of today's silent
  swallow.

Explicitly out of scope for Slice 2: the state scanner's own lists and optional
receipt reads (measured separately — watch-backing inventory alone does not
remove them), MCP's per-request `map list` self-exec, and one-shot commands.

## Success Before Implementation

**Slice 1 fixtures** (extend `cmd/cub-scout/watch_budget_test.go`, recorded-HTTP):

- Request count drops below the current 48 ceiling for the existing
  Deployments-only fixture (redundant empty-list requests removed).
- A new fixture containing Flux/Argo objects shows the duplicated-list *bytes*
  removed, not just requests.
- Byte-for-byte / verdict-for-verdict identical events and inventory versus the
  pre-coalescing collector for the same inputs (correctness-neutral).
- Idle remains byte-identical to cold for the coalesced request set.

**Slice 2 fixtures** (its own PR; extend the harness to serve `watch=true`
streams, or a fake-clientset watch):

- Idle cycle cost drops to stream heartbeats, not a full re-LIST (asserted
  request/byte reduction versus the LIST baseline).
- DELETE event → a `resource.deleted` event; a removed object is no longer
  silent.
- `410 Gone` / expired RV → relist and resume with no lost or duplicated events.
- Watch unavailable / RBAC-denied for a GVR → LIST fallback for that type, marked
  stale/partial, other types unaffected.
- Bounded-store overflow → relist for that type; memory stays capped.
- Cancellation tears down streams promptly; late deltas cannot overwrite newer
  state.

## Known Limitations

- Slice 1 does not reduce idle inventory bytes; it only removes redundant
  within-cycle reads. Idle full re-reads remain until Slice 2.
- Slice 2 changes the most-tested code path (stateless LIST-per-tick) and adds a
  public event type; it is intentionally deferred to its own PR behind Slice 1.
- The scanner's separate lists, optional receipt reads, and MCP's per-request
  `map list` self-exec are additional re-read surfaces addressed later.

## Later Work

Scanner list reduction; MCP/TUI inventory reuse of the bounded store;
connected/fleet observation cost; and interval/backoff tuning. Tracked with the
new observation-efficiency issue and #502/#505.

## Local Verification

Recorded 2026-09-12: `go build ./...`, `go vet ./...`, full `go test ./...`
(including `-race` on the watch/observation/bot tests) and the CLI-docs parity
checker pass; `go.mod`/`go.sum` unchanged (all imports are within the existing
client-go/apimachinery modules).

- Slice 1 proofs: the extended `#533` baseline (48 → 43 requests) and
  `TestWatchObservationCoalescing`.
- Slice 2 proofs: `TestBuildWatchEventsDeletion` (a removed resource yields one
  `resource.deleted` event, none when inventory is unchanged) and
  `TestWatchBackedClientServesFromCache` (after informer sync, cache reads make
  zero additional list API calls; a selector-scoped list falls through to the
  API), using a `dynamicfake` client with informers.

Watch-events contract, `README` User questions, command reference and roadmap
updated for the new `resource.deleted` event, the `watch-informer` observation
mode and the `--watch-backed` flag.
