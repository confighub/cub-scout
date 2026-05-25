# Watch Event Reference

`cub-scout watch` emits a closed set of event types over webhook + JSONL file sinks. This reference is the authoritative description of:

- The four event types (`resource.discovered`, `ownership.changed`, `drift.detected`, `scan.finding`) and when each fires
- The event JSON shape
- The inline-receipt attachment via `--emit-receipt-on`
- The per-poll backpressure cap via `--emit-receipt-batch-cap`
- The wire-shape contract consumers must check (`omitempty` over `null`)

For the operational walkthrough, see [`docs/howto/receipts-end-to-end.md`](../howto/receipts-end-to-end.md) § Stage 4. For the Pilot-consumer integration shape, see [`pilot-watch-alert-response`](../../skills/pilot-watch-alert-response/SKILL.md).

## Event types

The four types emitted by `buildWatchEvents` in `cmd/cub-scout/watch.go` are a **closed enumeration**. New types added to that function should also be added to:

1. `watchKnownEventTypes` in `cmd/cub-scout/watch_receipt.go` (so `--emit-receipt-on` accepts them)
2. `watchEventTypesWithReceiptSupport` (if receipt-build is wired)
3. This reference

| Event type | When it fires | Detection logic | `severity` field | `details` keys |
|---|---|---|---|---|
| `resource.discovered` | First poll observes a resource cub-scout had not seen before | `entriesByID[id]` exists in current state but not previous | — | `status` |
| `ownership.changed` | A previously-observed resource's ownership controller changed | `prevEntry.Owner != currEntry.Owner` | — | `before`, `after` (ownership values) |
| `drift.detected` | A scan finding categorized as `STATE` or `DRIFT` matches a resource | `strings.EqualFold(finding.Category, "STATE" or "DRIFT")` | propagated from the underlying finding (`critical` / `warning` / `info`) | `category`, `message` |
| `scan.finding` | Any new risk-pattern finding from `scan` (not just drift) | `currFindings[key]` not in `prevFindings` | propagated from the finding | `category`, `message` |

Note: `scan.finding` and `drift.detected` can fire for the **same** underlying finding when the category matches `STATE` / `DRIFT`. They are not deduplicated; consumers may want to filter one or the other.

## Event JSON shape

All four event types share this canonical shape (from `watchEvent` in `cmd/cub-scout/watch.go`):

```json
{
  "type": "drift.detected",
  "timestamp": "2026-05-25T11:00:00Z",
  "resource": {
    "kind": "Deployment",
    "name": "payments-api",
    "namespace": "prod"
  },
  "owner": {
    "type": "argo",
    "name": "payments-api"
  },
  "severity": "warning",
  "details": {
    "category": "DRIFT",
    "message": "image: live ghcr.io/org/api:v1.4.3-hotfix differs from git ghcr.io/org/api:v1.4.2"
  },
  "receipt": {
    "_type": "https://in-toto.io/Statement/v1",
    "subject": [...],
    "predicateType": "https://cub-scout.dev/receipt/v1",
    "predicate": {...}
  }
}
```

### Field reference

| Field | Type | Presence | Notes |
|---|---|---|---|
| `type` | string | Always | One of the four closed-enumeration values |
| `timestamp` | RFC 3339 | Always | Time of the watch poll that observed the event |
| `resource.kind` | string | Always | Kubernetes Kind (`Deployment`, `StatefulSet`, etc.) |
| `resource.name` | string | Always | Resource name |
| `resource.namespace` | string | Omitempty (cluster-scoped resources skip) | — |
| `owner.type` | string | Omitempty | Owner classification (`argo`, `flux`, `helm`, `crossplane`, `kro`, `confighub`, `native`, `unknown`) |
| `owner.name` | string | Omitempty | Owner-specific identifier (e.g., Argo Application name) |
| `severity` | string | Omitempty (only `drift.detected` + `scan.finding`) | `critical` / `warning` / `info` |
| `details` | object | Omitempty | Type-specific extra fields; see the per-type table above |
| `receipt` | object | Omitempty | In-toto Statement v1; present only when `--emit-receipt-on` matched + receipt-build succeeded + backpressure cap allowed |

