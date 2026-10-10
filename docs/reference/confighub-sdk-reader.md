# ConfigHub SDK reader

cub-scout reaches ConfigHub in two ways. Most commands run the `cub` CLI and
parse what it prints. Three reads go through the ConfigHub SDK's typed API
client instead ([#758](https://github.com/confighub/cub-scout/issues/758)),
with no `cub` process for the read. This page records what that route covers,
what it guarantees and what was measured.

**Status: the default for the reads it covers. Unreleased.** To read through
`cub` instead, as every release so far has, set
`CUB_SCOUT_CONFIGHUB_READER=cub`. That is the way back if the two ever
disagree, and a failed read says so when it could help.

`cub` is still required: the connected commands check the session with
`cub auth status`, and every other ConfigHub read runs `cub`.

## What it covers

Three commands, wherever cub-scout runs them: `cub unit get` for one Unit
named by slug in one named space, `cub unit list` for one named space, with
or without a `--where` or `--contains` filter, and `cub space list`.

```bash
# The default: these reads go through the SDK
cub scout compare source-truth deploy/api -n prod --strategy git-argo
./cub-scout compare source-truth deploy/api -n prod --strategy git-argo

# The way back: every read through cub
CUB_SCOUT_CONFIGHUB_READER=cub cub scout compare source-truth deploy/api -n prod --strategy git-argo
CUB_SCOUT_CONFIGHUB_READER=cub ./cub-scout compare source-truth deploy/api -n prod --strategy git-argo
```

| Value | Route |
|---|---|
| `cub` | The `cub` command, as every release before this default |
| unset, or `sdk` | GETs through the SDK client, and no `cub` process for the read. `unit get`: the space by slug, then the Unit by slug in that space. `unit list`: the space by slug, then every Unit in it, in one request as `cub` asks for it. `space list`: one request. Naming the space by its ID saves the first request, except for a unit list that comes back empty, where the space is then checked |
| anything else | An error for these reads. A misspelt route does not become the default one |

The route works on the command cub-scout was about to run. Every read already
goes through one function with `cub`'s own arguments, and its callers already
parse `cub`'s JSON. Under `sdk`, a command of exactly this shape is read through
the SDK and returned as the JSON `cub` prints for it: the same typed envelope,
with the same related entities expanded, encoded the same way. On the Unit
recorded from a real server the two are equal byte for byte. The callers do not
change.

| Taken by the SDK route | Left to `cub`, whatever the setting |
|---|---|
| `unit get <unit> -o json --space <space>`, flags in any order | Every other `cub` command |
| `unit get <space>/<unit> -o json` | A Unit named by its ID, in any spelling `cub` reads as an ID: canonical, 32 bare hex digits, braces, `urn:uuid:` |
| either, with `--quiet` | A space named by ID in any spelling but the canonical lower-case one, capitals included |
| a space named by its canonical ID | `*` as the space or the Unit; a space containing `/`; a `--space` value that starts with `-` |
| `unit list -o json --space <space>`, with or without `--quiet`, `--where <expression>` and `--contains <text>` | A unit list with a stored filter, a data or trigger filter, a selection, a limit, an ordering, a view or hidden entities; an empty `--where` or `--contains`, or either given twice |
| `space list -o json`, with or without `--quiet` | A space list with any filter or any of the same, a component, a `--space` or a positional; and a list with no `-o json`, which prints `cub`'s table |
| | Any other output format, any other flag, `--`, and `-o=json` or `--space=x` spelt with `=` |

Today that is every `unit get` cub-scout builds, given a slug and a named
space, and every plain `unit list` and `space list`. A test reads the source
of each call site: a `unit get` in a shape the route does not take fails it,
and so does a list that is neither taken nor named in the test with the reason
it stays with `cub`. The test models the arguments' shape, not the values a
user supplies.

| Where the read is used | Notes |
|---|---|
| `compare source-truth`, the MCP `compare_source_truth` tool, receipts built from it | Head revision and IDs |
| `compare` DRY/WET snapshots | Unit metadata; the configuration itself is `cub unit data`, still `cub` |
| MCP `confighub_unit_get` | With `space`, or `unit` as `<space>/<slug>`. By ID it stays with `cub` |
| `map` (connected hierarchy and unit detail) | |
| Import wizard checks | Reads only; its writes run `cub` |
| The lists: `map` (connected hierarchy, fleet, and joining cluster resources to their Units), `fleet outliers`, import's check for existing units and its link to the space, MCP `confighub_units` with or without `where` and `contains`, and the `views` lookups when they name one space | |
| Lists that stay with `cub`: `views` across every space, GitOps delivery evidence (a selection), `app list` and import's summary (they print `cub`'s own output) | |

A failed SDK read is the answer, reported with its reason. It does not fall
back to `cub`, and once the SDK route has taken a read, no failure of it is
worded as a `cub` failure. Where the failure is one `cub` might not have had
(the answer could not be decoded, was cut short, was refused by this reader's
own rules, timed out, or the request failed outright), the error ends by
naming `CUB_SCOUT_CONFIGHUB_READER=cub`. A denial, a missing Unit or a missing
credential is the same by either route, and nothing is suggested for those.

A value that names no route is an error for every read in the left-hand
column, in every command that makes one, and is not consulted for anything in
the right-hand column, which has only the `cub` route.

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
- **A list is whole or it is an error.** A unit list is for exactly one
  space: a Unit the server returns from any other space, an entry with no
  Unit, a 200 that is not JSON, or a response the server marks as cut short
  (`incomplete`) fails the read. None of them becomes a shorter or an empty
  list. A list with nothing in it, or a JSON `null`, is `[]`, as `cub` prints
  it. "Whole" means what the server says is whole: the reader asks for no
  limit and does not read in pages.
- **A filter narrows and never widens what is returned.** The caller's
  expression is sent as written, AND-ed with the space, as `cub` composes it.
  Nothing here reads the expression, so the check that every entry is a Unit
  in that space is what holds whatever it says: an expression the server read
  as reaching beyond the space would fail the read, where `cub` would print
  what came back. `cub` documents AND only, and ConfigHub v0.8.3 rejects an
  OR with HTTP 400 ("expected AND before") by either route, so on that server
  an expression cannot reach beyond the space this way. Entities the server expands inside an entry (an
  upstream, a link, a target) are as in `cub`'s output and are not checked.
- **A rejected filter keeps the server's reason.** For an HTTP 400 the
  server's message is passed on, cut to 300 characters, with control and
  format characters and line separators replaced by spaces so that nothing in
  it can act on a terminal. Any other failure keeps this reader's own words.
- **An empty list is of a space that exists.** A space named by slug is looked
  up first. A space named by ID is not, so when its list comes back empty the
  space is checked, and an ID that names no space is `not_found`, as `cub`
  reports it, not "no units".
- **The refusals of the `cub` route apply.** A call whose space was never
  resolved carries a placeholder in its place and is refused with the same
  message on either route, before credentials are resolved or anything is
  sent.
- **Credentials are read, not managed, and are the ones `cub` would use.**
  Started by `cub` as a plugin (`CUB_PLUGIN=1`) it uses the `CUB_SERVER` and
  `CUB_TOKEN` that `cub` passes; otherwise the local cub configuration,
  honouring `CUB_CONFIG` and `CUB_CONTEXT`. A `CUB_SERVER` and `CUB_TOKEN`
  exported in a shell are not used: the `cub` CLI does not read them, so
  they would send this reader to a server `cub` is not talking to. It writes nothing, does not log in and does
  not refresh a token. A context with no token is "not configured"; no request
  is sent without a credential.
- **Each failure keeps its reason**: `invalid_scope`, `not_configured`,
  `unauthorized`, `forbidden`, `not_found`, `ambiguous`, `timeout`,
  `canceled`, `malformed`, `refused`, `incomplete`, `request_failed`. A Unit that does not
  exist is an empty list and `not_found`; an HTTP 404 is a missing endpoint,
  so it is `request_failed` with advice to check the server URL, never "no
  such Unit". No message is built
  from the token or from an error that could quote it: a token file that
  cannot be parsed is reported without the parser's text.

## Measured (2026-10-09, SDK core v0.8.10, darwin/arm64, Go 1.27.1)

| | cub route | SDK route |
|---|---|---|
| `cub` processes per `unit get` | 1 | 0 |
| HTTP requests per `unit get` | not measured | 2 (the space is looked up each time; nothing is cached) |
| `cub auth status` processes per command | 1 | 1 |
| JSON returned for the recorded Unit | as `cub` printed it | equal, byte for byte |
| source-truth result on the recorded Unit, each route fed the same recorded object | Space, Unit, Revision `2`, URL | the same |
| Time to start `cub` at all (`cub version`, no network, 5 runs, on this Mac) | 0.40 to 0.53 s | not applicable |

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

## Against a real server (2026-10-09, CI run 37967084137)

`TestSDKReaderMatchesCubOnARealServer` runs in the Connected E2E lane against
the disposable ConfigHub that lane installs (server v0.8.3, `cub` v0.8.3;
cub-scout built with SDK core v0.8.10). It creates a space and two Units
through `cub`, then asks for one Unit by each route.

| Checked on the real server | Result |
|---|---|
| Reader's JSON against `cub unit get -o json`, space named by slug | identical, 2216 bytes |
| The same, space named by its ID | identical |
| Requests the reader made | 2 by slug, 1 by ID |
| The second Unit in the space, a Unit that does not exist, a space that does not exist | the second Unit and no other; `not_found`; `not_found` |
| MCP `confighub_unit_get` through the built binary, `cub` route against `sdk` route | the whole answer identical |
| Reader's JSON against `cub unit list -o json`, space by slug and by ID (CI run 37971287996) | identical, 4717 bytes, two Units |
| Reader's JSON against `cub space list -o json` (same run) | identical, 1456 bytes, two spaces |
| MCP `confighub_units` through the built binary, by each route (same run) | the whole answer identical |
| `unit list --where`, by slug and by a `LIKE` pattern, and one that matches nothing (CI run 37976845378) | identical; one Unit, the other Unit, and `[]` |
| A `--where` the server rejects (same run) | both routes fail; the reader's error carries the server's reason, "unrecognized attribute name" |
| `unit list --contains` (same run) | **not established**: this server answers the search with HTTP 500, to `cub` itself, so both routes fail |
| A `--where` with an OR (CI run 37978701723) | both routes fail with the server's HTTP 400, "expected AND before" |

| One read, 15 runs, milliseconds | min | median | max |
|---|---|---|---|
| `cub unit get` (a `cub` process, start to exit) | 79.5 | 86.4 | 97.7 |
| SDK reader, space by slug (credential resolved each time, 2 GETs) | 5.7 | 5.9 | 6.9 |
| SDK reader, space by ID (credential resolved each time, 1 GET) | 4.0 | 4.3 | 4.8 |

Those times are from one GitHub-hosted runner with the server in a kind
cluster on the same machine, so the network costs almost nothing and most of
the `cub` figure is starting a process. Against a server across a network,
each request costs both routes more; that was not measured. The recordings
are kept in `test/fixtures/confighub-sdk-parity-v083-recorded/`.

## On the current server version (2026-10-10, CI run 38034560256)

Before the route became the default, the lane's disposable server was moved
from v0.8.3 to the current release and the SDK was bumped to match: server
v0.8.12, `cub` v0.8.12, cub-scout built with SDK core v0.8.12. The same test,
read from the run's artifact:

| Checked on ConfigHub v0.8.12 | Result |
|---|---|
| `unit get`; `unit list` by slug and by ID; `space list` | identical to `cub`, byte for byte |
| `unit list --where`, by slug, by a `LIKE` pattern, and matching nothing | identical; one Unit, the other Unit, `[]` |
| `unit list --contains` | identical; this server answers the search, and it returns the one Unit that matches |
| A `--where` the server rejects, and one with an OR | both routes fail with the server's HTTP 400 |
| MCP `confighub_unit_get` and `confighub_units`, by each route | identical |
| The import round-trip tests of the same lane | pass |

| One read, 15 runs, milliseconds | min | median | max |
|---|---|---|---|
| `cub unit get` (a `cub` process, start to exit) | 51.4 | 55.8 | 62.7 |
| SDK reader, space by slug | 4.0 | 4.2 | 4.8 |
| SDK reader, space by ID | 2.7 | 2.9 | 3.5 |

The reader's unit tests also pass unchanged on SDK core v0.8.12 against the
v0.8.3 recordings: the same requests as the SDK's own helpers, and the same
bytes.

## Not measured, and not claimed

- **Two server versions, one kind of Unit.** v0.8.3 and v0.8.12, both
  disposable servers in CI, with plain Units: no target, no upstream, no live
  revision, and spaces with no Component. The lists had two entries each; a
  long list was not tried. A hosted server and a credential with fewer rights
  were not tried.
- **An older server with the newer SDK.** v0.8.3 was run with SDK core
  v0.8.10, not v0.8.12.
- **Time for a whole command.** The timings are for one read. A connected
  command also runs `cub auth status` on either route, and how many requests
  `cub` itself makes was not measured.
- **No agent cost or time claim.** A faster read is not a measured saving for
  an agent; no eval was run on it.

## Known differences and limits

- **Names the filter cannot carry.** The reader refuses a space or Unit name
  that the SDK cannot put into a filter (`invalid_scope`) before any request.
  What `cub` does with the same name was not compared.
- **Where the reader is stricter than `cub`.** For a 200 that is not JSON
  `cub` prints `[]`, for a list entry with no Unit it prints the entry, and
  for a list the server marks as cut short it prints the part it got. The
  reader fails each of these. With `CONFIGHUB_DEBUG=1`, `cub` writes debug
  lines to its standard output and the cub route's JSON no longer parses; the
  SDK route ignores that variable.
- **What the lists ask depends on the SDK version.** `cub space list` asks for
  each space's summary and its Component, and `cub unit list` for six related
  entities; the reader sends what the `cub` source at SDK core v0.8.10 sends.
  An older or newer `cub` may ask for something else, and then the two routes
  can differ for a space that has a Component or a Unit that has the entity.
- **A list is not bounded.** Without `--limit`, `cub` asks for every entity in
  one request, and so does the reader. A very large space is a very large
  response on either route.
- **One lookup per read.** The space is looked up by slug on every read. A
  caller that lists a space and then gets each Unit, as the fleet view does,
  makes two requests per Unit on the SDK route.
- **Fields the SDK does not know.** The JSON is the SDK's typed envelope
  encoded again, as `cub` does it. A field a newer server adds is absent on
  both routes until the SDK is bumped; with different SDK versions in `cub` and
  cub-scout the two routes can differ in such a field.
- **Receipts drop the reason.** A receipt built from source-truth discards the
  ConfigHub read's error on either route and reports only a missing surface,
  with advice that names `cub` login. That is existing behaviour; with the SDK
  route the advice is the wrong one.
- **The unit tests compare on a fixture.** They give the cub route the
  recorded `cub unit get` output and the SDK route the same object over HTTP
  from a test server. The run against ConfigHub is the Connected lane test
  above, which CI runs only when that lane is asked for.

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

## What the default changes for a user

- These three reads no longer start `cub`. A wrapper around `cub` that
  records or fakes its calls does not see them; `examples/confighub-space-scope/record-cub-calls.sh`
  sets `CUB_SCOUT_CONFIGHUB_READER=cub` for that reason.
- What cub-scout shows of a Unit follows the SDK version cub-scout was built
  with, not the installed `cub`. A field a newer server adds is missing until
  cub-scout bumps the SDK.
- A read that `cub` would have printed in part (a list cut short, a 200 that
  is not JSON, an entry with no Unit) is an error.
- `CUB_SERVER` and `CUB_TOKEN` exported in a shell are still not used; the
  credential is the one `cub` uses.

## Next

More commands behind the same adapter, one at a time, each held to the `cub`
route's output on a recording from a real server; one space lookup per command
rather than per read; the connected check, so that the SDK route can run without `cub`
installed; a real-server run with a Unit that has a target. Scout's observation interface stays read-only
throughout: the SDK's action calls are not exposed.
