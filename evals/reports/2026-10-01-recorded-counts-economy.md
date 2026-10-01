# Recorded owner-counts diagnostic — 2026-10-01

One serial Haiku 4.5 pair under `counts-economy.v1` timed out in both arms.
There were **zero verified answers**. This is a failed package-availability
diagnostic, not a benchmark result or evidence of savings. No retry followed.

| Arm | Answer | Reported turns | Seconds | Producer-reported cost |
|---|---|---:|---:|---:|
| Scout package available | Timeout / fail | 17 | 180 | $0.08080895 |
| No plugin | Timeout / fail | 10 | 180 | $0.040277 |
| Pair | 0/2 verified | 27 | 360 | $0.12108595 |

The owned runner exited 1 after 361.367 seconds without reaching its 390-second
outer deadline or receiving an interruption. Both case-level deadlines fired.
The harness reports `partial: false`; this does not mean the answers completed.
Its default `runsPerCase: 3` is overridden by the recorded `--runs 1` command.
Both traces lack a terminal `type=result` record. Cost per verified answer is
undefined. Reported costs are retained, but complete descendant spend is **not
reconciled**; do not present these amounts as verified complete billing.

## What was held constant

Preparation source was `88cdbe32cc453a4750a11f3f01d238c47d070133`.
Both arms received the same seven recorded exports, frozen question body,
strict answer-only suffix and regex grader. The copied frontmatter reduced
30 turns / 900 seconds to 12 turns / 180 seconds. The treatment added 35 Scout
skills and immutable recorded `map`/`explain`; this is not an MCP-only contrast.
The file-tools baseline is narrower than Experiment A's kubectl/Helm baseline.
The strict grader, source inputs and historical results were not altered.
Inherited fixed-error MCP mocks were removed from this explicit opt-in copy.

The pinned binary's fresh CLI/MCP preflight passed in 1.787 seconds before the
paid launch. Claude Code was 2.1.274, model `claude-haiku-4-5-20251001`, one run
per arm, concurrency one, no paid judge, no publication and no automatic retry.
`CLAUDE_CODE_DISABLE_FAST_MODE=1` was set; visible message usage reports standard
tier, but no terminal speed/usage summary exists. The $1 prelaunch estimate was
not a strict in-flight billing stop. No maximum-speed service was requested.

## Trace observations and unresolved limits

The init records list the same 12 ordinary tools and 16 background skills;
treatment adds exactly the two intended MCP tools and 35 Scout skills. This
establishes top-level inventory parity only. Both arms spawned three nested
tasks, reaching depth three. Visible tool blocks use `Agent` while the inventory
advertises `Task`; alias equivalence and descendant grants are not established.
The with-package arm's 17 reported turns exceed configured `maxTurns: 12`;
whether the metric includes descendants or the limit failed is unresolved.

Both arms hit Read's 256 KB limit on the 1 MB Deployment export, then continued
searching and delegating. Visible tool-use blocks show with-package Glob×3,
Read×6, Grep×11 and Agent×2; baseline Glob×1, Read×2, Grep×5 and Agent×2.
These counts do not cover all descendants. The parent/visible blocks contain no
MCP calls, but treatment depth-three progress reports **six map-name events**
with increasing tool-use counters. The name uses underscores
(`mcp__plugin_cub_scout_cub_scout__map`), unlike the hyphenated init/grant name.
Arguments and results are absent, so neither zero MCP use nor successful MCP
use is established. Baseline descendant progress also reports ToolSearch and
Skill activity absent from its visible tool-use blocks. No complete tool-use,
model, usage or successful answer accounting can be inferred from these traces.

The result's `promptMarkdown` is the staged question body, with no frontmatter
expected answer. This checks that recorded prompt field, not every hidden model
request. No sealed run homes were opened. Retain the failure and reconcile
turn limits, recursive delegation, actual tool names/grants and descendant costs
offline before another paid diagnostic. Full24 admission stays closed.

## Accounting and custody

This adds **$0.12108595 reported estimated cost** to the existing smoke ledger:
smoke/probe **$1.91745385**, live-only **$2.4692065**, baseline **$0**; recorded
total **$4.38666035**. This diagnostic has unresolved spend completeness, so the
total is the sum of reported estimates, not a certified all-in bill. Account
credits and development-agent dollars remain unmeasured. Do not infer spend
from account percentages or token counts, and do not consume the remaining
smoke allowance until the accounting/limit issue is resolved.

Raw result, launch/completion, prepared manifest, preflight and both explicit
`out/trace.jsonl` files are preserved in the ignored local archive
`evals/results/recorded-counts-economy-20261001/`, indexed by SHA-256. This report
contains no raw private trace text. The original external prepared packet is
retained separately and must not be rerun or overwritten.

| Artifact | SHA-256 |
|---|---|
| result.json | `ea68ea2c821758e8df42e9ef8c936a288bd50e7ef72fe818d2e3f80d94ca6c80` |
| launch.json | `3435bf0ccb4349d45b4dfdd040fd6eb689daac44c0eef06e7d2d2bedc1d7aa1d` |
| completion.json | `86d778a69b068ff12308f74ca937b422b192c3ac1850bc913ee640f6f8c9c899` |
| prepared.json | `b0e44f89609ecc556cd10295dea8748644fba4ada1f29a8c3d6b027b2a0dc3ad` |
| reviewed-summary.json | `f92dd73757081c7196761c11cb7b8f24d4e06b837f59ffd530610489f2ab2a47` |
| with trace | `6a64e02d0b3c2f9fb023806cc5131492f115986ba3a5b13c55f823128d349be4` |
| without trace | `3b47fb49f6360c4df3affe6023c64d75d17db33caf4f14c74b05e823fbd74e6b` |
| Product binary | `cdf37333b942ccd8112855863e14632684b175e7b1482a6bb61e96792203816b` |
| Source recording manifest | `e139701bf9d894ca9dd16ddb302fe0e4f28f3122922030cd9cdda57e8eb3fa22` |

Product binary source: `10c223dfdf792cb37ec61acfcbcc57983dabd29b`. [Launch scope](https://github.com/confighub/cub-scout/issues/603#issuecomment-5923478195) and [failure record](https://github.com/confighub/cub-scout/issues/603#issuecomment-5923561328) retain the review and admission sequence.
