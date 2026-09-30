# Fair attribution smoke — 2026-09-30

A single paired `changed-by-checkout` run on refreshed managedFields fixtures.
Both arms returned the correct required `CHANGED_BY` answer. Scout did **not**
save money or time on this case.

| Arm | Required answers | Agent cost | Mock cost | Total / answer | Seconds | Turns |
|---|---:|---:|---:|---:|---:|---:|
| With scout | 1/1 | $0.6185535 | $0.024624 | $0.6431775 | 111 | 15 |
| Without | 1/1 | $0.2242625 | $0 | $0.2242625 | 36 | 8 |

Total reported list-price spend: **$0.867440**, within the $3 smoke launch
ceiling and the adopted $20 smoke envelope. The harness's run and top-level costUsd already include mock and judge calls;
adding the breakdown fields again would double-count them. This was confirmed
against the terminal trace's primary cost and the installed 2.1.274 harness. Actual account credits and
incremental subscription billing are not measured.

Source: `d530f79`, Claude Code 2.1.274, primary model `claude-opus-5[1m]`,
`fast_mode_state: off` in both terminal traces. The model used by agent mocks
was not independently pinned by this command; their cost is included, and
this remains a method smoke rather than the controlled release benchmark.
Both arms received the same generated full raw export. The command used
`--case changed-by-checkout --ablation with-without --runs 1 --concurrency 1
--scaffold --mocks record --model 'claude-opus-5[1m]' --no-publish
--trust-plugin --keep-temp --max-cost-usd 3`, with fast mode disabled.

The scout arm called explain and trace but loaded no skill; the no-ingest
indicator passed. This is not evidence that forcing skill loading helps. Both
agents read raw evidence; the scout arm did extra corroboration. The final-line
grader does not certify the surrounding prose: the scout answer called recorded
MCP output independent live confirmation, and the baseline claimed an unseen
Git desired image. These are quality gaps, not verified conclusions.

Follow-up: declare recorded-tool provenance in both prompts (now applied),
then test a narrowly scoped reduction in redundant reads under #626. Keep
negative evidence and missing-source caveats. Do not buy the full campaign
until its fixture and grading gates are met, and do not extrapolate one pair
to a general advantage or disadvantage.
