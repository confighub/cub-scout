# Bounded Resource Evidence

Unreleased v2.10 work, tracked in [#519](https://github.com/confighub/cub-scout/issues/519).
This is the Scout-side provider foundation; a companion explorer panel is not
included or claimed as shipped.

## User Questions

| User question | Read-only answer |
|---|---|
| Can I inspect one exact resource without a cluster-wide sweep? | Explicit context, API version, Kind, namespace, and name select one object. A cold read fetches one discovery document and one object. |
| Can I reuse that observation? | The real MCP gateway and the TUI picker retain at most 16 observations, for less than 15 seconds, within their own process. A hit makes no discovery/object requests. |
| Can I force a fresh check? | MCP `refresh: true` and TUI `r` discard the cached observation and repeat the two reads. Failed refresh never returns the previous success. |
| Does readiness mean this delivery succeeded? | No. Object-local readiness does not prove source revision, controller delivery, desired/live agreement, related pod health, or application success. The JSON includes omissions. |

## CLI And Plugin

From the repository, using an existing non-production context and resource:

```sh
go build ./cmd/cub-scout
./cub-scout explain Deployment/api --namespace team-a \
  --bounded --api-version apps/v1 --kube-context my-test-context --format json
./cub-scout explain Deployment/api --namespace team-a \
  --bounded --api-version apps/v1 --kube-context my-test-context --format md
```

The installed plugin equivalent is `cub scout explain ...` with the same flags.
Use `--kube-context` for Kubernetes; the host reserves `--context` for ConfigHub.
Separate CLI invocations do not share observations; `--refresh` expresses fresh
intent but does not create a persistent CLI cache. No current-context mutation
occurs. Namespaced resources require an explicit namespace; cluster-scoped
resources require no namespace. Exact Kind casing is required for bounded reads;
discovery resolves the resource name, not a pluralization heuristic.

## MCP And TUI

Start the local stdio server with `./cub-scout mcp serve`. Call its existing
`explain` tool with:

```json
{
  "resource": "Deployment/api",
  "namespace": "team-a",
  "bounded": true,
  "api_version": "apps/v1",
  "context": "my-test-context"
}
```

Repeat the same call within 15 seconds: `resourceRead.cache` becomes `hit`,
`observedAt` stays unchanged, and both `reads` counters are zero. Add
`"refresh": true` for a new observation. Cache misses, refreshes, and expiry
perform discovery plus one GET. Context or selected credential-configuration
changes replace the reader; there is no global cross-context cache.

In `./cub-scout map`, press `Ctrl+e` to open the bounded picker from the existing
filtered inventory. This picker makes no new inventory request. Choose the exact
API/Kind/namespace/name, then Enter. `r` refreshes, and Esc returns/cancels. The
viewport shows the equivalent CLI command. Normal map startup/refresh still has
its existing inventory cost; the two-read budget applies only to the bounded
selected-object operation, not the entire map application.

## Evidence And Limits

- `resourceRead.available` means an identity-matching object was read, not that
  it is healthy or desired. UID/resourceVersion are included when supplied by
  Kubernetes. Read failure leaves availability false and timestamps zero.
- `resourceRead.observedAt`, `expiresAt`, `cache`, and `reads` describe the
  observation and reuse policy. TTL is not a guarantee the object stayed current.
- `omissions` records missing source/controller, pod/event, and comparison
  evidence. No controller CLI, ConfigHub command, object LIST, or event/pod
  fan-out occurs. Missing CRDs, RBAC denial, malformed responses, ambiguous
  discovery, and mismatched object identity remain unavailable, never healthy
  or orphaned conclusions.
- Known native workload API/kind pairs use the existing generation-aware
  kstatus model. Other resources need a supported Ready condition; e.g. a custom
  resource with only application-specific status remains unknown in this path.
- Secret payloads and subresources are excluded. Raw object data is never
  included in the explain output.
- Each read has a ten-second context timeout; successful response bodies are
  capped at 2 MiB. REST retries and HTTP redirects are disabled. Authentication transport traffic
  is outside discovery/object counters. This is not a fleet-wide API rate limit.
- TUI cancellation propagates to the request. The existing serial stdio MCP
  server does not add concurrent cancellation-notification handling in this slice;
  its bounded reads still have the timeout. No bot/watch polling changes are made.

## Deterministic Proof

No cluster or authentication is required:

```sh
go test ./pkg/agent ./cmd/cub-scout -run TestBounded -count=1
go test -race ./pkg/agent ./cmd/cub-scout -run TestBounded -count=1
```

HTTP fixtures assert methods, exact paths, read counts, TTL expiry, refresh,
failed-refresh invalidation, cancellation, context/namespace isolation, eviction,
RBAC/not-found/rate-limit responses, malformed/oversized data, and identity checks.
Recorded connected/fleet cases use two explicit kube contexts and duplicate
resource names; no ConfigHub or fleet API is contacted. These test the provider,
not a connected companion panel or fleet aggregator.

## Live Smoke

Opt in against an existing non-production object. This test builds a temporary
binary, runs CLI and plugin-mode checks, then exercises the actual stdio server.
When `cub` is installed it also runs through the actual host with an isolated
temporary plugin/config directory; no installed plugin is changed.
It creates no cluster fixtures and does not change the current kube context:

```sh
CUB_SCOUT_BOUNDED_LIVE_CONTEXT=my-test-context \
CUB_SCOUT_BOUNDED_LIVE_RESOURCE=Deployment/api \
CUB_SCOUT_BOUNDED_LIVE_APIVERSION=apps/v1 \
CUB_SCOUT_BOUNDED_LIVE_NAMESPACE=team-a \
go test ./cmd/cub-scout -run '^TestBoundedExplainLive$' -count=1 -v
```

Recorded 2026-09-11 on an existing local kind Deployment: CLI, plugin-mode, and
actual `cub` v0.4.4 host checks passed; real MCP cold/hit/refresh calls reported request counts `1+1`,
`0+0`, `1+1`. The cache hit kept the original observation timestamp. Current
context was unchanged. Deterministic tests separately assert actual HTTP
request counts, including error, isolation, and cancellation cases.
The live TUI picker was checked for namespace filtering, exact selection,
read/refresh, and returning to the original view. Narrow-screen layout and
in-flight cancellation/late-result rejection have deterministic tests.

Authenticated live ConfigHub integration is not part of this read path. The
separate v2.9 release check remains blocked by expired local auth; the published
bot image also still needs anonymous registry-pull verification in #520.
