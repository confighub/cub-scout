# Patterns Contract Reference

This document defines the contract for the patterns command surface.

> **v0.7+ contract surface.** Does not modify any v0.5 or v0.6 contracts.
>
> **v0.9 additions (Track J).** Pattern prerequisites and skip reasons (additive, backwards-compatible).

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
  skip_reason: <reason>  (v0.9+, only when status=SKIP and prereqs unmet)
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

**Remediation blocks (v0.8+):** When a finding includes remediation, it is rendered after the evidence:
```
    - [<severity>] <message>
      resource: <resource-id>
      evidence (<count>):
        - <evidence>
      Remediation: <summary>
        - <step 1>
        - <step 2>
      Links:
        - <url 1>
        - <url 2>
```

Rules for remediation rendering:
- `Remediation:` line is printed only if remediation is present and summary is non-empty
- Steps are printed only if the steps array is non-empty
- `Links:` section is printed only if links array is non-empty
- No placeholder text (like "(none)") is printed for absent optional blocks

#### JSON Output Format

```json
{
  "schema_version": "patterns.v1",
  "patterns": [
    {
      "id": "<pattern-id>",
      "name": "<pattern-name>",
      "status": "pass|fail|skip",
      "skip_reason": "<reason>",  // v0.9+, optional, only when status=skip
      "findings": [
        {
          "pattern": "<pattern-id>",
          "severity": "info|warning|error",
          "message": "<message>",
          "resource": "<resource-id>",  // optional
          "evidence": ["<evidence>", ...],  // optional
          "confidence": 0.9,  // optional, v0.8+
          "refs": ["<ref>", ...],  // optional, v0.8+
          "remediation": {  // optional, v0.8+
            "summary": "<summary>",
            "steps": ["<step>", ...],  // optional
            "links": ["<url>", ...]  // optional
          }
        }
      ]
    }
  ]
}
```

See [Finding Enrichment Fields (v0.8+)](#finding-enrichment-fields-v08) for details on optional fields.

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
  skip_reason: <reason>  (v0.9+, only when status=SKIP and prereqs unmet)
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
4. **Refs ordering (v0.8+)**: Refs are sorted alphabetically
5. **Steps ordering (v0.8+)**: Remediation steps maintain pattern-defined order (not re-sorted)
6. **Links ordering (v0.8+)**: Remediation links are sorted alphabetically
7. **Same input = same output**: Running patterns twice on the same graph produces identical output

---

## Finding Enrichment Fields (v0.8+)

Starting with v0.8, findings MAY include additional optional fields for actionability.
These fields are **additive** — consumers that only understand v0.7 fields may safely ignore them.

### confidence

- **Type:** number (0.0 to 1.0)
- **Required:** No
- **Description:** A deterministic confidence score for the finding. Must be stable given the same graph input.
- **Usage:** Fixed constants per finding type, not dynamic heuristics.

### refs

- **Type:** array of strings
- **Required:** No
- **Description:** Stable identifiers that tools may use to correlate findings across runs.
- **Examples:** `k8s:Deployment/ns/name`, `k8s:Pod/ns/name`, `crd:argoproj.io/Application`
- **Ordering:** Sorted alphabetically for deterministic output.

### remediation

- **Type:** object
- **Required:** No
- **Description:** Structured, user-facing guidance for resolving the finding.

**Remediation object fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `summary` | string | Yes (if remediation present) | Brief description of the fix |
| `steps` | array of strings | No | Ordered action steps |
| `links` | array of strings | No | Canonical documentation URLs |

**Ordering rules:**
- `steps` maintain pattern-defined order (not re-sorted) to preserve logical sequence
- `links` are sorted alphabetically for deterministic output

### Example with enrichment fields

```json
{
  "pattern": "k8s.ownership_chain_complete",
  "severity": "warning",
  "message": "ReplicaSet \"orphan-rs\" has no owning Deployment in graph",
  "resource": "test-cluster/default/ReplicaSet/orphan-rs",
  "confidence": 0.9,
  "refs": ["k8s:ReplicaSet/default/orphan-rs"],
  "remediation": {
    "summary": "Verify the ReplicaSet was created by a Deployment or is intentionally standalone.",
    "steps": [
      "Check if a Deployment with matching selector exists.",
      "Verify the ReplicaSet's ownerReferences field.",
      "If orphaned intentionally, consider adding an annotation to suppress this warning."
    ],
    "links": [
      "https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/"
    ]
  }
}
```

### Backwards Compatibility

- Consumers that only understand v0.7 fields MUST continue to function.
- New fields use `omitempty` in JSON — absent fields produce identical output to v0.7.
- Text output only renders remediation blocks when present (no placeholders).

---

## Pattern Prerequisites (v0.9+)

Patterns may declare prerequisites that must be satisfied before detection runs.
If prerequisites are not met, the pattern result is `skip` with a structured reason.

### Prerequisite Types

| Type | Description | Example |
|------|-------------|---------|
| `requires_node_kind` | Graph must contain at least one node of this kind | `Deployment` |
| `requires_any_of_kinds` | Graph must contain at least one node of any listed kind | `["Deployment", "ReplicaSet"]` |

Prerequisites are evaluated in declared order. The first unmet prerequisite determines the skip reason.

### Skip Reason (v0.9+)

When a pattern is skipped due to unmet prerequisites, the result includes a `skip_reason`:

**Text output:**
```
[SKIP] <pattern-id>
  <pattern-name>
  skip_reason: <reason>
```

**JSON output:**
```json
{
  "id": "<pattern-id>",
  "name": "<pattern-name>",
  "status": "skip",
  "skip_reason": "<reason>",
  "findings": []
}
```

The `skip_reason` field:
- Is only present when `status` is `skip` and prerequisites were unmet
- Is a deterministic, human-readable string
- Does not include timestamps or variable data

---

## Registered Patterns (v0.7+)

### k8s.ownership_chain_complete

**Category:** k8s

Checks that Deployment → ReplicaSet → Pod ownership chains are complete.
Incomplete chains may indicate orphaned resources or missing links.

**Prerequisites (v0.9+):**
- `requires_any_of_kinds`: `["Deployment", "ReplicaSet", "Pod"]`

**Status logic:**
- `pass`: All ownership chains are complete
- `fail`: Orphaned ReplicaSets or Deployments without ReplicaSets detected
- `skip`: Prerequisites not met (no Deployment/ReplicaSet/Pod nodes in graph)

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
