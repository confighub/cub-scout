# Patterns Contract Reference

This document defines the contract for the patterns command surface.

> **v0.7+ contract surface.** Does not modify any v0.5 or v0.6 contracts.
>
> **v0.9 additions (Track J).** Pattern prerequisites and skip reasons (additive, backwards-compatible).
>
> **v0.10 additions.** Optional `--git-root` flag for git-aware pattern evidence (additive, backwards-compatible).

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
| `--git-root <path>` | (v0.10+) Path to local Git repository for git-aware patterns |
| `--git-url <url>` | (v0.11+) GitHub repository URL for connected mode |
| `--git-ref <ref>` | (v0.11+) Git ref (commit SHA recommended for determinism) |
| `--git-subpath <path>` | (v0.11+) Optional subpath within repository |

**Flag exclusivity (v0.11+):** `--git-root` is mutually exclusive with `--git-url`/`--git-ref`.
Providing both results in exit code 2 (usage error).

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
| `--git-root <path>` | (v0.10+) Path to local Git repository for git-aware patterns |
| `--git-url <url>` | (v0.11+) GitHub repository URL for connected mode |
| `--git-ref <ref>` | (v0.11+) Git ref (commit SHA recommended for determinism) |
| `--git-subpath <path>` | (v0.11+) Optional subpath within repository |

**Flag exclusivity (v0.11+):** `--git-root` is mutually exclusive with `--git-url`/`--git-ref`.
Providing both results in exit code 2 (usage error).

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

## Git-Aware Patterns (v0.10+)

Patterns may optionally use local Git repository data for enhanced detection.
Git-aware evidence is **optional** — all patterns continue to function without `--git-root`.

### The --git-root Flag

```bash
cub-scout patterns detect --git-root /path/to/repo
cub-scout patterns explain <pattern-id> --git-root /path/to/repo
```

When provided:
- Patterns that support git-aware detection gain access to repository files
- Additional evidence or findings may be produced
- Graph-only patterns are unaffected

When omitted or unreadable:
- Git-aware patterns SKIP with a deterministic skip_reason
- Graph-only patterns run normally
- No error is raised for missing `--git-root`

### Determinism Constraints for Git Repository Scanning

Git-aware patterns must maintain deterministic output:

1. **Bounded scan**: Only files within the repository root are scanned
2. **No network**: No git fetch, clone, or remote operations
3. **No submodule expansion**: Submodules are not recursively scanned
4. **Lexicographic ordering**: File paths are processed in sorted order (locale-independent)
5. **Reproducible**: Same repository state = same output
6. **Deterministic cap**: Scanning MUST enforce a deterministic maximum file limit (fixed default; optionally configurable). When exceeded, results include only the first N paths in sorted order

### Skip Behavior for Git-Aware Patterns

Git-aware patterns declare a prerequisite for git-root availability.
When `--git-root` is not provided or the path is unreadable, the pattern is skipped.

**Skip reason strings (deterministic):**

| Condition | skip_reason |
|-----------|-------------|
| `--git-root` not provided | `no git_root provided` |
| Path does not exist | `git_root path does not exist` |
| Path is not a directory | `git_root path is not a directory` |
| Path is not a Git repository | `git_root path is not a git repository` |

**Example output (when `--git-root` provided but invalid):**
```
[SKIP] gitops.argocd.applicationset_generators
  ApplicationSet Generator Summary
  skip_reason: git_root path does not exist
  findings (0):
    (none)
```

Note: These skip reasons apply when `--git-root` is omitted for **git-aware** patterns, or when `--git-root` is provided but unusable. Hybrid patterns run normally (with reduced evidence) when `--git-root` is omitted.

### Git-Aware vs Graph-Only Patterns

| Pattern Type | Requires --git-root | Behavior when git-root absent |
|--------------|---------------------|-------------------------------|
| Graph-only | No | Runs normally |
| Git-aware | Yes | SKIPs with skip_reason |
| Hybrid | No (but enhanced) | Runs with reduced evidence |

**Hybrid patterns** use git-root when available but can produce useful output from the graph alone.
They do not SKIP when git-root is absent.

### No Exit Code Changes

The `--git-root` flag does not introduce new exit codes:
- Exit 0: All patterns passed (including skipped patterns)
- Exit 4: One or more patterns failed

Skipped patterns do not cause exit code 4.

