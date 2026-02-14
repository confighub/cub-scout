# Workflow Examples

## The Problem

Your CI produces cluster snapshots, but nobody can compare them across environments.
A developer asks: *"What's different between staging and production right now?"*

Without a structured workflow, you'd need live cluster access to both environments,
run `kubectl diff` manually, and hope you don't miss anything.

**cub-scout bundles and catalogs make this offline and reproducible:**

```
$ ./cub-scout bundle diff prod-bundle staging-bundle

  Environment Comparison: prod ↔ staging
  ═══════════════════════════════════════
  Image differences:
    web-frontend: v1.2.3 (prod) → v1.3.0-rc1 (staging)
  Drift differences:
    prod: 0 findings
    staging: 1 warning (image mismatch)
```

## What's Here

| Workflow | What It Shows | Duration |
|----------|---------------|----------|
| [artifact-workflow-demo.sh](artifact-workflow-demo.sh) | CI → bundle → inspect → replay → export | ~30 sec |
| [fleet-demo/](fleet-demo/) | Create catalog → add environments → compare | ~30 sec |

## Artifact Workflow

End-to-end debug bundle lifecycle:

```
CI Pipeline                    Developer Laptop
───────────                    ────────────────
1. Run debug session     →     3. Inspect bundle
2. Save bundle with           4. Replay sections offline
   git context                5. Export reports (JSON + text)
```

```bash
# Run the demo
./examples/workflows/artifact-workflow-demo.sh

# Expected output:
# Step 1: CI produces a debug bundle
# Step 2: Developer inspects the bundle (sees repo/commit/branch)
# Step 3: Developer replays drift findings
# Step 4: Developer exports reports
```

**Key properties:**
- File-based (no cluster access for replay)
- Deterministic (same input = same output)
- Git-aware (repo, commit, branch captured as metadata)

## Fleet Demo

Compare environments using catalogs:

```
dev bundle → ┐
              ├→ Catalog → Compare any two environments
staging bundle → ┤
              ├→ "What changed between staging and prod?"
prod bundle → ┘
```

```bash
# Run the demo
./examples/workflows/fleet-demo/fleet-demo.sh

# Expected output:
# Step 1: Create a catalog
# Step 2: Add bundles with environment labels
# Step 3: List the catalog overview
# Step 4: Compare prod vs staging
# Step 5: Compare staging vs dev
```

**Key properties:**
- "Fleet" = explicit set of bundles with labels (no inference)
- Environment labels are metadata (`env=production`)
- Ordering via `--sequence`, not timestamps
- Comparison is offline, deterministic, portable

## Pre-Built Bundles

The `fleet-demo/bundles/` directory contains pre-built bundles for three environments:

```
fleet-demo/bundles/
├── dev/       # Development environment snapshot
├── staging/   # Staging environment snapshot
└── prod/      # Production environment snapshot
```

These are real cub-scout bundle format — you can inspect them with `bundle inspect`.

## Quick Start

```bash
# Build cub-scout first
go build ./cmd/cub-scout

# Run either demo
./examples/workflows/artifact-workflow-demo.sh
./examples/workflows/fleet-demo/fleet-demo.sh
```

## See Also

- [CLI-GUIDE.md](../../CLI-GUIDE.md) — `bundle` and `catalog` command reference
- [Platform Example](../platform-example/) — Learning environment for cluster exploration
