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

These references are library values, not new command fields, a fleet membership
model, ownership proof, ConfigHub Target identity or a current-state assertion.
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
trace retain their bound providers; graph export, shell, import and command mode
remain unavailable until they can honor the captured binding. These actions
cannot silently consult a subsequently changed ambient context. Legacy mode is
unchanged; genuine acceptance remains pending.

The [follow-on process proof](offline-action-guard-process-proof.json) repeats
those CLI/MCP checks after the implicit-context guard repair, with the exact
host binary retained in the private package. It remains authored loopback proof;
the eight model controls exercise the TUI guard rather than a live TUI process.
The earlier process proof remains the historical first integration checkpoint.
