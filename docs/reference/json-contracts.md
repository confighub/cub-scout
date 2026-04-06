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

## Trace Secret Evidence Contract

When `trace` is run on a supported resource kind, the JSON output includes a `secrets` field containing secret evidence metadata. This is safe metadata only — secret data (`.data`, `.stringData`) is never read or exposed.

### Supported Resource Kinds

- Workloads: `Deployment`, `StatefulSet`, `DaemonSet`, `Pod`
- Flux sources: `GitRepository`, `HelmRepository`, `Bucket`
- Flux deployers: `Kustomization`, `HelmRelease`

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

### Field Reference

| Field | Type | Description |
|-------|------|-------------|
| `secrets.resource` | `ResourceRef` | The resource these secrets belong to |
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

## Historical Note

`v0.14-json-schema.md` is preserved for historical reference. It should not be treated as the canonical contract for current releases.

## Related Repo Tooling (Codex Handoff)

The Codex task handoff schema added for automation handoffs lives at:

- `tools/codex-task-output/codex-task-output.schema.json`
