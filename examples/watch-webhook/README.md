# Watch Webhook Example

Demonstrates `cub-scout watch --webhook` by streaming one-shot or continuous events
into a local receiver.

## What This Covers

- `cub-scout watch --webhook <url>` basic usage
- Event delivery for:
  - `resource.discovered`
  - `ownership.changed`
  - `drift.detected`
  - `scan.finding`
- Local webhook receiver for demos and validation

## Prerequisites

- `kubectl` context configured
- `cub-scout` binary available from this repo (`./cub-scout`)
- Python 3 for local receiver script

## Quick Run (One Shot)

1. Start the local receiver:

```bash
cd examples/watch-webhook
python3 mock_webhook.py --port 8787 --output /tmp/cub-scout-watch-events.jsonl
```

2. In another terminal, run one cycle:

```bash
cd /path/to/cub-scout
./cub-scout watch --webhook http://127.0.0.1:8787/events --once
```

3. Inspect captured events:

```bash
cat /tmp/cub-scout-watch-events.jsonl
```

## Continuous Run

```bash
./cub-scout watch \
  --webhook http://127.0.0.1:8787/events \
  --interval 20s \
  --namespace default \
  --severity warning,critical
```

Stop with `Ctrl+C`.

## File Sink (No Webhook)

Write events directly to local JSONL:

```bash
./cub-scout watch --output-file /tmp/cub-scout-watch-events.jsonl --once
```

## Sample Event

```json
{
  "type": "scan.finding",
  "timestamp": "2026-03-07T17:00:00Z",
  "resource": {
    "kind": "Deployment",
    "name": "api",
    "namespace": "prod"
  },
  "owner": {
    "type": "Flux",
    "name": "frontend-app"
  },
  "severity": "warning",
  "details": {
    "category": "STATE",
    "message": "out of sync"
  }
}
```

## Notes

- This example is read-only and does not mutate cluster state.
- Use `--once` for deterministic smoke checks in local validation scripts.
- For remote endpoints, ensure the webhook URL is reachable from the machine running `cub-scout`.
