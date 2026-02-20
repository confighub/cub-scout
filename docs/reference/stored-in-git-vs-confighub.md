# Stored in Git vs Stored in ConfigHub

This note defines the storage boundary for governed GitOps mutations.

## Core Principle

1. **Git is DRY**: store compact, reviewable, immutable linkage artifacts.
2. **ConfigHub is WET**: store full, rich, queryable operational and governance state.

For staged adoption and product packaging context, see:

1. `docs/reference/cub-track-mvp-upsell-and-dual-store.md`

## Why This Split Exists

Git excels at durable source history and human review.

ConfigHub excels at:

1. policy evaluation context,
2. decision/approval lifecycle,
3. runtime execution telemetry,
4. cross-repo correlation and search.

Trying to store all WET operational data in Git creates bloat, weak queryability, and
higher data leakage risk.

## Store in Git (DRY)

Use commit trailers + compact receipt objects.

### Required DRY artifacts

1. Commit linkage:
   - `Cub-Checkpoint`
   - `Cub-Intent`
   - `Cub-Agent` (optional)
2. Decision receipt (summary)
3. Execution receipt (summary)
4. Outcome receipt (summary)
5. Stable IDs and attestation digests linking back to ConfigHub records

### DRY constraints

1. Small payloads
2. No secrets/tokens
3. No high-volume telemetry
4. Append-only branch updates (`cub/checkpoints/v1`)

## Store in ConfigHub (WET)

### Required WET artifacts

1. Full policy input/output graph and rule traces
2. Approval chain metadata (who approved, when, why)
3. Token issuance metadata (scope, TTL, audience, issuance claims)
4. Full execution records from `confighub-actions`
5. Pre/post scan detailed findings and evidence links
6. Adapter-normalized evidence and cross-repo indexes
7. Retention-governed sensitive data (e.g., optional transcripts)

### WET constraints

1. Redaction and retention controls
2. Fine-grained access control
3. Query and reporting optimized for operations/audit

## Write-back Pattern

ConfigHub writes back only DRY receipts to Git:

1. `decision-receipt.v1`
2. `execution-receipt.v1`
3. `outcome-receipt.v1`

Each receipt references authoritative WET records in ConfigHub by ID and digest.

## Anti-Patterns

1. Writing full transcripts to Git by default
2. Writing token values or sensitive auth material to Git
3. Treating Git as the primary runtime telemetry store
4. Storing mutable indexes/search state in Git

## Practical Rule

If a record is needed for:

1. **code review and immutable commit linkage**, keep it DRY in Git.
2. **governance decisions and operational analytics**, keep it WET in ConfigHub.
