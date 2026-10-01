# HLT-04 offline report-freshness producer replay

This source-only harness prepares an isolated test tree from the pinned
`confighub/sveltos-confighub` source and injects a Go test that calls that
revision's real `internal/onboard.ReportStatus` with an in-memory `Runner`, a
fixed clock, and an in-memory writer. The Runner accepts only five authored
JSON reads; every unexpected command fails. No `kubectl`, `cub`, cluster,
provider, or live source process is run by the harness. The repository source
is exported with local `git archive` and is never edited.

The frozen benchmark question, wording, controls, and weight are unchanged.
This packet does not add an HLT-04 case mapping, prompt/grader, or benchmark
admission. No replay has been executed yet. An independent review is required
before the one offline replay.

## Expected results

All inputs are authored synthetic fixture data. The test uses `Refresh=10m`
and a fixed starting clock. These are expected producer-contract outcomes, not
claims about an actual Sveltos check or ConfigHub report:

| Case | Expected producer behavior |
|---|---|
| No held annotation | Write the synthetic report with the fixed reporter `observedAt`. |
| Existing held report with `observedAt` omitted | Rewrite it; this is distinct from a wholly absent annotation. |
| Same held report at +1 minute | Do not write; semantic fields are unchanged and held time is within refresh. |
| Same inputs at +16 minutes | Write again with a new reporter `observedAt`; health, message, and inferred revision remain equal. Source input hashes and separately authored synthetic check-execution evidence remain unchanged. |
| Malformed held `observedAt` | Rewrite; malformed time is not accepted as fresh. |
| Future held `observedAt` | Current implementation skips rewrite because negative age is less than `Refresh`; this is recorded as behavior, not freshness proof. |
| Release/apply clock skew | The code picks the latest release timestamp not after `lastAppliedTime`; the resulting revision is time-inferred and is never treated as digest proof. |
| Pre-apply condition transition | A `lastTransitionTime` before apply is treated as an old transition under the source code. It is not called a last-check time. |

For the old-check control, the test keeps a separate, explicitly authored
`synthetic-check-execution` object unchanged across renewal. It is not part of
the producer's consumed schema and is marked as synthetic in the summary. Its
timestamp is not sourced from Kubernetes or Sveltos; the test cannot claim a
real controller executed a check then. No fixture field or summary renames
`lastTransitionTime` to `lastCheckTime`.

## Pins and execution boundary

The source pin is commit
`8187910f9fe226e109e55c4d9c7c0e21297ff424` at the local repository
`/Users/alexis/code/sveltos-confighub-work`. The helper verifies an exact inventory and SHA-256 for every exported file:
`go.mod`, `go.sum`, the complete `internal/onboard/` tree, and the complete
`chartrender/` package imported by `charts.go`, including tests and fixtures. It
rejects missing, extra, or hash-mismatched files. The summary lookup uses the exact
`projectsveltos.io/cluster-profile-name` label; the separate health-check
profile label remains as defined by upstream.

The helper verifies:

- `internal/onboard/status.go` SHA-256
  `1a20679c5302c7fa7b54a0fab2da8017fd85ffc7ef796f8b031ef98bfbc4c32a`
- `internal/onboard/status_test.go` SHA-256
  `e2e7178602233ff161513697b3e4aab0e376789898589956d9e6e915b87b52d5`

It refuses a missing local commit/tool/cache, existing output path, symlink,
unexpected archive entry, hash mismatch, unsupported platform, or unavailable
macOS deny-network sandbox. The explicit replay command uses
`GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=off`, a private Go build cache,
network-denying `sandbox-exec`, and only
`go test ./internal/onboard -run '^TestHLT04OfflineReplay$' -count=1 -v`.
It caps total elapsed time at 90 seconds and combined stdout/stderr at 4 MiB.
Missing dependencies fail offline; nothing is fetched or installed. Each invocation
requires a fresh private output directory and preserves its pinned source,
stdout/stderr, hashes, tool pins, status, and cleanup record there. It accepts
success only after checking exact case names/results, per-case raw fixture
inputs, held annotation before/after, computed reports, exact write patches,
and confirmed owned-process-group cleanup plus parent reaping. Failure, timeout,
output overflow, or unconfirmed cleanup stays failed and preserves partial
output/provenance privately.

A network-denied `go test -c` compile-only preflight passed after this repair;
the resulting binary was not run. No producer replay or helper invocation has
run. After independent review, a future isolated offline replay would use a
fresh output path, for example:

```sh
python3 evals/sveltos-hlt-04-report-freshness/replay.py --execute \
  --source-root /Users/alexis/code/sveltos-confighub-work \
  --output /tmp/scout-hlt04-offline-replay-review
```

That command is documented, not authorized or executed in this source-only
packet. It does not run a benchmark arm or create a model-facing case. The
proposal and existing HLT-01 transcript remain the evidence references; the
transcript contains no raw report timestamps or controller check-execution
time. HLT-04 cannot claim live clock behavior, actual check age, applied
release identity, provider cost, or benchmark result from this replay.

## Pure helper checks

These checks do not export source, invoke Go, create sockets, or execute the
producer test:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s evals/sveltos-hlt-04-report-freshness -p 'test_*.py' -v
```
