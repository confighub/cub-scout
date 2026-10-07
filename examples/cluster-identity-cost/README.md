# Cluster identity and read-cost foundation

User question: "Can two contexts with the same label still be kept distinct,
and what did reading their identity cost?"

This foundation for [#599](https://github.com/confighub/cub-scout/issues/599)
has an **unreleased v2.14 candidate** opt-in map integration. Existing default
command request budgets remain unchanged. It complements the wider
[observation request baseline](../observation-budget/).

From the repository root, reproduce the deterministic loopback controls:

```sh
GOPROXY=off GOTOOLCHAIN=local \
  KUBECONFIG=/tmp/scout-offline-validation.kubeconfig \
  go test ./pkg/agent -run 'TestClusterIdentity|TestKubernetesReadMeter' -count=1 -v
```

Two independently configured HTTP fixtures use the same context label and
return different `kube-system` Namespace UIDs. The reader captures a copied REST
configuration before making requests. Changing the original configuration does
not retarget it. Construction makes zero requests; every successful read makes
one `GET /api/v1/namespaces/kube-system`, with no discovery, redirect, REST retry,
workload read, identity cache or second-context fallback. Existing bounded,
noninteractive exec authentication remains in use; authentication refresh does
not retry the identity request.

Only an exact `v1/Namespace` named `kube-system` with a nonempty observed UID
establishes verified Namespace-instance identity. This distinguishes the two
fixture instances; it is not a universal infrastructure identifier or a
ConfigHub Target join. Recreating that Namespace can change the identity.
Missing UID, denial, malformed responses, timeout and unreachable endpoints
return unverified identity, an omission reason and no successful observation
timestamp. They do not establish ownership, orphan status or workload health.
API endpoint output omits user information, query and fragment fields.

The controls compare transport attempts and consumed response-body bytes with
the fixture, including error bodies, early body limits and read errors. The
meter excludes HTTP/TLS headers, wire compression, authentication traffic and
other clients. A preexisting opaque transport wrapper explicitly makes cost
coverage partial. Duration includes cancellable waiting for this reader;
separate calls are serialized for attributable meter deltas, and `reused` is
always false. Concurrent meter controls are suitable for the Go race detector.

This is standalone loopback proof, using no cluster or credentials. It does not
prove connected/fleet identity, whole-command costs, CLI/MCP/TUI conformance,
merge-safe object references or genuine live acceptance. Those remain open in
#599 and the [3.0 execution plan](../../docs/roadmap-3.0-execution.md).

## Merge-safe object reference foundation

The pure `NewObservedResourceIdentity` helper requires verified cluster evidence
and an object's supplied API version, kind, name, UID and namespace consistent
with an explicitly supplied served API scope. The caller must collect both
observations with the same captured client and obtain scope from the served API.
The helper makes no requests and cannot prove that association or atomicity.
Absent GVK is refused; labels and context names do not fill missing fields.

```sh
GOPROXY=off GOTOOLCHAIN=local \
  KUBECONFIG=/tmp/scout-offline-validation.kubeconfig \
  go test ./pkg/agent -run '^TestObservedResourceIdentity' -count=1 -v
```

Authored fixture controls use the same `apps/v1 Deployment` named `team-a/api`
and the same object UID in two same-label cluster observations, with distinct
Namespace-instance UIDs. Their canonical JSON merge keys differ. Recreated
object UIDs, API groups, kinds, namespaces and names also differ; equivalent
served versions retain the same instance key. Core-group and cluster-scoped
identities are supported without inferring scope from missing namespace.
Opaque UID delimiters retain exact field boundaries. Unknown cluster identity,
malformed/missing fields, contradictory scope and invalid exported-field
mutations are refused. The original object remains unchanged.

These references started as library values. The candidate opt-in map integration
below exposes supported instance evidence; it is not a fleet membership model,
ownership proof, ConfigHub Target identity or a current-state assertion.
They do not complete #599 or its CLI/MCP/TUI and genuine acceptance gates.

## Opt-in map identity (v2.14 candidate, unreleased)

In the candidate source, `./cub-scout map list --cluster-identity --format json`
returns a separate `map-list-cluster-identity.v1` envelope. The matching MCP
argument is `cluster_identity: true`; the standalone TUI starts with
`./cub-scout map --cluster-identity` and shows loaded evidence in its existing
`V` view. All support `--kube-context`/MCP `context` for captured selection.
The flag admits one extra bounded Namespace GET per inventory refresh, with
no second lookup when opening the evidence view. Defaults and the ownership-only
diagnostic envelope keep their existing read budgets.

```sh
GOPROXY=off GOTOOLCHAIN=local \
  KUBECONFIG=/tmp/scout-offline-validation.kubeconfig \
  go test ./cmd/cub-scout -run '^TestMapClusterIdentity' -count=1 -v
```

Eight authored loopback/model controls exercise the production CLI collection and
ASCII/JSON/Markdown renderers, MCP argument/schema/data projection, and a
headless TUI's actual `V` action and visible viewport refresh. They cover a
captured selection after same-name kubeconfig retargeting; exact Namespace
request/body counts; denied identity with retained resources; readable empty
versus denied/unavailable inventory; default zero added identity reads;
pre-read mode/recorded/test-hook refusals; client/configuration failure with
zero requests; error redaction; and safe control/fence rendering.

`cluster.cost` measures only that identity reader. Inventory requests,
authentication, other clients and wire/header bytes are outside its scope;
`clusterCostScope` is `identity-reader`. Collection status is `complete` or
`partial` for the attempted inventory lists, or `unavailable` with an explicit
reason when configuration/client construction prevented collection. Neither
empty resources nor identity failure establish an orphan or health verdict.

Summary/count/names-only and ownership-only envelope combinations are refused
before reads. Recorded input cannot start live identity lookup. The TUI replaces
failed refresh identity with unavailable evidence and updates the visible panel;
it does not renew a previous successful timestamp. The tests use no real cluster
or credentials and do not replace genuine final acceptance, broader command
identity/cost, six-surface conformance or connected Target alignment.

The [offline process proof](offline-process-proof.json) runs the rebuilt local
CLI in JSON/ASCII/Markdown and the actual MCP stdio process against an authored
loopback HTTP server. Default inventory makes zero identity requests; the opt-in
makes one, preserving inventory on identity denial. Invalid combinations and an
unknown explicit context make zero API requests. The proof pins the modified
source base/diff, binary and private output package. It does not exercise a real
cluster, a TUI process, benchmark admission or live release acceptance.

In the opt-in identity TUI, implicit current-context selection uses the same
captured-provider safeguards as an explicit selector. Bounded explain, scan and
trace retain their bound providers; shell, import and command mode remain unavailable until they can honor the
captured binding. Graph export now uses the captured in-process collector
accepted in the [graph example](../graph-export/). These actions
cannot silently consult a subsequently changed ambient context. Legacy mode is
unchanged; genuine acceptance remains pending.

The [follow-on process proof](offline-action-guard-process-proof.json) repeats
those CLI/MCP checks after the implicit-context guard repair, with the exact
host binary retained in the private package. It remains authored loopback proof;
the eight model controls exercise the TUI guard rather than a live TUI process.
The earlier process proof remains the historical first integration checkpoint.

## Fresh integrated live CLI checkpoint — 2026-10-06

The [retained summary](live-cli-identity-proof.json) records one owned Kubernetes
1.35 cluster with actual administrator and restricted-service-account reads.
JSON/ASCII/Markdown map identity runs pass: the administrator ID matches the
actual kube-system Namespace UID; the viewer retains the workload inventory while
identity stays unverified with a denial omission. Actual write and Secret-read
authorization checks refuse. The cluster was removed and shared kubeconfig bytes
were unchanged. Private credentials and raw configuration remain local.

The first attempt used an unsupported `-n` shorthand and failed before Scout
read the cluster; its cleanup and failure remain in the summary. The corrected
harness uses `--namespace`. The binary was built from modified integrated source
before its commit; build metadata and the source-binding limit remain explicit.
This is CLI acceptance only, not a clean-commit release binary, TUI/MCP process,
two-cluster isolation, controller health, six-surface or total-command cost proof.

## Merge-safe map instances (#783, v2.14 candidate)

```sh
./cub-scout map list --cluster-identity --namespace team-a --kind Deployment --format json
./cub-scout map --cluster-identity
```

In the identity envelope, each returned row has `resourceIdentity`: a verified
observed reference/merge key or a precise omission. The legacy display ID is
unchanged. The key distinguishes verified cluster UID, API group/kind,
namespace/name and object UID, including deletion/recreation. API version is
retained without creating a second key for the same group/kind/UID.

The already returned objects supply these facts, so there is no extra GET or
discovery request. Registry-backed scope and exact response type/version are
required; custom resources without declared scope remain unverified. Identity
denial or missing UID retains ownership/inventory with no guessed key. CLI
ASCII/Markdown and MCP show the same evidence; TUI `V` shows it from loaded rows.
An unavailable identity refresh cannot continue displaying old verified keys.

Deterministic controls are `TestMapResourceIdentity*`, alongside existing
`TestObservedResourceIdentity*` and `TestMapClusterIdentity*`. They cover exact
keys, recreation/cluster collisions, missing metadata, response mismatch,
unknown scope, default JSON stability, unchanged request counts and retained
TUI evidence refusal. Their loopback inputs are authored, not live captures.

## Genuine instance acceptance (#783)

[Passing receipt](live-instance-proof.json) binds the candidate binary to clean
product source `44322725`. Two disposable Kubernetes 1.35 clusters used the same
context label and workload name: actual CLI ASCII/JSON/Markdown keys differed
across clusters, actual MCP agreed with CLI, and the actual standalone TUI showed
the observed UID. Deleting/recreating the owned workload changed its UID/key.
A restricted ServiceAccount retained the workload with an explicit unverified
identity and no merge key; an actual Namespace GET was forbidden. Both clusters
were removed and the shared kubeconfig stayed unchanged.

The [first receipt](live-instance-proof-failed.json) records a harness shutdown
timeout after the TUI had displayed its UID; recreation/reader checks had not
run. The repaired harness stops only its own child, including forced termination
after its deadline. This proves visible TUI evidence, not graceful exit. Compiler
VCS metadata was unavailable in this managed-worktree build; the receipts state
the clean source/build/hash binding instead. Raw captures and credentials remain
private; the committed receipts contain no token or kubeconfig.

To reproduce from a **clean committed checkout**, with Docker, kind, kubectl and
Go 1.24 available:

```sh
python3 examples/cluster-identity-cost/verify-live-instances.py
```

This creates and deletes only two uniquely named owned clusters, builds an
isolated candidate, uses private kubeconfigs, and stores raw evidence in a mode
0700 temporary directory. It makes no paid model calls. Do not run this against
a shared cluster. The receipt does not complete whole-command cost accounting,
Target binding, other commands or the wider six-surface/controller gates.

## Tree context follow-on (#794)

The candidate tree selector reuses the captured configuration foundation.
Run `./cub-scout tree ownership --kube-context selected --namespace team-a
--format json` to obtain the selected context label from the same capture.
Composition binds both typed-client reads and `kubectl` subprocesses to a private
configuration/cache; blank/missing contexts refuse before reads, including
aliases. `tree config` has no Kubernetes selection. Existing Git/patterns and
composition collection breadth and partial-list semantics are retained.

Deterministic coverage: `go test ./cmd/cub-scout -run
'TestTreeExplicit|TestTreeCompositionChild|TestRunTreePatterns' -count=1`.
The `verify-live-tree-context.py` harness creates and removes one owned kind
cluster, uses an unusable ambient selection and a private HOME/config, and
exercises all cluster views and explicit-selection refusals. Its proof measures
this scoped command acceptance, without identity/cost/Target or full six-surface
claims.

The [source-bound live proof](live-tree-context-proof.json) passed on one owned
Kubernetes 1.35 cluster with the ambient selection unusable. Two retained
attempts failed harness assertions (ownership JSON path and suggest display
name); their owned clusters were removed and shared kubeconfig unchanged. This
proof is dated at its recorded source checkpoint; it is not release acceptance.

## Map subcommand context follow-on (#804)

Every `map` subcommand that reads a cluster accepts `--kube-context`, with the
same capture-once semantics as `map list`:

```bash
./cub-scout map issues --kube-context selected
./cub-scout map cronjobs --kube-context selected --namespace team-a
./cub-scout map actions deployment/api --kube-context selected --namespace team-a
```

The selection is resolved once and every Kubernetes read in the command uses it.
A missing or blank selection refuses before any read; nothing falls back to the
ambient context or in-cluster credentials. Without the flag, behaviour is
unchanged. `map hub`, `map fleet` and `map queries` read no cluster and reject
the flag.

Deterministic coverage: `go test ./cmd/cub-scout -run
'TestMapSubcommandsReadOnlyTheSelectedContext|TestEveryMapSubcommandIsClassified' -count=1`.
Each subcommand runs against a selected and an ambient HTTP server; the ambient
server must receive zero requests.

`verify-live-map-context.py` creates and removes one owned kind cluster with a
private kubeconfig whose current-context is unusable, then runs all 21
subcommands with `--kube-context selected`, plus the blank and missing
refusals. The [source-bound live proof](live-map-context-proof.json) passed on
Kubernetes 1.35. The [first attempt](live-map-context-attempt-1.json) is
retained: `map status` exited 1 because it counts a Deployment scaled to zero
as a problem. That is a separate defect; the proof uses a running workload.

This shows which context each subcommand reads. It does not add cluster
identity, read cost, omission reporting or Target binding to these subcommands.
