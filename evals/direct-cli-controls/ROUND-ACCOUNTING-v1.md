# Direct CLI round accounting v1

`round_accounting_v1.py` is a separate, offline analysis validator for the
retained single-Read stop captured by the earlier direct CLI probe. It does not
replace or amend `probe.py`'s legacy validator, the source run status, or any
historical report. The original `validation_failed` status and exit code 1 are
carried into the analysis record.

Acceptance is deliberately narrow. The validator requires the exact retained
synthetic provider request and mock response, exactly one pinned-model request
advertising only `Read`, one matching fixture-path Read tool-use, one successful
CLI tool result containing the fixture marker, an explicit
`error_max_turns`/`is_error: true` terminal with exit code 1, the preflight plus
one accepted connection, and verified server/handler cleanup. If the provider
request history contains a copied tool result, it must match the CLI result by
ID and exact content. It rejects missing, duplicate, conflicting, extra, timed
out, and incomplete evidence.

The result reports `requested_limit`, `observed_model_requests`,
`observed_tool_use_rounds`, and `reported_terminal_num_turns` independently.
The terminal counter is preserved as an opaque positive integer. This validator
does not equate it to the requested limit, model requests, tool rounds, tokens,
or billed calls. The observed stop is accepted from reconciled request/tool/
terminal/transport evidence, not from that scalar.

This schema covers only a single synthetic Read response and its one successful
fixture result. It does not define general SDK turn semantics, parallel tool
rounds, full-process accounting, provider billing, credits, benchmark admission,
or the complete Experiment A treatment. The 24-case protocol and existing
reports remain unchanged. The analysis can be applied later to the retained
local bytes without invoking a CLI or provider.

Pure fixture checks:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s evals/direct-cli-controls -p 'test_round_accounting_v1.py' -v
```
