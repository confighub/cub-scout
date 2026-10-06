# Watch collection omissions

Unreleased v2.14 candidate for [#768](https://github.com/confighub/cub-scout/issues/768).
A denied inventory LIST is missing evidence, so watch/bot must not report the
previously returned resource as deleted.

```sh
GOPROXY=off GOTOOLCHAIN=local \
  KUBECONFIG=/tmp/scout-offline-validation.kubeconfig \
  go test ./cmd/cub-scout -run '^TestWatchCollection' -count=1 -v
```

The authored loopback fixture returns exactly `apps/v1 Deployment team-a/api`
with UID `instance`, denies its LIST twice, recovers with the same object, then
returns a successful empty list. Each denial emits `collection.partial` with
normalized `forbidden` omission and no deletion, object observation or receipt.
The previous object stays only in private diff history without timestamp renewal.
Recovery emits no discovery; the later empty list emits exactly one deletion.
The current returned inventory is never mutated by history retention.

Mixed-scope controls distinguish Deployment/Service and two namespace LISTs by
their actual successful LIST provenance. Unknown provenance conservatively
refuses deletion. Reordered omissions produce equal sorted diagnostics, and
owner/severity filters cannot hide collection-level missing evidence. Optional
missing APIs remain omissions; a complete empty list is distinct from denial.
No extra requests or new receipts are required. A startup-path control runs the
production watch loop with a one-hour poll interval and cancels on the first
collection diagnostic: omissions appear immediately, without a discovery/finding
burst. Selecting the diagnostic for receipt attachment never calls an object
receipt builder.

Bot uses the same watch engine. For interactive inspection, `./cub-scout map
list --ownership-evidence --format ascii|json|md` and TUI `V` expose inventory
omissions. These are authored regression controls, not real controller captures,
whole-scan coverage, connected/fleet acceptance or full six-surface conformance.
Genuine live acceptance remains deferred. See [the event contract](../../docs/reference/watch-events.md).
