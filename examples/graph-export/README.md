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

- `--max-nodes` applies to visual formats (`dot`, `svg`, `html`) to keep large
  clusters shareable.
- `--format json` remains the contract format for automation and schema checks.
- `--json` is still accepted as a legacy alias for `--format json`.
