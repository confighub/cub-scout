---
name: pilot-watch-alert-response
description: 'Use when Pilot is RESPONDING TO REAL-TIME CLUSTER EVENTS surfaced by `cub-scout watch`. Natural phrasings: "Pilot subscribes to the watch stream", "render verdicts on each drift event as they fire", "real-time acceptance kernel for cluster changes", "Pilot, decide on this drift.detected event", "what does Pilot say about the ownership.changed alert", "stream PASS/WATCH/ASK/BLOCK per event from cub-scout watch". Pilot consumes the `cub-scout watch` event stream (webhook or file sink, optionally with inline receipts via `--emit-receipt-on`) and renders a verdict per event, calling back into cub-scout (`explain`, `compare three-way`, `trace`) for context when needed. This is the ONLY event-driven skill in the pilot-* batch — others are query-driven. Do NOT load for: a single-resource pre-deploy gate (use pilot-cd-gate), a periodic fleet roll-up (use pilot-fleet-conformance), a drift-classification deep-dive (use pilot-patch-and-drift), a postmortem evidence pack (use pilot-incident-evidence), or any mutating action.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout watch *) Bash(cub-scout watch *) Bash(cub scout watch *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout compare *) Bash(cub-scout compare *) Bash(cub scout compare *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(kubectl get *) Bash(kubectl describe *) Bash(cub * get) Bash(cub * list) Bash(cub link list *) Bash(cub unit get *) Bash(argocd app get *) Bash(flux get *)

---

# pilot-watch-alert-response

The real-time event-driven scenario from **Pilot's** side. Pilot subscribes to `cub-scout watch` (which already exposes webhook / file sinks) and, for each event, decides PASS / WATCH / ASK / BLOCK — calling back into cub-scout for context as needed. This is the **only event-driven skill in the pilot-* batch**; the other four are query-driven.

With the receipts v2 surface shipped (#463), Pilot can ask `cub-scout watch` to **inline a receipt** with each matching event via `--emit-receipt-on drift.detected,ownership.changed`. That delivers a fingerprinted in-toto Statement v1 per event in the same JSONL line — Pilot reads the verdict + receipt in one shot, no synchronous call-back required for the common case.

## When to use

Explicit phrasings:

- "Pilot subscribes to the watch stream"
- "Render verdicts on each drift event as they fire"
- "Real-time acceptance kernel for cluster changes"
- "Pilot, decide on this `drift.detected` event"
- "What does Pilot say about the `ownership.changed` alert"
- "Stream PASS/WATCH/ASK/BLOCK per event from cub-scout watch"
- "Pilot reads receipts inline from the watch event stream"
- "Trigger Pilot on any cub-scout drift event"

Implicit intents:

- The flow is **event-driven**, not query-driven (Pilot reacts to a stream, not a polling job)
- The decision needs to **fire fast** — a Pilot verdict per event, ideally inline
- The receipt-attached path (v2 `--emit-receipt-on`) is preferred when audit durability matters
- Backpressure is a real concern when events fire fast (e.g., ApplicationSet expansion)

## Do not load for

- A **single-resource pre-deploy gate** — [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md)
- A **periodic fleet roll-up verdict** — [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md)
- A **drift-classification deep-dive** for one resource — [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md)
- A **postmortem evidence pack** — [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md)
- Pure **observation** without a Pilot policy decision — [`scout-observe`](../scout-observe/SKILL.md) covers the `watch` surface from the operator side
- Any **mutating action**. Pilot's mutation path is `cub` / Argo / Flux / kubectl — never cub-scout.

## The Pilot ↔ cub-scout watch loop

```
   cub-scout watch -n prod \
     --webhook https://pilot.acme/cub-scout-events \
     --emit-receipt-on drift.detected,ownership.changed \
     --output-file /var/log/cub-scout-watch.jsonl
        │
        │  ▼ JSONL event stream (one event per line)
        │  {
        │    "type": "drift.detected",
        │    "timestamp": "2026-05-22T11:00:00Z",
        │    "resource": {"kind": "Deployment", "name": "api", "namespace": "prod"},
        │    "owner": {"type": "argo", "name": "payments-api"},
        │    "severity": "warning",
        │    "details": {"category": "DRIFT"},
        │    "receipt": { in-toto Statement v1 with applied-matches-spec verdict }
        │  }
        ▼
   Pilot's event handler
        │  fast path: the event already carries a receipt
        │    │
        │    └► Pilot reads receipt.predicate.verdict → render verdict
        │
        │  slow path: the event doesn't carry a receipt
        │    │  (event type isn't in --emit-receipt-on, OR receipt-build failed)
        │    │
        │    ├──► cub-scout compare three-way <r> -n <ns> --format json
        │    ├──► cub-scout explain <r> -n <ns> --presentation ai
        │    └──► cub-scout receipt verify <r> -n <ns> --strategy <s> --out event-receipt.json
        │
        ▼
   Pilot's per-event verdict
        │  PASS  → log + continue
        │  WATCH → alert + log + (optional) attach receipt to a ticket
        │  ASK   → page on-call; the proof gap is structured in the receipt
        │  BLOCK → halt promotion / trigger Pilot's escalation policy
        └────────────────────────────────────────────────────
```

## Event types (`cub-scout watch` v1 + receipts v2)

`cub-scout watch` emits four event types today; `--emit-receipt-on` v1 attaches receipts to two of them:

| Event type | When it fires | Receipt-built in v1? | Pilot's typical reaction |
|------------|---------------|----------------------|--------------------------|
| `resource.discovered` | First poll sees a resource cub-scout hadn't seen before | No (would flood on initial poll burst; pass-through silently) | Log; no verdict needed. |
| `ownership.changed` | A resource's ownership controller changed (Argo → Flux, Flux → kubectl, etc.) | **Yes** — `applied-matches-spec` auto-detected. Receipt verdict reflects whether the new owner has a resolved git anchor. | WATCH (often a transient state during migration); ASK if `applied-matches-spec` returns INCONCLUSIVE. |
| `drift.detected` | `compare three-way` finds a divergence on a resource cub-scout watched into | **Yes** — `applied-matches-spec` auto-detected. Receipt verdict reflects whether the drift is controller-drift (PASS / reconciling) or manual-edit (BLOCK). | Drives [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md) shape: PASS = ACCEPT; WATCH = INVESTIGATE; BLOCK = REVERT/QUARANTINE. |
| `scan.finding` | A risk pattern matched on a resource (CVE, expiring cert, etc.) | No (would flood on initial scan burst; pass-through silently) | Severity-driven; route to security/SRE escalation policy. |

The `--emit-receipt-on all` sugar accepts every known event type for forward-compat, but cub-scout emits a one-time **startup warning** listing the event types it doesn't actually build receipts for (so the operator isn't surprised). Receipt-build failures are non-fatal — the underlying event still emits, just without the `receipt` key (uses `omitempty`; consumers should check key presence, not null-ness).