Invalid `--git-root` inputs MUST NOT change the command's exit code behavior beyond pattern results.
When `--git-root` is provided but unusable, affected patterns SKIP with deterministic reasons
rather than causing a global usage error (exit 2).

---

## Connected Mode (v0.11+)

Connected mode enables git-aware patterns to use **remote Git repository snapshots** instead of local repositories.
This is useful for CI/CD pipelines, auditing, and scenarios where the repository is not available locally.

### The --git-url and --git-ref Flags

```bash
cub-scout patterns detect --git-url https://github.com/org/repo --git-ref abc123def
cub-scout patterns explain <pattern-id> --git-url https://github.com/org/repo --git-ref main
```

| Flag | Required | Description |
|------|----------|-------------|
| `--git-url <url>` | Yes (for connected mode) | GitHub repository URL (e.g., `https://github.com/org/repo`) |
| `--git-ref <ref>` | Yes (for connected mode) | Git ref: commit SHA, branch name, or tag |
| `--git-subpath <path>` | No | Optional subpath within repository (e.g., `clusters/prod`) |

### Mutual Exclusivity

`--git-root` (local mode) and `--git-url`/`--git-ref` (connected mode) are **mutually exclusive**.

| Flags provided | Result |
|----------------|--------|
| Neither | Graph-only patterns run; git-aware/hybrid patterns SKIP or run reduced |
| `--git-root` only | Local mode (v0.10 behavior) |
| `--git-url` + `--git-ref` | Connected mode (v0.11+) |
| Both `--git-root` and `--git-url` | **Exit 2** (usage error) |

### Determinism in Connected Mode

**Commit SHA = deterministic.** When `--git-ref` is a full 40-character commit SHA, connected mode
produces deterministic output for a given repository state. Pinned commit SHAs provide reproducible
content snapshots for analysis.

**Branch/tag = non-deterministic.** When `--git-ref` is a branch name (e.g., `main`) or tag,
output may change over time as the ref advances. This is explicitly documented behavior.

| `--git-ref` value | Deterministic? | Notes |
|-------------------|----------------|-------|
| Full commit SHA (40 chars) | Yes | Same SHA = same output |
| Short commit SHA | No | May be ambiguous or provider-dependent; prefer full SHA |
| Branch name (e.g., `main`) | No | Output changes as branch advances |
| Tag name (e.g., `v1.0.0`) | No | Output changes if tag is force-pushed; recommend SHA |

**Recommendation:** For reproducible auditing and CI/CD, always use full 40-character commit SHAs.

### Usage Errors (exit 2)

The following flag combinations are invalid invocations and result in **exit code 2** (usage error):

| Condition | Exit code |
|-----------|-----------|
| `--git-url` without `--git-ref` | 2 |
| `--git-ref` without `--git-url` | 2 |
| `--git-subpath` without `--git-url` or `--git-root` | 2 |
| Both `--git-root` and `--git-url` | 2 |

These are syntax/requirement errors, not runtime failures. The command exits immediately with an error message.

### Skip Behavior for Connected Mode

Connected mode **runtime failures** result in **pattern-level SKIP**, not global command failure.
This maintains consistency with `--git-root` behavior.

**Skip reason strings (deterministic):**

