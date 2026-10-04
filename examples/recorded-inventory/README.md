# Recorded ownership inventory

From the repository root, build the local binary and inspect the existing
recorded Deployment list. These commands make no cluster or ConfigHub calls.

```bash
go build ./cmd/cub-scout
./cub-scout map list --recording evals/fixtures/scale/cluster/deployments.yaml --api-version apps/v1 --kind Deployment --namespace-prefix team- --format json
./cub-scout map list --recording evals/fixtures/scale/cluster/deployments.yaml --api-version apps/v1 --kind Deployment --namespace-prefix team- --owner Native --format json
./cub-scout map list --recording evals/fixtures/scale/cluster/deployments.yaml --api-version apps/v1 --kind Deployment --namespace-prefix team- --summary --format json
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

The `--owner Native` view selects the 12 no-built-in-marker objects while
retaining each object's detector evidence. `--summary` uses the separate
`map-list-recorded-summary.v1` schema with `view: "summary"`, the same scope
counts and owner totals, and no resource rows; ASCII, Markdown, JSON, MCP, and
the dedicated TUI say how to request per-object evidence. Its serialized JSON
is smaller than the full list for this recording, as is the owner-filtered list.
Those byte counts are an output-size check only, not measured token or dollar
savings.

Both benchmark arms must retain identical full raw evidence. This example and
smaller output do not establish dollar, token or credit savings; the benchmark
remains subject to its admission and paired quality gates.

## Recorded pages (2.14 candidate)

The authored `pagination.yaml` fixture contains five Deployments in two namespaces
and one excluded ConfigMap. It is a deterministic control, not a cluster capture.

```bash
./cub-scout map list --recording examples/recorded-inventory/pagination.yaml --api-version apps/v1 --kind Deployment --namespace-prefix team- --page-size 2 --format json
# Pass pagination.nextCursor from that response unchanged to --cursor.
./cub-scout map list --recording examples/recorded-inventory/pagination.yaml --api-version apps/v1 --kind Deployment --namespace-prefix team- --page-size 2 --tui
```

MCP `map` accepts `page_size: 2` and subsequent `cursor` arguments. Paged MCP
responses include the same report under `structuredContent.data`. The separate
`map-list-recorded-page.v1` schema adds `pagination` with page size, offset,
returned count and optional next cursor. Full-scope selected/excluded counts and
owner totals remain unchanged: they describe all selected objects, not one page.
Pages contain at most the requested number of rows (1..500), not a byte or token
cap. A large individual row may still produce a large response. No agent savings
or six-surface budget conformance is established by this feature.

Cursors bind exact input bytes, combined filters and page size. Changing any of
those, malformed/noncanonical cursors, and out-of-range offsets are refused.
Cursors are public continuation data, not authorization tokens. `--summary`
cannot combine with pagination, and pagination requires recorded input. Default
full and summary response schemas are unchanged. Empty selection returns an
explicit empty page, with no continuation. Capture time/completeness stay unknown.
The TUI's `n`/`p` keys navigate the loaded snapshot without file or API reads;
`p` can navigate before a supplied starting cursor, down to the first page.

Five authored controls in `recorded_map_page_test.go` cover page partitioning,
full-scope facts, cursor refusals, loaded TUI navigation, CLI/MCP parity and
pre-read argument refusal. The existing scale fixture remains a separate
regression control.
