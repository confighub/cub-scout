# Catalogs

**Version:** v0.15.0
**Purpose:** Index multiple debug bundles for multi-bundle operations

---

## What is a Catalog?

> A **catalog** is a file-backed manifest that indexes multiple bundles for multi-bundle operations (diffs, timelines).

### Formal Definition

A catalog is an **explicit, portable index** of debug bundles that:

- Declares bundle ordering (never inferred from filesystem)
- Enables grouping via scope and labels
- Provides the foundation for diffs and timelines

Key properties: *explicit*, *deterministic*, *portable*.

### What a Catalog IS

- A **manifest file** (`catalog.json`) with bundle references
- **Directory-based** — no database dependencies
- **Read-only** — catalogs never modify bundles
- **Ordering authority** for `--order manifest`

### What a Catalog is NOT

- A database
- A cache
- A live index
- A source of new meaning

### Catalog Guarantees

| Guarantee | Description |
|-----------|-------------|
| **Explicit ordering** | Bundle order is declared, not inferred |
| **Deterministic** | Same catalog + same bundles = same results |
| **Portable** | Self-contained directory structure |
| **Bundles unchanged** | Catalogs never modify bundle contents |

---

## Schema: `catalog.v1`

### catalog.json

```json
{
  "schema_version": "catalog.v1",
  "bundles": [
    {
      "id": "Deployment-api-20240115-103000",
      "path": "bundles/Deployment-api-20240115-103000",
      "created_at": "2024-01-15T10:30:00Z",
      "digest": "sha256:abc123...",
      "scope": {
        "cluster": "prod-east",
        "namespace": "prod",
        "target": "Deployment/api"
      },
      "labels": {
        "incident": "INC-123"
      },
      "sequence": 1
    }
  ]
}
```

### Entry Fields

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Stable unique identifier |
| `path` | Yes | Relative path from catalog directory to bundle |
| `created_at` | Yes | Copied from bundle's `metadata.createdAt` |
| `digest` | No | SHA256 hash for integrity verification |
| `scope` | No | Grouping metadata (cluster, namespace, target) |
| `labels` | No | Arbitrary key/value metadata |
| `sequence` | No | Explicit ordering integer for `--order sequence` |

---

## Commands

### Initialize a Catalog

Create a new empty catalog:

```bash
cub-scout catalog init <path>
```

**Example:**

```bash
cub-scout catalog init ./my-catalog
# Creates:
#   ./my-catalog/catalog.json
#   ./my-catalog/bundles/
```

### Add a Bundle

Add a bundle to the catalog:

```bash
cub-scout catalog add <catalog-path> <bundle-path> [--id <id>] [--label <key>=<value>]
```

**Options:**

| Flag | Description |
|------|-------------|
| `--id` | Override the auto-generated bundle ID |
| `--label` | Add a label (repeatable) |

**Example:**

```bash
cub-scout catalog add ./my-catalog ./debug-bundle-2024-01-15 \
  --id before-fix \
  --label incident=INC-123
```

### List Bundles

List all bundles in a catalog:

```bash
cub-scout catalog list <catalog-path> [--order <mode>] [--format <format>]
```

**Options:**

| Flag | Default | Description |
|------|---------|-------------|
| `--order` | `manifest` | Ordering: `manifest`, `created_at`, `sequence` |
| `--format` | `ascii` | Output format: `ascii`, `json` |

**Example:**

```bash
cub-scout catalog list ./my-catalog --order created_at
```

### Validate Catalog

Validate a catalog and its bundle references:

```bash
cub-scout catalog validate <catalog-path>
```

Checks:

- Schema version compatibility
- Bundle paths exist
- No duplicate IDs
- `created_at` matches bundle metadata

---

## Ordering Modes

| Mode | Description |
|------|-------------|
| `manifest` | Order as declared in `catalog.json` (default) |
| `created_at` | Sort by `created_at`, tie-break by `id` |
| `sequence` | Sort by `sequence` field, tie-break by `id` |
| `input` | Preserve CLI argument order (for `bundle diff`) |

**Tie-breaking:** When two bundles compare equal, the one with the lexicographically smaller `id` comes first. This ensures deterministic output.

---

## Directory Structure

```
my-catalog/
├── catalog.json          # Manifest file
└── bundles/
    ├── before-fix/       # Bundle directory
    │   ├── metadata.json
    │   ├── drift.json
    │   └── ...
    └── after-fix/        # Bundle directory
        ├── metadata.json
        ├── drift.json
        └── ...
```

---

## Examples

### Track an Incident

```bash
# Initialize catalog
cub-scout catalog init ./incident-123

# Add bundles as you debug
cub-scout catalog add ./incident-123 ./debug-initial --id initial --label stage=discovery
cub-scout catalog add ./incident-123 ./debug-after-fix --id after-fix --label stage=validation

# View timeline
cub-scout bundle timeline ./incident-123 --order created_at
```

### Compare Before/After

```bash
# Add two bundles
cub-scout catalog init ./comparison
cub-scout catalog add ./comparison ./before --id before
cub-scout catalog add ./comparison ./after --id after

# Diff them
cub-scout bundle diff ./comparison/bundles/before ./comparison/bundles/after
```

---

## Design Principles

1. **Explicit ordering** — Never infer order from filesystem (timestamps, names)
2. **Deterministic** — Same inputs always produce same outputs
3. **Scope enables grouping** — Use scope to track "same thing" across time
4. **Labels are user metadata** — Use labels for your own organization

---

## Limitations

**v0.15.0 scope:**

- No automatic bundle collection from cluster
- No catalog-level queries or filtering
- Scope grouping is advisory only (not enforced)

---

## See Also

- [Debug Bundles](debug-bundle.md) — What bundles contain
- [Bundle Diff](bundle-diff.md) — Pairwise comparison
- [Bundle Timeline](bundle-timeline.md) — N-bundle time series
