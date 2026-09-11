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
| Current-change rollout evidence | This doc (below) | Embedded in `doctor`, `explain`, and `compare three-way` JSON |
| Install receipt predicates | This doc (below) | Embedded in `receipt verify` JSON |
| Audited action event metadata | This doc (below) | Embedded in `trace`, `explain`, and `map activity` JSON |
| Trace/explain recent events | This doc (below) | Embedded in `trace` and `explain` JSON (v1.10+) |
| Compare three-way agreement summary | This doc (below) | Embedded in `compare three-way` JSON |
| GitOps controller coverage | This doc (below) | Embedded in `gitops status` JSON |
| Observation evidence | This doc (below) | Embedded in `snapshot` JSON as `observation`, `summary list` JSON as top-level and per-entry `observation`, `map list` JSON entries as `observation`, and watch/bot events as `observation` |
| Platform substrate evidence | This doc (below) | Embedded in `map list` JSON as `ownerEvidence`, in watch/bot events as `owner.evidence`, and in receipts as `predicate.evidence.platformSubstrate` |
| GitOps delivery evidence | This doc (below) | Embedded in `gitops status --with-confighub` and `doctor --with-confighub` JSON |
| Resource delivery evidence | This doc (below) | Embedded in `trace --with-confighub`, `explain --with-confighub`, and single-resource `receipt verify --with-confighub` JSON |
| Bounded resource read (unreleased v2.10) | This doc (below) | Additive `resourceRead`, `configHubOrigin`, and `omissions` on bounded `explain` only |
| Map activity delivery rows | This doc (below) | Embedded in `map activity --with-confighub` JSON rows |
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

## Bounded Resource Read (Unreleased v2.10)

`explain --bounded --format json` and MCP `explain` with `bounded: true` use
the same ExplainSummary as normal explain, plus:

| Field | Meaning |
|---|---|
| `resourceRead.available` | An identity-matching object was read or reused; not a health or desired-state verdict |
| `resourceRead.context` | Explicit selected kube context, never inferred from a ConfigHub Target |
| `resourceRead.resource` | Exact `apiVersion`, `kind`, `namespace`, `name`; namespace is empty for cluster-scoped resources |
| `resourceRead.uid`, `resourceRead.resourceVersion` | Kubernetes object identity/version when present |
| `resourceRead.observedAt`, `resourceRead.expiresAt` | UTC timestamps for observation and session reuse deadline; zero timestamps (`0001-01-01T00:00:00Z`) when unavailable |
| `resourceRead.cache` | `miss`, `hit`, or `refresh`; no persistent cache across CLI invocations |
| `resourceRead.reads.discovery`, `resourceRead.reads.object` | Request attempts for this operation (each 0 or 1); auth transport traffic excluded |
| `omissions[]` | Existing omission shape: `missing`, `reason`, `severity`; omitted evidence is not a negative finding |

Every bounded response declares omissions for `source-controller-evidence`,
`related-pod-event-evidence`, and `desired-live-comparison`. A failed read adds
`live-resource` with warning severity and leaves health unavailable and ownership
unknown. Read failures can return a successful CLI/MCP envelope containing this
unavailable summary: check `resourceRead.available`. Invalid input or kubeconfig
loading instead fails the command/tool.

`Source`, `DeployedVia`, `Risks`, and `Drift` summary fields (lower-camel-case in
JSON) explicitly say "Not assessed (bounded read)". Known native workload
API/Kind pairs can include generation-aware `currentChange` evidence; supported
generic Ready conditions can supply local health. Neither establishes controller
delivery, desired/live agreement, or application success. Raw object data is not
emitted. Hints preserve exact scope and recommend only a fresh bounded read.

The real MCP gateway and TUI each reuse up to 16 observations for less than 15
seconds. A hit keeps the original timestamps and uses zero requests. Refresh
invalidates old evidence even if the new read fails. Tests and live smoke:
[bounded resource evidence](../../examples/bounded-resource-read/).

### Observed Origin

Bounded explain optionally adds `configHubOrigin`, parsed from the selected
object's `confighub.com/origin` annotation without additional API calls:

```json
{
  "configHubOrigin": {
    "source": "annotation:confighub.com/origin",
    "spaceId": "11111111-1111-4111-8111-111111111111",
    "spaceSlug": "team-a",
    "unitId": "22222222-2222-4222-8222-222222222222",
    "unitSlug": "api",
    "revisionNum": 7
  }
}
```

- `spaceId` and `unitSlug` are required; `spaceSlug`, `unitId`, and `revisionNum`
  are optional. Absent revision differs from explicit zero. Revision is a
  nonnegative int64 JSON integer; consumers must preserve integer precision.
- This is observed, unverified metadata from the same `resourceRead`
  observation. It does not change ownership, readiness, rollout verdicts,
  `source`/`deployedVia`, or the source/controller/comparison omissions.
  No release, target, component, variant, or kube-context binding is inferred.
- Missing annotation omits `configHubOrigin` and adds an information-level
  `confighub-origin` omission. Malformed/duplicate JSON, missing required fields,
  unsupported values, or conflicting legacy SpaceID/UnitID/UnitSlug/RevisionNum
  labels or annotations add a warning omission instead. Text/Markdown/TUI
  also show the reason. Read failure provides only the existing live-resource
  omission; it cannot retain earlier origin evidence.
- Parsing is case-sensitive. Origin is limited to 8 KiB; nonempty identifiers
  to 256 ASCII letters/digits/dots/underscores/hyphens, starting with a letter
  or digit. Optional identifier strings may be empty. Unknown future keys are
  ignored, never echoed. There is no legacy fallback or field merging; legacy
  fields only check conflicts with fields actually supplied in origin.
- This first integration is bounded explain only, including CLI/plugin, MCP,
  and the existing TUI picker. Rich explain/trace, map inventory JSON,
  watch/bot, receipts, and connected Resource joins are not changed.

