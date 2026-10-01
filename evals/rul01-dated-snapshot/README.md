# RUL-01 dated snapshot fixture

This prepared, unrun case reuses the accepted PRE-02 after-Pod response and its
retained request-time receipt. The exact raw response is 5,355 bytes with SHA-256
`f264204dc296d06590bc357691a1b95db08222f4ac68b3c38935d6ef200832c3`; its source
receipt binds the `GET` response to the exact Pod path, HTTP 200, and request
start/end logged as `2026-10-01T06:03:25Z`. The request elapsed
`0.01204633410088718` seconds. Printed times have one-second precision, so the
equal displayed start/end seconds do not mean a zero-duration request or an
exact timestamp instant.

The two as-of values, `06:08:25Z` and `06:13:25Z`, are authored test-clock
inputs. Relative to the receipt's logged request-end second, the ages are 300
and 600 seconds. They test that the same snapshot keeps its capture time while
the arithmetic age advances. They do not assert the current state of any
cluster. Pod creation time and `PodScheduled` transition time are separate
fields, even though this recording prints both in the same second.

`contract.py` is a fixture validator, not a product feature. It verifies raw
bytes, source pins, request identity/timing, resource UID/condition and test
clocks; it uses no filesystem mtime or current clock. A missing or changed
binding is an error/unknown, never a license to infer capture time. The
recorded-object product interface does not currently expose trusted capture
time. Both benchmark arms stage byte-identical inputs using `scaffold.sh`.

No model, provider, or cluster was run for this case. It is not an independent
capture from PRE-02, a live freshness result, benchmark admission, or a savings
claim. Run its deterministic checks with:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/rul01-dated-snapshot -v
```
