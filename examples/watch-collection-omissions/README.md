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

## Strict watch/bot context selection (#787)

```sh
./cub-scout watch --kube-context selected --namespace team-a --once --output-file /tmp/watch.jsonl
./cub-scout bot --kube-context selected --namespace team-a --once --output-file /tmp/bot.jsonl
./cub-scout map --kube-context selected
```

Both streaming commands capture one explicit config before opening the sink.
Inventory, scan, receipt and optional informer reads share that config. A blank
or missing context refuses without consulting ambient/in-cluster credentials or
creating the file. Defaults retain the prior behavior. The TUI already supports
the same captured selection. This adds no verified cluster ID or whole-command
cost field, and does not renew informer evidence or establish fleet membership.

`TestWatchAndBot` controls select alpha while beta is ambient, then mutate the
private config after capture. The observed HTTPS request still reaches alpha
with its original token; beta receives zero requests. Missing/blank selections
make zero reads, do not open the sink, and preserve private kubeconfig bytes.
The initial plain-HTTP credential test refused credentials as client-go intends;
the repaired TLS fixture trusts only its own local certificate.
