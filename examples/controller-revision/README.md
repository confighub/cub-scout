# Compare a Controller's Reported Revision

Status: implemented on the development branch for v2.11, not in v2.10.1.

User question: **Does this selected controller report the immutable revision
I asked for?** No ConfigHub authentication or controller CLI is needed.

## Read One Controller

Use the exact kube context, namespace and controller name from your environment.
The commit below is a synthetic fixture, not a release to deploy:

```sh
./cub-scout explain Application/api -n delivery --bounded \
  --api-version argoproj.io/v1alpha1 --kube-context <context> \
  --expected-revision aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --format json

./cub-scout explain Kustomization/api -n delivery --bounded \
  --api-version kustomize.toolkit.fluxcd.io/v1 --kube-context <context> \
  --expected-revision aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa --format md
```

The plugin equivalent is `cub scout explain ...`. Use `--format ascii` for
plain text. An OCI expectation is `sha256:` followed by the full 64 lowercase
hex digits of the artifact digest, not its origin Git commit or tag.

MCP `explain` arguments:

```json
{
  "resource": "Application/api",
  "namespace": "delivery",
  "bounded": true,
  "api_version": "argoproj.io/v1alpha1",
  "context": "your-kube-context",
  "expected_revision": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}
```

In `./cub-scout map`, open `Ctrl+e`, select the controller, and use `e` to
enter an expected revision. Enter compares; an empty entry clears it. `r`
refreshes the same scoped expectation. Esc returns to the list and clears it.

## Interpret the Answer

| Result | Meaning |
|---|---|
| `match` | The immutable ID matches the eligible controller report. |
| `mismatch` | The eligible report contains a different immutable ID of the same type. |
| `unknown` | Evidence is unavailable, unsupported, malformed, ambiguous, deleting, not current-generation where exposed, or not in the required controller state. Read `reason` and `omissions`. |

This is **not** a claim that all workloads are running that revision or that the
application is healthy. The Application fixture deliberately has degraded
health while the revision matches. The Kustomization fixture deliberately has
an old Ready transition: a transition is not a last-reconcile timestamp. No
current controller heartbeat is proven for either family.

Applications require a single Git/OCI source, a matching `comparedTo` source,
destination and ignore policy, Synced status, no pending/unsuccessful operation
or conditions, and a non-future reconciliation time no older than 15 minutes.
Kustomizations require a named supported source, observed/Ready generations
matching metadata, Ready=True, no suspension/reconciling/stalled status, and no
conflicting attempted revision. Multi-source Applications, source hydration,
chart versions and other controller kinds remain unknown. See the
[full contract](../../docs/reference/json-contracts.md#controller-revision-comparison-unreleased-v211).

## Cost and Limits

Cold: one discovery GET plus one object GET. MCP/TUI session hit: zero requests
for less than 15 seconds, even when changing only the expectation. The original
observation timestamp remains visible. Refresh/expiry discards old evidence;
an unavailable refresh cannot return an earlier matching report. CLI processes
do not share a cache. TUI inventory loading has its own, unchanged cost.

There is no new watch/bot revision filter, shared cluster cache, receipt gate,
source fetch or workload join in this slice. The separate companion panel has
not gained expected-revision input. Those limits do not change existing modes.

## Reproduce the Proof

From the repository root:

```sh
go test ./pkg/agent -run 'TestControllerRevision|TestValidateExpectedRevision' -count=2
go test ./cmd/cub-scout -run 'TestBoundedRevision' -count=2
```

`application.json` and `kustomization.json` are synthetic inputs read by the
tests, not installation manifests. Pure tests use a fixed clock. HTTP fixtures
adjust the Application time at serving time, exercise the real bounded reader
and gateway, count requests, test context/namespace separation, and reject
writes. Process tests build and invoke the CLI in standalone/plugin mode in
ASCII/JSON/Markdown, the actual plugin host when installed (isolated config),
and stdio MCP cold/hit/refresh. TUI tests cover validation, reuse, narrow terminals and
late results. No live cluster, credentials or connected service is needed.

Source schemas are pinned in the contract. This is fixture proof, not a claim
of live end-to-end release-to-workload verification or controller liveness.

An opt-in read-only live smoke is also available. Set
`CUB_SCOUT_BOUNDED_LIVE_CONTEXT`, `CUB_SCOUT_BOUNDED_LIVE_RESOURCE` (exact
Kind/name), `CUB_SCOUT_BOUNDED_LIVE_APIVERSION`,
`CUB_SCOUT_BOUNDED_LIVE_NAMESPACE`, and
`CUB_SCOUT_BOUNDED_LIVE_EXPECTED_REVISION`, then run
`go test ./cmd/cub-scout -run '^TestBoundedExplainLive$' -count=1 -v`.
It checks standalone, plugin mode, the actual plugin host when installed, and
stdio MCP cold/hit/refresh against only that selected existing object. It does
not create a controller, sync anything or change kube context. A native
Deployment is an unsupported revision source and should return unknown.
