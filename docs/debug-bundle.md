# Debug Bundles

**Version:** v0.14.6
**Purpose:** Portable snapshots for offline inspection, replay, and sharing

---

## What is a Debug Bundle?

> A **debug bundle** is an immutable snapshot of cub-scout's derived facts from a single run, designed for deterministic offline replay.

### Formal Definition

A bundle is a **closed set of deterministic artifacts** that, taken together, are sufficient to reproduce the *same JSON and ASCII outputs* that cub-scout produced at capture time, **without accessing the cluster, git, or wall-clock time**.

Key properties: *closed*, *deterministic*, *reproducible*.

### What a Bundle IS

- An **immutable, portable snapshot** of already-derived facts
- Produced by a **single cub-scout execution**
- Captured at a **specific point in time**
- **Self-contained** — no external dependencies for replay

### What a Bundle is NOT

- A live view
- A cache
- A database
- A time-series
- A source of new meaning

### Bundle Guarantees

If two people have the same bundle:

| Guarantee | Description |
|-----------|-------------|
| Identical JSON | `bundle replay` produces byte-identical JSON |
| Identical ASCII | Output identical (modulo terminal width) |
| Identical exit codes | CI behavior is reproducible |
| No external access | Cluster, git, network never consulted |

This makes bundles safe for: CI artifacts, incident handoff, offline debugging, long-term archiving.

### What a Bundle Does NOT Imply

A bundle does **not** imply:

- Continuity with any other bundle
- "Before" or "after"
- "Latest"
- "Regression"
- "Same object across time"

All temporal meaning requires **explicit multi-bundle semantics** (catalogs, ordering, join rules) — see v0.15+.

### Bundle vs Timeline

| Concept | Scope | Mutability | Meaning Source |
|---------|-------|------------|----------------|
| Bundle | One execution snapshot | Immutable | Existing facts |
| Timeline | Multiple bundles | Derived | New schema (v0.15+) |

A timeline is *computed from bundles*. A bundle is never computed from a timeline.

---

## Use Cases

Debug bundles are designed for:

- **Offline inspection** — analyze issues without cluster access
- **Sharing** — send to teammates, attach to tickets, store for later
- **Reproducibility** — same bundle always produces identical output
- **Archiving** — preserve debug state for future reference

Debug bundles contain **captured facts only** — no new interpretation happens during creation.

---

## What's Inside a Bundle

A bundle is a directory with these files:

| File | Required | Contents |
|------|----------|----------|
| `metadata.json` | Yes | Bundle version, creation time, tool version, target resource |
| `session.json` | No | Debug session data (workload health, ownership chain, root cause) |
| `drift.json` | No | Drift findings from comparison |
| `events.json` | No | Kubernetes events (timeline) |
| `logs.json` | No | Container log samples and detected patterns |
| `README.md` | Yes | Human-readable summary |

### metadata.json

```json
{
  "formatVersion": "v1",
  "cubScoutVersion": "v0.14.6",
  "createdAt": "2024-01-15T10:30:00Z",
  "label": "prod-incident-123",
  "target": {
    "kind": "Deployment",
    "name": "api",
    "namespace": "prod",
    "cluster": "prod-east"
  },
  "contents": {
    "hasSession": true,
    "hasDrift": true,
    "hasEvents": true,
    "hasLogs": true
  }
}
```

### Optional Files

Each optional file contains the same JSON structures used by live commands:

- **session.json** — Debug session with workload health, ownership, deployer status, source status, root cause analysis
- **drift.json** — Array of `DriftFinding` objects (same schema as `drift --format json`)
- **events.json** — Array of `TimelineEvent` objects
- **logs.json** — Array of `ContainerLogResult` objects with patterns

---

## Commands

### Inspect a Bundle

Show bundle metadata and contents summary:

```bash
cub-scout bundle inspect <path>
```

**Options:**

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `ascii` | Output format: `ascii`, `json` |

**Example output:**

