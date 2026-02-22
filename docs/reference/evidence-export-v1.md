# Evidence Export v1

**Status:** Specification
**Version:** evidence-export.v1
**Since:** v0.19
**Issue:** #166

---

## Purpose

This document defines the **evidence export format** for cub-scout debug bundles.
Evidence exports are derived from bundle contents and formatted for consumption by
external systems (tickets, chat, CI/CD, audit storage).

**Key invariant:** Creating or exporting evidence MUST NOT modify intended or runtime state.
Evidence is read-only and observational.

---

## Ownership Split

| Concern | Owner | Notes |
|---------|-------|-------|
| Bundle capture (cluster observation) | cub-scout | `cub-scout debug` captures bundle |
| Bundle summarization | cub-scout | `bundle summarize --format <fmt>` |
| Evidence export API | ConfigHub (future) | POST to Slack, Jira, S3 |
| Evidence correlation (across bundles) | ConfigHub (future) | Timeline, ChangeSet linkage |

cub-scout owns the **summary schema** (`BundleSummary`) and the **rendering formats**
(ticket, PR, Slack, ASCII, JSON). ConfigHub owns the **export API** and **evidence storage**.

---

## BundleSummary Schema (cub-scout)

The `BundleSummary` is the structured output of `bundle summarize --format json`.
This is the canonical evidence payload that cub-scout produces.

```json
{
  "formatVersion": "v1",
  "cluster": "prod-east",
  "namespace": "payments",
  "target": "Deployment/payments-api",
  "capturedAt": "2026-02-06T14:30:00Z",
  "gitContext": {
    "commit": "abc123def456",
    "branch": "main",
    "remote": "https://github.com/acme/apps"
  },
  "changes": {
    "driftCount": 3,
    "eventCount": 12,
    "categories": ["config", "replica-count"],
    "objects": ["Deployment/payments-api", "HPA/payments-api-hpa"]
  },
  "riskSignals": [
    {
      "level": "critical",
      "message": "3 critical drift finding(s)"
    }
  ],
  "evidence": {
    "bundlePath": "./debug-bundle-2026-02-06",
    "bundleHash": "sha256:789xyz..."
  }
}
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `formatVersion` | string | Yes | Always `"v1"` for this schema |
| `cluster` | string | No | Cluster name from bundle metadata |
| `namespace` | string | No | Target namespace |
| `target` | string | Yes | Target resource (e.g., `Deployment/api`) |
| `capturedAt` | string | Yes | ISO 8601 timestamp of bundle creation |
| `gitContext` | object | No | Git context at capture time |
| `gitContext.commit` | string | No | Git commit SHA |
| `gitContext.branch` | string | No | Git branch name |
| `gitContext.remote` | string | No | Git remote URL |
| `changes` | object | Yes | Summary of detected changes |
| `changes.driftCount` | int | Yes | Number of drift findings |
| `changes.eventCount` | int | Yes | Number of Kubernetes events |
| `changes.categories` | []string | No | Categories of changes detected |
| `changes.objects` | []string | No | Affected resource identifiers |
| `riskSignals` | []object | No | Potential issues detected |
| `riskSignals[].level` | string | Yes | `"warning"` or `"critical"` |
| `riskSignals[].message` | string | Yes | Human-readable risk description |
| `evidence` | object | Yes | Bundle provenance |
| `evidence.bundlePath` | string | Yes | Path to the source bundle |
| `evidence.bundleHash` | string | No | SHA-256 digest of bundle contents |

---

## Rendering Formats

`bundle summarize --format <fmt>` produces the following formats:

| Format | Use Case | Content Type |
|--------|----------|--------------|
| `json` | Machine consumption, CI/CD | `application/json` |
| `ascii` | Terminal display | `text/plain` |
| `ticket` | Jira, Linear, GitHub Issues | `text/markdown` |
| `pr` | Pull request description | `text/markdown` |
| `slack` | Slack Block Kit | `application/json` |

### Stable Rendering Rules

1. **Deterministic:** Same bundle + same format = identical output
2. **No external lookups:** Renderers use only bundle contents
3. **Omit absent fields:** Optional fields are omitted, not null
4. **Sorted arrays:** Categories, objects, risk signals sorted for determinism

---

## Mapping: Bundle Contents → BundleSummary

The `BundleSummary` is derived entirely from the debug bundle's internal files:

| BundleSummary field | Source in bundle | Derivation |
|---------------------|-----------------|------------|
| `formatVersion` | Hard-coded | Always `"v1"` |
| `cluster` | `metadata.json → target.cluster` | Direct copy |
| `namespace` | `metadata.json → target.namespace` | Direct copy |
| `target` | `metadata.json → target.kind/name` | Concatenated |
| `capturedAt` | `metadata.json → createdAt` | Direct copy |
| `gitContext` | `metadata.json → gitContext` | Direct copy (if present) |
| `changes.driftCount` | `drift.json → len(findings)` | Count of drift findings |
| `changes.eventCount` | `events.json → len(events)` | Count of K8s events |
| `changes.categories` | `drift.json → unique(finding.category)` | Deduplicated, sorted |
| `changes.objects` | `drift.json → unique(finding.resource)` | Deduplicated, sorted |
| `riskSignals` | `session.json → rootCause + drift severity` | Aggregated from analysis |
| `evidence.bundlePath` | Input argument | Path provided to CLI |
| `evidence.bundleHash` | Computed | SHA-256 of bundle directory |

---

## Mapping: BundleSummary → Slack Block Kit

The Slack format maps `BundleSummary` fields to Slack Block Kit elements:

| BundleSummary field | Slack Block Kit element |
|---------------------|------------------------|
| `target` | Header block text |
| `cluster`, `namespace` | Section fields |
| `capturedAt` | Context element |
| `changes.driftCount` | Section field (bold count) |
| `changes.categories` | Section field (comma-joined) |
| `riskSignals` | Context elements with level emoji |
| `evidence.bundlePath` | Not included (local path) |

---

## Mapping: BundleSummary → Ticket Markdown

The ticket format maps `BundleSummary` to markdown:

| Section | Source |
|---------|--------|
| Title (`# Summary`) | `target` + `cluster` |
| Metadata table | `cluster`, `namespace`, `capturedAt` |
| Changes section | `changes.driftCount`, `changes.eventCount` |
| Categories | `changes.categories` as bullet list |
| Risk Signals | `riskSignals` as warning/critical markers |
| Git Context | `gitContext.commit`, `gitContext.branch` |
| Evidence | `evidence.bundlePath` |

