# GitOps Checkpoint Schemas

This page defines the versioned schema contracts for GitOps checkpoint objects.

## Why These Schemas Exist

Session transcripts are optional evidence. The primary checkpoint primitive is a
governed change transaction.

Current schema set:

1. `change-intent.v1`
2. `execution-report.v1`
3. `change-interaction-card.v1`
4. `decision-receipt.v1`
5. `execution-receipt.v1`
6. `outcome-receipt.v1`

Contract files:

1. `docs/reference/schemas/change-intent.v1.schema.json`
2. `docs/reference/schemas/execution-report.v1.schema.json`
3. `docs/reference/schemas/change-interaction-card.v1.schema.json`
4. `docs/reference/schemas/decision-receipt.v1.schema.json`
5. `docs/reference/schemas/execution-receipt.v1.schema.json`
6. `docs/reference/schemas/outcome-receipt.v1.schema.json`

## Versioning Rules

1. New meaning requires a new schema version.
2. Existing required fields are not removed inside a version.
3. Unknown values must be explicit (`"unknown"`), not omitted, for critical audit fields.

## Object Roles

### `ChangeIntent`

Describes proposed change scope, actor, and targets.

### `ExecutionReport`

Describes decision result, token issuance, runtime apply status, and post-scan outcome.

### `ChangeInteractionCard`

Joined object used by explain/search/audit:

`intent + evidence + decision + execution + outcome`

### Receipts (`decision|execution|outcome`)

Compact DRY write-back objects intended for Git storage.

They contain:

1. stable IDs,
2. attestation digests,
3. linkage to authoritative WET records in ConfigHub.

See also:

1. `docs/reference/stored-in-git-vs-confighub.md`

## Minimal Example (`change-interaction-card.v1`)

```json
{
  "schema_version": "change-interaction-card.v1",
  "card_id": "cic_7f8e9d0c1b2a3f44",
  "created_at": "2026-02-14T15:00:00Z",
  "trust_tier": 2,
  "identity": {
    "repo": "github.com/acme/platform",
    "branch": "main",
    "commit_sha": "9f29a5c2a0f44f3a9d1ad5f2aaf9d8f85c9f1234",
    "trailers": {
      "Cub-Checkpoint": "7f8e9d0c1b2a",
      "Cub-Agent": "codex"
    }
  },
  "intent": {
    "intent_id": "ci_1f7d30e8f1d44b37",
    "summary": "Roll out payment API config update",
    "domain": "app",
    "targets": [
      {
        "kind": "Deployment",
        "namespace": "payments",
        "name": "payment-api"
      }
    ]
  },
  "decision": {
    "result": "ALLOW",
    "reason": "Policy checks passed",
    "policy_refs": ["policy.gitops.tier2.standard"]
  },
  "execution": {
    "runtime": "confighub-actions",
    "run_id": "run_2f4cb2ad",
    "status": "succeeded",
    "started_at": "2026-02-14T15:00:08Z",
    "ended_at": "2026-02-14T15:00:24Z"
  },
  "post_scan": {
    "status": "pass",
    "finding_count": 0
  },
  "outcome": {
    "result": "applied",
    "message": "Deployment healthy after rollout"
  }
}
```
