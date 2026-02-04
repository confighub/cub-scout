# Bundle Timeline

**Version:** v0.15.0
**Purpose:** View how objects evolve across multiple bundles

---

## What is a Bundle Timeline?

> A **bundle timeline** aligns multiple bundles from a catalog into a time-series view showing how objects evolve across snapshots.

### Formal Definition

A timeline is a **derived view** over a catalog that:

- Shows presence/absence of objects across bundles
- Provides per-point summaries (not full diffs)
- Treats gaps as first-class status
- Requires explicit ordering and join mode

Key properties: *derived*, *deterministic*, *gap-aware*.

### What a Timeline IS

- A **time-series view** across N bundles
- A **new JSON schema** (`bundle-timeline.v1`)
- **Object-centric** — one series per object identity
- **Summary-level** — shows counts, not full content

### What a Timeline is NOT

- A modification to bundles or catalog
- A full diff between all pairs
- A live view
- A database query result

### Timeline vs Diff

| Concept | Scope | What It Shows |
|---------|-------|---------------|
| Diff | Two bundles | Per-section changes between A and B |
| Timeline | N bundles | Presence and summary at each point |

Use diffs for detailed comparison. Use timelines for evolution overview.

---

## Schema: `bundle-timeline.v1`

```json
{
  "schema_version": "bundle-timeline.v1",
  "catalog_ref": {
    "path": "./my-catalog",
    "bundle_count": 3,
    "schema_version": "catalog.v1"
  },
  "ordering": {
    "mode": "created_at",
    "tie_break": "id"
  },
  "join_mode": "object_id",
  "summary": {
    "bundle_count": 3,
    "series_count": 2,
    "total_points": 5,
    "gap_count": 1
  },
  "series": [
    {
      "identity": "Deployment:prod/api",
      "points": [
        {
          "bundle_id": "initial",
          "created_at": "2024-01-15T10:00:00Z",
          "status": "present",
          "sections": { "drift_count": 3, "has_correlation": false }
        },
        {
          "bundle_id": "after-scale",
          "created_at": "2024-01-15T11:00:00Z",
          "status": "missing"
        },
        {
          "bundle_id": "after-fix",
          "created_at": "2024-01-15T12:00:00Z",
          "status": "present",
          "sections": { "drift_count": 0, "has_correlation": true }
        }
      ]
    }
  ]
}
```

---

## Commands

### Build Timeline

```bash
cub-scout bundle timeline <catalog> [--order <mode>] [--join <mode>] [--format <format>]
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `catalog` | Path to the catalog directory |

**Options:**

| Flag | Default | Description |
|------|---------|-------------|
| `--order` | `manifest` | Ordering: `manifest`, `created_at`, `sequence` |
| `--join` | `object_id` | Join mode: `object_id` |
| `--format` | `ascii` | Output format: `ascii`, `json` |

**Example:**

```bash
# Timeline ordered by creation time
cub-scout bundle timeline ./my-catalog --order created_at

# JSON for automation
cub-scout bundle timeline ./my-catalog --format json
```

---

## Point Status

Each point in a series receives one of these statuses:

| Status | Description |
|--------|-------------|
| `present` | Object found in this bundle |
| `missing` | Object not found in this bundle (gap) |
| `ambiguous` | Multiple matches found |
| `unjoinable` | Object couldn't be matched |

---

## Point Sections

When a point is `present`, it includes section summaries:

| Field | Description |
|-------|-------------|
| `drift_count` | Number of drift findings for this object |
| `has_correlation` | Whether correlation data exists |

---

## Ordering Modes

| Mode | Description |
|------|-------------|
| `manifest` | Order as declared in `catalog.json` (default) |
| `created_at` | Sort by `created_at`, tie-break by `id` |
| `sequence` | Sort by `sequence` field, tie-break by `id` |

**Important:** Ordering is always explicit. The timeline never infers order from filesystem metadata.

---

## Join Modes

| Mode | Description | Status |
|------|-------------|--------|
| `object_id` | Join on `object_id` field | Supported |
| `composite` | Join on (kind, namespace, name) | Not yet implemented |
| `none` | No joining | Not yet implemented |

---

## Output Formats

### ASCII (Default)

```
Bundle Timeline
──────────────────────────────────────────────────

Catalog:  ./my-catalog
Bundles:  3
Order:    created_at
Join:     object_id

Summary
  Series:  2 object(s)
  Points:  5 present, 1 gaps

                         initial    after-scale  after-fix
  Deployment:prod/api    D:3        ·            D:0 C
  Deployment:prod/web    D:1        D:1          D:0

Legend: D:N = drift count, C = has correlation, · = gap
```

### JSON

Use `--format json` for machine-readable output (see schema above).

---

## Examples

### Track an Incident

```bash
# Initialize and populate catalog
cub-scout catalog init ./incident-123
cub-scout catalog add ./incident-123 ./capture-1 --id initial
cub-scout catalog add ./incident-123 ./capture-2 --id after-restart
cub-scout catalog add ./incident-123 ./capture-3 --id after-fix

# View timeline
cub-scout bundle timeline ./incident-123 --order created_at
```

### CI Regression Check

```bash
# Build timeline and check for gaps
cub-scout bundle timeline ./test-catalog --format json > timeline.json

# Alert if key object disappears
if jq -e '.series[] | select(.identity == "Deployment:prod/api") | .points[] | select(.status == "missing")' timeline.json > /dev/null; then
  echo "Warning: api deployment has gaps in timeline"
fi
```

### Correlate with Drift

```bash
# View timeline to identify which bundles to diff
cub-scout bundle timeline ./my-catalog --order created_at

# Then diff specific pair
cub-scout bundle diff ./my-catalog/bundles/before ./my-catalog/bundles/after
```

---

## Determinism Guarantees

| Guarantee | Description |
|-----------|-------------|
| **Sorted series** | Series sorted by identity |
| **Aligned points** | Points ordered by bundle order |
| **Explicit tie-break** | Always by `id` when values equal |
| **ASCII = f(JSON) + g** | ASCII derived from timeline JSON |

---

## Design Principles

1. **Gaps are first-class** — Missing objects are explicit, not hidden
2. **Presence + summary** — Not full content at each point
3. **No implicit ordering** — Never infer from filesystem
4. **Derived from catalog** — Catalogs are the authority

---

## Limitations

**v0.15.0 scope:**

- Only `object_id` join mode supported
- No filtering by identity or label
- No automatic gap detection alerts
- Summary only (use diff for details)

---

## See Also

- [Debug Bundles](debug-bundle.md) — What bundles contain
- [Catalogs](catalog.md) — Managing multiple bundles
- [Bundle Diff](bundle-diff.md) — Pairwise comparison
