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

## Git Context (v0.18+)

When creating bundles, cub-scout automatically captures git repository context:

```json
{
  "gitContext": {
    "repoRoot": "/home/runner/work/myapp/myapp",
    "commitSHA": "abc123def456789",
    "branch": "main",
    "remoteURL": "https://github.com/myorg/myapp.git"
  }
}
```

### What's captured

| Field | Description |
|-------|-------------|
| `repoRoot` | Absolute path to repository root |
| `commitSHA` | Current HEAD commit SHA |
| `branch` | Current branch name |
| `remoteURL` | Origin remote URL |

### Key design decisions

- **Pure metadata** — git context is informational only; replay semantics are unchanged
- **Automatic capture** — captured at bundle creation time if running in a git repository
- **Graceful absence** — if not in a git repo, `gitContext` is omitted (not null/empty)
- **No live git ops** — replay never touches git; context is for correlation only

### Use cases

1. **CI traceability** — link bundle to the exact commit that produced it
2. **Issue correlation** — include commit SHA when sharing bundles in bug reports
3. **Audit trail** — know which branch/repo a bundle came from

### Viewing git context

```bash
# Bundle inspect shows git context when present
cub-scout bundle inspect ./debug-bundle

# Output includes:
# Git Context
#   Repo:            /home/runner/work/myapp/myapp
#   Commit:          abc123def456789
#   Branch:          main
#   Remote:          https://github.com/myorg/myapp.git
```

## Fleet & Environment Views (v0.18+)

"Fleet" in cub-scout means: an **explicit set of bundles with labels**.

No inference from namespaces, clusters, or git branches. Environment labels are metadata you declare.

### Quick start

```bash
# Run the demo
./examples/workflows/fleet-demo/fleet-demo.sh
```

### Creating a catalog

```bash
# Initialize catalog
cub-scout catalog init ./my-catalog

# Add bundles with environment labels
cub-scout catalog add ./my-catalog ./bundle-prod \
  --id prod \
  --label env=production \
  --sequence 1

cub-scout catalog add ./my-catalog ./bundle-staging \
  --id staging \
  --label env=staging \
  --sequence 2

cub-scout catalog add ./my-catalog ./bundle-dev \
  --id dev \
  --label env=development \
  --sequence 3
```

### Viewing the fleet

```bash
# List all bundles in catalog
cub-scout catalog list ./my-catalog

# Output:
#   [1] prod
#       Labels:  env=production
#       Seq:     1
#   [2] staging
#       Labels:  env=staging
#       Seq:     2
#   [3] dev
#       Labels:  env=development
#       Seq:     3
```

### Comparing environments

```bash
# Compare prod vs staging
cub-scout bundle diff ./my-catalog/bundles/prod ./my-catalog/bundles/staging

# Compare staging vs dev
cub-scout bundle diff ./my-catalog/bundles/staging ./my-catalog/bundles/dev
```

### Key design decisions

- **Explicit labeling** — `env=production` is a label you set, not inferred
- **Sequence ordering** — `--sequence` defines promotion order, not timestamps
- **Catalog-based** — catalogs are portable directories, not databases
- **Offline comparison** — diff reads bundles, no cluster access

### What "fleet" is NOT

- Not cluster discovery
- Not namespace inference
- Not git branch mapping
- Not automatic aggregation

It's just: "here are my bundles, here are their labels, compare them."

## Commands Reference

| Command | Purpose |
|---------|---------|
| `cub-scout debug ... --save-bundle <dir>` | Produce a bundle |
| `cub-scout bundle inspect <dir>` | View bundle metadata |
| `cub-scout bundle replay <dir>` | Replay bundle sections |
| `cub-scout bundle replay <dir> --format json` | Export as JSON |
| `cub-scout catalog init <dir>` | Create a new catalog |
| `cub-scout catalog add <catalog> <bundle>` | Add bundle with labels |
| `cub-scout catalog list <catalog>` | List bundles in catalog |
| `cub-scout bundle diff <a> <b>` | Compare two bundles |

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

- [Artifact workflow demo](../../examples/workflows/artifact-workflow-demo.sh)
- [Fleet demo](../../examples/workflows/fleet-demo/fleet-demo.sh)
- [Debug Bundle schema](../reference/schemas.md)
