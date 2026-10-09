# ConfigHub SDK reader (experimental)

cub-scout reaches ConfigHub by running the `cub` CLI and parsing what it prints.
[#758](https://github.com/confighub/cub-scout/issues/758) evaluates reading
through the ConfigHub SDK's typed API client instead. This page records the
first slice: one read, opt-in, measured.

**Status: experimental and off by default.** Unreleased. Nothing changes unless
you set the variable below.

## What it covers

One read: the Unit lookup in `compare source-truth` (the Unit's head revision
and IDs), which is also what the MCP `compare_source_truth` tool and receipts
built from it use.

```bash
CUB_SCOUT_CONFIGHUB_READER=sdk cub scout compare source-truth deploy/api -n prod --strategy git-argo
CUB_SCOUT_CONFIGHUB_READER=sdk ./cub-scout compare source-truth deploy/api -n prod --strategy git-argo
```

| Value | Route |
|---|---|
| unset, or `cub` | `cub unit get <unit> --space <space> -o json`, as every release has |
| `sdk` | Two GETs through the SDK client: the space by slug, then the Unit by slug in that space. No `cub` process |
| anything else | An error. A misspelt route does not become the default one |

A failed SDK read is reported as a ConfigHub omission with its reason. It does
not fall back to `cub`.

## What the reader guarantees

`internal/hubread` is the only package that imports the SDK.

- **Read-only at the transport.** Its HTTP client refuses every method except
  GET and HEAD before the request is sent, and follows no redirect. The SDK's
  write calls are present in the binary and cannot leave it through this
  client; a test calls two of them and checks that nothing reaches the server.
- **Exactly one space and one Unit.** An empty or `*` scope is refused before
  any request. Slugs match exactly, including case, and are checked again on
  the response; two matches are an error, never a choice.
- **Credentials are read, not managed.** As a `cub` plugin it uses `CUB_SERVER`
  and `CUB_TOKEN`; otherwise the local cub configuration, honouring
  `CUB_CONFIG` and `CUB_CONTEXT`. It writes nothing, does not log in and does
  not refresh a token. A context with no token is "not configured"; no request
  is sent without a credential.
- **Each failure keeps its reason**: `invalid_scope`, `not_configured`,
  `unauthorized`, `forbidden`, `not_found`, `ambiguous`, `timeout`,
  `malformed`, `refused`, `request_failed`. No message contains the token.

## Measured (2026-10-09, SDK core v0.8.10, darwin/arm64, Go 1.27.1)

| | cub route | SDK route |
|---|---|---|
| `cub` processes per read | 1 | 0 |
| HTTP requests per read | not measured | 2 |
| Result on the recorded Unit | Space, Unit, Revision `2`, URL | identical |
| Time to start `cub` at all (`cub version`, no network, 5 runs) | 0.40 to 0.53 s | not applicable |

| Cost of adopting the SDK | Before | After |
|---|---|---|
| `cub-scout` binary (local build) | 79.7 MB | 87.2 MB (+7.4 MB, +9.3%) |
| Modules linked into the binary | 66 | 87 |
| `go.mod` requirements | 73 | 94 |
| Minimum Go | 1.25 (raised for this in #846) | 1.25 |

Among the 21 modules newly linked are `sigs.k8s.io/kustomize/kyaml` and, by way
of `github.com/cockroachdb/errors`, `github.com/getsentry/sentry-go`. Neither
SDK core v0.8.10 nor cub-scout initialises Sentry or calls a reporting
function, so it is linked and inert.

## Not measured, and not claimed

- **No run against a real ConfigHub server.** The tests use an object recorded
  from a real v0.8.3 server
  (`test/fixtures/confighub-governance-v083-recorded/unit-get.json`); the list
  envelope and the server's handling of the `where` filter are simulated. That
  the two GETs return this shape from a live server is not proven here.
- **End-to-end time for either route**, and how many HTTP requests `cub` itself
  makes, were not measured. The `cub version` timing is only a lower bound on
  what starting a process costs.
- **No agent cost or time claim.** Fewer processes is not a measured saving
  for an agent.

## Found on the way

- `cubapi.ResolveClient` in SDK core v0.8.10 passes `CUB_CONFIG`, which names
  the config **directory**, to `LoadConfig` as if it were the config **file**,
  and fails with "is a directory" whenever `CUB_CONFIG` is set. The reader
  resolves credentials with `LoadEnvironment`, `LoadConfig("")` and
  `Store.Use` instead. Not reported upstream from here.
- A token file with no `accessToken` makes `NewClientFromConfig` build a client
  that sends requests with no `Authorization` header. The reader checks for
  that first.

## Next, if this is adopted

More reads behind the same interface, one at a time, each held to the `cub`
route's result on a recorded response; then a proof against a real server and a
timing comparison; then a decision on the default. Scout's observation
interface stays read-only throughout: the SDK's action calls are not exposed.
