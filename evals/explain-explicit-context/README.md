# Explicit-context enriched Explain: source-only agent case

This opt-in case sits outside the frozen 24-case benchmark. Its inputs are
**authored contract fixtures**, not actual Scout/MCP output, real Kubernetes
RBAC, live reconciliation or a model result. No model execution has run.
Both arms must receive the same `cluster/inputs.json` via the source-only
scaffold; it copies no executable, oracle, sibling case or credentials.

The fixture fixes alpha selection while beta is ambient. The same exact
workload exists in both contexts, with different Application source URLs.
A denied read stays unknown. Omitted context preserves the legacy default;
that compatibility rule does not promise capture across the unselected legacy
path. Context labels are not stable cluster IDs. The source contract is
`cmd/cub-scout/explain_context_test.go`; its in-process transport replay verifies
actual selected CLI/MCP observations without listeners or child processes.

The existing [RUL-03 context/denial recording](../rul03-context/README.md) is
related controller evidence. This case neither rewrites nor replaces that
recording or the benchmark manifest. Model/evaluator integration and genuine
CLI/TUI cluster acceptance remain pending under the user's no-live constraint.

Offline fixture/metadata and strict authored-vector controls:

```bash
PYTHONDONTWRITEBYTECODE=1 python3 evals/explain-explicit-context/test_case.py
```

These Python regex checks target only authored vectors with the declared `s`
flag and `last_message`; they are not official evaluator or universal regex
conformance proof. Correct answers derive from the fixture's selected request,
ownership marker, Application source and explicit evidence limits.