```python
# Idiomatic Python consumer
for line in stream:
    event = json.loads(line)
    if "receipt" in event:        # key-present check, NOT `is not None`
        verdict = pilot.decide_from_receipt(event["receipt"])
    else:
        verdict = pilot.fallback_decide(event)  # slow path
    handle(verdict)
```

## Wire shape

A `drift.detected` event with an attached receipt (one JSONL line, pretty-printed here):

```json
{
  "type": "drift.detected",
  "timestamp": "2026-05-22T11:00:00Z",
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
    "category": "DRIFT"
  },
  "receipt": {
    "_type": "https://in-toto.io/Statement/v1",
    "subject": [
      {
        "name": "k8s-live://apps/v1/Deployment/prod/payments-api",
        "digest": {"sha256": "abc..."}
      }
    ],
    "predicateType": "https://cub-scout.dev/receipt/v1",
    "predicate": {
      "version": "1",
      "predicateName": "applied-matches-spec",
      "verdict": "BLOCK",
      "claim": "applied matches spec at apps/prod/payments-api",
      "scope": {"kind": "Deployment", "name": "payments-api", "namespace": "prod"},
      "evidence": {
        "attribution": {"cause": "manual-edit", "managerHint": "kubectl-edit"},
        "gitSource": {"repoUrl": "https://github.com/org/payments", "revision": "abc123", "path": "apps/prod/payments-api"}
      },
      "omissions": [],
      "inputAttestations": [],
      "nextSteps": [],
      "verifier": {"tool": "cub-scout", "version": "v2.2.1"},
      "verifiedAt": "2026-05-22T11:00:00Z",
      "fingerprint": "sha256:1a2b3c..."
    }
  }
}
```

Pilot reads `receipt.predicate.verdict` directly: `BLOCK` → halt; `WATCH` → alert; `PASS` → log + continue; `INCONCLUSIVE` → ASK.

## Step-by-step

### Step 1 — confirm the watch stream is shaped for Pilot

```bash
# Pre-flight: doctor + status
$ cub-scout doctor
$ cub-scout status

# Confirm the event taxonomy Pilot expects
$ cub-scout watch --help | grep -A 3 '\-\-emit-receipt-on'
```

If standalone (no ConfigHub auth), the receipts in the stream will have an `OmissionConfigHubUnitSubject` entry — Pilot can still consume but the verdict ceiling is WATCH for receipts that need the connected subject.

### Step 2 — start the watch stream

