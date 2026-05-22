# Receipt Example: Real-time Emission with `watch --emit-receipt-on`

This example walks through using `cub-scout watch --emit-receipt-on <event-types>` to attach a fingerprinted receipt to each matching watch event payload inline. Pilot or another acceptance-judge tool subscribes to the watch stream, reads the inline receipt's verdict, and acts on each event without a synchronous call back into cub-scout.

Issue: [`#449`](https://github.com/confighub/cub-scout/issues/449).
Parent capability: [`#446`](https://github.com/confighub/cub-scout/issues/446).
Shipped in: [`#463`](https://github.com/confighub/cub-scout/pull/463) (v1) + [`#470`](https://github.com/confighub/cub-scout/pull/470) (resource.discovered / scan.finding + backpressure).

## The shape

```
   cub-scout watch -n prod \
     --webhook https://pilot.acme/events \
     --output-file ./watch.jsonl \
     --emit-receipt-on drift.detected,ownership.changed,resource.discovered,scan.finding \
     --emit-receipt-batch-cap 10

        │  per-event JSONL stream
        ▼
   {
     "type": "drift.detected",
     "timestamp": "...",
     "resource": {...},
     "owner": {...},
     "severity": "warning",
     "details": {...},
     "receipt": {              ◄── inline in-toto Statement v1
       "_type": "https://in-toto.io/Statement/v1",
       "predicateType": "https://cub-scout.dev/receipt/v1",
       "predicate": {
         "predicateName": "applied-matches-spec",
         "verdict": "BLOCK",
         ...
       }
     }
   }
        ▼
   Pilot's per-event handler reads receipt.predicate.verdict
   → PASS  → log + continue
   → WATCH → alert + log
   → BLOCK → escalate (pages on-call, opens ticket)
   → INCONCLUSIVE → ASK (manual review)
```

## Event types supported

All four known watch event types build receipts (as of [`#470`](https://github.com/confighub/cub-scout/pull/470)):

| Event type | Predicate | Notes |
|---|---|---|
| `drift.detected` | `applied-matches-spec` (auto-detected) | Highest signal: drift event implies a verdict question |
| `ownership.changed` | `applied-matches-spec` | Owner shifts often indicate delivery-chain changes worth attesting |
| `resource.discovered` | `applied-matches-spec` | The discovery moment captures the live state at first observation; backpressure-gated |
| `scan.finding` | `applied-matches-spec` | Receipt records the resource state at finding time; the finding detail lives on the event's `details` field; backpressure-gated |

The sugar value `all` enables all four. The flag is **forward-compatible**: future event types that don't yet have receipt-build support pass through with a startup warning listing them.

## Backpressure: `--emit-receipt-batch-cap N`

When a single poll produces more than `N` receipt-eligible events:

- The first N get receipts built and attached
- The remaining events still emit (the watch stream is preserved) but their `receipt` key is **omitted** (omitempty)
- A single stderr summary fires per poll:
  > `backpressure: suppressed receipt-build for X events of N eligible (cap=N); raise --emit-receipt-batch-cap to enable, or set 0 to disable entirely`

The cap is **per-poll**: quiet polls between bursts don't accumulate suppression state.

| Value | Behaviour |
|---|---|
| Default `10` | Conservative; bounds first-poll bursts |
| `0` | Disables receipt-build entirely while keeping the flag explicit |
| `1000` | Effectively disables the cap |
| Negative | Rejected upfront |

## Wire shape — `omitempty` is load-bearing

The `receipt` key uses `omitempty`. When receipt-build fails OR the cap suppresses, the key is **omitted from the JSON entirely** (not emitted as `"receipt": null`). Consumers must check for key presence, not null-ness:

```python
# Correct
for line in stream:
    event = json.loads(line)
    if "receipt" in event:
        verdict = event["receipt"]["predicate"]["verdict"]
        handle_verdict(verdict)
    else:
        # No receipt was attached — fall back to live evidence if needed
        handle_event_without_receipt(event)

# Incorrect — raises KeyError when receipt was suppressed
verdict = event["receipt"]["predicate"]["verdict"]
```

## Pilot-side consumer (sketch)

```python
import json, sys

def handle_event(event):
    if "receipt" not in event:
        # Slow path: ask cub-scout synchronously for evidence
        return ask_cub_scout_for_evidence(event["resource"])

    # Fast path: read the inline receipt
    verdict = event["receipt"]["predicate"]["verdict"]
    if verdict == "PASS":
        log(event)
    elif verdict == "WATCH":
        alert(event, "ambiguous evidence; review")
    elif verdict == "BLOCK":
        escalate(event, "hard mismatch; review and rollback")
    elif verdict == "INCONCLUSIVE":
        page_oncall(event, "missing evidence; manual review needed")

for line in sys.stdin:
    handle_event(json.loads(line))
```

## Running the watch

```bash
# Start the watch — both file sink (for replay) AND webhook (for Pilot)
$ cub-scout watch -n prod \
    --webhook https://pilot.acme/events \
    --output-file /var/log/cub-scout-watch.jsonl \
    --emit-receipt-on all \
    --emit-receipt-batch-cap 10 \
    --severity warning,critical
```

The watch loop:
- Polls every `--interval` (default 20s)
- Emits each event to the webhook AND appends to the JSONL file
- For each receipt-eligible event, attempts receipt-build (subject to the per-poll cap)
- Receipt-build failures fire a stderr warning (rate-limited per `#463`); the event still emits with the `receipt` key omitted

## Receipt-build failures are non-fatal

If receipt-build fails for an event (transient cluster-read failure, missing CRD, etc.):

- The watch event itself **still emits**
- The `receipt` key is **omitted** from the event payload
- A stderr warning fires the first 10 times per long-running session; afterwards a summary fires every 100 suppressions (the `#463` rate-limit pattern)

The stream stays unbroken. Pilot's handler sees no receipt and falls back to the slow path.

## Local replay during postmortems

The `--output-file` sink writes one JSONL line per event. During a postmortem, you can replay the bundle offline:

```bash
$ jq -s '
  [.[] | select(.type == "drift.detected" and (.receipt.predicate.verdict // "") == "BLOCK")]
' /var/log/cub-scout-watch.jsonl
```

This pairs naturally with `cub-scout bundle` for the full incident-evidence pack (see [`pilot-incident-evidence`](../../../skills/pilot-incident-evidence/SKILL.md)).

## Receipt validate on inline receipts

To verify an inline receipt's fingerprint integrity (e.g., it traversed a webhook):

```bash
$ jq '.[0].receipt' /var/log/cub-scout-watch.jsonl > event-receipt.json
$ cub-scout receipt validate event-receipt.json
# exit 0 = fingerprint intact; 1 = mismatch (refuse to act on it); 2 = I/O
```

## See also

- [`../README.md`](../README.md) — receipt v1 + v2 overview
- [`../chained/README.md`](../chained/README.md) — chained receipts via `--input-attestation`
- [`../aggregate/README.md`](../aggregate/README.md) — aggregate receipts (related v2 surface)
- [`../ci-gate/README.md`](../ci-gate/README.md) — CI gating with `--fail-on` (related v2 surface)
- [`docs/reference/json-contracts.md`](../../../docs/reference/json-contracts.md) § Receipt Contract — watch-emit wire format
- [`docs/reference/commands.md`](../../../docs/reference/commands.md) § `watch` — `--emit-receipt-on` + `--emit-receipt-batch-cap`
- Pilot consumer skill: [`pilot-watch-alert-response`](../../../skills/pilot-watch-alert-response/SKILL.md)
- Issues: [`#449`](https://github.com/confighub/cub-scout/issues/449); shipped in [`#463`](https://github.com/confighub/cub-scout/pull/463) (v1) + [`#470`](https://github.com/confighub/cub-scout/pull/470) (full event-type set + backpressure)