---

## Exit Codes

| Exit Code | Meaning |
|-----------|---------|
| 0 | Summary generated successfully |
| 1 | Bundle read error or invalid format |

---

## Relationship to Future Evidence API

When ConfigHub implements the evidence export API, cub-scout's `BundleSummary` will
be the payload. The API surface is out of scope for cub-scout:

```
cub-scout (produces)          ConfigHub (consumes)
─────────────────────         ──────────────────────
BundleSummary (JSON)    →     Evidence ingestion API
                              Evidence storage
                              Slack/Jira export
                              Timeline correlation
```

cub-scout will NOT implement:
- Posting to Slack (ConfigHub API does this)
- Creating Jira tickets (ConfigHub API does this)
- S3 upload (ConfigHub API does this)
- Evidence correlation across bundles (ConfigHub Timeline does this)

cub-scout WILL continue to own:
- Bundle capture from cluster
- Bundle summarization (all 5 formats)
- Deterministic replay
- Local CLI rendering

---

## See Also

| Doc | Purpose |
|-----|---------|
| [../debug-bundle.md](../debug-bundle.md) | Debug bundle format specification |
| [cli-contract.md](cli-contract.md) | CLI contract for `bundle summarize` |
| [json-contracts.md](json-contracts.md) | JSON contract index |
| [../bundle-diff.md](../bundle-diff.md) | Bundle diff v1 specification |
| [../bundle-timeline.md](../bundle-timeline.md) | Bundle timeline v1 specification |
