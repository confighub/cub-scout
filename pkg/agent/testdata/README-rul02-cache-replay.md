# RUL-02 bounded-cache replay

`rul02-cache-replay.json` is the authored deterministic input-and-check record for `TestBoundedReadRUL02IdentityCacheReplay`. The named test serves its exact HTTP bodies to the production `BoundedResourceReader` through a local `httptest` listener and uses the fixed clock in the record. With `-v`, it emits one `RUL02_REPLAY_JSON=` line containing actual results: neutral step IDs, clock/refresh inputs, full returned object or null, full `BoundedReadEvidence`, sanitized error classification, actual per-step method/path/status/body/hash records, and cumulative request counts. The emitted record contains no fixture step labels or expected values; expected checks stay in the test fixture.

Expected before execution: the initial read is a miss; an unexpired repeat is a hit with original observation/expiry times; changed server bytes remain hidden by that valid hit until explicit refresh; refresh then returns the replacement UID and changed image digest. A second refresh changes digest while UID stays the same. At TTL expiry the next read fetches current bytes. Missing UID and immutable digest remain absent. A failed refresh and the following ordinary read both error rather than return a prior success.

Run from the repository root with an explicit offline kubeconfig:

```sh
KUBECONFIG=/tmp/scout-offline-validation.kubeconfig go test ./pkg/agent -run '^TestBoundedReadRUL02IdentityCacheReplay$' -count=1 -v
```

Reviewed replay status: the named test passed once at source revision `f7ad343740fb9c1ac488a06117c63466810afaa8` in 3.837292250012979 seconds, exit 0, without timeout. Its byte-exact extracted output and wrapper proof are retained in the ignored `evals/results/rul02-cache-replay-20261001/` archive. The packaged equal-arm fixture is `evals/rul02-cache-replay/fixtures/rul02-cache-replay.json`; its source pins and neutral scope are in `capture-scope.json`. The replay exercises authored loopback responses only. It is not a live-cluster recording, cache-push invalidation claim, freshness guarantee outside its fixed steps, savings result, or paid benchmark run. The frozen question, reference, controls, and weights are unchanged; benchmark admission remains false and the suite non-executable.

Each emitted input also retains the authored server response configured for that
step, even when a cache hit performs no HTTP request. This distinguishes changed
fixture state from actual served responses without calling it a live observation.
