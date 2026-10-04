# Exact-space ChangeOrder read contract

Success was defined before implementation in
[#597 comment 5978640058](https://github.com/confighub/cub-scout/issues/597#issuecomment-5978640058).
This packet implements only a settled read adapter; #597 remains open.

```bash
./cub-scout history changeorder rollout --space production --format json
./cub-scout history changeorder rollout --space production --format md
./cub-scout history changeorder rollout --space production --tui
```

The adapter runs one `cub changeorder get rollout -o json --space production`.
Space must be explicitly supplied and exact; CUB_SPACE is not a fallback.
Order and space slugs are case-sensitive. Missing, mismatched or ambiguous
identity, malformed/duplicate JSON and unsafe selectors fail without an
alternative lookup. Optional qualified order slugs must name the same space.

Source contract: ConfigHub SDK v0.6.8 commit
`4c8d2fc3885fed0d7af6835f2aac0a24387b6221`,
`core/openapi/goclient-new/models.gen.go` (ChangeOrder, ExtendedChangeOrder,
ChangeWorkflowSpec) and `cmd/cub/changeorder_get.go`. This pins the inspected
read shape, not the server executing an eventual read. No SDK dependency changes.

CLI ASCII/JSON/Markdown, the single-object TUI and connected-only MCP
`confighub_changeorder_get` share the projection. It exposes reported Stage/State
and stored workflow/prerequisite declarations in their original order. Evaluation
is always unknown: this GET contract does not expose prerequisite evaluation,
approval, gate acceptance, publish eligibility or automatic advancement.
`Completed` is retained as a reported value; it proves no runtime health or live
convergence. Missing workflow fields do not mean ungoverned or approved.

Deterministic fake-runner tests cover exact identity/space, two spaces with the
same order slug, malformed/duplicate JSON, wrong types, missing/partial workflows,
Completed, cancellation, read failures, declaration ordering and render parity.
No server/listener/provider/model runs are part of offline validation. Fleet
fanout and mutations are outside this exact read. Standalone calls fail through
the connected gate; denied/partial evidence cannot become approval.

The authored opt-in [agent case](../../evals/changeorder-read-contract/README.md)
is outside the frozen 24-case benchmark. Genuine server captures, evaluated
authority contracts and final live CLI/TUI acceptance remain pending.
