# RUL-02 bounded-cache replay

`rul02-cache-replay.json` is an authored deterministic input-and-result record for `TestBoundedReadRUL02IdentityCacheReplay`. Its exact HTTP bodies and SHA-256 values are served by a local `httptest` listener to the production `BoundedResourceReader`; the test uses an injected clock and checks every cache result, identity, digest, evidence time, and request count against the record.

Expected before execution: the initial read is a miss; an unexpired repeat is a hit with the original observation/expiry times; changed server bytes remain hidden by that still-valid hit until explicit refresh; refresh then returns the replacement UID and changed image digest. A second refresh changes digest while UID stays the same. At TTL expiry the next read fetches current bytes. Missing UID and immutable digest remain absent. A failed refresh and the following ordinary read both error rather than return a prior success.

Run from the repository root:

```sh
go test ./pkg/agent -run '^TestBoundedReadRUL02IdentityCacheReplay$' -count=1
```

This exercises the bounded reader against authored loopback responses only. It is not a live-cluster recording, cache-push invalidation claim, freshness guarantee before TTL/refresh, savings result, or benchmark admission. The frozen benchmark mapping, prompts, and weights are unchanged.
