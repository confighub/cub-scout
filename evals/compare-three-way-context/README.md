# Three-way context interpretation scaffold

This opt-in case uses hand-authored synthetic evidence, outside the fixed
24-case suite. The case, prompt and strict grader are prepared. Agent/model
execution has **not run**. This is not actual CLI/MCP output, live-controller
acceptance, real ConfigHub governance evidence or a savings measurement.

Both arms receive the exact same `cluster/evidence.json` through `scaffold.sh`.
`TestCompareThreeWayContextEvalScaffoldIsSyntheticAndExact` validates exact
copying, provenance and expected interpretation inputs without model spending.
Product behavior is checked separately by the two-endpoint TLS contracts in
`cmd/cub-scout/compare_three_way_context_test.go`.

```bash
GOPROXY=off GOTOOLCHAIN=local go test ./test/unit \
  -run TestCompareThreeWayContextEvalScaffoldIsSyntheticAndExact -count=1
```
