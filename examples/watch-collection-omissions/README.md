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

## Genuine watch/bot selected-context acceptance

[Live receipt](live-context-proof.json) binds the isolated binary to product
source `22c3f958`. Actual [watch events](live-watch-events.jsonl) and
[bot events](live-bot-events.jsonl) come from one owned Kubernetes 1.35 cluster.
Both commands selected the valid private context while ambient current-context
was unusable; their discovered workload references agree. Explicit blank/missing
contexts and unusable ambient selection refused before file creation. Private
config and shared kubeconfig stayed unchanged, and the owned cluster was removed.
This is local CLI bot acceptance, not an in-cluster bot-image deployment claim.

The original capture copied a binary built at that clean source immediately
beforehand. The reproducible harness builds its own isolated candidate:

```sh
# Clean committed checkout; Docker, kind, kubectl and Go 1.24 available
python3 examples/watch-collection-omissions/verify-live-context.py
```

Credentials and raw config stay in a mode 0700 temporary directory; committed
receipts contain no tokens or kubeconfigs. Failed HTTP token-fixture and globally
forced-Offline TUI runs remain in the local verification log; neither prompted
a golden update or a production TLS change. Normal-mode full Go suite passes,
as do targeted denied/unreachable endpoint controls with zero ambient requests.
The wider live denial/recovery/informer, scanner-coverage, identity/cost and
six-surface gates remain required before v2.14.

## Polling identity (2.14 candidate)

Watch and local bot now share the map instance-identity model (#789):

```bash
./cub-scout watch --kube-context selected --cluster-identity -n team-a \
  --once --output-file /tmp/scout-watch.jsonl
./cub-scout bot --kube-context selected --cluster-identity -n team-a \
  --once --output-file /tmp/scout-bot.jsonl
```

Both emit `cluster.observed` and object events with the same verified merge key,
or an explicit identity omission. Cost is one identity-reader operation per
cycle, repeated on events; count only `cluster.observed`. No complete scanner,
whole-cycle cost, Target-binding or informer-age claim follows. Missing UID and
ambiguous finding targets remain unverified. Deleted instances keep old UIDs;
same-cluster verified UID replacement emits separate discovery/deletion.
`--watch-backed` is refused for this option. Default events remain unchanged.

Deterministic success proof: `TestWatchIdentity*` and
`TestWatchAndBotIdentityUsesSelectedConfigOneRead` in
`cmd/cub-scout/watch_cluster_identity_test.go`. They exercise exact input UIDs,
cluster collisions/denial, recreation, missing/ambiguous objects, one selected
identity GET, unchanged kubeconfig and default wire shape. Existing map tests
validate the same observed-reference serializer. Run actual owned-cluster proof
with `python3 examples/watch-collection-omissions/verify-live-identity.py`;
its private output directory retains logs and credential-bearing setup separately
from the public proof and event captures. Runtime acceptance is pending until a
source-bound passing receipt is retained.
