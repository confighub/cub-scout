# Bundle Diff

**Version:** v0.15.0
**Purpose:** Compare two bundles and produce structured change reports

---

## What is a Bundle Diff?

> A **bundle diff** compares two bundles and produces a new JSON artifact (`bundle-diff.v1`) describing what changed between them.

### Formal Definition

A diff is a **new fact in a new schema** that:

- Describes changes between two bundles
- Never modifies the source bundles
- Requires explicit join mode to match objects
- Treats ambiguous/unjoinable as first-class statuses

Key properties: *additive*, *deterministic*, *explicit*.

### What a Diff IS

- A **structured comparison** of two bundles
- A **new JSON schema** (`bundle-diff.v1`)
- **Object-centric** — shows changes per object
- **Section-aware** — tracks drift and correlation separately

### What a Diff is NOT

- A modification to either bundle
- A patch or merge
- A live comparison
- An implicit "before/after" (you decide which is which)

---

## Schema: `bundle-diff.v1`

```json
{
  "schema_version": "bundle-diff.v1",
  "from": {
    "id": "before-fix",
    "created_at": "2024-01-15T10:30:00Z"
  },
  "to": {
    "id": "after-fix",
    "created_at": "2024-01-15T11:00:00Z"
  },
  "join_mode": "object_id",
  "summary": {
    "added": 0,
    "removed": 0,
    "changed": 1,
    "unchanged": 2,
    "ambiguous": 0,
    "unjoinable": 0
  },
  "objects": [
    {
      "identity": "Deployment:prod/api",
      "status": "changed",
      "sections": [
        {
          "name": "drift",
          "status": "changed",
          "counts": { "before": 3, "after": 1, "delta": -2 }
        },
        {
          "name": "correlation",
          "status": "unchanged"
        }
      ]
    }
  ]
}
```

---

## Commands

### Diff Two Bundles

```bash
cub-scout bundle diff <from> <to> [--join <mode>] [--format <format>]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `from` | Path to the source bundle |
| `to` | Path to the target bundle |

**Options:**

| Flag | Default | Description |
|------|---------|-------------|
| `--join` | `object_id` | Join mode: `object_id` |
| `--format` | `ascii` | Output format: `ascii`, `json` |

**Example:**

```bash
# Human-readable diff
cub-scout bundle diff ./before ./after

# JSON for automation
cub-scout bundle diff ./before ./after --format json
```

---

## Object Status

Each object receives one of these statuses:

| Status | Description |
|--------|-------------|
| `added` | Object in "to" but not in "from" |
| `removed` | Object in "from" but not in "to" |
| `changed` | Object in both, with section differences |
| `unchanged` | Object in both, with no differences |
| `ambiguous` | Multiple matches found (join unclear) |
| `unjoinable` | Object couldn't be matched |

---

## Section Status

Each section (drift, correlation) receives one of these statuses:

| Status | Description |
|--------|-------------|
| `changed` | Section content differs |
| `unchanged` | Section content identical |
| `missing_from` | Section not in "from" bundle |
| `missing_to` | Section not in "to" bundle |

---

## Join Modes

| Mode | Description | Status |
|------|-------------|--------|
| `object_id` | Join on `object_id` field | Supported |
| `composite` | Join on (kind, namespace, name) | Not yet implemented |
| `none` | No joining | Not yet implemented |

**Note:** Attempting to use `composite` or `none` returns an explicit error:

```
error: join mode 'composite' not yet implemented
error: join mode 'none' not yet implemented
```

---

## Output Formats

### ASCII (Default)

```
Bundle Diff
──────────────────────────────────────────────────

From:     before-fix
          2024-01-15 10:30:00 UTC
To:       after-fix
          2024-01-15 11:00:00 UTC
Join:     object_id

Summary
  Added:     0
  Removed:   0
  Changed:   1
  Unchanged: 2

Objects

  [CHANGED] Deployment:prod/api
    drift:       3 → 1 (Δ-2)
    correlation: unchanged
```

### JSON

Use `--format json` for machine-readable output (see schema above).

---

## Examples

### Basic Diff

```bash
# Compare two bundles
cub-scout bundle diff ./debug-before ./debug-after
```

### CI Integration

```bash
# Capture diff as JSON artifact
cub-scout bundle diff ./baseline ./current --format json > diff.json

# Check for regressions
if jq -e '.summary.added > 0' diff.json > /dev/null; then
  echo "New issues detected"
  exit 1
fi
```

### From Catalog

```bash
# Diff bundles within a catalog
cub-scout bundle diff ./my-catalog/bundles/before ./my-catalog/bundles/after
```

---

## Determinism Guarantees

| Guarantee | Description |
|-----------|-------------|
| **Sorted objects** | Objects sorted by identity |
| **Fixed section order** | Sections always in order: drift, correlation |
| **Tie-break by ID** | Consistent ordering when values match |
| **ASCII = f(JSON) + g** | ASCII output derived from JSON, not bundles |

---

## Design Principles

1. **New meaning = new schema** — Diffs are `bundle-diff.v1`, not modified bundles
2. **Explicit join mode** — Continuity requires explicit matching rules
3. **Ambiguous is a status** — Never guess when matching is unclear
4. **ASCII from JSON only** — Renderer consumes diff JSON, not source bundles

---

## Limitations

**v0.15.0 scope:**

- Only `object_id` join mode supported
- No deep diff within sections (just changed/unchanged)
- No diff of logs or events sections

---

## See Also

- [Debug Bundles](debug-bundle.md) — What bundles contain
- [Catalogs](catalog.md) — Managing multiple bundles
- [Bundle Timeline](bundle-timeline.md) — N-bundle time series