| Condition | skip_reason |
|-----------|-------------|
| Repository not found (404) | `git_source repository not found` |
| Ref not found (branch/tag/SHA doesn't exist) | `git_source ref not found` |
| Fetch failed (network error, timeout) | `git_source fetch failed` |
| Authentication required but not provided | `git_source authentication required` |
| Tarball extraction failed | `git_source tarball invalid` |
| Rate limited (HTTP 429) | `git_source rate limited` |

**Example output (fetch failure):**
```
[SKIP] gitops.argocd.applicationset_generators
  ApplicationSet Generator Summary
  skip_reason: git_source repository not found
  findings (0):
    (none)
```

### Exit Code Summary

Connected mode uses standard exit codes:
- Exit 0: All patterns passed (including skipped patterns)
- Exit 2: Usage error (invalid flag combination; see [Usage Errors](#usage-errors-exit-2))
- Exit 4: One or more patterns failed

**Runtime failures** (network errors, 404, auth) cause pattern-level SKIP, not exit 2.
This ensures graceful degradation when remote repositories are temporarily unavailable.

### Authentication

Connected mode supports optional authentication via the `GITHUB_TOKEN` environment variable.

| `GITHUB_TOKEN` | Behavior |
|----------------|----------|
| Not set | Public repository access only (best-effort) |
| Set | Used for tarball download authentication |

**Notes:**
- Authentication is optional; public repos work without tokens
- Private repos require `GITHUB_TOKEN` with appropriate permissions
- Token is never logged or included in output
- Invalid tokens result in pattern SKIP with `git_source authentication required`

### Implementation Constraints

Connected mode maintains the same constraints as local mode:

1. **No git binary required**: Uses GitHub tarball API, not git clone
2. **No cloning**: Downloads and extracts tarball in memory or temp directory
3. **No submodule expansion**: Submodules are not recursively fetched
4. **Bounded scan**: Same file limits as local mode
5. **Lexicographic ordering**: Same deterministic file ordering
6. **Repo-relative refs**: All refs in output use `git:path:<relative>` format, never absolute paths

### Subpath Filtering

The optional `--git-subpath` flag limits scanning to a subdirectory:

```bash
cub-scout patterns detect \
  --git-url https://github.com/org/repo \
  --git-ref abc123 \
  --git-subpath clusters/prod
```

Only files under `clusters/prod/` are scanned. Refs in output remain relative to subpath root.

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

### gitops.argocd.resources_present (v0.9.2+)

**Category:** gitops

Reports the presence and count of Argo CD resources in the graph.

**Prerequisites:**
- `requires_any_of_kinds`: `["Application", "ApplicationSet"]`

**Status logic:**
- `pass`: At least one Argo CD resource found
- `skip`: No Application or ApplicationSet nodes in graph

**Findings:**
- Info: Argo CD Applications detected: N
- Info: Argo CD ApplicationSets detected: N

---

### gitops.flux.resources_present (v0.9.2+)

**Category:** gitops

Reports the presence and count of Flux resources in the graph.

**Prerequisites:**
- `requires_any_of_kinds`: `["Kustomization", "HelmRelease", "GitRepository"]`

**Status logic:**
- `pass`: At least one Flux resource found
- `skip`: No Kustomization, HelmRelease, or GitRepository nodes in graph

**Findings:**
- Info: Flux Kustomizations detected: N
- Info: Flux HelmReleases detected: N
- Info: Flux GitRepositories detected: N

---

### gitops.argocd.applicationset_generators (v0.10+)

**Category:** gitops
**Type:** Hybrid

Summarizes ApplicationSet generators. Runs without `--git-root` with reduced evidence;
enriched when `--git-root` is provided.

**Prerequisites:**
- `requires_any_of_kinds`: `["ApplicationSet"]`

**Status logic:**
- `pass`: ApplicationSet resources found in graph
- `skip`: No ApplicationSet nodes in graph (prerequisite unmet)

**Behavior by mode:**
- Without `--git-root`: Info findings based on graph-visible ApplicationSet metadata only
- With valid `--git-root`: Enriched findings with generator details from repo files
- With invalid `--git-root`: SKIP with deterministic `skip_reason`

**Findings:**
- Info: ApplicationSet count and basic metadata (graph-only mode)
- Info: Generator summary (cluster list, git, matrix, etc.) (git-enhanced mode)

---

### gitops.flux_kustomization_paths (v0.10+, planned)

**Category:** gitops
**Type:** Hybrid

Correlates Flux Kustomization paths with Git repository structure. Runs without `--git-root`
with reduced evidence; enriched when `--git-root` is provided.

**Prerequisites:**
- `requires_any_of_kinds`: `["Kustomization"]`

**Status logic:**
- `pass`: Kustomization resources found in graph
- `skip`: No Kustomization nodes in graph (prerequisite unmet)

**Behavior by mode:**
- Without `--git-root`: Info findings based on `spec.path` from cluster resources only
- With valid `--git-root`: Enriched findings validating paths exist in repo
- With invalid `--git-root`: SKIP with deterministic `skip_reason`

**Findings:**
- Info: Kustomization paths from cluster (graph-only mode)
- Info: Path correlation summary with repo validation (git-enhanced mode)

---

## Schema Version

The schema version is `patterns.v1`. Changes to the schema require a new version.

The schema version is included in all output formats to enable compatibility checks.

---

## See Also

- [Graph Contract Reference](graph-contract.md) — v0.6 graph schema
- [Graph Explain Contract](graph-explain-contract.md) — v0.6 graph explain output
- [CLI Contract Reference](cli-contract.md) — v0.5 CLI contracts