Source shape pinned to [SDK Origin at `1303148`](https://github.com/confighub/sdk/blob/1303148d2bc952d8b9a4feb4fcca4d11e6e74722/configkit/k8skit/k8skit.go#L288).
The stricter rejection rules above are Scout's evidence policy, not additional
claims about the upstream annotation schema. See the
[executable fixture](../../examples/bounded-resource-read/#observed-origin).

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
  "confighubUrl": "https://confighub.com/units/sp-123/u-123",
  "confighubRevisionsUrl": "https://confighub.com/units/sp-123/u-123?tab=2",
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
| `confighubUrl` | string | Exact ConfigHub unit detail URL when a representative connected unit ID is known |
| `confighubRevisionsUrl` | string | Exact ConfigHub unit revisions tab URL when a representative connected unit ID is known |
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

## Field Mutation Attribution Contract

When `compare three-way` and `explain` are run in connected mode (and the live cluster is reachable), each field mismatch is annotated with a `cause` classifying the mutation source — controller drift vs manual edit — derived from K8s `metadata.managedFields` co-signaled with the resource owner detected from labels and annotations. This is the first stage of the attribution layer; see `pkg/agent/field_ownership.go`.

### compareFieldMismatch additions (compare three-way / compare three-way per-resource)

```json
{
  "mismatches": [
    {
      "field": "replicas",
      "dry": "3",
      "wet": "3",
      "live": "1",
      "cause": "manual-edit",
      "managerHint": "kubectl-edit"
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `cause` | string enum | `controller-drift`, `manual-edit`, or `unknown`. Omitted when classification yields no signal. |
| `managerHint` | string | Representative manager string from `metadata.managedFields` for transparency. Omitted when no manager string was identified. |

When the live K8s resource has decodable `FieldsV1` data in `metadata.managedFields`, `cause` and `managerHint` are resolved **per-field-path** (A1.5) — each field mismatch gets the classification specific to its path. When `FieldsV1` is absent or the field name doesn't map to a single canonical path (e.g., `images` which spans container list items), the classifier falls back to the resource-level rollup (A1).

The per-field-path map is also exposed under `live.attributionByPath`, keyed by canonical path strings as rendered by `sigs.k8s.io/structured-merge-diff/v4/fieldpath.Path.String` (for example `.spec.replicas` or `.spec.template.spec.containers[name="api"].image`).

### Cause values

| Value | Meaning |
|-------|---------|
| `controller-drift` | The resource's expected GitOps/orchestration controller (per `pkg/agent/ownership.go`) is reconciling fields. A mismatch with desired state is likely transient. |
| `manual-edit` | A `kubectl-*` or other interactive tool has written fields. Includes the mixed case where both a controller and an interactive tool have managed fields. |
| `unknown` | The cause cannot be confidently determined — `managedFields` missing/empty, or only unrecognized manager strings present. Omitted from JSON output. |

### ExplainSummary additions (explain --format json)

```json
{
  "resource": "Deployment/api",
  "namespace": "prod",
  "owner": "ArgoCD",
  "drift": "Detected by ConfigHub",
  "currentChange": {
    "resource": {
      "apiVersion": "apps/v1",
      "kind": "Deployment",
      "namespace": "prod",
      "name": "api"
    },
    "progress": {
      "phase": "applied",
      "clockSource": "status.observedGeneration<metadata.generation"
    },
    "verdict": "WATCH",
    "reason": "stale_generation",
    "evidence": {
      "kstatusStatus": "InProgress",
      "generation": 2,
      "observedGeneration": 1,
      "observedAt": "2026-07-09T10:30:00Z"
    }
  },
  "mutationCause": "manual-edit",
  "mutationManager": "kubectl-edit"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `currentChange` | object | Optional generation-scoped rollout progress/verdict for workload resources. Omitted for non-workloads or when live rollout evidence cannot be fetched. |
| `currentChange.progress.phase` | string enum | One of `pending`, `applied`, `rolling_out`, `stalled`, `complete`, or `unknown`. |
| `currentChange.verdict` | string enum | `PASS`, `WATCH`, `BLOCK`, or `INCONCLUSIVE`. Uses the same vocabulary as receipts. |
| `currentChange.reason` | string enum | Stable reason such as `workload_converged`, `rollout_progressing`, `stale_generation`, `progress_stalled`, `runtime_failed`, `rollout_failed`, `workload_missing`, or `evidence_missing`. |
| `currentChange.evidence` | object | Reviewable kstatus, generation, observed generation, pod reason, and observation timestamp evidence used to build the verdict. |
| `mutationCause` | string enum | Same enum as `cause` above. Best-effort; omitted on fetch failure or when no signal is present. |
| `mutationManager` | string | Representative manager string for transparency. |

### DoctorSummary rollout additions (doctor --format json)

When live workload rollout evidence is available, `doctor --format json`
includes a `rollouts` object. The field is omitted when no workload rollout
evidence could be collected.

```json
{
  "rollouts": {
    "total": 4,
    "pass": 2,
    "watch": 1,
    "block": 1,
    "inconclusive": 0,
    "currentChanges": [
      {
        "resource": {
          "apiVersion": "apps/v1",
          "kind": "Deployment",
          "namespace": "prod",
          "name": "api"
        },
        "progress": {"phase": "stalled"},
        "verdict": "BLOCK",
        "reason": "runtime_failed",
        "evidence": {
          "podReasons": [
            {"pod": "api-abc", "reason": "CrashLoopBackOff"}
          ]
        }
      }
    ]
  }
}
```

`currentChanges[]` contains non-`PASS` rollout decisions, sorted by severity
(`BLOCK`, then `INCONCLUSIVE`, then `WATCH`) and bounded by `doctor --top`.

### DoctorSummary delivery additions (doctor --with-confighub --format json)

When bounded ConfigHub delivery evidence is requested, `doctor --format json`
includes two additive fields:

- `delivery`: a compact scope-level rollup intended for quick triage.
- `deliveryEvidence`: the raw bounded evidence envelope described in
  [GitOps Delivery Evidence Contract](#gitops-delivery-evidence-contract).

```json
{
  "delivery": {
    "scope": {
      "namespace": "prod",
      "space": "prod",
      "since": "24h",
      "staleAfter": "15m0s",
      "maxItems": 10
    },
    "liveStatus": {
      "total": 2,
      "delivery": {
        "pass": 1,
        "watch": 1,
        "block": 0,
        "inconclusive": 0
      },
      "applicationHealth": {
        "pass": 1,
        "watch": 0,
        "block": 1,
        "inconclusive": 0
      },
      "fresh": 2,
      "stale": 0,
      "unknownFreshness": 0
    },
    "eventConsumers": {
      "total": 1,
      "ready": 0,
      "notReady": 1
    },
    "recentReleases": 1,
    "recentUnitEvents": 1,
    "omissions": [
      {
        "layer": "confighub.releases",
        "reason": "trimmed release rows to maxItems=10"
      }
    ]
  },
  "deliveryEvidence": {
    "observedAt": "2026-09-10T12:00:00Z",
    "scope": {"space": "prod", "since": "24h"},
    "configHub": {
      "liveStatuses": [],
      "releases": [],
      "unitEvents": []
    }
  }
}
```

`delivery` and `deliveryEvidence` appear only with `--with-confighub`.
Concrete failed/stale delivery feedback, failed unit events, and unhealthy
observed event consumers may be promoted into `topIssues` using the same
severity ordering as other doctor issues. Missing connection, missing writeback,
RBAC/list failures, and incomplete ConfigHub evidence remain structured
omissions.

### Three-way resource rollout additions (compare three-way --format json)

Each `resources[]` entry may include `currentChange` for workload resources
when live rollout evidence is available:

```json
{
  "resources": [
    {
      "result": {"resource": "Deployment/api", "namespace": "prod"},
      "severity": "warning",
      "pattern": "rollout-pending",
      "currentChange": {
        "progress": {"phase": "rolling_out"},
        "verdict": "WATCH",
        "reason": "rollout_progressing",
        "evidence": {
          "kstatusStatus": "InProgress",
          "generation": 2,
          "observedGeneration": 2
        }
      }
    }
  ]
}
```

`currentChange` is omitted for non-workload resources and when rollout
evidence cannot be read. It does not change conformance exit-code semantics;
`compare three-way --fail-on` remains based on resource severity.

### Verified manager strings

The classifier matches against a verified enumeration of upstream field-manager strings — sources documented in `pkg/agent/manager_strings.go`. Strings not in the enumeration fall through to `unknown` rather than being guessed. Recognized sources include Argo CD, Flux (kustomize / helm / source controllers), Helm direct, Crossplane (composite / composed / claim / MRD / reference resolver), kro (applyset / applyset-parent / labeller), Sveltos (`application/apply-patch`), Modelplane via Crossplane composition managers, and `kubectl-*` interactive paths.

### Git source anchor (A2)

When `compare three-way` runs against a resource managed by Argo CD or Flux (including ConfigHub-delivered-via-GitOps), the resource-level git source anchor is collected via the existing tracers (`pkg/agent/argo_trace.go`, `pkg/agent/flux_trace.go`) and surfaced on both the live side and each field mismatch.

```json
{
  "live": {
    "gitSource": {
      "repoUrl": "https://github.com/org/platform-config",
      "revision": "abc123def456",
      "path": "apps/prod/payments"
    }
  },
  "mismatches": [
    {
      "field": "replicas",
      "dry": "3",
      "wet": "3",
      "live": "1",
      "cause": "manual-edit",
      "managerHint": "kubectl-edit",
      "gitSource": {
        "repoUrl": "https://github.com/org/platform-config",
        "revision": "abc123def456",
        "path": "apps/prod/payments"
      }
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `gitSource.repoUrl` | string | Source repository URL (`spec.url` for Flux GitRepository / `spec.source.repoURL` for Argo Application). |
| `gitSource.revision` | string | Observed commit SHA, tag, or OCI digest. |
| `gitSource.path` | string | Subdirectory within the repo (`spec.path` for Flux Kustomization / `spec.source.path` for Argo). |
| `gitSource.line` | int | Reserved for stage B (rendering-aware back-resolution); always `0` at A2. |

At A2 the anchor is resource-level — every field mismatch carries the same anchor. Stage B refines to per-field anchors with file path + line resolution when `--source-path <local-checkout>` is passed: cub-scout walks YAML manifests under `<local-checkout>/<gitSource.path>`, finds the document matching the resource's kind/name/namespace, and records the line where the field's canonical path (e.g., `.spec.replicas`) is set.

```json
"gitSource": {
  "repoUrl": "https://github.com/org/platform-config",
  "revision": "abc123def456",
  "path": "apps/prod/payments",
  "file": "deployment.yaml",
  "line": 9
}
```

Stage B handles **raw YAML** manifests only — Helm/Kustomize template back-resolution requires rendering-aware mapping that is out of scope. For Helm/Kustomize sources, `file` and `line` remain empty while the resource-level anchor (`repoUrl`, `revision`, `path`) is still populated.

Best-effort: omitted when no GitOps owner is detected, when the tracer CLI is unavailable, when no `--source-path` is provided, or when the chain root carries no useful data.

Source: `pkg/agent/manager_strings.go`, `pkg/agent/field_ownership.go`, `pkg/agent/git_source_anchor.go`

### Incoming bindings (C1)

When `compare three-way` (or `compare` per-resource) runs in connected mode against a resource whose live unit slug is known, cub-scout queries `cub link list` for incoming Links (where `FromUnitID = <this-unit-id>`) and surfaces them under `incomingBindings`. Each entry describes one ConfigHub Link influencing this unit's config data — the variant-management directed graph as read-only evidence.

```json
{
  "incomingBindings": [
    {
      "linkId": "01HFK...A1",
      "slug": "image-from-app",
      "displayName": "Image From App",
      "updateType": "NeedsProvides",
      "toUnitId": "01HFK...XY",
      "toSpaceId": "01HFK...SP",
      "autoUpdate": true,
      "whereResource": "kind=Deployment",
      "bindingsCount": 3
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `linkId` | string | Unique identifier for the Link entity. |
| `slug` | string | URL-safe identifier (useful for `cub link get <slug>` follow-up). |
| `displayName` | string | Human-friendly name when set. |
| `updateType` | string | Operation kind: `NeedsProvides`, `UpgradeUnits`, `MergeUnits`, `Insert`, `Upsert` (and future `Transform`). |
| `toUnitId` | string | Upstream (producer) unit ID. |
| `toSpaceId` | string | Upstream unit's space ID. |
| `autoUpdate` | bool | True when the link auto-propagates upstream changes downstream. |
| `whereResource` | string | Filter selecting which upstream resources are eligible for propagation. |
| `bindingsCount` | int | Number of explicit binding expressions on the link. The per-field expansion of `Bindings` is C2. |

Best-effort: omitted when no live unit is known, when `cub link list` fails, or when the result is empty.

### Field-level binding source (C2)

When an incoming binding's `downstreamPath` matches a `compareFieldMismatch`'s canonical path, the mismatch is annotated with a `bindingSource` pointing at the specific Link + binding that supplies the value. This answers the variant-management punch-line question: "this field's value came from upstream unit X at path Y via link Z."

```json
{
  "mismatches": [
    {
      "field": "replicas",
      "dry": "3",
      "wet": "3",
      "live": "1",
      "cause": "manual-edit",
      "managerHint": "kubectl-edit",
      "bindingSource": {
        "linkId": "01HFK...A1",
        "linkSlug": "replicas-from-scale",
        "upstreamUnitId": "01HFK...XY",
        "upstreamPath": ".spec.scale.value",
        "transformExpr": "to_int"
      }
    }
  ],
  "incomingBindings": [
    {
      "linkId": "01HFK...A1",
      "slug": "replicas-from-scale",
      "updateType": "NeedsProvides",
      "toUnitId": "01HFK...XY",
      "bindingsCount": 1,
      "bindings": [
        {"downstreamPath": ".spec.replicas", "upstreamPath": ".spec.scale.value", "transformExpr": "to_int"}
      ]
    }
  ]
}
```

| Field | Type | Description |
|-------|------|-------------|
| `bindingSource.linkId` | string | Link entity ID. |
| `bindingSource.linkSlug` | string | Link slug (useful for `cub link get`). |
| `bindingSource.upstreamUnitId` | string | Producer unit ID supplying the value. |
| `bindingSource.upstreamPath` | string | Canonical path within the upstream unit. |
| `bindingSource.transformExpr` | string | Optional transform expression (Go template, CEL, etc.). |

In ASCII output, the binding source renders as an indented annotation under the field's diff line:

```
Diff Highlights
  - replicas: DRY=3 | WET=3 | LIVE=1
      <- bound from unit:01HFK...XY path:.spec.scale.value via link:replicas-from-scale
```

Best-effort: omitted when the field name has no canonical-path mapping (e.g., `images` which spans containers) or when no incoming binding's `downstreamPath` matches.

`incomingBindings[].bindings` carries the same per-field expansion at the resource level — `BindingsCount` (C1) remains the raw count even when expansion produces an empty list (unrecognized binding JSON shape).

Source: `cmd/cub-scout/compare_bindings.go`

## GitOps Controller Coverage Contract

`gitops status --format json` includes additive `controllerCoverage[]` records
when the command builds status from a Kubernetes client. This tells callers what
cub-scout actually inspected, what it observed, and which reads were omitted
instead of silently claiming absence.

### Schema Sketch

```json
{
  "controllerCoverage": [
    {
      "family": "Flux",
      "status": "found",
      "resourceKinds": ["HelmRelease", "Kustomization"],
      "foundKinds": ["Kustomization"],
      "found": 1
    },
    {
      "family": "Modelplane",
      "status": "unreadable",
      "resourceKinds": ["ModelDeployment"],
      "found": 0,
      "omissions": [
        {
          "resource": "modeldeployments.modelplane.ai/v1alpha1",
          "reason": "forbidden",
          "message": "modeldeployments.modelplane.ai is forbidden"
        }
      ]
    }
  ]
}
```

### Field Rules

| Field | Rule |
|---|---|
| `family` | Stable controller-family label. Current values include `Flux`, `ArgoCD`, `ConfigHub`, `Sveltos`, and `Modelplane`. |
| `status` | One of `found`, `not_found`, `partial`, or `unreadable`. |
| `resourceKinds[]` | Kinds cub-scout knows to check for that family in this release. |
| `foundKinds[]` | Kinds observed at least once. Empty does not mean unsupported; use `resourceKinds[]` for support scope. |
| `found` | Count of observed controller-family objects across checked kinds. |
| `omissions[]` | Safe omission details for list calls that could not be completed. Missing CRDs are treated as `not_found`, not omissions. |
| `omissions[].resource` | API resource identifier in `<resource>.<group>/<version>` form when a group exists. |
| `omissions[].reason` | One of `forbidden`, `unauthorized`, `timeout`, or `list_failed`. |
| `omissions[].message` | Raw Kubernetes/client error message when available; callers should not parse it for decisions. |

## Observation Evidence Contract

`observation` describes where cub-scout read an observed fact from and when the
read happened. It is a freshness boundary for the evidence, not a promise that
the fact remains true afterward.

Current embedding points:

- `snapshot`: top-level `observation`
- `summary list --format json`: top-level query `observation` plus per-entry
  stored-record `observation`
- `map list --format json`: per-entry `observation` on live Kubernetes reads
- `watch` / `bot` events: top-level event `observation`

### Schema Sketch

```json
{
  "source": "kubernetes-api",
  "mode": "watch-poll",
  "observedAt": "2026-09-10T14:15:00Z",
  "freshness": "point-in-time",
  "scope": {
    "cluster": "kind-dev",
    "namespace": "prod",
    "kind": "Deployment"
  }
}
```

### Field Rules

| Field | Rule |
|---|---|
| `source` | Read source. Current live-cluster value: `kubernetes-api`; connected summary storage uses `summary-store`. Future ConfigHub/resource-index producers must use a different explicit value. |
| `mode` | Producing surface. Current values: `map-list`, `snapshot`, `summary-list`, `watch-poll`. `bot` uses `watch-poll` because it runs the watch engine. |
| `observedAt` | RFC 3339 timestamp captured once for the relevant read or poll and normalized to UTC. |
| `freshness` | Current value: `point-in-time`. Consumers must not treat this as a TTL or cache-validity guarantee. |
| `scope.cluster` | Cluster/context label when cub-scout can name it. |
| `scope.namespace` | Namespace filter or resource namespace when present. |
| `scope.kind` | Kind filter or resource kind when present. |

For `summary list --format json`, top-level `observation` describes the local
summary-store query. Each entry's `observation.observedAt` is the persisted
summary timestamp, so callers can reuse the record without treating the query as
a fresh Kubernetes API read.

The field is omitted when a fixture or future producer cannot state the source
and observation timestamp. Omission means "freshness not declared", not "stale"
or "untrusted".

## Platform Substrate Evidence Contract

Some higher-level platforms deliberately use another controller family as their
implementation substrate. cub-scout keeps the higher-level owner when that owner
has stronger explicit signals, but it may also emit substrate evidence so
reviewers can see the lower layer without confusing it for ownership.

Current producer: Modelplane resources backed by Crossplane composition labels,
annotations, or verified Crossplane field-manager strings.

Current embedding points:

- `map list --format json`: `ownerEvidence`
- `watch` / `bot` events: `owner.evidence`
- `receipt verify` and watch/bot inline receipts: `predicate.evidence.platformSubstrate`

### Schema Sketch

```json
{
  "platform": "modelplane",
  "substrate": "crossplane",
  "composite": "x-qwen",
  "claim": {
    "name": "qwen-claim",
    "namespace": "models"
  },
  "compositionResource": "engine",
  "fieldManager": "apiextensions.crossplane.io/composed-qwen",
  "sources": [
    "label:crossplane.io/composite",
    "label:crossplane.io/claim-name",
    "label:crossplane.io/claim-namespace",
    "annotation:crossplane.io/composition-resource-name",
    "managedFields:apiextensions.crossplane.io/composed-qwen"
  ]
}
```

### Field Rules

| Field | Rule |
|---|---|
| `platform` | Stable higher-level platform owner. Current value: `modelplane`. |
| `substrate` | Stable implementation substrate. Current value: `crossplane`. |
| `composite` | From `crossplane.io/composite` or `apiextensions.crossplane.io/composite` labels. |
| `claim.name` | From `crossplane.io/claim-name`. |
| `claim.namespace` | From `crossplane.io/claim-namespace` when present. |
| `compositionResource` | From `crossplane.io/composition-resource-name` label or annotation, preferring the label when both exist. |
| `fieldManager` | Deterministic first verified Crossplane field manager accepted for `OwnerModelplane`; unknown Crossplane-looking manager strings are ignored. |
| `sources[]` | Exact metadata paths used to populate the evidence. Empty evidence is omitted instead of emitting an empty object. |

This evidence is emitted only after Modelplane ownership has already been proven
from Modelplane API group, explicit Modelplane label, or Modelplane ownerRef
signals. Crossplane labels alone do not make a resource Modelplane-owned.

## GitOps Delivery Evidence Contract

When `gitops status --with-confighub --format json`,
`doctor --with-confighub --format json`,
`trace --with-confighub --format json`,
`explain --with-confighub --format json`,
`map activity --with-confighub --format json`, or single-resource
`receipt verify --with-confighub --format json` is used, the output may include
ConfigHub delivery evidence. `gitops status` and `doctor` expose the shared
bounded `deliveryEvidence` envelope; `trace`, `explain`, and `receipt verify`
embed a resource-scoped correlated form; `map activity` projects that same
evidence into timeline rows. This evidence does not replace Argo, Flux,
Sveltos, Modelplane, ConfigHub, or Kubernetes as the status authority.

### Schema Sketch

```json
{
  "deliveryEvidence": {
    "observedAt": "2026-09-10T12:00:00Z",
    "scope": {
      "namespace": "prod",
      "space": "prod",
      "since": "24h",
      "staleAfter": "15m0s",
      "maxItems": 10
    },
    "configHub": {
      "liveStatuses": [
        {
          "space": "prod",
          "spaceId": "sp-123",
          "source": "argobot",
          "app": "prod",
          "syncStatus": "Synced",
          "healthStatus": "Healthy",
          "operationPhase": "Succeeded",
          "revision": "sha256:abc",
          "observedAt": "2026-09-10T11:55:00Z",
          "freshness": "fresh",
          "freshnessSeconds": 300,
          "deliveryVerdict": "PASS",
          "applicationHealthVerdict": "PASS"
        }
      ],
      "releases": [
        {
          "slug": "release-42",
          "releaseId": "r-42",
          "space": "prod",
          "target": "cluster-prod",
          "digest": "sha256:abc",
          "createdAt": "2026-09-10T11:50:00Z"
        }
      ],
      "unitEvents": [
        {
          "eventId": "ue-1",
          "action": "ReleasePublished",
          "result": "Succeeded",
          "unit": "api",
          "space": "prod",
          "target": "cluster-prod",
          "createdAt": "2026-09-10T11:51:00Z"
        }
      ]
    },
    "eventConsumers": [
      {
        "kind": "Deployment",
        "namespace": "argobot",
        "name": "argobot",
        "ready": true,
        "replicas": 1,
        "readyReplicas": 1,
        "evidenceLabel": "app=argobot"
      }
    ],
    "omissions": [
      {
        "layer": "confighub.liveStatus",
        "reason": "space \"prod\" has no confighub.com/live-status annotation",
        "impact": "no event-consumer status writeback is available for this space"
      }
    ]
  }
}
```

### Field Rules

| Field | Rule |
|---|---|
| `scope.space` | Defaults to the current cub space when available; `*` is accepted only when explicitly supplied. |
| `scope.since` | Bounds release and unit-event reads by `CreatedAt > <cutoff>`. |
| `scope.maxItems` | Caps rows kept in output after the time-window query. |
| `configHub.liveStatuses[]` | Parsed from the `confighub.com/live-status` Space annotation. Missing or malformed annotations become omissions. |
| `liveStatuses[].freshness` | `fresh`, `stale`, or `unknown`, based on `observedAt` and `--confighub-stale-after`. |
| `liveStatuses[].deliveryVerdict` | Delivery-facing verdict from sync/operation state. Stale `PASS` downgrades to `WATCH`. |
| `liveStatuses[].applicationHealthVerdict` | Application-health verdict from reported health. Stale `PASS` downgrades to `WATCH`. |
| `eventConsumers[]` | Conservative, label-selected Kubernetes Deployment detection by `app=argobot` or `app.kubernetes.io/name=argobot`. cub-scout searches all namespaces when allowed and records an omission if RBAC forces namespace fallback. Absence is an omission, not proof no consumer exists. |
| `omissions[]` | Structured explanation for unavailable, missing, stale, malformed, disconnected, or RBAC-blocked evidence. |

The command never consumes ConfigHub event cursors and never mutates ConfigHub,
the event consumer, the delivery controller, or Kubernetes.

### Map activity delivery rows

`map activity --with-confighub --format json` keeps the normal activity payload
shape. ConfigHub-owned rows carry row-specific delivery evidence, and matching
`argocd.application` rows may carry additive live-status evidence when the
ConfigHub space is non-wildcard, an observed Application Space ID matches the
reported Space ID, and the Application name matches exactly and is unique.
Missing/conflicting Space ID metadata or ambiguous names/statuses produce
`deliveryEvidence.kind=omission` with `layer=confighub.correlation`.
Space IDs come from `confighub.com/space-id` or `confighub.com/SpaceID`
labels/annotations on the Application. An unjoined ConfigHub row looks like:

```json
{
  "activity": [
    {
      "time": "2026-09-10T11:59:00Z",
      "source": "confighub.liveStatus",
      "resource": "ConfigHubLiveStatus/prod/api",
      "action": "delivery-status",
      "result": "pending",
      "owner": "ConfigHub",
      "message": "app=api sync=OutOfSync health=Healthy op=Running delivery=WATCH app-health=PASS freshness=fresh revision=sha256:abc",
      "deliveryEvidence": {
        "kind": "liveStatus",
        "namespace": "prod",
        "space": "prod",
        "spaceId": "sp-123",
        "app": "api",
        "syncStatus": "OutOfSync",
        "healthStatus": "Healthy",
        "operationPhase": "Running",
        "freshness": "fresh",
        "deliveryVerdict": "WATCH",
        "applicationHealthVerdict": "PASS"
      }
    }
  ],
  "count": 1
}
```

ConfigHub activity row `source` values:

| Source | `deliveryEvidence.kind` | Meaning |
|---|---|---|
| `confighub.liveStatus` | `liveStatus` | Latest delivery/application-health writeback observed on a ConfigHub Space annotation. |
| `confighub.release` | `release` | Recent ConfigHub release publication row within the bounded lookback window. |
| `confighub.unitEvent` | `unitEvent` | Recent ConfigHub unit event row within the bounded lookback window. |
| `confighub.eventConsumer` | `eventConsumer` | Label-selected in-cluster event-consumer Deployment health. |
| `confighub.omission` | `omission` | Missing, inaccessible, malformed, stale, disconnected, or trimmed evidence surfaced as a timeline fact. |

Field rules:

| Field | Rule |
|---|---|
| `deliveryEvidence.kind` | One of `liveStatus`, `release`, `unitEvent`, `eventConsumer`, or `omission`. |
| `deliveryEvidence.namespace` | The explicit Kubernetes namespace scope used for the collection, when supplied. It is not inferred from ConfigHub space names. |
| `deliveryEvidence.matchedBy[]` | Optional exact join keys. Argo Application joins include `scope.space`, `argocdApplication.spaceId`, and `argocdApplication.name`. Absent on standalone ConfigHub status rows. |
| `result` | Timeline rendering bucket: `success`, `pending`, `failed`, `inconclusive`, or `normal`, derived from the row's observed status/verdict only. |
| `owner` | `ConfigHub` for these rows so `--owner ConfigHub` can select them. |
| `source` | Stable row source; consumers should dispatch on `source` or `deliveryEvidence.kind`, not parse `message`. |

Namespace filtering remains conservative: ConfigHub rows match
`--namespace <ns>` only when the row carries that explicit namespace scope.
On joined `argocd.application` rows, `deliveryEvidence` is supporting context;
the Argo row's `result` remains controller-owned. The command never consumes
ConfigHub event cursors.

## Resource Delivery Evidence Contract

When `trace --with-confighub --format json`,
`explain --with-confighub --format json`, or single-resource
`receipt verify --with-confighub --format json` is used, output may include an
additive `deliveryEvidence` object. Unlike `gitops status --with-confighub`,
this object is correlated to one resource and only includes release,
unit-event, or live-status rows when exact identifiers prove the join. A
non-wildcard `--confighub-space` may supply the exact space for live-status
application matching; wildcard `*` is never treated as exact object-level
space correlation.

### Schema Sketch

```json
{
  "deliveryEvidence": {
    "source": "confighub",
    "observedAt": "2026-09-10T12:00:00Z",
    "scope": {
      "namespace": "prod",
      "space": "payments-prod",
      "since": "24h",
      "staleAfter": "15m0s",
      "maxItems": 10
    },
    "correlation": {
      "unitSlug": "payments-api",
      "unitId": "u-123",
      "space": "payments-prod",
      "spaceId": "sp-123",
      "target": "prod",
      "targetId": "t-123",
      "application": "payments-app",
      "matchedBy": [
        "confighub.unitSlug",
        "chain.application",
        "chain.configHubOCI.target"
      ]
    },
    "liveStatus": {
      "app": "payments-app",
      "syncStatus": "Synced",
      "healthStatus": "Healthy",
      "operationPhase": "Succeeded",
      "freshness": "fresh",
      "deliveryVerdict": "PASS",
      "applicationHealthVerdict": "PASS",
      "matchedBy": [
        "spaceId",
        "liveStatus.app==chain.application"
      ]
    },
    "releases": [
      {
        "slug": "release-42",
        "space": "payments-prod",
        "target": "prod",
        "digest": "sha256:abc",
        "createdAt": "2026-09-10T11:50:00Z",
        "matchedBy": ["spaceId", "targetId"]
      }
    ],
    "unitEvents": [
      {
        "eventId": "ue-1",
        "action": "ReleasePublished",
        "result": "Succeeded",
        "unit": "payments-api",
        "unitId": "u-123",
        "createdAt": "2026-09-10T11:51:00Z",
        "matchedBy": ["unitEvent.unitId==confighub.unitId"]
      }
    ],
    "eventConsumers": [
      {
        "kind": "Deployment",
        "namespace": "confighub-ops",
        "name": "argobot",
        "ready": true,
        "replicas": 1,
        "readyReplicas": 1,
        "evidenceLabel": "app=argobot"
      }
    ],
    "omissions": [
      {
        "layer": "confighub.releases",
        "reason": "no release row matched the traced resource by exact space plus target",
        "impact": "recent release publishing evidence is omitted for this object"
      }
    ]
  }
}
```

### Field Rules

| Field | Rule |
|---|---|
| `source` | Current value is `confighub`. |
| `scope.space` | Defaults to the resource's ConfigHub space or ConfigHub OCI source space. If neither exists, connected history reads are skipped unless `--confighub-space` is supplied. |
| `correlation.matchedBy[]` | Lists the exact identifiers used to form the resource correlation. |
| `liveStatus` | Included only when a live-status row matches by exact space plus Argo Application name or exact space plus ConfigHub unit slug. |
| `liveStatus.matchedBy[]` | Lists the exact live-status join keys, for example `spaceId` and `liveStatus.app==chain.application`. |
| `releases[]` | Included only for rows matching exact space plus target ID or target slug. A space-only match is not object-level evidence. |
| `unitEvents[]` | Included only for rows matching exact unit ID, or exact unit slug plus space. |
| `eventConsumers[]` | Cluster-observed event-consumer Deployment health. This is contextual evidence, not proof that the traced object was synced. |
| `omissions[]` | Structured explanation for missing identity, missing scope, missing/malformed writeback, disconnected ConfigHub, non-matching rows, or RBAC/list failures. |

The command never consumes ConfigHub event cursors and never mutates ConfigHub,
the event consumer, the delivery controller, or Kubernetes.

## MCP Structured Content Contract

When MCP tools return JSON-backed data, the gateway keeps the raw JSON string in `content[0].text`.
For the highest-value connected/read-only surfaces, it may also add `structuredContent`.

### Current Additive MCP Rule

- `compare_three_way` returns parsed CLI JSON under `structuredContent.data`
- `compare_three_way` may mirror `confighubUrl`, `confighubRevisionsUrl`, and `nextSteps` from the CLI JSON at the top level of `structuredContent`
- `compare_source_truth` returns parsed CLI JSON under `structuredContent.data`; its `strategy` enum matches the CLI strategy registry.
- `confighub_units`, `confighub_unit_get`, and `confighub_changesets` may add:
  - `structuredContent.data`
  - `structuredContent.nextSteps`
  - `structuredContent.confighubUrl` when an exact unit detail URL is known
  - `structuredContent.confighubRevisionsUrl` when an exact unit revisions URL is known
- `confighub_live_status` may add:
  - `structuredContent.data`
  - `structuredContent.liveStatuses[]`
  - `structuredContent.omissions[]`
- `confighub_k8s_resources`, `confighub_k8s_types`, and `confighub_resources` return parsed ConfigHub Resource-backed JSON under `structuredContent.data`; these are ConfigHub-stored intended-configuration reads, not live-cluster reads.
- `confighub_releases` and `confighub_unit_events` return parsed ConfigHub JSON under `structuredContent.data`

Connected ConfigHub history/status MCP tools require an explicit `space`
argument. Passing `*` is supported only as an explicit all-spaces request.
The ConfigHub Kubernetes Resource MCP tools require an explicit `space` or
`target` scope; passing `space: "*"` is likewise an explicit all-spaces request.
The generic Resource entity MCP tool requires an explicit `space`; passing
`space: "*"` is likewise an explicit all-spaces request.

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
    "confighubUrl": "https://confighub.com/units/sp-123/u-123",
    "confighubRevisionsUrl": "https://confighub.com/units/sp-123/u-123?tab=2",
    "nextSteps": [
      {
        "actionType": "human-decision",
        "reason": "Agreement is proven for this scope; open the governed unit to review the audit trail before sign-off.",
        "nextSurface": "https://confighub.com/units/sp-123/u-123"
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
      },
      {
        "type": "Normal",
        "reason": "WebAction",
        "message": "operator@example.com requested restart for Deployment/prod/api",
        "count": 1,
        "age": "1m",
        "severity": "info",
        "source": "controller-web",
        "lastSeen": "2026-07-09T10:05:00Z",
        "action": {
          "action": "restart",
          "actor": "operator@example.com",
          "groups": ["platform", "oncall"],
          "subject": "Deployment/prod/api",
          "raw": {
            "event.toolkit.fluxcd.io/change-token": "chg-123"
          }
        }
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
| `action` | object | Optional audited action metadata when explicitly present on the Kubernetes Event |
| `action.action` | string | Action name from event annotations |
| `action.actor` | string | Actor/username from event annotations |
| `action.groups[]` | string[] | Actor groups from event annotations |
| `action.subject` | string | Target subject from event annotations |
| `action.raw` | object | Unknown action annotations preserved as raw evidence |

Action metadata is omitted for ordinary events. Missing actor, group, or subject
fields are omitted rather than guessed.

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

## Receipt Contract (`cub-scout receipt verify`)

> A **receipt** is a stamped, hand-offable record of one verification: an in-toto Statement v1 envelope around a verdict, evidence, omissions, and optional upstream receipt references. Its **proof** is the verifiable integrity property created by the fingerprint: SHA-256 over RFC 8785 canonical JSON of the full Statement, with only `predicate.fingerprint` removed before hashing. Any third party can recompute that fingerprint to confirm the receipt has not been edited since it was stamped. That is tamper-evidence, not producer authentication or formal proof of truth. A receipt without proof is a claim; a receipt with proof is evidence.

For the *vocabulary* — what "receipt" and "proof" mean in cub-scout, and how they relate to log / journal / record / ledger / provenance — see [docs/concepts/receipts-and-proofs.md](../concepts/receipts-and-proofs.md). This section documents the *wire format* of the artifact itself.

The receipt surface emits typed, fingerprinted, immutable evidence artifacts
wrapping cub-scout's existing field-level evidence (compareThreeWay,
attribution, sourceTruth, gitSource, and optional deliveryEvidence) into a
verifiable record. Current shipping releases use fingerprint-only integrity
(SHA-256 over RFC 8785 canonical JSON of the full in-toto Statement v1 envelope
minus only `predicate.fingerprint`). Cryptographic signing (e.g., DSSE wrapped
in a Sigstore Bundle, or a comparable scheme) is a future hardening direction —
purely additive to the wire format, no envelope change required.

Receipts are **historical, immutable records** of past events. Updates produce
new receipts, never mutate old ones. cub-scout never mutates the cluster or
ConfigHub.

### Wire Format

The wire format is the **in-toto Statement v1 envelope** (`_type =
"https://in-toto.io/Statement/v1"`) wrapping the cub-scout predicate URI
`https://cub-scout.dev/receipt/v1`.

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [
    {
      "name": "k8s-live://apps/v1/Deployment/prod/api",
      "digest": { "sha256": "..." }
    }
  ],
  "predicateType": "https://cub-scout.dev/receipt/v1",
  "predicate": {
    "version": "v1",
    "claim": "applied matches spec at apps/prod/api",
    "scope": { "kind": "Deployment", "name": "api", "namespace": "prod" },
    "verifier": { "tool": "cub-scout", "version": "vX.Y.Z" },
    "verifiedAt": "2026-05-21T10:30:00Z",
    "predicateName": "applied-matches-spec",
    "spec": {
      "anchor": {
        "type": "git",
        "repoUrl": "https://github.com/org/repo",
        "revision": "abc123",
        "path": "apps/prod/api"
      }
    },
    "verdict": "PASS",
    "evidence": {
      "attribution": { "...": "..." },
      "gitSource": { "...": "..." },
      "deliveryEvidence": { "...": "optional with --with-confighub" }
    },
    "omissions": [],
    "inputAttestations": [],
    "nextSteps": [
      {
        "actionType": "read-only",
        "reason": "controller is reconciling; spec anchor matches the resolved git source",
        "nextCommand": "cub-scout explain",
        "nextSurface": "cub-scout"
      }
    ],
    "fingerprint": "sha256:..."
  }
}
```

### Subjects

| Scheme | When Emitted | Digest Body |
|--------|--------------|-------------|
| `k8s-live://<apiVersion>/<kind>/<namespace>/<name>` | Always | SHA-256 over canonical JSON of the live object with dynamic fields pruned (`status`, `metadata.managedFields`, `metadata.resourceVersion`, `metadata.generation`, `metadata.uid`, `metadata.creationTimestamp`) |
| `confighub-unit://<slug>@rev=<n>` | Connected mode + ConfigHub-linked resource | SHA-256 over the unit canonical body returned by ConfigHub |
| `rendered-object-set://sha256/<id>` | `object-set-matches` and `object-set-diff` receipts | SHA-256 over the canonical desired rendered object set after dynamic fields are pruned |
| `k8s-live-object-set://namespace/<ns>` or `k8s-live-object-set://cluster` | `object-set-matches` and `object-set-diff` receipts | SHA-256 over the canonical live projection for the desired object identities |

Standalone-mode single-resource receipts emit only the `k8s-live://`
subject and record an `OmissionConfigHubUnitSubject` entry in
`predicate.omissions`. Connected mode with no ConfigHub linkage records
the same omission with a different reason string. Object-set receipts use
the rendered/live object-set subject pair instead.

### Verdicts

| Verdict | Meaning |
|---------|---------|
| `PASS` | Evidence supports the claim |
| `WATCH` | Evidence is ambiguous; situation needs monitoring |
| `BLOCK` | Evidence contradicts the claim |
| `INCONCLUSIVE` | Evidence is missing or unavailable; always carries one or more `omissions[]` entries explaining what's missing |

### v1 Predicates

| Predicate | Required signals | Verdict Logic |
|-----------|------------------|---------------|
| `applied-matches-spec` | Controller-resolved `gitSource` on the resource | `Spec missing` or `GitSource missing` → INCONCLUSIVE + `OmissionGitSourceAnchor`. `Anchor mismatch (repoUrl/revision/path)` → BLOCK. `Cause = manual-edit` → BLOCK. `Cause = controller-drift` → PASS. `Cause = unknown` or unrecognized → INCONCLUSIVE + `OmissionManagedFields`. |
| `source-truth-pass` | `--strategy` (one of nine) + connected-mode ConfigHub auth | `--strategy` empty → INCONCLUSIVE + `OmissionStrategyMissing`. No source-truth evidence body → INCONCLUSIVE + `OmissionSourceTruthEvidence`. Strategy mismatch between caller and evidence → BLOCK + `OmissionStrategyMismatch`. Otherwise: Status PASS → PASS, WATCH → WATCH, BLOCK → BLOCK, **ASK → WATCH** (per the locked synthesis; receipt-level INCONCLUSIVE is reserved for receipts that themselves can't be built — not for cases where the underlying source-truth derivation just couldn't classify). Source-truth `proof_gaps[]` are mirrored into `omissions[]` under `source-truth-complete` regardless of verdict. |
| `no-manual-edits-since` | `--since <RFC3339>` cutoff | `--since` zero → INCONCLUSIVE + `OmissionSinceMissing`. Live nil or no managedFields → INCONCLUSIVE + `OmissionManagedFields`. Any interactive (`kubectl-*`) manager with `Time > since` → BLOCK. Any interactive manager with nil `Time` → INCONCLUSIVE + `OmissionManagedFieldsTime`. Otherwise → PASS. |
| `object-set-matches` | `--file <manifest.yaml\|dir>` + live cluster access | PASS when every desired object identity is present live and every authored field still matches. BLOCK when any desired object is missing or any authored field differs. INCONCLUSIVE when an API mapping or live read could not be checked. Kubernetes server-added map fields and `status` are outside the claim. |
| `object-set-diff` | `compare object-set --dry-from <manifest.yaml\|dir>` + live cluster access | PASS when no authored-field, added-object, or removed-object deltas are present. WATCH when only closure deltas are present. BLOCK when any shared object has authored-field deltas. INCONCLUSIVE is reserved for load/build errors before receipt construction. |
| `workloads-converged` | `--file <manifest.yaml\|dir>` + live cluster access | PASS when every desired workload is present and kstatus reports it current. WATCH when any workload is still progressing, including stale generation status (`status.observedGeneration < metadata.generation`). BLOCK when a workload is missing, has terminal pod/container failure evidence, or has made no current-generation progress beyond `--grace-window`. INCONCLUSIVE when an API mapping or live read could not be checked. |
| `prerequisites-met` | `--prerequisites <yaml\|json>` + live cluster access | PASS when every declared fact is present. BLOCK when any declared fact is missing. INCONCLUSIVE when any fact could not be checked. |

#### `deliveryEvidence` supporting evidence

Single-resource `receipt verify <kind>/<name> --with-confighub` may attach
object-correlated delivery evidence under
`predicate.evidence.deliveryEvidence`:

```json
{
  "source": "confighub",
  "observedAt": "2026-09-10T12:00:00Z",
  "scope": {
    "namespace": "prod",
    "space": "payments-prod",
    "since": "24h",
    "staleAfter": "15m0s",
    "maxItems": 10
  },
  "correlation": {
    "application": "payments-api",
    "space": "payments-prod",
    "matchedBy": ["scope.space", "chain.application"]
  },
  "liveStatus": {
    "source": "argobot",
    "app": "payments-api",
    "syncStatus": "Synced",
    "healthStatus": "Healthy",
    "operationPhase": "Succeeded",
    "freshness": "fresh",
    "deliveryVerdict": "PASS",
    "applicationHealthVerdict": "PASS",
    "matchedBy": ["space", "liveStatus.app==chain.application"]
  },
  "releases": [],
  "unitEvents": [],
  "eventConsumers": [],
  "omissions": []
}
```

The shape is the resource-scoped `deliveryEvidence` model described in
[GitOps Delivery Evidence Contract](#gitops-delivery-evidence-contract).
Because it lives under `predicate.evidence`, it is covered by the receipt
fingerprint. The selected receipt predicate still owns the receipt-level
`predicate.verdict`; delivery evidence is additive supporting evidence and does
not make cub-scout the authority for delivery status or application health.

In this release, `--with-confighub` is supported only for single-resource
receipts. Aggregate, object-set, workload-convergence, and prerequisites
receipts reject the flag upfront rather than silently omitting the field.

#### `platformSubstrate` supporting evidence

Modelplane-owned resources with explicit Crossplane substrate signals may carry
the platform/substrate evidence under
`predicate.evidence.platformSubstrate`:

```json
{
  "platform": "modelplane",
  "substrate": "crossplane",
  "composite": "x-qwen",
  "claim": {
    "name": "qwen-claim",
    "namespace": "models"
  },
  "compositionResource": "engine",
  "fieldManager": "apiextensions.crossplane.io/composed-qwen",
  "sources": [
    "label:crossplane.io/composite",
    "label:crossplane.io/claim-name",
    "label:crossplane.io/claim-namespace",
    "annotation:crossplane.io/composition-resource-name",
    "managedFields:apiextensions.crossplane.io/composed-qwen"
  ]
}
```

Because it lives under `predicate.evidence`, it is covered by the receipt
fingerprint. It is substrate context only: `predicate.owner.type` remains
`modelplane`, and Crossplane evidence does not override ownership.

#### `object-set-matches` evidence

`object-set-matches` records its details under
`predicate.evidence.objectSet`:

```json
{
  "desiredSource": {
    "type": "directory",
    "ref": "out/manifests",
    "digest": "sha256-of-input-files",
    "objectCount": 14
  },
  "scope": {"kind": "namespace", "namespace": "redis"},
  "matchMode": "authored-fields",
  "desiredDigest": "sha256-of-normalized-rendered-set",
  "liveDigest": "sha256-of-live-projection",
  "summary": {
    "desired": 14,
    "matched": 14,
    "missing": 0,
    "mismatched": 0,
    "inconclusive": 0
  },
  "objects": [
    {
      "id": {
        "apiVersion": "apps/v1",
        "kind": "StatefulSet",
        "namespace": "redis",
        "name": "redis-master"
      },
      "status": "matched",
      "desiredDigest": "sha256...",
      "liveDigest": "sha256..."
    }
  ]
}
```

`matchMode: authored-fields` means cub-scout projects live objects onto
the desired manifest shape before comparing. Every field present in the
rendered YAML must match. Server-added map fields are ignored, but list
length changes are considered material. This catches missing objects,
changed images, changed replicas, changed RBAC rules, changed Service
ports, injected list items, and similar install drift without failing on
normal Kubernetes defaulting.

#### `object-set-diff` evidence

`object-set-diff` records its details under
`predicate.evidence.objectSetDiff`:

```json
{
  "desiredSource": {
    "type": "directory",
    "ref": "out/manifests",
    "digest": "sha256-of-input-files",
    "objectCount": 14
  },
  "scope": {"kind": "namespace", "namespace": "redis"},
  "desiredDigest": "sha256-of-normalized-rendered-set",
  "liveDigest": "sha256-of-live-projection",
  "changedObjects": [
    {
      "id": {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "namespace": "redis",
        "name": "redis-master"
      },
      "desiredDigest": "sha256...",
      "liveDigest": "sha256...",
      "differences": [
        {
          "field": ".spec.template.spec.containers[0].image",
          "desired": "redis:7.2.4",
          "live": "redis:7.2.3",
          "cause": "controller-drift",
          "managerHint": "helm-controller"
        }
      ]
    }
  ],
  "addedObjects": [],
  "removedObjects": [],
  "summary": {"total": 14, "changed": 1, "added": 0, "removed": 0},
  "normalizationProfile": "k8s-zero-defaults/v1"
}
```

`changedObjects[]`, `addedObjects[]`, and `removedObjects[]` are sorted
deterministically before fingerprinting. `addedObjects[]` / `removedObjects[]`
are populated only when closed-world diffing is requested. Otherwise the receipt
records an `object-set-diff-closure` omission and makes no claim about objects
outside the desired identity set. Field-level `cause` and `managerHint` are
optional; omitted values mean cub-scout could not attribute that field delta
from live managedFields.

#### `workloads-converged` evidence

`workloads-converged` records its details under
`predicate.evidence.workloads`:

```json
{
  "desiredSource": {
    "type": "directory",
    "ref": "out/manifests",
    "digest": "sha256-of-input-files",
    "objectCount": 2
  },
  "scope": {"kind": "namespace", "namespace": "redis"},
  "graceWindow": "5m0s",
  "observedAt": "2026-06-24T10:30:00Z",
  "desiredDigest": "sha256-of-normalized-rendered-workloads",
  "liveDigest": "sha256-of-live-workload-status-projection",
  "summary": {
    "desired": 2,
    "converged": 1,
    "progressing": 1,
    "failed": 0,
    "missing": 0,
    "inconclusive": 0
  },
  "workloads": [
    {
      "id": {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "namespace": "redis",
        "name": "redis-master"
      },
      "status": "progressing",
      "kstatusStatus": "InProgress",
      "kstatusMessage": "Deployment is waiting for updated replicas to become available",
      "generation": 2,
      "observedGeneration": 2,
      "ageSeconds": 86400,
      "progressAgeSeconds": 42,
      "progressClockSource": "status.conditions[Progressing].lastUpdateTime"
    }
  ]
}
```

`status` is one of `converged`, `progressing`, `failed`, `missing`, or
`inconclusive`. The receipt-level verdict is `PASS` only when every workload
is `converged`; `WATCH` when at least one workload is still `progressing`;
`BLOCK` when any workload is `failed` or `missing`; and `INCONCLUSIVE` when
any workload could not be classified.

The progress clock is deliberately generation-aware. If
`status.observedGeneration < metadata.generation`, cub-scout treats the
status as stale for timeout purposes and reports
`progressClockSource: "status.observedGeneration<metadata.generation"` with
no `progressAgeSeconds`; the workload remains `WATCH` rather than being
failed because the Kubernetes object itself is old. Otherwise cub-scout uses
the best available current-generation progress timestamp, preferring
`status.conditions[Progressing].lastUpdateTime`, then
`lastTransitionTime`, then the newest condition timestamp, and finally
`metadata.creationTimestamp` when no condition timestamp is available.

Terminal pod/container waiting reasons such as `CrashLoopBackOff`,
`ImagePullBackOff`, or `CreateContainerConfigError` are surfaced in
`podReasons[]` and make the workload `failed` without waiting for the rest of
the grace window.

For Deployment-style workloads, cub-scout treats kstatus `Current` as the
success snapshot. It does not expose a separate rollout-success knob; Kubernetes
already folds the workload's availability semantics into status, including
`availableReplicas` and any configured `minReadySeconds`.

#### `prerequisites-met` evidence

`prerequisites-met` records its details under
`predicate.evidence.prerequisites`:

```json
{
  "source": {
    "type": "file",
    "ref": "prereqs.yaml",
    "digest": "sha256-of-prereq-file",
    "objectCount": 3
  },
  "scope": {"kind": "namespace", "namespace": "redis"},
  "declaredDigest": "sha256-of-declared-facts",
  "liveDigest": "sha256-of-live-fact-statuses",
  "summary": {
    "required": 3,
    "present": 2,
    "missing": 1,
    "inconclusive": 0
  },
  "facts": [
    {
      "kind": "Secret",
      "namespace": "redis",
      "name": "app-db-secret",
      "status": "missing"
    }
  ]
}
```

### Auto-Detection Priority

When `--predicate` is not passed, cub-scout picks one from these signals
in priority order:

1. `OwnerArgo` / `OwnerFlux` / `OwnerConfigHub` **with** a resolved git
   anchor (`evidence.gitSource` non-nil) → `applied-matches-spec`
2. `--strategy` provided → `source-truth-pass`
3. `--since` provided → `no-manual-edits-since`
4. Same owners **without** a resolved git anchor, no other signals → `""`
   (no predicate) + `OmissionAutoDetectedPredicate` — the predicate
   evaluator could only emit INCONCLUSIVE anyway, so we record the
   *missing default* rather than masking the auto-detect-declined-here
   case as a different omission.
5. Other owners with no signals → `""` + `OmissionAutoDetectedPredicate`.

The result is always an INCONCLUSIVE receipt when auto-detect declines;
the omission entry names exactly which signal was missing.

Explicit `--predicate` always wins over auto-detect. The CLI also
rejects contradictory pairs upfront: `--predicate source-truth-pass`
without `--strategy` errors, and `--predicate no-manual-edits-since`
without `--since` errors — silently demoting either to INCONCLUSIVE
would hide the operator's intent from the receipt.

`--file <path>` is a separate install/object-set mode: it selects
`object-set-matches` and does not accept a positional resource subject.

### Omissions

Every receipt carries a required `omissions[]` array (possibly empty).
Each entry explicitly converts a silent PASS into an honest PASS:

| Omission `missing` | Meaning |
|--------------------|---------|
| `confighub-unit-subject` | No ConfigHub unit subject available (standalone mode, or connected mode but no linkage) |
| `managedFields` | Attribution layer returned `cause:unknown`, or no-manual-edits-since saw no entries on the live object |
| `managedFields-time` | An interactive `managedFields` entry has nil `Time`; no-manual-edits-since cannot place it on the timeline |
| `git-source-anchor` | No spec anchor or no controller-resolved git anchor |
| `auto-detected-predicate` | No signal supported a default predicate; pass `--predicate`, `--strategy`, or `--since` |
| `strategy` | source-truth-pass invoked without `--strategy`; cub-scout does not infer delivery strategy |
| `strategy-mismatch` | The receipt's declared strategy disagrees with the strategy recorded in the source-truth evidence body |
| `source-truth-evidence` | source-truth-pass invoked but no source-truth evidence body was attached |
| `source-truth-complete` | A proof gap from `compare source-truth` (e.g. `runtime.helm_chart_anchor`); mirrored into `omissions[]` regardless of receipt verdict |
| `since` | no-manual-edits-since invoked without `--since`; cub-scout does not invent a cutoff |
| `object-set-coverage` | One or more desired objects in an `object-set-matches` receipt could not be checked because API mapping or live lookup was inconclusive |
| `extra-live-object-coverage` | `object-set-matches` verified desired object identities and authored fields, but did not prove that no extra live resources exist outside the desired set |
| `extra-live-objects` | `object-set-matches --no-extras` found extra live objects of rendered kinds in scope that are not in the desired set |
| `object-set-diff-closure` | `object-set-diff` compared authored fields of the desired set but did not compute added/removed object closure because closed-world diffing was not requested |
| `workload-convergence-snapshot` | `workloads-converged` reflects readiness observed at `verifiedAt`; it does not prove workloads stay converged afterward |
| `workload-convergence-coverage` | One or more desired workloads in a `workloads-converged` receipt could not be checked because API mapping or live lookup was inconclusive |
| `prerequisites-snapshot` | `prerequisites-met` reflects required facts observed at `verifiedAt`; it does not prove they remain present afterward |
| `prerequisites-coverage` | One or more declared facts in a `prerequisites-met` receipt could not be checked because the live read failed |
| `next-step-allowed-action` | A nextStep with mutating `actionType` was dropped at receipt-emit time |
| `next-step-allowed-command` | A nextStep with mutating `nextCommand` (apply/edit/patch/delete/sync/create/update/replace/scale/rollout/reconcile/annotate/label/set/exec/debug/`helm install`/`helm upgrade`) was dropped |

### Read-Only Triad Lock

Receipts emit artifacts; they never mutate. Two static guards enforce this:

1. `TestReceiptPackageReadOnlyClient` (in `cmd/cub-scout/`) scans all
   `receipt*.go` sources and fails the build if any forbidden mutating
   K8s client method appears (`.Create(`, `.Update(`, `.UpdateStatus(`,
   `.Patch(`, `.Apply(`, `.ApplyStatus(`, `.Delete(`, `.DeleteCollection(`).
2. `FilterNextSteps` in `pkg/agent/receipt_predicates.go` is called on
   every receipt before fingerprint stamping, and drops any nextStep
   with a mutating `actionType` or `nextCommand`. Defense in depth.

### Fingerprint Scope

The fingerprint covers the **full Statement** (`_type` + `subject` +
`predicateType` + `predicate`) with only the `predicate.fingerprint`
field **removed from the JSON shape** (not zeroed in place). Hashing
predicate-only would leave `subject`, `predicateType`, and the in-toto
envelope unprotected; the full-Statement scope closes that. Zeroing the
field in place would leave a `"fingerprint":""` key in the canonical
bytes, which a third-party verifier following the contract prose would
compute differently — so the implementation parses to a generic map and
deletes the key before canonicalization.

Algorithm: SHA-256 over RFC 8785 JSON Canonicalization Scheme of the
Statement with the `predicate.fingerprint` key removed. The
canonicalizer is `github.com/gowebpki/jcs`, the reference Go
implementation of RFC 8785. The resulting digest is written back as
`predicate.fingerprint = "sha256:<hex>"`.

The implementation is conformance-tested against RFC 8785 reference
vectors (`TestCanonicalJSON_RFC8785_ReferenceVectors`) and tamper-
tested against subject, subject digest, predicateType, verdict,
omissions, and `_type` mutations.

`VerifyStatementFingerprint(stmt)` recomputes and compares; tamper
detection on any field except `predicate.fingerprint` will fail
verification.

### Output Formats

| `--format` | Behavior |
|------------|----------|
| `ascii` (default) | Concise one-screen human-readable summary (see `renderReceiptASCII` in `cmd/cub-scout/receipt_render.go`) |
| `json` | Full in-toto Statement v1 JSON envelope; the canonical machine-readable form |

`--out <path>` always writes the **JSON form** to disk regardless of console
`--format`. The on-disk artifact is the long-lived evidence; ASCII is for
human review.

### Receipt Management Surface (`show` / `validate` / `list`)

cub-scout ships three read-side subcommands that operate on receipt
artifacts produced earlier:

| Subcommand | Purpose | Exit code |
|------------|---------|-----------|
| `receipt show <path>` | Render an on-disk receipt as ASCII or JSON. Does NOT verify fingerprint. | 0 on success |
| `receipt validate <path>` | Recompute fingerprint and compare against stamped value. | 0 OK, 1 mismatch, 2 I/O |
| `receipt list [--dir <path>]` | Walk the store directory; one row per receipt. | 0 (partial list on parse failure with warning) |

#### Storage Convention

The receipt store is a flat directory of `*.receipt.json` files keyed
by a deterministic sortable name:

```
<verifiedAt-rfc3339-safe>__<predicateName>__<kind>-<name>__<short-fingerprint>.receipt.json
```

`verifiedAt-rfc3339-safe` replaces `:` with `-` (POSIX-portable);
`short-fingerprint` is the first 12 hex chars after the `sha256:`
prefix. The shape makes `ls` chronological and `ls | grep <predicate>`
useful.

Default store directory, resolved in priority order:

1. `--save-dir <path>` (on `receipt verify`) or `--dir <path>` (on `receipt list`)
2. `$CUB_SCOUT_RECEIPTS_DIR` env var
3. `$XDG_DATA_HOME/cub-scout/receipts`
4. `$HOME/.local/share/cub-scout/receipts`

`receipt verify --save` writes into the store. The store is
**immutable**: `SaveStatement` refuses to overwrite an existing
filename. A re-verify at the same instant on the same scope with the
same fingerprint produces the same name → "already saved" warning on
stderr, exit 0, the on-disk artifact unchanged.

#### `ReceiptListEntry` JSON Shape

`receipt list --format json` emits a sorted `ReceiptListEntry[]`:

```json
[
  {
    "path": "/home/user/.local/share/cub-scout/receipts/2026-05-22T10-30-00Z__applied-matches-spec__Deployment-api__a1b2c3d4e5f6.receipt.json",
    "verifiedAt": "2026-05-22T10:30:00Z",
    "predicateName": "applied-matches-spec",
    "scope": { "kind": "Deployment", "name": "api", "namespace": "prod" },
    "verdict": "PASS",
    "fingerprint": "sha256:a1b2c3d4e5f6...",
    "freshness": {
      "status": "fresh",
      "observedAt": "2026-05-22T10:30:00Z",
      "expiresAt": "2026-05-22T11:30:00Z",
      "ttl": "1h0m0s"
    }
  }
]
```

Sort order: `verifiedAt` descending (newest first).

`freshness.status` is a read-time summary derived from the saved
receipt's immutable `predicate.freshness` stamp:

| Value | Meaning |
|-------|---------|
| `fresh` | `predicate.freshness` exists and the list-time clock is before or equal to `expiresAt`. |
| `stale` | `predicate.freshness` exists and the list-time clock is after `expiresAt`. |
| `not-declared` | The receipt was created without `--ttl`; it makes no freshness claim. |
| `invalid` | The receipt contains malformed or incomplete freshness fields. |

When freshness is `not-declared`, `observedAt`, `expiresAt`, and `ttl`
are omitted from the list entry. Listing a receipt never rewrites the
on-disk Statement; this is an index over historical evidence, not a new
Kubernetes API read.

### v2 Extensions

The v1 envelope locks `inputAttestations[]`, the verdict enum, and the
receipt store. v2 layers three CI/agent-shaped extensions on top
without changing the wire format.

#### `--fail-on <verdict-list>` on `cub-scout receipt verify` (`#451`)

Exit non-zero (code 2) when the receipt's verdict matches one of the
listed values. The receipt is still printed / saved / written to
`--out` — the gate fires AFTER the artifact is durable.

Accepted values:

- `WATCH`, `BLOCK`, `INCONCLUSIVE` — exact match (comma-separated for multiple)
- `any-non-pass` — sugar for `WATCH,BLOCK,INCONCLUSIVE`
- `PASS` — rejected upfront (gating on a passing receipt is a no-op; treated as a workflow bug)

Exit codes:

| Exit | Meaning |
|------|---------|
| `0` | Verdict is PASS, or verdict not in the fail-on set |
| `2` | Verdict matches the fail-on set (CI gate fired) |
| `1` | Operational error (bad flag value, cluster unreachable, etc.) — same as everything else |

Implementation: `cmd/cub-scout/receipt.go` `parseReceiptFailOn` +
`exitCodeError` from `cmd/cub-scout/exit_code.go`.

#### `--input-attestation <path>` chained receipts (`#448` chained half)

A new receipt can reference prior receipts via `predicate.inputAttestations[]`
for chain / DAG semantics. Each `--input-attestation <path>` flag
(repeatable) loads a prior receipt, recomputes its fingerprint, and
attaches an `AttestationRef` to the new receipt's `inputAttestations[]`.

```json
"inputAttestations": [
  {
    "uri": "cub-scout-receipt://abc123def456",
    "digest": {
      "sha256": "abc123def456abc123def456abc123def456abc123def456abc123def456abcd"
    }
  }
]
```

URI scheme: `cub-scout-receipt://<short-fingerprint>` where the short
form is the first 12 hex chars after the `sha256:` prefix. The full
SHA-256 is in the `digest.sha256` field — that's what's cryptographically
meaningful; the URI is a readable label.

**Integrity check at chain-construction time:** `BuildAttestationRef`
calls `VerifyStatementFingerprint` on the referenced receipt and
**refuses** to chain a tampered receipt. The downstream receipt's
fingerprint covers the `inputAttestations[]` field by construction, so
tampering an upstream digest invalidates the downstream fingerprint too.

Implementation: `pkg/agent/receipt_inputattestations.go`
(`BuildAttestationRef`, `BuildAttestationRefsFromPaths`,
`AttestationURIScheme`); `cmd/cub-scout/receipt.go` flag plumbing.

#### `--scope` aggregate-with-discovery (`#448` aggregate half)

The aggregate-with-discovery half of `#448` adds two CLI shapes that
produce **one aggregate receipt** over **N per-resource receipts**:

```bash
# Namespace auto-discovery: walk Deployment / StatefulSet / DaemonSet /
# CronJob / Job in the namespace, build a per-resource receipt for
# each, then build the aggregate over them.
cub-scout receipt verify --scope namespace/<ns> --strategy <s> --save

# Comma-list batch: explicit set of resources (kinds normalized per
# the single-resource positional rules).
cub-scout receipt verify <kind>/<name>,<kind>/<name>,... -n <ns> --strategy <s>
```

In both shapes, the CLI emits N per-resource receipts to stdout as
JSONL lines, followed by 1 aggregate receipt as pretty-printed JSON.
`--fail-on` applies to the **aggregate** verdict.

Aggregate receipt wire shape:

```json
{
  "_type": "https://in-toto.io/Statement/v1",
  "subject": [
    {
      "name": "synthetic-aggregate://sha256/1a2b3c4d5e6f7890",
      "digest": {"sha256": "1a2b3c4d5e6f7890..."}
    }
  ],
  "predicateType": "https://cub-scout.dev/receipt/v1",
  "predicate": {
    "version": "v1",
    "predicateName": "aggregate-verdict",
    "verdict": "BLOCK",
    "claim": "aggregate verdict BLOCK over 12 receipt(s) in namespace prod (policy=max-severity)",
    "scope": {"kind": "namespace", "namespace": "prod"},
    "evidence": {},
    "omissions": [
      {
        "missing": "aggregate-partial-coverage",
        "reason": "2 of 12 input attestation(s) carried verdict INCONCLUSIVE; aggregate verdict may not reflect full coverage",
        "severity": "warning"
      }
    ],
    "inputAttestations": [
      {"uri": "cub-scout-receipt://abc123def456", "digest": {"sha256": "abc123def456..."}},
      {"uri": "cub-scout-receipt://9e7d12fa11b2", "digest": {"sha256": "9e7d12fa11b2..."}}
    ],
    "nextSteps": [],
    "verifier": {"tool": "cub-scout", "version": "vX.Y.Z"},
    "verifiedAt": "2026-05-22T22:00:00Z",
    "fingerprint": "sha256:..."
  }
}
```

Key shape rules (verified against `pkg/agent/receipt_aggregate.go`):

- **Subject scheme:** `synthetic-aggregate://sha256/<aggregate-id>` where `<aggregate-id>` is the first 16 hex chars of the SHA-256 over the deterministically-sorted concatenation of input sha256 digest hex (one per line). The full SHA-256 lives in `subject[0].digest.sha256`. Reordering the inputs produces the **same subject digest** — the subject is set-shaped.
- **`predicateName`:** the literal string `"aggregate-verdict"` (distinct from the per-resource predicate names `applied-matches-spec` / `source-truth-pass` / `no-manual-edits-since`).
- **Verdict synthesis:** the default policy is **max-severity** (`BLOCK > INCONCLUSIVE > WATCH > PASS`). `--aggregate-policy max-severity` is explicit; future policies (`majority`, weighted) are wired through the same flag.
- **`omissions[]`:** any input attestation with verdict `INCONCLUSIVE` triggers an `aggregate-partial-coverage` omission entry so the consumer knows the aggregate verdict may not reflect full coverage. Per-resource verify failures (load errors, marshal errors) also surface here when discovery couldn't reach a resource.
- **`inputAttestations[]`:** one entry per per-resource receipt successfully verified, **emitted in caller order**. Each entry's fingerprint is **verified at chain-construction time** via the same `VerifiedAttestationRef` typed wrapper the single-resource chained path uses (per `#463` Codex round-6 P1 fix; `pkg/agent/receipt_aggregate.go:BuildAggregateReceipt` rejects zero-value wrappers).
- **Fingerprint coverage:** the aggregate's `fingerprint` covers every field including `inputAttestations[]`. Tampering with the input set (adding, removing, or reordering the entries) invalidates the recomputed fingerprint. Note that the **subject digest** is set-shaped (sorted-input concatenation; reordering is a no-op on the subject) but the **receipt-level fingerprint** is list-shaped (covers the wire-order array). A future correctness pass may sort `inputAttestations[]` before stamping so the receipt-level fingerprint also becomes set-shaped; the current contract only guarantees order-independence for the subject.

Per-resource receipt failures during discovery are **non-fatal**: the aggregate is composed from the successful subset, with an `aggregate-partial-coverage` omission entry recording the failure count.

Implementation: `pkg/agent/receipt_aggregate.go`
(`BuildSyntheticAggregateSubject`, `BuildAggregateReceipt`,
`MaxSeverityPolicy`, `PredicateAggregateVerdict`);
`cmd/cub-scout/receipt_aggregate.go`
(`runReceiptVerifyScoped`, `discoverNamespaceWorkloads`,
`parseAggregateScope`).

#### `watch` / `bot` `--emit-receipt-on <event-types>` + `--emit-receipt-batch-cap` (`#449`)

`cub-scout watch` and `cub-scout bot` accept a flag that attaches a receipt to
each matching event payload inline. Both commands use the same watch event shape
with an optional `receipt` field carrying the full in-toto Statement.

Event-type set (current; all four supported as of #449):

| Event type | Receipt-build? | Notes |
|------------|---------------|-------|
| `drift.detected` | Yes — `applied-matches-spec` auto-detected | Highest signal: drift event already implies a verdict question |
| `ownership.changed` | Yes — `applied-matches-spec` auto-detected | Owner shifts often indicate a delivery-chain change worth attesting |
| `resource.discovered` | Yes — `applied-matches-spec` auto-detected | The discovery moment captures the live state at first observation; backpressure-gated via `--emit-receipt-batch-cap` |
| `scan.finding` | Yes — `applied-matches-spec` auto-detected | The receipt records the resource state at finding time; the finding detail lives on the event's `details` field; backpressure-gated |
| `all` | Sugar — accepts every known event type |

**Backpressure (`--emit-receipt-batch-cap N`, default 10):** when a
single poll produces more receipt-eligible events than the cap, the
first N get receipts built; the remaining events still emit but with
the `receipt` key **omitted** (omitempty) and a single stderr summary
line ("backpressure: suppressed receipt-build for X events of N
eligible"). The cap is per-poll (one watch poll = one call), so a
long-running watch with quiet polls between bursts doesn't accumulate
suppression state. Set to 0 to disable receipt-build entirely while
keeping the flag explicit; set to a large value (e.g. 1000) to
effectively disable the cap.

The flag is **lenient on type, strict on receipt-build**: unsupported event
types pass through without a warning (so `--emit-receipt-on all` is
forward-compatible as new event types land — if a future type lands
without receipt-build support, the startup warning fires automatically).
Build failures on supported types emit a stderr warning and continue —
the underlying watch event still emits, but with the `receipt` key
**omitted** from the JSON (the field uses `omitempty`; consumers should
check for key presence rather than null-ness):

```python
# correct
if "receipt" in event:
    process(event["receipt"])

# incorrect — `event["receipt"]` raises KeyError when receipt-build
# was skipped or failed
```

Sample JSONL line with a receipt:

```json
{"type":"drift.detected","timestamp":"2026-05-22T10:30:00Z","resource":{"kind":"Deployment","name":"api","namespace":"prod"},"owner":{"type":"argo","name":"payments-api"},"severity":"warning","details":{"category":"DRIFT"},"receipt":{"_type":"https://in-toto.io/Statement/v1","subject":[...],"predicateType":"https://cub-scout.dev/receipt/v1","predicate":{...}}}
```

Implementation: `cmd/cub-scout/watch.go` (flag init + per-event
attachment loop) + `cmd/cub-scout/watch_receipt.go` (the
`parseWatchEmitReceiptOn` / `attachReceiptsIfRequested` /
`watchBuildReceiptForEvent` helpers + `watchReceiptBatchCap`).

Per-poll backpressure ships in `v2.3.0` via `--emit-receipt-batch-cap N`
(default 10). When a single poll produces more receipt-eligible events
than the cap, the first N get receipts; the rest emit with the
`receipt` key omitted plus a single stderr summary line per poll. The
cap is per-poll (not across-poll), so quiet polls between bursts don't
accumulate suppression state. See [`docs/reference/watch-events.md`](watch-events.md)
for the dedicated event-type reference.

Source: `pkg/agent/receipt*.go`, `cmd/cub-scout/receipt*.go`,
`cmd/cub-scout/watch*.go`. See
`docs/proposals/receipts-way-forward.md` for the full design synthesis and
the Codex review rounds that locked the wire format.

## Historical Note

`v0.14-json-schema.md` is preserved for historical reference. It should not be treated as the canonical contract for current releases.

## Related Repo Tooling (Codex Handoff)

The Codex task handoff schema added for automation handoffs lives at:

- `tools/codex-task-output/codex-task-output.schema.json`
