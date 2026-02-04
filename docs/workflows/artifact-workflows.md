# Artifact Workflows

cub-scout produces and consumes **portable artifacts** that can move between CI, local machines, and reviews without re-deriving context.

## Quick Start

```bash
# Run the demo
./examples/workflows/artifact-workflow-demo.sh
```

## Bundle Layout

A debug bundle is a directory with predictable structure:

```
<bundle>/
  metadata.json      # Bundle metadata (required)
  session.json       # Debug session info (optional)
  drift.json         # Drift findings (optional)
  events.json        # Timeline events (optional)
  logs.json          # Container logs (optional)
  attribution.json   # Ownership graph (optional)
  README.md          # Human-readable summary
```

All files are JSON (except README.md). All outputs are deterministic.

## Workflow: CI to Local

### 1. CI produces a bundle

```bash
# In CI pipeline
cub-scout debug deploy/web-frontend -n production \
  --non-interactive \
  --save-bundle ./artifacts/debug-bundle

# Upload as CI artifact
# (GitHub Actions, GitLab CI, etc.)
```

### 2. Developer downloads and inspects

```bash
# Download artifact from CI

# Inspect bundle metadata
cub-scout bundle inspect ./debug-bundle

# Replay drift findings
cub-scout bundle replay ./debug-bundle --section drift

# Replay attribution graph
cub-scout bundle replay ./debug-bundle --section attribution
```

### 3. Export reports

```bash
# JSON (machine-readable)
cub-scout bundle replay ./debug-bundle --section drift --format json > drift.json
cub-scout bundle replay ./debug-bundle --section attribution --format json > attribution.json

# ASCII (human-readable)
cub-scout bundle replay ./debug-bundle --section drift > drift.txt
cub-scout bundle replay ./debug-bundle --section attribution > attribution.txt
```

## Key Properties

| Property | Guarantee |
|----------|-----------|
| **Deterministic** | Same bundle always produces identical output |
| **Self-contained** | Bundle has everything needed for replay |
| **Offline** | No cluster access required for replay |
| **Portable** | Works on any machine with cub-scout |

## Commands Reference

| Command | Purpose |
|---------|---------|
| `cub-scout debug ... --save-bundle <dir>` | Produce a bundle |
| `cub-scout bundle inspect <dir>` | View bundle metadata |
| `cub-scout bundle replay <dir>` | Replay bundle sections |
| `cub-scout bundle replay <dir> --format json` | Export as JSON |

## Example: GitHub Actions

```yaml
jobs:
  debug:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Debug deployment
        run: |
          cub-scout debug deploy/web-frontend -n production \
            --non-interactive \
            --save-bundle ./artifacts/debug-bundle

      - name: Upload bundle
        uses: actions/upload-artifact@v4
        with:
          name: debug-bundle
          path: ./artifacts/debug-bundle
```

## See Also

- [Demo script](../../examples/workflows/artifact-workflow-demo.sh)
- [Debug Bundle schema](../reference/schemas.md)
