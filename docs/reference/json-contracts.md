# JSON Contracts and Output Model

Start here for machine-readable output contracts.

## TL;DR

1. JSON is the canonical data contract.
2. ASCII/Markdown are deterministic renderings of the same facts.
3. There is no single monolithic schema for every command in current releases.
4. The old v0.14 schema doc is historical and archived.

## Fast Entry Points

| If you need... | Read this |
|----------------|-----------|
| JSON vs ASCII meaning model | [../semantic-contract.md](../semantic-contract.md) |
| Stable command and flag surface | [cli-contract.md](cli-contract.md) |
| Exact command usage and JSON-capable flags | [commands.md](commands.md) |
| Historical v0.14 schema document | [../archive/v0.14-json-schema.md](../archive/v0.14-json-schema.md) |

## Field Naming Conventions

JSON field casing is **per-surface and frozen by compatibility**. No global normalization.

| Surface | Convention | Example fields |
|---------|------------|----------------|
| CLI commands (map, trace, bundle summarize) | camelCase | `formatVersion`, `capturedAt`, `driftCount` |
| Debug bundle metadata | camelCase | `cubScoutVersion`, `createdAt`, `gitContext` |
| Versioned schema artifacts (graph, catalog, bundle-diff/timeline, checkpoints) | snake_case + `schema_version` | `schema_version`, `join_mode`, `bundle_count` |

Existing surfaces keep their original field names — no renames for style consistency.
When fields cross surface boundaries, mapping is explicit (e.g., metadata `createdAt` → catalog `created_at`).

## Current Contract Sources by Surface

| Surface | Primary contract doc | Schema version signal |
|---------|----------------------|-----------------------|
| Graph export/explain | [graph-contract.md](graph-contract.md) | `graph.v1` |
| Patterns | [patterns-contract.md](patterns-contract.md) | `patterns.v1` |
| Bundle diff | [../bundle-diff.md](../bundle-diff.md) | `bundle-diff.v1` |
| Bundle timeline | [../bundle-timeline.md](../bundle-timeline.md) | `bundle-timeline.v1` |
| Catalog | [../catalog.md](../catalog.md) | `catalog.v1` |
| Evidence export | [evidence-export-v1.md](evidence-export-v1.md) | `evidence-export.v1` |
| General CLI JSON behavior | [cli-contract.md](cli-contract.md) + [commands.md](commands.md) | Command-specific |
| GitOps checkpoint proposal schemas | [gitops-checkpoint-schemas.md](gitops-checkpoint-schemas.md) | `change-intent.v1`, `execution-report.v1`, `change-interaction-card.v1`, `decision-receipt.v1`, `execution-receipt.v1`, `outcome-receipt.v1` |
| Trace secret evidence | This doc (below) | Embedded in `trace` JSON |
| Trace/explain recent events | This doc (below) | Embedded in `trace` and `explain` JSON (v1.10+) |
| Compare three-way agreement summary | This doc (below) | Embedded in `compare three-way` JSON |
| MCP standalone tools | CLI JSON contract of the wrapped command | Embedded in MCP `content[0].text` |
| MCP connected trust guidance | This doc (below) | Additive `structuredContent` wrapper |

## Tree / Map / Trace / Drift JSON Today

These surfaces are documented as command contracts and deterministic output behavior, not a single shared JSON schema file.

Use this sequence:

1. Command behavior and flags: [commands.md](commands.md)
2. JSON vs ASCII model: [../semantic-contract.md](../semantic-contract.md)
3. Stability + source-of-truth rule: [cli-contract.md](cli-contract.md)
4. Real output fixtures: `test/golden/`

Useful golden directories:

- `test/golden/map-list-json/`
- `test/golden/map-deployers-json/`
- `test/golden/ownership/`
- `test/golden/trace/`
- `test/golden/map-status/`
- `test/golden/bundle-summarize/`

For MCP, the current rule is:

1. tool arguments map to an existing CLI JSON command
2. the tool returns that JSON payload as text content
3. the wrapped CLI surface remains the contract source of truth
4. selected connected tools may add `structuredContent` with parsed data plus read-only trust guidance

## Trace Secret Evidence Contract

When `trace` is run on a supported resource kind, the JSON output includes a `secrets` field containing secret evidence metadata. This is safe metadata only — secret data (`.data`, `.stringData`) is never read or exposed.

### Supported Resource Kinds

- Workloads: `Deployment`, `StatefulSet`, `DaemonSet`, `Pod`
- Flux sources: `GitRepository`, `HelmRepository`, `Bucket`
- Flux deployers: `Kustomization`, `HelmRelease`
- Crossplane: `ProviderConfig`

### SecretEvidenceResult Schema

