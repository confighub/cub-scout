# `cub-track` MVP, Upsell Path, and Dual-Store Data Model

**Status:** Draft  
**Date:** 2026-02-15  
**Audience:** ConfigHub product, engineering, GTM

## Locked Decisions (Current)

1. Ship as a **separate OSS project** first: `cub-track`.
2. Keep MVP command surface small: `enable`, `explain`, `search`.
3. Use a Git-native mutation linkage contract (trailers + append-only metadata branch).
4. Keep the path open to fold into `cub` CLI later as `cub track`.

## What `cub-track` Does (MVP)

`cub-track` creates a governance-grade mutation ledger for AI-assisted GitOps changes:

1. Link a commit to mutation metadata.
2. Explain a commit in intent/decision/outcome terms.
3. Search mutation history by text, file, agent, and decision state.

No controller replacement and no required backend dependency in OSS-first mode.

## MVP Git Contract

### Commit trailers

1. `Cub-Checkpoint: <id>` (MVP-compatible key; aliasable to `Cub-Mutation` later)
2. `Cub-Intent: <id>` (optional)
3. `Cub-Agent: <agent-name>` (optional)

### Metadata branch

1. Preferred: `cub/mutations/v1`
2. Legacy-compatible read alias: `cub/checkpoints/v1`
3. Append-only by default (no force-push in normal operation)

## Upsell Table: OSS -> Connected ConfigHub

| Stage | What user installs | What they get immediately | Git writes (DRY) | ConfigHub writes (WET) | Portfolio integration | Third-party AI support |
|---|---|---|---|---|---|---|
| 0. OSS Local | `cub-track` only | Commit-linked AI mutation history; explain/search in repo | Trailers + mutation cards + compact receipts | None required | None required | Adapter input from Codex/Claude/Gemini/Cursor logs into canonical card fields |
| 1. Connected Evidence | `cub-track` + optional ConfigHub org/project credentials | Cross-repo search, centralized provenance index, shared team visibility | Same DRY artifacts; plus stable external IDs/digests | Ingested card index, adapter identity normalization, evidence catalog | `cub-scout` reads normalized evidence for explorer views | Same adapters; identity is normalized centrally |
| 2. Governed Apply | Stage 1 + policy/runtime services | Policy-gated mutation flow: `ALLOW|ESCALATE|BLOCK`; attested execution | Decision/execution/outcome receipts are written back | Full policy traces (`confighub-scan`), approvals, token issuance, runtime attestation (`confighub-actions`) | `confighub-scan`, `confighub`, `confighub-actions`, `cub-scout` aligned by shared IDs | Third-party AI can propose intent, but apply rights remain tier/policy bound |
| 3. Enterprise Ops | Stage 2 + org controls | Audit-grade reporting, retention controls, compliance exports | Same compact Git receipts | Full retention policy, RBAC, analytics, org-wide audit/export | Adds portfolio-level dashboards/workflows (for example `uxbow`) | Same adapter model, stronger policy and attestation requirements |

## Dual-Store Data Model (Git + ConfigHub)

Git is DRY. ConfigHub is WET.  
Each record has one authority and optional mirrored linkage.

| Record | Git (DRY) | ConfigHub (WET) | Authority | Write-back rule |
|---|---|---|---|---|
| Mutation anchor (`commit_sha`, trailers) | Yes | Indexed copy | Git | Never rewrite historical commit linkage |
| `ChangeIntent` | Compact card field | Full normalized object + enrichment | ConfigHub once connected; Git in OSS-only mode | Keep semantic IDs stable across both stores |
| Decision result | `decision-receipt.v1` summary | Full rule graph, explanations, approver chain | ConfigHub | Write only digest + IDs to Git |
| Execution result | `execution-receipt.v1` summary | Full runtime logs, token claims, attestation | ConfigHub | Write only status/timestamps/digest to Git |
| Outcome/post-scan | `outcome-receipt.v1` summary | Full findings, evidence, trend correlation | ConfigHub | Write only counts/result/digest to Git |
| Adapter session/transcript | Optional pointer only | Retained data (opt-in), redacted by policy | ConfigHub | Do not write raw transcript by default to Git |
| Search index | No | Yes | ConfigHub | Git remains immutable source history, not mutable index |

## Canonical ID and Digest Strategy

Use shared IDs across both stores:

1. `checkpoint_id`
2. `intent_id`
3. `decision_id`
4. `execution_id`
5. `outcome_id`
6. `card_id`

Each DRY receipt includes:

1. `confighub_ref` (object ID/URL)
2. `attestation_digest`
3. `schema_version`

This preserves Git portability while keeping ConfigHub as the operational source of truth.

## Anti-Confusion Positioning (Internal + External)

### What this is

1. OSS, Git-native mutation ledger for AI-assisted GitOps changes.
2. Fast adoption wedge for Flux/Argo/Helm users.

### What this is not

1. Not a Flux/Argo replacement.
2. Not a complete hosted governance platform by itself.
3. Not a reason to duplicate all operational state into Git.

## Minimum GA Bar for This Direction

1. Flux/Argo/Helm team can run `enable -> commit -> explain -> search` in under 10 minutes.
2. All connected-mode receipts resolve to full WET records by stable IDs.
3. No secrets/tokens or bulky runtime logs written to Git.
4. Tier policy prevents unauthorized apply attempts by default.

## Related Docs

1. `docs/reference/stored-in-git-vs-confighub.md`
2. `docs/reference/gitops-checkpoint-prd.md`
3. `docs/reference/gitops-checkpoint-schemas.md`
