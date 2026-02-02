# Patterns Contract Reference (v0.7)

This document defines the v0.7 contract for the patterns command surface.

> **v0.7 contract surface.** Does not modify any v0.5 or v0.6 contracts.

## Overview

The patterns engine provides deterministic pattern detection against the resource graph.
Patterns analyze the graph and report findings with status (pass/fail/skip).

## Commands

### patterns list

Lists all registered patterns.

```bash
cub-scout patterns list
```

#### Output Format

```
PATTERNS LIST
Schema: patterns.v1

Registered patterns (<count>):
  - <pattern-id>
    <pattern-name>
  ...
```

- Patterns are listed in **sorted order by ID** (deterministic)
- Output includes schema version for compatibility checks

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |

---

### patterns detect

Runs all registered patterns against the resource graph.

```bash
cub-scout patterns detect [flags]
```

#### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace to collect (empty = all namespaces) |
| `--empty` | Use empty graph (skip cluster collection) |
| `--json` | Output as JSON |

#### Text Output Format

```
PATTERNS DETECT
Schema: patterns.v1

[<STATUS>] <pattern-id>
  <pattern-name>
  findings (<count>):
    - [<severity>] <message>
      resource: <resource-id>  (if applicable)
      evidence (<count>):
        - <evidence>
        ...

...
```

Where:
- `<STATUS>` is one of: `PASS`, `FAIL`, `SKIP`
- `<severity>` is one of: `info`, `warning`, `error`

The **findings block is always printed**, even when empty:
```
  findings (0):
    (none)
```

#### JSON Output Format

```json
{
  "schema_version": "patterns.v1",
  "patterns": [
    {
      "id": "<pattern-id>",
      "name": "<pattern-name>",
      "status": "pass|fail|skip",
      "findings": [
        {
          "pattern": "<pattern-id>",
          "severity": "info|warning|error",
          "message": "<message>",
          "resource": "<resource-id>",  // optional
          "evidence": ["<evidence>", ...]  // optional
        }
      ]
    }
  ]
}
```

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All patterns passed |
| 4 | One or more patterns failed |

---

### patterns explain

Explains a specific pattern and shows its detection results.

```bash
cub-scout patterns explain <pattern-id> [flags]
```

#### Flags

| Flag | Description |
|------|-------------|
| `-n, --namespace` | Namespace to collect (empty = all namespaces) |
| `--empty` | Use empty graph (skip cluster collection) |

#### Output Format

```
PATTERN EXPLAIN
ID: <pattern-id>
Name: <pattern-name>
Category: <category>

Description:
  <description>

Result: [<STATUS>]
  findings (<count>):
    - [<severity>] <message>
      ...
```

#### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (pattern found, status is pass or skip) |
| 2 | Usage error (missing argument) |
| 3 | Unknown pattern ID |
| 4 | Pattern failed |

---

## Determinism Guarantees

All patterns output is deterministic:

1. **Pattern ordering**: Patterns are always listed/processed in sorted order by ID
2. **Finding ordering**: Findings are sorted by (severity, resource, message)
3. **Evidence ordering**: Evidence strings are sorted alphabetically
4. **Same input = same output**: Running patterns twice on the same graph produces identical output

---

## Registered Patterns (v0.7)

### k8s.ownership_chain_complete

**Category:** k8s

Checks that Deployment → ReplicaSet → Pod ownership chains are complete.
Incomplete chains may indicate orphaned resources or missing links.

**Status logic:**
- `pass`: All ownership chains are complete
- `fail`: Orphaned ReplicaSets or Deployments without ReplicaSets detected
- `skip`: No Deployments in graph

**Findings:**
- Warning: ReplicaSet has no owning Deployment
- Warning: Deployment has no ReplicaSets
- Info: Pod is not owned by any ReplicaSet (may be standalone or other controller)

---

### gitops.controller_presence

**Category:** gitops

Checks for the presence of GitOps controller CRDs (Argo CD Applications/ApplicationSets,
Flux Kustomizations/HelmReleases). Detects which GitOps tools are in use.

**Status logic:**
- `pass`: At least one GitOps controller CRD found
- `fail`: No GitOps controller CRDs found

**Findings:**
- Info: Argo CD controller detected (with counts)
- Info: Flux controller detected (with counts)
- Warning: No GitOps controllers detected

---

## Schema Version

The schema version is `patterns.v1`. Changes to the schema require a new version.

The schema version is included in all output formats to enable compatibility checks.

---

## See Also

- [Graph Contract Reference](graph-contract.md) — v0.6 graph schema
- [Graph Explain Contract](graph-explain-contract.md) — v0.6 graph explain output
- [CLI Contract Reference](cli-contract.md) — v0.5 CLI contracts