```
Debug Bundle Inspection
──────────────────────────────────────────────────

Metadata
  Path:            ./debug-bundle-2024-01-15
  Format version:  v1
  Created by:      cub-scout v0.14.6
  Created at:      2024-01-15 10:30:00 UTC
  Label:           prod-incident-123

Target
  Kind:            Deployment
  Name:            api
  Namespace:       prod

Contents
  ✓ session.json
  ✓ drift.json      (3 items)
  ✓ events.json     (12 items)
  ✓ logs.json       (2 items)

Summary
  Contains: 3 drift finding(s), 12 event(s), 2 log result(s), session
```

### Replay Bundle Contents

Re-render bundle contents using existing renderers:

```bash
cub-scout bundle replay <path>
```

**Options:**

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `ascii` | Output format: `ascii`, `json` |
| `--section` | `drift` | Section to replay: `drift`, `correlation` |
| `--fail-on` | (none) | Exit non-zero if max severity >= level: `info`, `warning`, `critical` |

**Sections:**

- **drift** — Re-render drift findings (same output as `drift` command)
- **correlation** — Re-render drift-debug correlation narrative

---

## CI Integration

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | No failure (or no `--fail-on` specified) |
| 1 | Operational error (bad arguments, read failure) |
| 2 | Findings met the `--fail-on` severity threshold |

Exit codes match the `drift` command semantics exactly.

### Example: CI Replay

```bash
# Fail if captured drift includes warnings or critical
cub-scout bundle replay ./debug-bundle --fail-on warning
echo $?  # 2 if warnings found, 0 otherwise
```

---

## Examples

### Basic Inspection

```bash
# Inspect as ASCII (human-readable)
cub-scout bundle inspect ./debug-bundle-2024-01-15

# Inspect as JSON (machine-readable)
cub-scout bundle inspect ./debug-bundle-2024-01-15 --format json
```

### Replay Drift

```bash
# Replay drift as ASCII
cub-scout bundle replay ./debug-bundle-2024-01-15

# Replay drift as JSON
cub-scout bundle replay ./debug-bundle-2024-01-15 --format json
```

### Replay Correlation

```bash
# Show drift + failure correlation narrative
cub-scout bundle replay ./debug-bundle-2024-01-15 --section correlation
```

### CI Gating

```bash
# Fail CI on warning or critical drift
cub-scout bundle replay ./debug-bundle --fail-on warning --format json > replay.json
```

---

## Determinism and Contracts

### Guarantees

| Guarantee | Description |
|-----------|-------------|
| **Same output** | Same bundle always produces identical output |
| **No wall-clock** | Uses captured timestamps only, never current time |
| **Offline** | No cluster, git, or filesystem access (beyond the bundle) |
| **No new facts** | No JSON schema changes, no new interpretation |
| **ASCII = f(JSON) + g** | ASCII is always derived from JSON facts |

### What Replay Does NOT Do

- Contact the cluster
- Consult git or any external source
- Generate new timestamps
- Add or modify facts
- Change JSON schemas

---

## Versioning and Compatibility

### Format Version

The `formatVersion` field in `metadata.json` identifies the bundle format:

| Version | cub-scout | Notes |
|---------|-----------|-------|
| `v1` | v0.14.6+ | Initial release |

### Compatibility Rules

- **Backward compatible within v1**: Bundles created with v0.14.6 will be readable by v0.14.7+
- **New fields are additive**: Future versions may add optional fields but won't remove or change existing ones
- **Breaking changes require v2**: Any incompatible change will increment the major format version

---

## FAQ

**Q: Can I replay against a live cluster?**

No. Replay is offline by design — it only reads from the bundle. Use `drift`, `debug`, or other live commands to query your cluster.

**Q: What if a bundle is missing files?**

Optional files (`session.json`, `drift.json`, `events.json`, `logs.json`) may be absent. Inspect and replay will work with whatever is present. Only `metadata.json` is required.

**Q: Can I create bundles manually?**

Bundles follow a documented JSON schema. You can create them programmatically, but using cub-scout commands ensures correct structure and metadata.

**Q: How do I compare two bundles?**

Currently, compare them externally (e.g., `diff -r bundle1 bundle2`). Future versions may add bundle comparison features.

---

## See Also

- [Drift Detection](drift.md) — Understanding drift findings
- [v0.14 JSON Schema](v0.14-json-schema.md) — Full JSON schema reference
- [Semantic Contract](semantic-contract.md) — f(JSON) + g model