```bash
$ cub-scout watch -n prod \
    --webhook https://pilot.acme/cub-scout-events \
    --emit-receipt-on drift.detected,ownership.changed \
    --output-file /var/log/cub-scout-watch.jsonl \
    --severity warning,critical
```

- `--webhook` is Pilot's listener (POST per event).
- `--output-file` keeps a local JSONL log (useful for replay during postmortems; see [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md)).
- `--emit-receipt-on` attaches receipts to the two event types where it's meaningful.
- `--severity` filters scan-finding events to the actionable subset.

The watch loop is **read-only by design**. cub-scout's only cluster operations are `Get` / `List` / `Watch`; `--emit-receipt-on` adds an in-process `BuildReceipt` per matching event, which is also pure (the receipts code path is statically guarded by `TestReceiptPackageReadOnlyClient`).

### Step 3 — Pilot's per-event handler

Pilot's logic, per event:

```python
def handle_event(event: dict) -> Verdict:
    # Fast path: event carries an inline receipt
    if "receipt" in event:
        receipt = event["receipt"]
        verdict_str = receipt["predicate"]["verdict"]  # PASS|WATCH|BLOCK|INCONCLUSIVE
        # The receipt also carries omissions[], attribution, gitSource — Pilot can drill in
        if verdict_str == "PASS":
            return Verdict.PASS
        elif verdict_str == "WATCH":
            attach_receipt_to_ticket(receipt)
            return Verdict.WATCH
        elif verdict_str == "BLOCK":
            escalate(event, receipt)
            return Verdict.BLOCK
        elif verdict_str == "INCONCLUSIVE":
            return Verdict.ASK

    # Slow path: no inline receipt (event type not in --emit-receipt-on, OR receipt-build failed)
    return synchronous_evidence_gathering(event)
```

The slow path calls back into cub-scout synchronously:

```bash
$ cub-scout compare three-way deploy/payments-api -n prod --format json > evidence.json
$ cub-scout receipt verify deploy/payments-api -n prod --save --out event.receipt.json
```

### Step 4 — verify the receipt fingerprint (optional but recommended)

Pilot may want to verify that the inline receipt's fingerprint is intact — particularly when the event arrived via a webhook that traversed network boundaries. cub-scout's receipt validate path is exit-coded:

```bash
$ echo "$event" | jq '.receipt' > event-receipt.json
$ cub-scout receipt validate event-receipt.json
# exit 0 = fingerprint matches; 1 = mismatch (refuse to act on it); 2 = I/O
```

## Operational defaults Pilot should know

