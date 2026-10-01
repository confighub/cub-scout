# Direct CLI round accounting v1

`round_accounting_v1.py` is a separate, offline analysis validator for the
retained single-Read stop captured by the earlier direct CLI probe. It does not
replace or amend `probe.py`'s legacy validator, the source run status, or any
historical report. The original `validation_failed` status and CLI exit code 1 are
carried into the analysis record; the original helper exit remains 2.

Acceptance is deliberately narrow. The validator requires the exact retained
synthetic provider request and mock response, exactly one pinned-model request
advertising only `Read`, one matching fixture-path Read tool-use, one successful
CLI tool result containing the fixture marker, an explicit
`error_max_turns`/`is_error: true` terminal with exit code 1, and verified
server/handler cleanup. HTTP `request_count` is separate from model request
count: it includes the one accepted POST and permits zero or one exactly
evidenced startup HEAD decline. TCP connections reconcile to one preflight
connection plus those HTTP requests. The first and sole provider request must
not contain tool-use/result history, since that would imply an unobserved prior
round. Missing, duplicate, conflicting, extra, timed out, and incomplete
evidence is rejected.

The result reports `requested_limit`, `observed_model_requests`,
`observed_tool_use_rounds`, and `reported_terminal_num_turns` independently.
The terminal counter is preserved as an opaque positive integer. This validator
does not equate it to the requested limit, model requests, tool rounds, tokens,
or billed calls. The observed stop is accepted from reconciled request/tool/
terminal/transport evidence, not from that scalar.

The source run must retain the exact legacy error
`turn-limit terminal num_turns is missing or inconsistent with its cap`; a
different validation error, timeout, or runner failure cannot be replayed as
this accounting case.

This schema covers only a single synthetic Read response and its one successful
fixture result. It does not define general SDK turn semantics, parallel tool
rounds, full-process accounting, provider billing, credits, benchmark admission,
or the complete Experiment A treatment. The 24-case protocol and existing
reports remain unchanged. The [October 1 offline reanalysis](../reports/2026-10-01-direct-cli-round-accounting.json)
accepted one observed model request and one tool-use round, with two HTTP
requests including the startup HEAD, three connections including preflight,
and terminal-reported `num_turns: 2`. It used retained local bytes without
invoking a CLI or provider. Original validation failure remains unchanged.

Pure fixture checks:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s evals/direct-cli-controls -p 'test_round_accounting_v1.py' -v
```
