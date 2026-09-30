# Recorded attribution-routing diagnostic — 2026-09-30

This was one paired development diagnostic over the same fixed recorded input,
using Claude Code 2.1.274, the pinned `claude-haiku-4-5-20251001` model, and
the same runner settings. Within this pair, the treatment added the recorded
`explain` MCP tool and one generic attribution-routing skill; the prompt,
answer grader, recording, ordinary tools, model, and limits were held constant.
Relative to the previous economy-purpose pair, the only intended change was
adding this skill. The two separate one-pair comparisons are descriptive only:
they do not isolate a causal skill effect across stochastic runs or differing
cache state. This single case is not evidence about the full cub-scout plugin,
other models, benchmark performance, or general savings or quality.

| Arm | Answer grader | Reads | Skill/MCP calls | Turns | Result duration | Inclusive estimated list cost |
|---|---:|---:|---:|---:|---:|---:|
| Routing skill available | 0/1 fail | 2 | 0 / 0 | 3 | 22 s | $0.0811486 |
| No routing skill | 0/1 fail | 2 | 0 / 0 | 3 | 19 s | $0.0795736 |
| Pair total | 0/2 verified answers | 4 | 0 / 0 | 6 | 41 s | **$0.1607222** |

The traces show both arms read the complete `deployments.yaml` recording and
`recording.json`; neither called MCP. The treatment inventory listed the
`recorded-field-attribution` skill, but neither trace called `Skill`. Both
inventories had the same 12 ordinary tools; treatment added the recorded
`explain` MCP tool. The `Skill` tool was available in both arms, so the table
reports zero Skill calls in each. These observations show the routing skill was
available in treatment but not invoked. They do not establish why it was not
invoked or support a causal conclusion about routing instructions.

Both answers gave `fieldManagers: ["kubectl-set"]` and
`resourceOwner: "UNKNOWN"`; the frozen answer grader expects `Flux` for the
resource-level classification. The with-skill response also added prose around
the JSON, so it failed the exact-format requirement; the without-skill answer
was fenced JSON but still failed the owner value. No regrade or retry was
performed. With zero verified answers, cost per verified answer is undefined.
The $0.001575 observed cost difference is not a savings result.

Both runs completed without timeout or interruption, reported three turns under
`maxTurns: 8`, used standard service tier and speed, and had no judge. The
result is `partial: false`; the command exited 1 because neither answer passed.
The invocation used `--runs 1`, overriding the plugin-eval default
`runsPerCase: 3`; this was one expected run per arm, not an incomplete suite.
The [earlier plumbing report](2026-09-30-recorded-mcp-probe.md) documents the
same CLI override behavior.
Result durations are rounded (22 s and 19 s); raw trace durations were 21.009 s
and 18.652 s. Token usage was, respectively, 18 input / 34,086 cache-write /
22,186 cache-read / 2,148 output (1,679 thinking), and 18 input / 33,971
cache-write / 22,086 cache-read / 1,881 output (1,610 thinking). Costs are
inclusive producer list-price estimates; account credits were not measured.

The pair ran once per arm under a $1 prelaunch ceiling within the existing $20
smoke tranche. It adds $0.1607222 to the recorded September 30 smoke total
($1.7963679) and total new paid-eval list-price spend ($4.2655744). Live-only
completion remains $2.4692065; baseline spend remains $0. No general plugin
comparison or 24-case suite was run.

## Provenance and limits

- Result SHA-256: `0d71b87d0f3e7dbdbedf3bfd861ccf35b01d3f2e89dd0f81533f2b908e1ec9bf`.
- Scope record SHA-256: `91a82a283acf4b5b61f6deb626b385f84a954e33c46be66f0df1a9a2fdb1a160`.
- Prepared manifest SHA-256: `5be2621ca1dd776089e1f2632db897162439fc7858db38baad574edf4b59491b`.
- Preparation source commit: `f2da69f3e3cf2492b410dda06f8affb5499d06a1`.
- Prepared prompt SHA-256: `2f2d2f10bd85e94ea94a199cf47627df5810210194f02f1470d92e5e2fd5b01d`.
- Routing skill SHA-256: `9f77fb53bec0cf60e42df7b86e2300a081e2dfacaa8f848adf3150265007215d`.
- Answer grader SHA-256: `5dda566cdc398ebbd65eeffcd1801945842568e0f7ee934c2b4f7803f6b94db6`.
- Recording SHA-256: `305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8`.
- Binary SHA-256: `e94fe84d3e16b6c111ffa68b532523395c6c6f66649be3a890d3033e3afd94c5`.
- Pinned binary source commit: `5165f14` (the binary predates the preparation source commit).
- With-skill trace SHA-256: `6a7312ad8c39dd3302a4de31a79795a79b2c0d3aa30990a42010fa2af08325ac`.
- Without-skill trace SHA-256: `8ab20237109c41c23d5e6daa28fbef0f77460846a6d9f31d304551129f9a7b52`.

The raw result, scope, launch/completion records, prepared manifest, trace audit,
and full traces are preserved in the ignored local archive
`evals/results/recorded-routing-diagnostic-20260930/`, indexed by its
`archive-index.json`. The only traces inspected and archived were the two paths
explicitly authorized for this review. Their contents are not checked into this
report. This diagnostic does not admit a benchmark case or establish reduced
work, quality uplift, savings, or benchmark readiness.
