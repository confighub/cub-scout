# Exact-space ChangeOrder read contract (authored, opt-in)

This source-only case stays outside `benchmark-v1.json` and the frozen 24-case
manifest. Its fixtures were authored from the inspected SDK v0.6.8 parser
contract at `4c8d2fc3885fed0d7af6835f2aac0a24387b6221`; they are not captured server, CLI or TUI evidence.
The pin is not a runtime server version claim. The two spaces deliberately share
an order slug but contain different declarations. Missing and denied reads keep
evaluated governance and runtime outcomes unknown.

Run offline controls with `python3 evals/changeorder-read-contract/test_case.py`.
These controls match authored answer vectors with Python regex using the declared
flags and last_message target; they do not execute an official evaluator, model,
provider, server, or prove universal regex dialect equivalence. The copy scaffold
stages only `cluster/inputs.json`; it does not call cub or cub-scout.

Product example and success criteria: [ChangeOrder read](../../examples/changeorder-read-contract/).
Genuine connected captures and CLI/TUI acceptance remain pending.
