# Recorded MCP economy diagnostic — 2026-09-30

This was one paired development diagnostic using the economy-purpose prompt in
the prepared, recorded-only `cub-scout` MCP package. It is a result for this
single case, pinned Haiku model, and runner configuration; it is not evidence
about the full cub-scout plugin, other models, benchmark performance, or general
cost/savings behavior.

| Arm | Answer grader | MCP calls | Turns | Runner duration | Inclusive list-price cost |
|---|---:|---:|---:|---:|---:|
| MCP available | 0/1 fail | 0 | 3 | 19 s | $0.0798042 |
| No plugin | 0/1 fail | N/A | 3 | 20 s | $0.0790946 |
| Pair total | 0/2 verified answers | 0 MCP calls | 6 | 39 s | **$0.1588988** |

Both traces advertised the same 12 ordinary tools. The treatment added only
`mcp__plugin_recorded-mcp-probe_cub-scout__explain`, but neither arm called it
or a Skill. Both read the complete 67,696-byte recorded Deployment list and its
recording metadata. Thus this pair did not show reduced evidence retrieval or
MCP-assisted work for this prompt.

Both answers identified `kubectl-set` for the requested exact image field, but
returned `resourceOwner: UNKNOWN`; the frozen grader expects `Flux`, supported
by the object's recorded Kustomize ownership labels. The MCP-available answer
also included explanatory prose before its JSON, so it failed the exact answer
format. The no-plugin answer used fenced JSON but still failed on the owner
value. No post-hoc regrade or prompt repair was performed. With zero answers
passing the correctness grader, cost per verified answer is undefined. The
small observed cost difference is not a savings result.

The result reports `runsPerCase: 3`, reflecting the default because the case
has no `runs` override. The invocation used `--runs 1`, producing one run per arm; this is a recorded
override, not an accidental partial suite. Both runs completed with three turns
under `maxTurns: 8`. The result is `partial: false`, while the CLI exited 1
because neither answer passed. It was not timed out or interrupted. The runs
used Claude Code 2.1.274, `claude-haiku-4-5-20251001`, standard service tier and
speed, fast mode disabled, and no paid judge. Reported costs are inclusive
producer list-price estimates; account credits were not measured. The raw trace
durations were 18.404 s and 19.457 s; the result rounds them to 19 s and 20 s.

The pair was launched under a $1 prelaunch ceiling within the existing $20
smoke tranche. Its $0.1588988 brings the September 30 smoke total to
$1.6356457 and total new paid-eval list-price spend to $4.1048522. Live-only
completion remains $2.4692065; baseline spend remains $0. The prior recorded
MCP plumbing result, including its five-turn observation against configured
`maxTurns: 4`, is unchanged; that earlier discrepancy remains unresolved.

## Provenance and limits

- Prepared economy prompt SHA-256:
  `2f2d2f10bd85e94ea94a199cf47627df5810210194f02f1470d92e5e2fd5b01d`.
- Answer grader SHA-256:
  `5dda566cdc398ebbd65eeffcd1801945842568e0f7ee934c2b4f7803f6b94db6`.
- Prepared fixture SHA-256:
  `305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8`.
- MCP binary SHA-256:
  `e94fe84d3e16b6c111ffa68b532523395c6c6f66649be3a890d3033e3afd94c5`.
- Source prepared packet commit: `bc37bf9264891e02bbab731491918b05be9e282f`.
- Aggregate result SHA-256:
  `b6dceac0293a8c98c4851d8ca01b15cd00e7d232245ee334b9b6781dfa444d35`.
- With trace SHA-256:
  `b9ae27d9a339b688486c0c4a8891c6777d0934f72348c9e7ae9d332fad8179ba`.
- Without trace SHA-256:
  `33b3c3e7b59a41072ac14334d9de122a99fa07fe4e5cf564fc2789fb87765b81`.

The raw aggregate result, scope, launch/completion records, prepared manifest,
trace audit, and full traces are preserved in the ignored local archive
`evals/results/recorded-mcp-economy-20260930/`; its local `archive-index.json`
hashes each file. The full traces are not included in this change. No sealed
run homes were accessed. This diagnostic adds no benchmark admission or
product-quality, reduced-work, or savings claim.