```json
{
  "secrets": {
    "resource": {
      "kind": "Deployment",
      "name": "my-app",
      "namespace": "prod"
    },
    "secrets": [
      {
        "name": "db-credentials",
        "namespace": "prod",
        "refType": "envFrom.secretRef",
        "refPath": "containers[0].envFrom[0]",
        "status": "present",
        "secretType": "Opaque",
        "createdAt": "2026-03-15T10:30:00Z",
        "owner": {
          "type": "flux",
          "subType": "kustomization",
          "name": "app-secrets",
          "namespace": "flux-system"
        },
        "optional": false
      },
      {
        "name": "missing-secret",
        "namespace": "prod",
        "refType": "volume.secret",
        "status": "missing",
        "statusReason": "secret not found"
      }
    ],
    "summary": {
      "total": 2,
      "present": 1,
      "missing": 1,
      "unreadable": 0,
      "unresolved": 0
    }
  }
}
```

**Note:** In the v0.14 trace JSON schema, the `secrets.resource` field is omitted because the trace's top-level `target` field already identifies the resource. See [v0.14-json-schema.md](../archive/v0.14-json-schema.md) for the trace-specific schema.

### Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `secrets.resource` | `ResourceRef` | The resource these secrets belong to (omitted in v0.14 trace) |
| `secrets.secrets[]` | `SecretEvidence[]` | List of secret references and their evidence |
| `secrets.summary` | `SecretEvidenceSummary` | Counts by status |

### SecretEvidence Fields

| Field | Type | Presence | Description |
|-------|------|----------|-------------|
| `name` | string | Always | Secret name |
| `namespace` | string | Always | Secret namespace |
| `refType` | string | Always | How the secret is referenced (see below) |
| `refPath` | string | Optional | Specific path where reference was found |
| `status` | string | Always | Resolution status: `present`, `missing`, `unreadable`, `unresolved` |
| `statusReason` | string | Optional | Additional context for status |
| `secretType` | string | When present | Kubernetes secret type (e.g., `Opaque`, `kubernetes.io/tls`) |
| `createdAt` | timestamp | When present | Secret creation time |
| `owner` | Ownership | When present | Detected ownership of the secret |
| `optional` | boolean | Optional | Whether the reference is marked optional |

### Reference Types (`refType`)

| Value | Description |
|-------|-------------|
| `envFrom.secretRef` | secretRef in envFrom |
| `env.valueFrom.secretKeyRef` | secretKeyRef in env variables |
| `volume.secret` | Secret volume |
| `volume.projected.secret` | Projected secret volume |
| `imagePullSecrets` | Image pull secrets |
| `serviceAccount.imagePullSecrets` | ServiceAccount image pull secrets |
| `spec.secretRef` | Flux source/deployer secretRef |
| `spec.credentials.secretRef` | Crossplane ProviderConfig credential secretRef |

### Status Values

| Status | Meaning |
|--------|---------|
| `present` | Secret exists and is readable |
| `missing` | Secret does not exist (NotFound) |
| `unreadable` | Secret exists but RBAC denies read access (Forbidden) |
| `unresolved` | Reference could not be resolved |

### Safety Guarantee

Secret evidence exposes only safe metadata. The following are **never** read or exposed:
- `.data` fields
- `.stringData` fields
- Actual secret values

Source: `pkg/agent/secret_evidence.go`

## Compare Three-Way Agreement Contract

When `compare three-way --format json` is used, the JSON output includes `summary.agreement` as the compact convergence/coverage summary for the selected scope.

### AgreementSummary Schema

```json
{
  "confighubUrl": "https://confighub.com/spaces/sp-123/units/payments-api",
  "summary": {
    "agreement": {
      "state": "converging",
      "summary": "2/5 resources converging",
      "reasons": [
        "2 changes in progress (controller syncing)"
      ],
      "sources": {
        "confighub": 5,
        "deployer": 5,
        "cluster": 5,
        "total": 5
      }
    }
  },
  "nextSteps": [
    {
      "actionType": "waiting",
      "reason": "Re-run the three-way comparison after controller convergence to confirm this scope has settled.",
      "nextCommand": "cub-scout compare three-way --scope namespace/prod --format json"
    }
  ]
}
```

### Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `summary.agreement.state` | string | Overall agreement state |
| `summary.agreement.summary` | string | Compact human-readable summary of the scope |
| `summary.agreement.reasons[]` | string[] | Deterministic supporting reasons |
| `summary.agreement.sources` | `SourceCoverage` | Evidence coverage counts used to derive the summary |
| `confighubUrl` | string | Exact ConfigHub unit URL when a representative connected unit can be identified |
| `nextSteps[]` | `StructuredHint[]` | Deterministic read-only follow-up guidance for trust review or convergence re-checks |

### Agreement States

