# Recorded ownership inventory

From the repository root, build the local binary and inspect the existing
recorded Deployment list. These commands make no cluster or ConfigHub calls.

```bash
go build ./cmd/cub-scout
./cub-scout map list --recording evals/fixtures/scale/cluster/deployments.yaml --api-version apps/v1 --kind Deployment --namespace-prefix team- --format json
./cub-scout map list --recording evals/fixtures/scale/cluster/deployments.yaml --api-version apps/v1 --kind Deployment --namespace-prefix team- --tui
./cub-scout mcp serve --recording evals/fixtures/scale/cluster/deployments.yaml
```

For MCP, call `map` with `{"api_version":"apps/v1","kind":"Deployment","namespace_prefix":"team-"}`.
The pinned file has 302 objects: 300 match this scope and two are excluded.
Counts are 120 Flux, 90 ArgoCD, 45 Helm, 33 ConfigHub and 12 Native.
Native means no built-in owner marker observed, not proof that nobody manages
the object. The input SHA-256 binds these results to the file; capture time and
completeness are unknown. Do not call the result current cluster state.

`TestBuildRecordedMapReportPinnedScaleDeployments` verifies counts, the exact
twelve no-marker identities and the recording manifest hash.
`TestRecordedMapSurfacesShareModelWithoutLiveReads` checks CLI/MCP/TUI parity
and zero requests despite an available Kubernetes endpoint. Missing metadata,
malformed input or duplicate identities fail conservatively; an empty selection
returns an empty scoped result. No live fallback is available.

Both benchmark arms must retain identical full raw evidence. This example and
smaller output do not establish dollar, token or credit savings; the benchmark
remains subject to its admission and paired quality gates.
