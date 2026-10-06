# Enriched Explain context binding — success contract

Defined before implementation for the remaining explicit-context scope of #599.

`./cub-scout explain deployment/api -n team-a --kube-context alpha --format json`
must capture the selected kubeconfig endpoint and configuration once. Static
file credentials are snapshotted; configured exec-auth retains its refresh and
file-access behavior. The offline tests exercise static bearer tokens. Ownership,
controller lineage, events, field attribution, rollout and connected comparison
reads must all use that private session, including partial-result paths. An empty
or unknown explicit context fails before reads without default or in-cluster
fallback. Namespace is the explicit requested namespace (legacy omitted namespace
remains `default`). ConfigHub space/auth remains a separate boundary.

Default enriched Explain behavior remains compatible. `--api-version`,
`--expected-revision` and `--refresh` remain bounded-only outside recorded input.
CLI ASCII/JSON/Markdown, MCP and the enriched single-object TUI render the same
ExplainSummary; context labels are not stable cluster identities. Missing/denied
evidence stays unknown with omissions, never a false Native/orphan/healthy claim.

Offline success tests use fake recorded API responses with duplicate same-name
objects in alpha/beta and different namespaces. They verify retargeting after
capture cannot move reads, selected static tokens/namespace are retained, denial
is explicit, invalid selection never falls back, and MCP/TUI share the model.
Connected tests use injected ConfigHub reads only; no server gate, attestation,
or prerequisite outcome is invented. Fleet fanout, stable IDs and request cost
remain outside this packet.

Run the focused context contract tests with cached dependencies. Genuine cluster
acceptance and model/evaluator evidence remain external gates under the current
no-live constraint; this document and tests do not claim that acceptance.

The implementation packet's six `TestEnrichedExplain*` contracts pass using
in-process recorded transports, without listeners or subprocesses. Existing
Explain summary/text/Markdown checks pass in the same focused run:

```bash
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test ./cmd/cub-scout \
  -run '^Test(EnrichedExplain|BuildExplainSummary|ExplainText|ExplainMarkdown)' -count=1
```

The [opt-in authored agent case](../../evals/explain-explicit-context/README.md)
is outside the frozen 24-case benchmark; its three offline metadata/vector
controls pass. No agent run or live acceptance is claimed.