| Value | Meaning |
|-------|---------|
| `agreed` | All compared resources align across the available layers |
| `converging` | Changes are in progress and expected to settle |
| `diverged` | Stale or unexplained disagreement exists |
| `partial` | Evidence is incomplete for a meaningful full-agreement claim |

### SourceCoverage Fields

| Field | Type | Description |
|-------|------|-------------|
| `confighub` | int | Resources with ConfigHub-side DRY/WET evidence |
| `deployer` | int | Resources counted by the current comparison flow as deployer-side coverage |
| `cluster` | int | Resources with observed LIVE evidence |
| `total` | int | Total resources in scope |

Source: `cmd/cub-scout/three_way.go`, `cmd/cub-scout/compare_three_way.go`

## MCP Structured Content Contract

When MCP tools return JSON-backed data, the gateway keeps the raw JSON string in `content[0].text`.
For the highest-value connected/read-only surfaces, it may also add `structuredContent`.

### Current Additive MCP Rule

- `compare_three_way` returns parsed CLI JSON under `structuredContent.data`
- `compare_three_way` may mirror `confighubUrl` and `nextSteps` from the CLI JSON at the top level of `structuredContent`
- `confighub_units`, `confighub_unit_get`, and `confighub_changesets` may add:
  - `structuredContent.data`
  - `structuredContent.nextSteps`
  - `structuredContent.confighubUrl` when an exact unit URL is known

### Example MCP Result Shape

```json
{
  "isError": false,
  "content": [
    {
      "type": "text",
      "text": "{\"summary\":{\"agreement\":{\"state\":\"agreed\"}}}"
    }
  ],
  "structuredContent": {
    "data": {
      "summary": {
        "agreement": {
          "state": "agreed"
        }
      }
    },
    "confighubUrl": "https://confighub.com/spaces/sp-123/units/payments-api",
    "nextSteps": [
      {
        "actionType": "human-decision",
        "reason": "Agreement is proven for this scope; open the governed unit to review the audit trail before sign-off.",
        "nextSurface": "https://confighub.com/spaces/sp-123/units/payments-api"
      }
    ]
  }
}
```

## Trace and Explain Recent Events Contract

As of v1.10, `trace` and `explain` commands include recent Kubernetes events for the target resource. Events are bounded (top 5), prioritized by severity (warnings/errors first), and contain safe metadata only.

### TraceEvents Schema (trace --format json)

```json
{
  "events": {
    "events": [
      {
        "type": "Warning",
        "reason": "BackOff",
        "message": "Back-off restarting failed container",
        "count": 3,
        "age": "5m",
        "severity": "warning",
        "source": "kubelet",
        "firstSeen": "2026-04-09T10:00:00Z",
        "lastSeen": "2026-04-09T10:05:00Z"
      }
    ],
    "totalCount": 10,
    "warningCount": 3,
    "errorCount": 1
  }
}
```

### ExplainSummary Events Schema (explain --format json)

```json
{
  "events": {
    "events": [...],
    "totalCount": 10,
    "warningCount": 3,
    "errorCount": 1
  }
}
```

### Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `events.events[]` | TraceEvent[] | Bounded list of recent events (max 5) |
| `events.totalCount` | int | Total events found for the resource |
| `events.warningCount` | int | Number of Warning-type events |
| `events.errorCount` | int | Number of error-severity events |

### TraceEvent Fields

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Event type: `Normal` or `Warning` |
| `reason` | string | Event reason (e.g., `Pulled`, `BackOff`, `FailedScheduling`) |
| `message` | string | Human-readable message (truncated to 200 chars) |
| `count` | int | Number of occurrences |
| `age` | string | Human-readable age (e.g., `5m`, `2h`, `3d`) |
| `severity` | string | Derived severity: `info`, `warning`, or `error` |
| `source` | string | Component that generated the event |
| `firstSeen` | string | RFC3339 timestamp of first occurrence |
| `lastSeen` | string | RFC3339 timestamp of last occurrence |

### Severity Mapping

| Severity | Condition |
|----------|-----------|
| `error` | Warning events with reasons: `CrashLoopBackOff`, `FailedScheduling`, `ImagePullBackOff`, `BackOff`, `FailedMount`, `FailedAttachVolume`, `ErrImagePull`, `Failed`, `FailedCreate`, `FailedKillPod` |
| `warning` | Other Warning-type events |
| `info` | Normal events |

### Sort Order

Events are sorted by:
1. Severity (error > warning > info)
2. Most recent first (by lastSeen timestamp)

Source: `pkg/agent/event_timeline.go`, `internal/mapsvc/jsonout.go`

## Historical Note

`v0.14-json-schema.md` is preserved for historical reference. It should not be treated as the canonical contract for current releases.

## Related Repo Tooling (Codex Handoff)

The Codex task handoff schema added for automation handoffs lives at:

- `tools/codex-task-output/codex-task-output.schema.json`
