# Graph Export Formats

Use `graph export` to generate shareable topology artifacts from the same
`graph.v1` data model.

## Commands

```bash
# Canonical JSON contract payload
./cub-scout graph export --format json > graph.json

# Graphviz DOT
./cub-scout graph export --format dot > graph.dot

# Static embeddable SVG
./cub-scout graph export --format svg --output graph.svg

# Self-contained interactive HTML
./cub-scout graph export --format html --output graph.html
```

## Notes

- TUI parity: in `./cub-scout map`, press `M` for Maps, then `e` (HTML) or `E` (SVG).
- `--max-nodes` applies to visual formats (`dot`, `svg`, `html`) to keep large
  clusters shareable.
- `--format json` remains the contract format for automation and schema checks.
- `--json` is still accepted as a legacy alias for `--format json`.

## Explicit context (2.14 candidate)

```bash
./cub-scout graph export --kube-context selected --namespace team-a --format json
./cub-scout snapshot --kube-context selected --namespace team-a
```

Both pin one selected config and refuse missing/blank explicit contexts before
output-file creation. They never change current-context. Graph collection honors
command cancellation; explicit selection cannot silently become an empty graph.
`--empty` and fixture-time graph mode refuse explicit contexts. Default behavior
and graph/GSF schemas remain unchanged. In explicit mode the `cluster` string is
the captured context label, **not verified cluster identity or a Target binding**.
GSF's existing skipped-LIST behavior does not prove complete inventory; whole
command cost and structured collection omissions remain tracked in #599.

Deterministic checks are `TestGraphAndSnapshot*` and
`TestGraphContextOfflineAndCancellationRefuse`. The owned-cluster reproducer is
`python3 examples/watch-collection-omissions/verify-live-identity.py`; graph and
snapshot controls are additional to its watch/bot identity proof. Genuine
acceptance of this extension is pending until a matching clean-source receipt.

With explicit context, the map TUI Maps export action reads the captured config
in-process and preserves its selected namespace. It never invokes a PATH/ambient
CLI fallback; denied/missing bindings refuse without writing a graph.
`TestScopedTUIGraphExportUsesCapturedBindingAndNamespace` verifies this against
selected/ambient endpoints after the private kubeconfig is made invalid.

Actual CLI graph/snapshot context selection and invalid-selector refusal pass
at `b7331f78`, retained in the shared `live-export-context-proof.json`. That
receipt predates the TUI implementation and does not prove the TUI action.
The extended reproducer now opens an owned PTY, uses Maps → SVG export, checks
the selected label/workload in the artifact and terminates only its own child.
