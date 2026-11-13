# Proposal: one envelope for every answer

Status: design, 2026-11-13. Nothing here is built. It is package B of the
[agent plan](../roadmap-3.0-execution.md#agent-plan--2026-11-13), with
addition G3 (the witness rule), and the remaining scope of
[#599](https://github.com/confighub/cub-scout/issues/599) (cluster identity
and reported cost) and [#596](https://github.com/confighub/cub-scout/issues/596)
(conformance). It needs decisions before any code.

## The problem, from recorded output

Five answers recorded on a real cluster in the `live-status` lane (run
43491874238, Flux), and how each says the same six things:

| | `gitops status` | `trace` | `explain --bounded` | `compare source-truth` |
|---|---|---|---|---|
| Which cluster | `context` | `context` | `resourceRead.context` | `context` |
| When observed | only inside the ConfigHub evidence: `deliveryEvidence.observedAt`, and per status | not stated | `resourceRead.observedAt` | not stated |
| The state | `controllerCoverage[].status`, `clusterCheck.state` | none at top level; `reason` only when there is no chain | `health`, `healthMeasurement.status`, `currentChange.verdict` | `status`, `source_truth` |
| What is not known | `omissions`, `notes`, `clusterCheck.limitation` | `warnings` | `omissions`, `notes` | `proof_gaps`, `collection_errors` |
| What to read next | none | none | `nextSteps` | `safe_next_action` |
| What it cost | none | none | `resourceRead.reads` | none |

An agent that has learned to read one of these has learned nothing about the
next. "What is not known" has six names. Two answers carry no time, so an
agent cannot tell a fresh observation from one it is replaying. Two carry a
word that reads as a verdict (`verdict`, `status: PASS`). Field names are
camelCase in four commands and snake_case in the fifth.

Every fact in that table is already collected. The inconsistency is in how it
is said.

## The design

Every JSON answer, from the CLI and from MCP, gains one top-level block with
the same shape. Working name: `answer`.

```json
"answer": {
  "state": "landed",
  "reason": "revision-matches",
  "summary": "Deployment argobot/argobot runs unit revision 3 of space argobot-scout-live.",
  "observedAt": "2026-11-13T19:20:45Z",
  "cluster": { "context": "kind-scout-live", "id": "", "idSource": "not-read" },
  "basis": [
    { "claim": "controller applied sha256:0828...", "restsOn": "controller-report",
      "ref": { "apiVersion": "kustomize.toolkit.fluxcd.io/v1", "kind": "Kustomization", "namespace": "flux-system", "name": "argobot-scout-live" } }
  ],
  "unknowns": [
    { "what": "digest equality with the Release", "kind": "not-compared", "reason": "not implemented for this strategy" }
  ],
  "next": [
    { "command": "cub-scout gitops status --with-confighub --confighub-space argobot-scout-live --format json", "why": "what the deploying tool recorded" }
  ],
  "cost": { "requestsMade": 4, "responseBodyBytes": 12679, "durationMillis": 180, "reused": false }
}
```

| Field | Rule |
|---|---|
| `state` | One word from a closed list per command, documented. Never a verdict: `agreed`, not `PASS`. `not-observable` exists in every list. |
| `reason` | A stable key for the state. Decide on this, not on words. |
| `summary` | One sentence a person can read. May change between releases. |
| `observedAt` | Always present. The time of the read, not of the command's start. |
| `cluster` | `context` always. `id` and `idSource` from the existing cluster identity reader when the caller asks for it; otherwise `idSource: "not-read"`. Never inferred from a name. |
| `basis[]` | The witness rule: each claim says whose word it rests on. `restsOn` is one of `object-metadata`, `controller-report`, `deployer-report`, `confighub-record`, `cub-scout-comparison`. `ref` is an object that exists and can be fetched. |
| `unknowns[]` | One vocabulary for what the six fields above say today. `kind` is one of `not-observable`, `denied`, `stale`, `not-requested`, `not-compared`. An answer with nothing unknown has `[]`, and that is a claim the conformance test checks against fixtures with known gaps. |
| `next[]` | Exact commands, read-only, each with why. Follows from `reason`. |
| `cost` | The existing `KubernetesReadCost` shape, for the whole command. |

Text that came from the cluster (messages, annotation values) appears only
inside `basis[].quoted`, never in `summary` or `next`, so an agent can tell
cub-scout's words from the cluster's.

## Migration

- **v2.16:** `answer` is added to `trace`, `explain`, `gitops status`,
  `compare source-truth` and `doctor`, and to their MCP tools. Nothing is
  removed. Deprecation notices name the fields it supersedes.
- **3.0:** the superseded fields go: `warnings`, `omissions`, `notes`,
  `proof_gaps`, `collection_errors`, `safe_next_action`, `nextSteps`, and the
  verdict words. `compare source-truth` moves to camelCase. This is the
  breaking change, in the release where breaking changes belong.

## What it costs

- **Size.** In v2.16 the block is added beside the old fields, so every answer
  grows, by an estimated 400 to 900 bytes. That works against package C until
  3.0 removes the duplicates. The catalog size test's approach applies: a
  declared ceiling per command's `answer` block.
- **Reads.** `cost` needs a metered transport around each command's reads.
  `map` and `watch` have one; the other commands do not. `cluster.id` is one
  more GET, which is why it stays opt-in.
- **A contract for consumers.** `compare source-truth` is consumed by Pilot.
  Its `status` values are in Pilot's fixtures. The 3.0 removal needs that
  side's agreement and a date.

## Conformance

One test runs every command against recorded input and checks: `answer` is
present with every field; `state` is in the command's list; `reason` is a
documented key; every `basis[].ref` names an object in the recording; every
`next[].command` is a cub-scout or `kubectl get`-class read with no mutating
verb; a fixture with a known gap has it in `unknowns`. The live lane checks
the same on a real cluster.

## For an agent, by the six questions

1. *Question:* `state` and `reason` answer it in two words, the same two
   fields in every answer.
2. *Who says so:* `basis[].restsOn`.
3. *Unknowns:* `unknowns[]`, one vocabulary.
4. *Time:* `observedAt` always; a `converging` state where it applies (G1).
5. *Cost:* `cost`, and a size ceiling on the block.
6. *Safety:* quoted cluster text is fenced; `next` is read-only by test.

## Not proposed

- Changing what any command observes.
- A second output format. `answer` is part of the existing JSON.
- Putting the block in ASCII or Markdown output; those render the same facts
  for people.

## Decisions for the maintainer

1. The name: `answer`, `evidence`, or something else.
2. `state` words per command. A first list: `trace`: `traced`,
   `no-owner-marker`, `chain-not-ready`, `not-observable`. `compare
   source-truth`: `agreed`, `not-agreed`, `incomplete`, `not-observable`.
   `gitops status`: `settled`, `converging`, `failing`, `not-observable`.
   `explain`: `explained`, `partly-explained`, `not-observable`.
3. May 3.0 remove `status: PASS|WATCH|ASK|BLOCK` from `compare source-truth`
   in favour of `state`? That is the question asked on 2026-11-13 and not
   yet answered, and Pilot's side must agree.
4. `cluster.id`: opt-in as proposed, or always read (one extra GET a
   command).
5. Start with which command. `compare source-truth` is the smallest and has
   the consumer with the most at stake; `trace` is the most read.
