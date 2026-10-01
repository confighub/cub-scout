# RUL-02 replay evidence report

The source-pinned `TestBoundedReadRUL02IdentityCacheReplay` at
`f7ad343740fb9c1ac488a06117c63466810afaa8` passed once in 3.837292250012979
seconds with exit 0 and no timeout. The wrapper recorded and verified the
Go test output, exact marker extraction, actual result, and source hashes. The
replay exercises `BoundedResourceReader` against authored local `httptest`
responses and a fixed clock. It is not a live-cluster observation.

Nine sequential results show: an initial miss; two unexpired hits with zero
requests and unchanged observation/expiry; a configured UID-B response not
served during a hit that returns cached UID-A; explicit refresh to UID-B and a
new digest; another refresh with UID-B but a changed digest; expiry followed by
UID-C; an object with no UID and only a mutable tag; and a 503 failed refresh
followed by an ordinary read that again errors instead of returning a previous
success. All configured and served bodies retain SHA-256 checks. Full proof and
wrapper artifacts are archived under the ignored
`evals/results/rul02-cache-replay-20261001/` directory; the equal-arm actual
result and neutral scope metadata are under `evals/rul02-cache-replay/`.

This establishes only the named implementation's behavior for this authored
response sequence and clock. It establishes no push/automatic invalidation,
current-cluster state, general freshness guarantee, model performance, cost,
or savings. No model/provider/live cluster was used and no production code was
changed.
