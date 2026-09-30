# Recorded attribution contract smoke — September 30

The revised exact-field answer contract produced the correct evidence in both
arms. **It did not demonstrate a cub scout cost advantage.** Neither arm called
MCP or loaded a skill; the scout-enabled arm cost more. This is one diagnostic
pair, not a release benchmark or a controlled test of descriptor wording alone.

## Protocol and result

Claude Code 2.1.274, primary model `claude-opus-5[1m]`, normal speed verified in
both terminal results (`fast_mode_state: off`, standard service/speed). One run
per arm, sequential, default with-then-without order; no counterbalanced order
or repeat campaign. Source before execution: `5d96b2a`; tool catalog captured
from `04f92bc`. Both arms received the same full recorded raw exports, including
managedFields. MCP responses were explicitly identified as recordings.

The new prompt asks for the manager owning the checkout image field, the
manager-class attribution, human-identity limitation and source scope. It does
not provide the winning manager. A matching resource-level mutation hint is
not sufficient: the raw field path must support the answer. The narrow enum
contract does not establish safety of arbitrary explanatory prose.

| Arm | Inclusive list-price estimate | Seconds | Turns | MCP calls | Original grader | Amended offline grader |
|---|---:|---:|---:|---:|---|---|
| With scout | $0.243698 | 13 | 4 | 0 | Fail | Pass |
| Without scout | $0.134305 | 9 | 4 | 0 | Fail | Pass |
| Total | $0.378003 | | | | | |

Both final answers supplied the exact correct line inside one matching pair of
inline backticks. The original anchored regex rejected that wrapper. Amendment
`9643758` permits only the same complete line, either bare or inside one
matching single-backtick wrapper. Tests reject unmatched/double wrappers,
additional prose, wrong manager/path/attribution, identified humans and claims
of live or Git confirmation. The amendment was made **after observing this
pair**; original failures remain in the unmodified result. No paid retry was
run. The two amended passes are an offline protocol diagnostic, not two
pre-registered successes. Original cost per verified answer is undefined;
under the amended narrow contract it is the respective arm cost above.

No mock or judge charge occurred. Costs are producer estimates at list price,
not measured account-credit consumption or subscription charges. Both arms
used Glob, Grep and Read. The scout arm recorded 19,434 cache-creation input
and 67,286 cache-read input tokens, versus 10,159 and 39,550 without scout;
output tokens were 627 versus 516. These observations do not isolate context
or cache effects from order or variance. The changed prompt and answer format
also prevent attributing differences from the earlier smoke to routing alone.

Tool-access limit: this was a file-only smoke. Ordinary tools were Read, Glob
and Grep, plus harness task, skill and search tools; Bash, kubectl, Helm, jq and
Python were unavailable. This does **not** satisfy roadmap Experiment A.
Preflight safe, equivalent ordinary command access for both arms before a
further paid attribution campaign.

## Reproduction and retained evidence

```bash
CLAUDE_CODE_DISABLE_FAST_MODE=1 claude plugin eval . \
  --case changed-by-checkout --ablation with-without --runs 1 --concurrency 1 \
  --scaffold --mocks record --model 'claude-opus-5[1m]' \
  --no-publish --trust-plugin --keep-temp --max-cost-usd 3 \
  --json evals/results/fair-checkout-routing-20260930.json
```

Do not rerun that command merely to repair formatting. The original result,
both traces, original/amended prompt and grader, and offline audit are retained
locally under ignored `evals/results/evidence-routing-20260930/`, indexed by
SHA-256. They are not committed transcripts.

- Original result SHA-256: `37b15a1bdfa4691bb4a068699e682b11560f7247184b72febdc7bbc3d8366f0e`.
- Amended audit SHA-256: `ac4f473aba6fc31c596ea3e8df48c3ddf78db860e2f014f7a635ff25f93b8a7c`.
- Amended audit uses only explicitly allowed saved-trace directories; source
  result and costs are preserved, and current grader hashes are recorded.

At the time of this report, smoke-stage spend was $1.245443 of its $20
envelope; including live-only completion, new paid eval spend was $3.7146495.
A later command-access diagnostic added $0.0655016, bringing current smoke spend
to $1.3109446 and new paid eval spend to $3.7801511. The $200 baseline campaign
remains unspent. Account credits and development-agent costs remain unmeasured.
See #603/#626/#649 and the execution cost ledger. Next work should reduce
required evidence/context reads and complete negative controls, rather than
spend on a larger campaign before the product can show a benefit.