## Inline receipts via `--emit-receipt-on`

`watch --emit-receipt-on <event-types>` attaches a fingerprinted in-toto Statement v1 receipt to each matching event payload. All four event types support receipt-build:

| Event type | Predicate auto-detected |
|---|---|
| `resource.discovered` | `applied-matches-spec` (live state at first observation) |
| `ownership.changed` | `applied-matches-spec` (owner shift may indicate delivery-chain change) |
| `drift.detected` | `applied-matches-spec` (the drift event implies the verdict question) |
| `scan.finding` | `applied-matches-spec` (the resource state at finding time; the finding detail lives on `details`) |

The receipt is the **full** in-toto Statement v1 envelope inline — not a digest reference. Consumers can read `event.receipt.predicate.verdict` directly:

```
PASS         → log + continue
WATCH        → alert + log + attach receipt to ticket
BLOCK        → escalate (page on-call, open ticket)
INCONCLUSIVE → ASK (manual review; proof gaps in omissions[])
```

Receipt verdicts are restricted to **four values**: PASS / WATCH / BLOCK / INCONCLUSIVE. Source-truth `ASK` is a separate enum value on `compare source-truth --status`; when wrapped in a receipt, ASK maps to receipt WATCH (per `pkg/agent/receipt_predicates.go`).

### Flag values

```bash
# Single event type
--emit-receipt-on drift.detected

# Multiple (comma-separated)
--emit-receipt-on drift.detected,ownership.changed

# All known types (forward-compatible)
--emit-receipt-on all
```

Unknown event types are rejected upfront (`parseWatchEmitReceiptOn` checks against `watchKnownEventTypes`). The `all` sugar enables every type in `watchKnownEventTypes` — including any future types added to the enumeration.

### Forward-compat warning

If a future event type is added to `watchKnownEventTypes` without receipt-build wiring, the watch loop fires a **one-time startup stderr warning** listing the unsupported subset:

```
Warning: --emit-receipt-on includes event types that don't yet build a receipt: hypothetical.future.event.
The flag accepts these for forward-compat (so `--emit-receipt-on all` keeps working as new types land),
but receipt-build is skipped for them. Currently supported: drift.detected, ownership.changed,
resource.discovered, scan.finding.
```

As of this writing, all four known types build receipts, so the warning never fires for the known set.

## Per-poll backpressure

`--emit-receipt-batch-cap N` (default 10) caps receipt-build attempts per poll. The cap is **load-bearing** for `resource.discovered` and `scan.finding` because both can fire many events on the first poll (initial discovery burst, initial scan burst).

### How the cap behaves

For each call to `attachReceiptsIfRequested` (one call = one watch poll):

1. Count receipt-eligible events (`emitOn[type] && watchEventTypesWithReceiptSupport[type]`)
2. Build receipts for the **first N** events
3. **Skip** receipt-build for the remaining events (the `receipt` key is omitted on those)
4. Fire **one** stderr summary line per poll if any events were skipped:
   > `backpressure: suppressed receipt-build for X events of N eligible (cap=N); raise --emit-receipt-batch-cap to enable, or set 0 to disable entirely`

### Cap values

| Value | Behaviour |
|---|---|
| Default `10` | Bounds first-poll bursts |
| `0` | Disables receipt-build entirely while keeping the flag explicit |
| Large value (e.g., `1000`) | Effectively disables the cap |
| Negative | Rejected upfront |

### Why per-poll, not across-poll

The cap resets every call. A long-running watch with quiet polls between bursts doesn't accumulate suppression state. The pattern parallels `makeReceiptWarnFn`'s warning rate-limit (first 10 verbatim, summary every 100 after), but applied to receipt-build attempts rather than warning lines.

### Receipt-build failures

