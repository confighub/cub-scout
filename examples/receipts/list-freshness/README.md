# Receipt List Freshness

This example shows the local receipt-store index for saved receipts with and
without `--ttl`.

`receipt verify --ttl <duration>` stamps immutable freshness fields into the
receipt. `receipt list` does not re-read Kubernetes and does not modify the
receipt; it derives a list-time `freshness.status` from the saved
`observedAt`, `expiresAt`, and `ttl` fields.

```bash
./cub-scout receipt verify deploy/api -n prod --ttl 1h --save --save-dir ./tmp-receipts
./cub-scout receipt list --dir ./tmp-receipts --format json
```

[`receipt-list.json`](./receipt-list.json) shows the expected list-output
shape.

Expected status meanings:

| Status | Meaning |
|---|---|
| `fresh` | TTL-backed receipt has not expired at list time |
| `stale` | TTL-backed receipt has expired at list time |
| `not-declared` | Receipt was created without `--ttl` |
| `invalid` | Freshness fields are present but malformed |
