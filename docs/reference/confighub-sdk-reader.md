# ConfigHub SDK reader (experimental)

cub-scout reaches ConfigHub by running the `cub` CLI and parsing what it prints.
[#758](https://github.com/confighub/cub-scout/issues/758) evaluates reading
through the ConfigHub SDK's typed API client instead. This page records what
that route covers so far, what it guarantees and what was measured.

**Status: experimental and off by default.** Unreleased. Nothing changes unless
you set the variable below.

## What it covers

One command, wherever cub-scout runs it: `cub unit get` for one Unit named by
slug in one named space.

```bash
CUB_SCOUT_CONFIGHUB_READER=sdk cub scout compare source-truth deploy/api -n prod --strategy git-argo
CUB_SCOUT_CONFIGHUB_READER=sdk ./cub-scout compare source-truth deploy/api -n prod --strategy git-argo
```

| Value | Route |
|---|---|
| unset, or `cub` | `cub unit get <unit> --space <space> -o json`, as every release has |
| `sdk` | Two GETs through the SDK client: the space by slug, then the Unit by slug in that space (one GET when the space is given by its ID). No `cub` process for the read |
| anything else | An error for this read. A misspelt route does not become the default one |

The route works on the command cub-scout was about to run. Every read already
goes through one function with `cub`'s own arguments, and its callers already
parse `cub`'s JSON. Under `sdk`, a command of exactly this shape is read through
the SDK and returned as the JSON `cub` prints for it: the same typed envelope,
with the same related entities expanded, encoded the same way. The callers do
not change and cannot tell which route answered.

| Taken by the SDK route | Left to `cub`, whatever the setting |
|---|---|
| `unit get <unit> -o json --space <space>`, flags in any order | Every other `cub` command |
| `unit get <space>/<unit> -o json` | A Unit named by its ID (cub finds it in any space) |
| either, with `--quiet` | `--space "*"`, any other output format, any other flag, `-o=json` or `--space=x` spelt with `=` |

That is every `unit get` cub-scout builds from a slug; a test reads the source
and fails when a call site is added in a shape the route does not take.

| Where the read is used | Notes |
|---|---|
| `compare source-truth`, the MCP `compare_source_truth` tool, receipts built from it | Head revision and IDs |
| `compare` DRY/WET snapshots | Unit metadata; the configuration itself is `cub unit data`, still `cub` |
| MCP `confighub_unit_get` | With `space`, or `unit` as `<space>/<slug>`. By ID it stays with `cub` |
| `map` (connected hierarchy and unit detail) | |
| Import wizard checks | Reads only; its writes run `cub` |

A failed SDK read is the answer, reported with its reason. It does not fall
back to `cub`, and no message says that `cub` failed when no `cub` ran.

**The `cub` CLI is still required.** The connected commands first ask
`cub auth status` whether ConfigHub can be read, once per command, on either
route. The SDK route removes the `cub` process for the read, not for that
check, and every other ConfigHub read still runs `cub`.

## What the reader guarantees

`internal/hubread` is the only package that imports the SDK.

- **Read-only at the transport.** Its HTTP client refuses every method except
  GET and HEAD before the request is sent, and follows no redirect. The SDK's
  write calls are present in the binary and cannot leave it through this
  client; a test calls two of them and checks that nothing reaches the server.
- **Exactly one space and one Unit.** An empty or `*` scope is refused before
  any request. Slugs match exactly, including case, and are checked again on
  the response; two matches are an error, never a choice. A space is taken as
  an ID only in the canonical 36-character UUID form; anything else is looked
  up as a slug.
- **The refusals of the `cub` route apply.** A call that reached this point
  without its space is refused with the same message on either route, before
  credentials are resolved or anything is sent.
- **Credentials are read, not managed.** As a `cub` plugin it uses `CUB_SERVER`
  and `CUB_TOKEN`; otherwise the local cub configuration, honouring
  `CUB_CONFIG` and `CUB_CONTEXT`. It writes nothing, does not log in and does
  not refresh a token. A context with no token is "not configured"; no request
  is sent without a credential.
- **Each failure keeps its reason**: `invalid_scope`, `not_configured`,
  `unauthorized`, `forbidden`, `not_found`, `ambiguous`, `timeout`,
  `canceled`, `malformed`, `refused`, `request_failed`. No message is built
  from the token or from an error that could quote it: a token file that
  cannot be parsed is reported without the parser's text.

## Measured (2026-10-09, SDK core v0.8.10, darwin/arm64, Go 1.27.1)

| | cub route | SDK route |
|---|---|---|
| `cub` processes per `unit get` | 1 | 0 |
| HTTP requests per `unit get` | not measured | 2 (the space is looked up each time; nothing is cached) |
| `cub auth status` processes per command | 1 | 1 |
| JSON returned for the recorded Unit | as `cub` printed it | equal, value for value |
| source-truth result on the recorded Unit, each route fed the same recorded object | Space, Unit, Revision `2`, URL | the same |
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

## Known differences and limits

- **Names the filter cannot carry.** The reader refuses a space or Unit name
  that the SDK cannot put into a filter (`invalid_scope`) before any request.
  What `cub` does with the same name was not compared.
- **Fields the SDK does not know.** The JSON is the SDK's typed envelope
  encoded again, as `cub` does it. A field a newer server adds is absent on
  both routes until the SDK is bumped; with different SDK versions in `cub` and
  cub-scout the two routes can differ in such a field.
- **Receipts drop the reason.** A receipt built from source-truth discards the
  ConfigHub read's error on either route and reports only a missing surface,
  with advice that names `cub` login. That is existing behaviour; with the SDK
  route the advice is the wrong one.
- **The "same result" above is a fixture comparison.** The cub route was given
  the recorded `cub unit get` output and the SDK route the same object over
  HTTP from a test server. Neither ran against ConfigHub.

## Found on the way

- `cubapi.ResolveClient` in SDK core v0.8.10 passes `CUB_CONFIG`, which names
  the config **directory**, to `LoadConfig` as if it were the config **file**,
  and fails with "is a directory" when `CUB_CONFIG` names an existing
  directory (a path that does not exist yields an empty configuration). The reader
  resolves credentials with `LoadEnvironment`, `LoadConfig("")` and
  `Store.Use` instead. Not reported upstream from here.
- A token file with no `accessToken` makes `NewClientFromConfig` build a client
  that sends requests with no `Authorization` header. The reader checks for
  that first.

## Next, if this is adopted

More commands behind the same adapter, one at a time (`unit list` and
`space list` first), each held to the `cub` route's output on a recorded
response; the connected check, so that the SDK route can run without `cub`
installed; then a proof against a real server and a timing comparison; then a
decision on the default. Scout's observation interface stays read-only
throughout: the SDK's action calls are not exposed.
