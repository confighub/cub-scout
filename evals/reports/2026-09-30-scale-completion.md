# Saved scale answers and live-only completion — 2026-09-30

This is an exploratory workflow comparison, not the controlled 24-case
benchmark or a release-wide savings claim. The historical paired scale run
and interrupted live-only run were regraded against the stricter current regex
patterns without rerunning those agents. The original JSON, all 24 traces and
source/trace/grader hashes were preserved locally.

| Historical run | Attempts | Exact required answers | Full reported cost | Cost / required answer |
|---|---:|---:|---:|---:|
| Paired scale, with scout | 9 | 9 | $11.104722 | $1.233858 |
| Paired scale, export-only | 9 | 8 | $11.030221 | $1.378778 |
| Partial live-only, saved | 6 | 5 | $5.146915 | $1.029383 |

The partial row includes the interrupted failed attempt and its spend. Its
missing case is not silently treated as a completed campaign. The historical
paired difference is about 10.5% on this small sample, not the adopted 20%
release gate or a confidence-bounded estimate.

The saved live-only run had three valid ownership-count answers and two valid
unmanaged-list answers. September 30 completion therefore requires one
unmanaged-list execution plus three failing-workload executions, rather than
repeating all nine. Reusing saved evidence avoids buying five duplicate runs.

## Completion result

All four new executions matched their exact required answer. New reported
spend was **$2.469207**: unmanaged list $0.986926 (228 s), failing workloads
$0.609465 / $0.484947 / $0.387870 (149 / 99 / 62 s). Every terminal trace records
`fast_mode_state: off` and `claude-opus-5[1m]` list-price accounting.

Across saved and new live-only attempts: **9 required answers / 10 attempts**,
including the interrupted failure, **$7.616122 total**, **$0.846236 per required
answer**. That is approximately 38.6% below the historical export-only cost per
required answer, under different evidence conditions and observation dates.
It is an exploratory comparison, not a causal savings estimate or the 3.0 gate.

No new run hit its launch ceiling. The four-run completion used $2.47 of the
adopted $15 live-only completion envelope. No further paid completion is needed.

## Completion protocol

Claude Code 2.1.274; requested model `claude-opus-5[1m]`; normal mode enforced by
`CLAUDE_CODE_DISABLE_FAST_MODE=1` and checked in each terminal trace. Runs are
serial, use the explicit `kind-scout-evals-scale` context, skip scaffolding,
use the real read-only MCP server, retain temporary evidence and do not publish
the harness report. Runtime binary is from `8c0409d`; case/grader source is the
same revision. The reporting/audit implementation is in #646.

The 300 team-namespace Deployments matched the committed fixture identities,
labels and specs at preflight. The cluster also contains two infrastructure
Deployments outside the requested team scope. The old export and new live
reads are not simultaneous snapshots: timestamps, runtime status and the
observation date differ. Do not call this an equal-information experiment.

Spend is the harness/provider's reported list-price estimate, including agent,
judge and mock costs where present; it is not an invoice. Actual account credits
and incremental subscription billing are **not measured**. Dollar, token and
credit claims must not be substituted for one another.

## Grading limits and next work

These regex graders verify the requested final answer line. They do not certify
all surrounding prose. For example, the new unmanaged answer returned the exact
12 names but included an inconsistent aggregate count in its discussion. It
also sampled and cross-checked ownership well beyond the requested list. Both
are useful follow-ups for #603/#626: score unsupported surrounding claims and
measure redundant investigation, without rewarding an agent merely for calling
scout tools.

The full baseline still needs complete raw managedFields evidence in both arms,
all 24 executable cases, pinned evidence, paired randomized order and the
adopted quality/cost confidence checks. None is inferred from this comparison.
