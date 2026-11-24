# Migration notes for 3.0

This guide records compatibility changes for the 3.0 major release. For the
full 3.0 migration guide with before/after commands verified against the built
binary, see [docs/releases/v3.0.0-migration.md](releases/v3.0.0-migration.md).

## What 3.0 removes

Each item below printed a deprecation notice on stderr in `v2.18.0` and
`v2.19.0` ([#950](https://github.com/confighub/cub-scout/pull/950)), meeting
the one-minor deprecation window before 3.0
([#595](https://github.com/confighub/cub-scout/issues/595)). On `main` (3.0),
every removed flag and command fails and names its replacement.

| Removed in 3.0 | Use instead |
|---|---|
| `trace --json` | `trace --format json` |
| `tree --json` | `tree --format json` |
| `graph export --json` | `graph export --format json` |
| `gitops status --json` | `gitops status --format json` |
| `connect --go` | `connect --map` |
| `app create --set-context` | `--space <name>` on the commands that need a space; the flag is already ignored |
| `hierarchy` | `map --hub` or `map hub` |
| `fleet outliers` | none; see below |
| The verdict words in JSON and MCP output | the `answer` block's `state` and `reason`; see below |

A 3.0 command given a removed flag fails and names the replacement; it is not
ignored silently.

## `fleet outliers`

`fleet outliers` is deprecated for planned removal in 3.0. It remains available
in 2.x, but its current slug-based, single-space comparison does not establish
reliable cross-cluster lineage or fleet outliers. The safe zero-comparison
response remains “not compared”; it does not mean clusters are consistent or
missing units.

There is no equivalent replacement in cub-scout 2.x. `map fleet` can provide
connected inventory, but it does not perform lineage-based outlier analysis.
Do not compare unit or target names across spaces as identity. Removal in 3.0
is the adopted default. Retain or rebuild the command only if ConfigHub provides
stable cross-cluster identity and fixture-backed tests demonstrate correct
comparisons. The existence of an identity field alone is not sufficient proof.

## The verdict words

`compare source-truth`'s `status` (`PASS`, `WATCH`, `ASK`, `BLOCK`) and
`source_truth` (`AGREED` and the rest) leave the JSON and MCP output in 3.0 and
stay in the ASCII and Markdown output (maintainer's decision, 2026-11-16). An
agent reads `PASS` as a decision already made; a person at a terminal is helped
by it. Read the `answer` block's `state` and `reason` instead
([commands](reference/commands.md), the verdict words).
