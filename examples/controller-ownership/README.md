# Controller Ownership Fixtures

Static fixtures for controller ownership detection. These manifests document the
metadata cub-scout parses for controllers beyond the original Flux / Argo CD /
Helm set.

## Sveltos

`sveltos-modelplane.yaml` includes a Deployment with:

- `projectsveltos.io/owner-kind: ClusterProfile`
- `projectsveltos.io/owner-name: config-to-production`
- `projectsveltos.io/reference-*` annotations identifying the ConfigMap source
- Helm labels, proving Sveltos provenance wins over generic Helm metadata

Expected owner:

```text
Sveltos / clusterprofile / config-to-production
```

## Modelplane

The same fixture includes:

- a `ModelEndpoint` served from the `modelplane.ai` API group
- a Crossplane Helm `Release` labeled `modelplane.ai/release`

Expected owners:

```text
Modelplane / modelendpoint / qwen-demo-0
Modelplane / release / traefik
```

When equivalent objects are present in a cluster, inspect them with:

```bash
./cub-scout map list --owner Sveltos
./cub-scout map list --owner Modelplane
./cub-scout tree ownership --owner Sveltos
./cub-scout tree ownership --owner Modelplane
```

## Opt-in ownership detection diagnostics

Before adding implementation, success is defined as an opt-in diagnostics
surface that leaves ordinary compact output unchanged:

```sh
./cub-scout map list --ownership-evidence --format json
```

This returns `schema: "map-list-ownership-evidence.v1"`, a compact ownership
projection (identity, existing owner/ref, and platform substrate evidence),
and a collection status with omissions. The ordinary full map JSON remains the
drilldown surface. Each returned object has an `ownershipDetection` result derived from the existing
`DetectOwnership` result:

```json
{
  "status": "detected",
  "source": "annotation:projectsveltos.io/owner-kind/name"
}
```

A successfully returned object with no recognized marker has
`status: "no_known_marker"` and a reason limited to “no supported ownership
marker observed on this returned object.” That does not prove it is orphaned or
unmanaged. A resource whose list request failed has no invented entry; its
omission is reported separately with API version/resource/namespace scope and
a sanitized reason such as `forbidden`, `not_found`, or `request_failed`.
`not_found` describes that request only; it does not claim the API is globally
unsupported.

The same evidence is available through MCP `map` with
`ownership_evidence: true`, and in the TUI ownership-evidence detail view.
These use already loaded data and add no Kubernetes reads. ASCII and Markdown
diagnostics expose the same source/reason and list omissions. Default JSON and
the compact count, names-only, and summary outputs stay byte-identical when
evidence is omitted; combining per-entry evidence with compact modes is
rejected. The deterministic test contract uses this file plus a marker-free
Deployment object and fake list responses for forbidden/not-found errors. It
checks exact source, bounded no-marker reason, separate omissions, cross-
surface parity, compact/default compatibility, and unchanged API request
counts. Fixture bytes and request counts are local engineering measurements,
not agent savings claims.