Independent of the cap, receipt-build can fail (transient cluster-read, missing CRD, etc.). When it fails:

- The watch event **still emits** (the stream stays unbroken)
- The `receipt` key is **omitted** from the event payload (omitempty)
- A stderr warning fires the first 10 times per long-running session; subsequent failures contribute to a periodic summary (first 10 + summary every 100, per `makeReceiptWarnFn`)

## The `omitempty` wire contract

The `Receipt` field on `watchEvent` uses Go's `omitempty` JSON tag. When the field is nil — which happens whenever:

- `--emit-receipt-on` doesn't include this event's type, OR
- The event's type isn't in `watchEventTypesWithReceiptSupport`, OR
- Receipt-build failed, OR
- The per-poll backpressure cap was hit

— the `receipt` key is **omitted from the JSON entirely**, not emitted as `"receipt": null`.

### Consumer contract

Consumers **must check key presence**, not null-ness:

```python
# Correct — handles all four "no receipt" paths
if "receipt" in event:
    verdict = event["receipt"]["predicate"]["verdict"]
    handle_verdict(verdict)
else:
    # No receipt attached — fall back to live evidence if needed
    handle_event_without_receipt(event)
```

```python
# Incorrect — raises KeyError when receipt was suppressed
verdict = event["receipt"]["predicate"]["verdict"]
```

```javascript
// Correct (JS/TS)
if ("receipt" in event) {
  const verdict = event.receipt.predicate.verdict;
  handleVerdict(verdict);
}
```

This is the contract the Pilot consumer skills assume (see [`pilot-watch-alert-response`](../../skills/pilot-watch-alert-response/SKILL.md) § Wire shape — omitempty is load-bearing).

## CLI invocation patterns

### Standalone-mode JSONL replay

```bash
cub-scout watch -n prod \
  --output-file /var/log/cub-scout-watch.jsonl \
  --emit-receipt-on all \
  --emit-receipt-batch-cap 10 \
  --severity warning,critical
```

The JSONL file captures every event one-per-line; useful for postmortem replay (see [`pilot-incident-evidence`](../../skills/pilot-incident-evidence/SKILL.md) § Step 4).

### Webhook + file dual-sink

```bash
cub-scout watch -n prod \
  --webhook https://pilot.acme/cub-scout-events \
  --output-file /var/log/cub-scout-watch.jsonl \
  --emit-receipt-on drift.detected,ownership.changed \
  --emit-receipt-batch-cap 10
```

Webhook handles real-time consumption; file sink survives webhook outages and powers offline replay.

### `--once` for a single poll

```bash
cub-scout watch -n prod \
  --output-file ./snapshot.jsonl \
  --emit-receipt-on all \
  --once
```

Runs one collection cycle and exits. Useful for CI gates that want a point-in-time snapshot rather than a long-running stream.

## See also

- **How-to**: [`docs/howto/receipts-end-to-end.md`](../howto/receipts-end-to-end.md) § Stage 4
- **Worked example**: [`examples/receipts/watch-emit/`](../../examples/receipts/watch-emit/) — Pilot consumer sketch + full event payload sample
- **Reference**: [`commands.md` § watch](commands.md) — the `--webhook` / `--output-file` / `--severity` flag table
- **JSON Contract**: [`json-contracts.md` § Receipt Contract](json-contracts.md) — receipt envelope wire format
- **Skills**: [`pilot-watch-alert-response`](../../skills/pilot-watch-alert-response/SKILL.md) — consumer-side Pilot integration; [`scout-observe`](../../skills/scout-observe/SKILL.md) — operator-side `watch` usage
- **Code**: `cmd/cub-scout/watch.go` (`watchEvent` struct + `buildWatchEvents`); `cmd/cub-scout/watch_receipt.go` (`attachReceiptsIfRequested` + `watchReceiptBatchCap` + `parseWatchEmitReceiptOn`)
- **Issues**: `#449` (closed via `#470`) — full v2 surface
