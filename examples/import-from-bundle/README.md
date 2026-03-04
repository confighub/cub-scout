# Import from Bundle

Import proposal generation from an existing debug bundle, without cluster access.

This example demonstrates the `--from-bundle` path added to `cub-scout import`.
It reuses the sample bundle in:

`examples/workflows/fleet-demo/bundles/dev`

## Run

```bash
./cub-scout import --from-bundle examples/workflows/fleet-demo/bundles/dev --dry-run --json
```

## Expected Output

Expected JSON is committed at:

`examples/import-from-bundle/expected-output/suggestion.json`

The output shape is identical to live-cluster dry-run JSON:
- `namespaces[]`
- `workloads[]`
- `proposal`
- `evidence` (`source=bundle`, `bundlePath=<path>`)

Only the source of facts changes (`bundle` vs `live cluster`).
