# Observation Freshness

This example shows the shared `observation` metadata emitted by broad live
reads and event streams.

Use it when the question is:

- "Did this answer come from a live Kubernetes API read or from stored evidence?"
- "When was this inventory or event observed?"
- "Can I hand the same evidence to another reviewer without re-querying the API?"

## Surfaces

`map list --format json` entries include per-entry observation metadata:

```bash
./cub-scout map list --format json
```

See [`map-entry.json`](./map-entry.json).

`snapshot` includes top-level observation metadata:

```bash
./cub-scout snapshot --namespace prod --kind Deployment
```

See [`snapshot.json`](./snapshot.json).

`watch` and `bot` events include top-level event observation metadata:

```bash
./cub-scout watch --output-file /tmp/cub-scout-events.jsonl --once
./cub-scout bot --output-file /tmp/cub-scout-events.jsonl --once
```

See [`watch-event.json`](./watch-event.json).

## Contract

The shared object has:

- `source`: where cub-scout read the fact from, currently `kubernetes-api`
- `mode`: the producing surface, such as `map-list`, `snapshot`, or `watch-poll`
- `observedAt`: the collection time in UTC
- `freshness`: currently `point-in-time`
- `scope`: optional cluster/namespace/kind scope when known

`point-in-time` is not a TTL. It means the evidence was true when observed, and
callers must re-read, use a snapshot/receipt, or consume a watch/bot event if
they need a different freshness boundary.