- **Receipt-build failures are non-fatal**: the event still emits, the `receipt` key is omitted (omitempty), and cub-scout logs a stderr warning. Pilot's handler must check for key presence, not null-ness.
- **Warnings are rate-limited** (Codex round-6 P2 fix in #463): the first 10 receipt-build warnings pass through verbatim; afterwards, a summary line fires every 100 suppressed warnings. A long-running watch with transient cluster-read failures won't spam stderr.
- **Unsupported event types pass through silently**: `resource.discovered` and `scan.finding` accept the flag (so `--emit-receipt-on all` works) but cub-scout skips receipt-build for them in v1. A startup warning lists them.
- **Backpressure is not yet handled** (open in `#449`): if events fire faster than the consumer drains, the watch can queue up to `--max-queued-events` and then drops oldest. ApplicationSet expansion is the canonical "events fire fast" scenario.
- **Connected vs standalone**: receipts attached to events include the `confighub-unit://` subject only when connected. Standalone-mode receipts carry an `OmissionConfigHubUnitSubject` entry — Pilot should treat verdict ceiling as WATCH for those (the receipt is honest about what's missing).

## Worked example: drift event triggers Pilot

A `drift.detected` event fires for `deploy/payments-api` in prod. Pilot's webhook receives:

```json
{
  "type": "drift.detected",
  "timestamp": "2026-05-22T11:00:00Z",
  "resource": {"kind": "Deployment", "name": "payments-api", "namespace": "prod"},
  "owner": {"type": "argo", "name": "payments-api"},
  "severity": "warning",
  "details": {"category": "DRIFT"},
  "receipt": {
    "_type": "https://in-toto.io/Statement/v1",
    "predicateType": "https://cub-scout.dev/receipt/v1",
    "predicate": {
      "predicateName": "applied-matches-spec",
      "verdict": "BLOCK",
      "evidence": {
        "attribution": {"cause": "manual-edit", "managerHint": "kubectl-edit"}
      },
      "fingerprint": "sha256:1a2b3c..."
    }
  }
}
```

Pilot's handler:
1. Sees `"receipt" in event` → fast path.
2. Reads `receipt.predicate.verdict: "BLOCK"`.
3. Reads `receipt.predicate.evidence.attribution: {cause: manual-edit, managerHint: kubectl-edit}` → confirms human kubectl-edited an Argo-owned resource.
4. Renders Pilot verdict: **BLOCK**.
5. Escalates: opens a ticket, attaches the receipt, pages the on-call.

Total round-trip: one webhook POST, zero synchronous cub-scout calls. The receipt is fingerprint-verifiable later if the ticket is audited.

If the same event had arrived without an inline receipt (e.g., `--emit-receipt-on` wasn't set), Pilot would call `cub-scout compare three-way deploy/payments-api -n prod --format json` to get the same evidence on the slow path.

## Open scope on the cub-scout side (issue `#449`)

This skill calls out gaps that may surface during real deployment. They live in the open scope of `#449`, not this skill:

- **More event types**: `attribution.cause-flipped` (a resource's `cause` changes from `controller-drift` → `manual-edit`); `source-truth.verdict-changed` (a `compare source-truth` verdict flips PASS → WATCH); `bindingSource.changed` (a Link now resolves to a different upstream). All v2+ scope.
- **Backpressure / batching**: high-frequency events from ApplicationSet expansion need a documented drop / batch policy. Currently `--max-queued-events` controls in-process queue depth; downstream filtering is Pilot's call.
- **Pilot-shaped emit format**: a `--format pilot` or `--shape acceptance-event` that bundles enough context per event for Pilot to verdict-render without any synchronous call-back. Receipts inline already cover the common case, but a fuller shape could include `compareThreeWay` per event.
- **`resource.discovered` / `scan.finding` receipt support**: gated on the backpressure design (would flood on initial poll burst). Open in `#449`.

Filing these is a non-goal of this skill; surfacing them during real-deployment authoring is a goal.

## Tool boundary

- **Allowed (cub-scout):** `watch`, Compare verbs, `explain`, `trace`, `doctor`, `map`, `receipt verify / show / validate / list`. All read-only.
- **Allowed (cluster):** `kubectl get/describe`. NOT `kubectl apply / edit / patch / delete`.
- **Allowed (ConfigHub):** `cub * get / list`, `cub unit get`, `cub link list`. NOT `cub * create / update / delete`.
- **Allowed (controllers):** `argocd app get`, `flux get`. NOT `argocd app sync`, `flux reconcile`.
- **Pilot's mutation path** (REVERT, QUARANTINE, escalation) is out-of-scope for this skill.

## References

- [`scout-observe`](../scout-observe/SKILL.md) — the `watch` surface from the operator side
- [`scout-verify`](../scout-verify/SKILL.md) — receipt-emit (the `--emit-receipt-on` flag wiring)
- [`scout-compare`](../scout-compare/SKILL.md) — Compare verbs Pilot falls back to on the slow path
- [`pilot-cd-gate`](../pilot-cd-gate/SKILL.md) — pre-deploy companion (query-driven)
- [`pilot-fleet-conformance`](../pilot-fleet-conformance/SKILL.md) — periodic fleet companion (query-driven)
- [`pilot-patch-and-drift`](../pilot-patch-and-drift/SKILL.md) — drill-down for BLOCK / WATCH events
- [`pilot-incident-evidence`](../pilot-incident-evidence/SKILL.md) — replay watch logs during postmortems
- Wire format: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § Receipt Contract — Watch event emission (v2.2)
- Receipts v1+v2: `#446` (v1), `#449` (this flag), `#463` (the shipping merge)
- Read-only triad: [`scout-observe`](../scout-observe/SKILL.md), [`scout-attribute`](../scout-attribute/SKILL.md), [`scout-compare`](../scout-compare/SKILL.md), [`scout-verify`](../scout-verify/SKILL.md)

## Constraints

- Receipts on `resource.discovered` and `scan.finding` events are **not** built in v1 of `--emit-receipt-on`. Both fire on initial poll bursts and would flood without backpressure. Pilot's policy should treat those events as low-signal until v2 expands the set.
- The `receipt` key uses `omitempty` — consumers MUST check key presence, not null-ness. `event["receipt"]` raising `KeyError` is the correct empty signal; `if event["receipt"] is None` is the wrong pattern.
- The wire format is the **in-toto Statement v1 envelope** wrapping `https://cub-scout.dev/receipt/v1`. Pilot's receipt parser should be schema-aware to handle future envelope additions (e.g., signing layer).
- `--emit-receipt-on` requires the v2.2+ cub-scout binary (the flag shipped in `#463`). Older binaries don't have the flag and the watch event payload won't include a `receipt` field.
- Backpressure / batching is **deferred** — high-throughput environments should expect events to queue (`--max-queued-events`) and drop on overflow. Filing follow-on issues for batched emit shapes is a real outcome of this skill's authoring; this skill itself doesn't implement them.
