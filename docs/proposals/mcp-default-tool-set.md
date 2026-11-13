# Proposal: a smaller default MCP tool set

Status: parts 1 and 2 are built (2026-11-13, after v2.15.1), with the default
unchanged; how to use them is in the
[command reference](../reference/commands.md#mcp-serve). Part 3, changing the
default, waits on an eval run, which is not approved today. This belongs to
package C of the
[agent plan](../roadmap-3.0-execution.md#agent-plan--2026-11-13) and was asked
for by the maintainer, who confirmed `map` in the core.

Measured after building: the core catalog is 9,292 bytes standalone and
12,239 with ConfigHub, close to the estimates below; the index
`cub_scout_more` returns is 832 and 2,695 bytes. Three things differ from the
design as first written, each for a reason found while building or in review:

- `cub_scout_more` cannot be named in an exact list. It reaches every tool,
  so naming it would make the list no allow-list at all.
- The plugin manifest does not name a profile. A newer manifest passing
  `--tools` to an older installed binary would stop it starting.
- A profile set only in the environment is ignored with `--recording`.

## The problem

An agent pays for the whole tool catalog at the start of every session.

| Session | Tools | Catalog |
|---|---|---|
| Standalone | 8 | 15,120 bytes |
| With ConfigHub | 21 | 31,126 bytes |

Measured as `tools/list` returns it, after the first shortening
(`cmd/cub-scout/mcp_catalog_size_test.go`). Four bytes is roughly a token.

The [answer-size measurement](../reference/answer-sizes.md) shows what that
costs: one cub-scout answer saves an agent roughly 8 to 15 KB against raw
`kubectl`, so in a standalone session the first question does not repay the
catalog, and with ConfigHub the first two do not. Wording is nearly as short
as it can get: the descriptions are dense, and almost every sentence is pinned
by a test because it once fixed an agent's wrong choice. The remaining lever
is how many tools are listed.

## What the evidence says about which tools matter

Counted across the files under `evals/reports` and `evals/results`: how often
each standalone tool's MCP name (`cub-scout__<tool>`) appears, and in how many
files. A mention is a call or an allow-list entry; the two were not separated.

| Tool | Mentions | Files | Size in bytes |
|---|---|---|---|
| `map` | 544 | 42 | 2,249 |
| `explain` | 456 | 35 | 2,756 |
| `doctor` | 86 | 13 | 1,624 |
| `gitops_settings` | 54 | 15 | 2,343 |
| `gitops_status` | 12 | 6 | 1,834 |
| `trace` | 0 | 0 | 1,503 |
| `scan` | 0 | 0 | 692 |
| `release_check` | 0 | 0 | 2,110 |

This is biased: it reflects the questions the suite asks (ownership and
attribution, mostly) and a tool with its own eval case scores higher. It is
still the only record there is. Two things stand out. `map` and `explain`
appear far more than anything else. And `trace`, which the change-loop
argument would put in any core set, does not appear at all.

No connected tool has usage data: the connected cases are few and mocked.

## The design

Three parts. The first two can ship without changing what anyone sees.

### 1. A tool profile, chosen by the host

```
cub-scout mcp serve --tools all        # today's behaviour
cub-scout mcp serve --tools core       # the small set, plus cub_scout_more
cub-scout mcp serve --tools doctor,explain,trace   # exactly these
```

Also `CUB_SCOUT_MCP_TOOLS`, for hosts that cannot pass flags. An unknown name
is an error at start-up, not a silent omission. `initialize` reports the
profile in `serverInfo`, so a transcript shows which catalog the agent had.

### 2. The core set, and one tool to reach the rest

`core` lists these, by the evidence above:

| Standalone core | Why |
|---|---|
| `doctor` | The stated first tool; every other description routes through it. |
| `explain` | One resource: owner, health, field attribution. Most called with `map`. |
| `map` | Inventory and ownership counts. Most called. |
| `gitops_status` | What the controllers report: the witness read. |
| `cub_scout_more` | New. Reaches every tool not listed. |

With a ConfigHub session, `core` adds `compare_source_truth` (the
revision-anchored comparison) and `confighub_units` (the lookup the other
ConfigHub tools are reached from).

`cub_scout_more` has three uses, in one tool:

- no arguments: an index of the unlisted tools, one line each;
- `tool`: that tool's full description and input schema, exactly as
  `--tools all` would have listed it;
- `tool` and `arguments`: run it. The server validates the arguments against
  the same schema, and the answer is the tool's own.

Estimated catalog (current sizes, plus about 700 bytes for the new tool):

| Session | `all` | `core` | Change |
|---|---|---|---|
| Standalone | 15,120 | about 9,200 | -39% |
| With ConfigHub | 31,126 | about 12,100 | -61% |

Every tool stays reachable. The cost moves from every session to the sessions
that need an unlisted tool: one extra call for the index or description, about
1 to 3 KB, once.

### 3. The default, changed only on evidence

The default stays `all` until an eval run compares the two profiles on the
same cases. If `core` answers as correctly and costs less, the default becomes
`core` in 3.0, the release where breaking changes belong, with `--tools all`
documented as the way back. The plugin manifest names its profile explicitly
from the day the flag exists, so the flip cannot surprise a plugin user.

## Why a dispatcher and not a changing tool list

The protocol has another way: the server could add tools mid-session and send
`notifications/tools/list_changed`. Not proposed, for three reasons:

- **It depends on the client.** Not every MCP host acts on that notification.
  A dispatcher is an ordinary tool and works everywhere.
- **It defeats prompt caching.** Hosts cache the prompt prefix, and tool
  definitions sit at the front of it. A tool list that changes mid-session
  makes the host pay for the whole prefix again, which can cost more than the
  catalog saved. A fixed list with a dispatcher keeps the prefix stable.
- **It is harder to reason about afterwards.** With a fixed list, a transcript
  shows what the agent could call at every step.

The dispatcher has costs too, and they are the things to measure:

- an agent that needs an unlisted tool spends an extra call finding it;
- the host cannot check a dispatched call's arguments against a schema before
  sending it; the server does, and must return an error that gives the right
  shape (the rule that an error gives the command that works);
- routing guidance for unlisted tools ("load after doctor", "do not use for
  X") is read only when the agent asks for the description. For the
  connected readers that guidance is most of their text.

## What is not proposed

- **Merging the ConfigHub readers into one parameterised tool.** It would
  shrink `all`, but a union schema is probably harder for an agent to fill
  correctly than thirteen small ones (not measured), and `core` already
  removes them from the default.
- **Removing any tool.** `trace`, `scan` and `release_check` do not appear in
  the recorded runs; that is a fact about those cases, not a reason to delete
  a command.
- **A different core per question.** One fixed core; the host can name its
  own list.

## For an agent, by the six questions

1. *Question:* unchanged; the same tools answer the same questions.
2. *Who says so:* unchanged.
3. *Unknowns:* a call to a tool that exists but is not listed says so and
   names `cub_scout_more`; it is never "unknown tool".
4. *Time:* unchanged.
5. *Cost:* the point of the change; declared per profile and held by the
   catalog size test.
6. *Safety:* `cub_scout_more` runs only cub-scout's own read-only tools, by
   exact name. It is not a general command runner.

## Tests before any code is claimed

- Unit: profile parsing; an unknown name fails at start; `core` lists exactly
  the core; `cub_scout_more` returns the same descriptor bytes `all` lists and
  the same answer a direct call returns; a dispatched call with bad arguments
  is refused with the schema.
- Catalog size test: a ceiling per profile.
- A connected tool that is unlisted and unavailable still gives the ConfigHub
  session error, not "unknown tool".
- Eval, when a run is approved: the same cases under `all` and `core`, three
  runs an arm, scored on correctness, turns and tokens. At least two cases
  must need an unlisted tool, or the comparison only measures the saving and
  not the cost. The recorded `_tools.json` is re-recorded per profile.

## Graceful degradation

A host that ignores the flag gets `all`, as today. A missing ConfigHub session
behaves as today in either profile.

## Decisions for the maintainer

1. Is the core right: `doctor`, `explain`, `map`, `gitops_status`, and with
   ConfigHub `compare_source_truth` and `confighub_units`? The usage count
   argues for `map` over `trace`; the change loop argues for `trace`.
2. Ship parts 1 and 2 in v2.16 with the default unchanged?
3. Approve the eval run that part 3 waits on.
