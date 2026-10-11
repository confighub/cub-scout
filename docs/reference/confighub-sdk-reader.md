# ConfigHub SDK reader

cub-scout reaches ConfigHub in two ways. It can run the `cub` CLI and parse
what it prints, which is how every read worked until v2.13.4 and how every
write still does. The reads listed here go through the ConfigHub SDK's typed API
client instead ([#758](https://github.com/confighub/cub-scout/issues/758)),
with no `cub` process for the read. This page records what that route covers,
what it guarantees and what was measured.

**Status: the default for the reads it covers, since v2.14.0.** To read through
`cub` instead, as every release before it did, set
`CUB_SCOUT_CONFIGHUB_READER=cub`. That is the way back if the two ever
disagree, and a failed read says so when it could help.

**When `cub` is needed.** On the SDK route the connected commands check the
session without `cub` (see [the session check](#the-session-check)), so a
command whose reads are all in the left-hand column below runs on a machine
with no `cub` binary, given a cub configuration to take the credential from.
`cub` is still needed to log in, for every write (`import` and the commands
that create or change things in ConfigHub), and for the reads in the
right-hand column. A read that needs `cub` on a machine without it fails at
that read and says the `cub` CLI is not on PATH.

`CUB_SCOUT_OFFLINE=true` and the telemetry switch turn these reads off on
either route. The SDK route checks them itself, before it looks for a
credential, so a machine that is offline stays offline whether or not `cub`
is installed.

## What it covers

These commands, wherever cub-scout runs them: `cub unit get` for one Unit
named by slug in one named space; `cub unit list`, `cub worker list`,
`cub target list`, `cub changeset list`, `cub link list`, `cub resource list`,
`cub release list` and `cub unit-event list` for one named space, with or
without a `--where` or `--contains` filter, and for every space when the
caller names every space (`--space '*'`, except releases); the unit events of
one Unit; `cub organization list`; `cub context get`, which is read from this
machine's cub configuration and asks no server;
`cub space list`; and the reads of one thing: `cub unit data`,
`cub revision get`, `cub revision data`, `cub changeorder get` by slug and
`cub view get` by ID.

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
| `worker list`, `target list`, `changeset list`, `link list` and `attestation list`, each `-o json --space <space>` with or without `--quiet`, `--where <expression>` and `--contains <text>` | Any of them across every space, with a positional, or with any of the options a unit list is left to `cub` for |
| `resource list`, `release list` and `unit-event list`, in the same shape; and `unit-event list <unit>` for the events of one Unit named by slug | Any of them across every space (`--space '*'`), which the MCP tools allow when asked; a resource list with a selection, a stored filter, a view or raw data; the events of a Unit named by ID |
| `unit data <unit> --space <space>`: the Unit's configuration, as bytes | `unit data` of many Units (`--where`), to a file, or of a Unit named by ID |
| `revision get <unit> <number> --space <space> -o json` and `revision data <unit> <number> --space <space>`, the number written in plain digits | A revision named relative to the head (`-1`), by a name such as `HeadRevisionNum`, or with a selection |
| `changeorder get <slug> -o json --space <space>` | A change order named by ID; `--container-images`, which adds a second request |
| `view get <id> --space <space> -o json`, the View named by its canonical ID, in a named space or in every space (`*`) | A View named by slug |
| Any of those lists except releases with `--space '*'`: the whole organization's list, asked for by name | The releases of every space, which `cub` reads from another endpoint and orders by time; any list with no `--space` at all |
| `organization list -o json`; `context get -o json` | Either with a filter, a selection, a space or a name; `context get` as text; `context list`; `context use` |
| `space list -o json`, with or without `--quiet` | A space list with any filter or any of the same, a component, a `--space` or a positional; and a list with no `-o json`, which prints `cub`'s table |
| | Any other output format, any other flag, `--`, and `-o=json` or `--space=x` spelt with `=` |

Today that is every `unit get` cub-scout builds, given a slug and a named
space, every plain `unit list` and `space list`, and every worker, target,
change set, link, release and unit-event list, and the resource list unless
it is given a selection or a stored filter. A test reads the source
of each call site: a `unit get` in a shape the route does not take fails it,
and so does a list that is neither taken nor named in the test with the reason
it stays with `cub`. The test models the arguments' shape, not the values a
user supplies. The release, resource and unit-event lists are built in ways
that scan cannot read, so their test calls each builder instead and pins how
many places spell each command: a new call site fails the count until someone
adds it, which is weaker than the scan, since nothing checks the new site
itself.

| Where the read is used | Notes |
|---|---|
| `compare source-truth`, the MCP `compare_source_truth` tool, receipts built from it | Head revision and IDs |
| `compare` DRY/WET snapshots | Unit metadata; the configuration itself is `cub unit data`, still `cub` |
| MCP `confighub_unit_get` | With `space`, or `unit` as `<space>/<slug>`. By ID it stays with `cub` |
| `map` (connected hierarchy and unit detail) | |
| Import wizard checks | Reads only; its writes run `cub` |
| The lists: `map` (connected hierarchy, fleet, and joining cluster resources to their Units), `fleet outliers`, import's check for existing units and its link to the space, MCP `confighub_units` with or without `where` and `contains`, and the `views` lookups when they name one space | |
| The worker list: `status`, the connected hierarchy in `map`, and the local cluster view. The target list: import, the import wizard and the hierarchy. The change set list: `history`, `audit` and MCP `confighub_changesets`. The link list: `compare bindings` and `map` | A worker's `Secret` is not in what the SDK route returns; see below |
| The release and unit-event lists: the ConfigHub part of `gitops status` and of `doctor --with-confighub`, MCP `confighub_releases` and `confighub_unit_events`. The resource list: MCP `confighub_resources` | Releases come from the space's own endpoint and are sorted as `cub` sorts them, highest number first. Unit events are sorted newest first, as `cub` does |
| `unit data`: the intended configuration in `compare` (DRY/WET snapshots and `compare resource`), the import wizard's checks and `import argocd`. `revision get`, `revision data` and the attestation list: attestation evidence in `trace` and receipts, and the MCP `confighub_attestations` tool. `changeorder get`: `history changeorder` and MCP `confighub_changeorder_get`. `view get`: `views resolve`, `views project` and `compare three-way --view` | Attestation evidence takes at most 1 MiB per response by either route: by the SDK route each response is cut off when it passes the limit, and the read fails |
| Lists that stay with `cub`: `views` across every space, GitOps delivery evidence (a selection), `app list` and import's summary (they print `cub`'s own output) | |

A failed SDK read is the answer, reported with its reason. It does not fall
back to `cub`, and once the SDK route has taken a read, no failure of it is
worded as a `cub` failure. Where the failure is one `cub` might not have had
(the answer could not be decoded, was refused by this reader's own rules, or
ran past this reader's 30-second limit, which `cub` does not have), the error
ends by naming `CUB_SCOUT_CONFIGHUB_READER=cub`. It does not name it for a
list the server cut short, or for an answer that held something from another
space or Unit: `cub` prints both as they came, so that route would trade the
error for a wrong answer with nothing to mark it. A denial, a
missing Unit, a missing credential, a server error or a server that cannot be
reached is the same by either route, since both use one server and one
credential, and nothing is suggested for those.

A value that names no route is an error for every read in the left-hand
column, in every command that makes one, and is not consulted for anything in
the right-hand column, which has only the `cub` route.

## The session check

Every connected command first asks whether ConfigHub can be read, once per
command. By the `cub` route that is `cub auth status`. By the SDK route it is
the same check made without `cub`:

| `cub auth status` checks | The SDK route |
|---|---|
| A stored token for the active context | The same, through the credential `cub` would use; none is "no ConfigHub session this machine can use" |
| The token has not expired, by its own `exp` claim | The same, before any request |
| The server accepts it: one GET of the caller's own membership | The same GET; a 200 that names no user, organization or external identity is not accepted, which is stricter than `cub`, which takes any 200 |
| This `cub` is not older than the server by a minor version | The SDK version in cub-scout's `go.mod`, which `cub-scout version` prints (a constant, held to `go.mod` by a test), is not older than the server by a minor version. A server ahead is refused; the error says to upgrade cub-scout or to read through an up-to-date `cub` with `CUB_SCOUT_CONFIGHUB_READER=cub`. An SDK ahead of the server is allowed, with a warning on stderr, as `cub` does |

A refusal for no credential, a rejected or expired one, or a 403 is "not
authenticated" by either route, and a client too old is a version problem by
either route, in words that do not mention a `cub` check that was not run.
The routes differ for the rest: a server error, a timeout or an answer that
is not the server's is "could not be checked" on the SDK route, which says
nothing about the session, where `cub auth status` reports every failure as
not authenticated. `status` then leaves the session unset; it does not call
it expired and does not ask a second time. `CUB_SCOUT_OFFLINE=true` and the
telemetry switch are honoured first, and then nothing is sent. With reads off,
`status` says only what this machine's files say, whether a credential is
stored (and, by the SDK route, that it has not expired), and does not look for
a worker.

`map fleet`, `map clusterdata --connected`, `fleet outliers`, `impact` and the
existing-unit lookup of `import --json` now ask this gate like every other
connected command (#863). `impact` no longer reports "no dependents" when the
space's links could not be read; it says they could not be read. `import
--json` still shows a workload as not connected when the lookup could not be
made, with nothing to mark it; that is tracked on #863.

A read that does not go through the gate is still not sent with a token that
says it has expired: it fails with the expiry and `cub auth login`, where the
server would have answered a bare 401.

The reader takes the credential from cub's configuration and never writes it.
With no configuration at all, `cub context get` (seen with `cub` v0.8.7)
creates a default context and writes it; the SDK route reports that there is
none.

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
- **The same rules for every list of a space.** Workers, targets, change
  sets, links and attestations are read as Units are: one named space, every
  entry checked to be in it, and nothing broken read as empty.
- **A worker's credentials are not passed on.** The server returns each
  worker's `Secret`, the token the worker authenticates with, to anyone who
  may list workers, and `cub worker list -o json` prints it. A worker also has
  a `ClaimToken` while it holds a claim. Nothing in cub-scout uses either, so
  the reader drops both fields before returning the list. This is the one
  place its JSON is not `cub`'s: the worker list is `cub`'s with those fields
  gone. By the `cub` route they are in the output cub-scout parses, as they
  always were; no command prints them. The `Secret` was seen on a real server;
  a `ClaimToken` was not, since the recorded worker was not running.
- **One thing is found by a list that must hold exactly one match.** A
  revision, a change order and a View are found the way `cub` finds them: a
  list filtered to the name, number or ID, from which exactly one entry must
  be the one asked for. None is `not_found`; two are `ambiguous`; a revision
  of another Unit under the Unit's own endpoint, or an entry that is not the
  kind of thing asked for, fails the read.
- **Configuration data is held to the hash ConfigHub reports for it.** A
  Unit and a revision each report the SHA-256 of their data, on both server
  versions recorded here. The data is downloaded from the entity's own
  endpoint and, when a hash is reported, returned only if it has that hash. A
  page from something in between, a truncated body, and a Unit that changed
  between the lookup and the download all fail, and the error does not quote
  what was downloaded. `cub` makes no such check, so the error does not
  suggest the `cub` route. An entity that reports no hash is held to the size
  it reports; one that reports neither cannot be checked and is returned as
  `cub` returns it. A revision with no data at all is an error, as it is for
  `cub`.
- **A View named by ID is found by the ID alone.** `cub` checks that the
  space it was given exists and then finds the View wherever it is; so does
  the reader, so `--space` does not have to be the View's own space.
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
  `canceled`, `malformed`, `refused`, `incomplete`, `too_large`, `version_skew`, `request_failed`. A Unit that does not
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

## The other lists of a space (2026-10-10, CI run 38071527126)

The same test creates a worker, a Target, a ChangeSet, a Link between its two
Units and an attestation through `cub`, on ConfigHub v0.8.12, and lists each
by both routes. Read from the run's artifact:

| Checked on ConfigHub v0.8.12 | Result |
|---|---|
| The server let the test create | all five |
| `target list`, `changeset list`, `link list`, `attestation list`: reader against `cub`, space by slug and by ID | identical, byte for byte; one entry each (1426, 2951, 5824 and 770 bytes) |
| `worker list`, the same way | identical once the `Secret` line is taken out of what `cub` printed; 1747 bytes before |
| Each list with a filter that matches nothing (not attestations, which have no slug) | `[]` by both routes |
| Each list in a space that does not exist | `not_found` from the reader |

That run compared the worker list whole, secret included, and kept it in the
run's artifact; the worker belonged to a server deleted when the run ended.
The reader has dropped the field since, the test removes it before keeping
anything, and the recordings in
`test/fixtures/confighub-space-lists-v0812-recorded/` are stored without it.

Each list had one entry, the Target had no worker, the worker was not running,
the ChangeSet was in state `New` and the Link was within one space.

## Releases, resources and unit events (2026-10-10, CI run 38073699271)

`TestSDKReaderMatchesCubOnReleasesResourcesAndEvents` makes a Target, sets it
as the space's release Target, assigns two Units to it, publishes a release,
changes one Unit and publishes again, all through `cub`, on ConfigHub
v0.8.12. The rows about a space or Unit that does not exist and about a
rejected filter are assertions of the test, which passed in that run; the
artifact keeps no file for them. Every other row is read from the artifact.

| Checked on ConfigHub v0.8.12 | Result |
|---|---|
| Every step, including both publishes | worked |
| `release list`, reader against `cub`, space by slug and by ID | identical, 3505 bytes, two releases, release 2 before release 1 |
| `release list --where "ReleaseNum = 1"`, and with the `CreatedAt >` filter `gitops status` sends | identical; one release, and two |
| `resource list`, and with `--contains` | identical; two resources (2542 bytes), and one |
| `unit-event list`, with the same `CreatedAt >` filter, and for one Unit | identical, and **empty**: `[]` by both routes |
| Each list in a space that does not exist; the events of a Unit that does not exist | `not_found` from the reader, a failure from `cub` |
| A release filter the server rejects | both routes fail |
| MCP `confighub_releases`, `confighub_resources`, `confighub_unit_events` through the built binary, by each route | the whole answer identical |

**Unit events were not exercised with an event in them.** Nothing on that
server applies a Unit, so it has none. That the two routes agree on an empty
list shows the request is accepted and the space is read; it does not show
that an event is printed as `cub` prints it, or that several come out in
`cub`'s order. Those rest on the SDK's types, which both use, and on a unit
test of the ordering, not on a recording. One known difference: for a
space's events `cub` also reads the Units they belong to, for its table, and
fails if that second read fails. The reader does not make that read, so it
can answer where `cub` would not. The recordings are in
`test/fixtures/confighub-release-lists-v0812-recorded/`.

## One Unit's data, its revisions, a View, a change order (2026-10-10, CI run 38078870310)

`TestSDKReaderMatchesCubOnEntitiesAndData` creates a Unit, uploads a second
version of it, and creates a filter, a View and a change order, through `cub`,
on ConfigHub v0.8.12. Every row is read from the run's artifact except the
last, which is an assertion of the test, which passed. That the server lets
the test create a View and a change order, that the Unit's data is the file
uploaded last and that an earlier revision holds the first upload were notes
in that run and are assertions since.

| Checked on ConfigHub v0.8.12 | Result |
|---|---|
| The server let the test create | the View and the change order |
| `unit data`, reader against `cub`, space by slug and by ID | identical, 99 bytes: the file uploaded last |
| The Unit's `DataHash` | the SHA-256 of those bytes, which is what the reader's check rests on |
| `revision get` for revisions 1, 2 and 3 | identical (2610, 3306 and 3219 bytes) |
| `revision data` for revisions 2 and 3 | identical; each has the hash its revision reports, and revision 2 is the first upload |
| `revision data` for revision 1, which the Unit was created with | no data: `cub` fails and the reader returns `not_found` |
| `changeorder get` by slug | identical, 3194 bytes |
| `view get` by ID, with no space, with its space by slug and by ID | identical, 2046 bytes |
| A Unit, a revision, a change order and a View that do not exist; a space that does not exist | `not_found` from the reader, a failure from `cub` |

The older recordings of these reads, made with `cub` v0.8.3, are not equal
byte for byte to what this SDK prints for the same objects: the newer types
have a `ReleaseTagID` on a change order, and a UUID field is printed even
when it is zero. Tests on those compare every value `cub` printed and allow
only that field to be added. The recordings from this run are compared
exactly, and are kept in `test/fixtures/confighub-entities-v0812-recorded/`.

Not shown: a revision made by a function, a merge or a change order; a change
order with a workflow or promotions; a View with expression or data-path
columns; data that is large or is not Kubernetes YAML; the attestation
evidence reads end to end on a real server (the revision reads they make are
the ones above, and the attestation list is the one recorded earlier).

## Not measured, and not claimed

- **Two server versions, one kind of Unit.** v0.8.3 and v0.8.12, both
  disposable servers in CI, with plain Units: no target, no upstream, no live
  revision, and spaces with no Component. The lists had two entries each; a
  long list was not tried. A hosted server and a credential with fewer rights
  were not tried.
- **An older server with the newer SDK.** v0.8.3 was run with SDK core
  v0.8.10, not v0.8.12.
- **A session that `cub` can renew.** The reader uses the stored token and
  does not renew it. In the `cub` source read for this work, `cub` renews a
  key-authenticated session at login, not on use; whether the released `cub`
  does more was not checked. If it renews on use, such a session would work
  through `cub` and fail here with `unauthorized`.
- **A very large list.** The reader gives each request 30 seconds and `cub`
  sets no limit, so a space large enough to take longer fails here with
  `timeout` and names the way back.
- **Time for a whole command.** The timings are for one read. How many
  requests a whole command makes by each route was not measured.
- **Which commands run with no `cub` at all.** Shown through the built
  binary: the gate and `views resolve` in the unit lane, and the MCP
  `confighub_units` tool on the real server in the Connected lane. No
  command-by-command list was made; a command runs without `cub` when every
  read it makes is one this page lists as taken. The OCI registry lookup
  still asks `cub`, on either route.
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

- These reads no longer start `cub`. A wrapper around `cub` that
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

The reads still left to `cub` (a selection, a stored filter, a View's
evaluated columns, an entity named by ID, the releases of every space,
`unit tree`, `k8s get`), each held to the `cub` route's output
on a recording from a real server; one space lookup per command rather than
per read; the OCI registry lookup, which still asks `cub` for the context; a
real-server run with a Unit that has a target and with unit events. Scout's observation interface stays read-only
throughout: the SDK's action calls are not exposed.
