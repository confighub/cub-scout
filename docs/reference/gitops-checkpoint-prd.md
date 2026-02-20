# GitOps Checkpoint PRD (Proposal)

**Status:** Draft  
**Date:** 2026-02-14  
**Audience:** Product, platform, and engineering teams shaping post-Flux/post-Argo strategy

## 1. Problem

AI agents generate large config/code deltas quickly. Git captures what changed, but not
the governance path that should answer:

1. What intent was proposed?
2. What evidence was collected?
3. Why was this allowed or blocked?
4. How was it executed?
5. What happened after execution?

Session logs alone are not enough for GitOps governance.

## 2. Product Thesis

A GitOps checkpoint should be a **governed change transaction**, not just a transcript.

It records:

`commit -> ChangeIntent -> Evidence -> Decision -> Token -> Execution -> Outcome`

This makes AI-assisted operations explainable in GitOps terms.

## 3. Positioning

User-facing concept: **GitOps Explorer**  
Implementation detail: **Explorer Adapters**

Core boundary model:

1. `cub-scout` = GitOps Explorer + Evidence Normalizer
2. `confighub-scan` = Risk/Policy Signal Engine
3. `confighub` = Decision + Attestation + Approval Authority
4. `confighub-actions` = Execution Runtime (tokened after `ALLOW`)

## 4. Scope

### In Scope (MVP)

1. Git-native checkpoint records linked to commits
2. Canonical records for intent, evidence, decision, execution, and outcome
3. Read-only explain/search across checkpoints
4. Trust-tier aware apply gating
5. Flux/Argo interoperability through adapter evidence

### Out of Scope (MVP)

1. Replacing Flux/Argo controllers
2. Generic transcript warehousing as the primary object
3. Unbounded full-session retention by default
4. Hosted analytics dependency for base workflow

## 5. Core Objects

1. `ChangeIntent` (proposal and target scope)
2. `ExecutionReport` (decision, token, runtime execution, post-scan)
3. `ChangeInteractionCard` (joined checkpoint object used by explain/search/audit)

Schema contracts are defined in:

1. `docs/reference/schemas/change-intent.v1.schema.json`
2. `docs/reference/schemas/execution-report.v1.schema.json`
3. `docs/reference/schemas/change-interaction-card.v1.schema.json`

## 6. Trust Tier Model

1. Tier 0: Observe only (no apply rights)
2. Tier 1: Low-risk apply domains
3. Tier 2: Medium-risk with human approval
4. Tier 3: High-risk/prod with strong attestation + dual approval

Execution rights are tier-bound and enforced by policy decision before token issuance.

## 7. UX/CLI Surface (Proposed)

1. `cub trace enable|disable|status`
2. `cub trace explain [--commit <sha>|--card <id>|--intent <id>]`
3. `cub trace search [--text <query>|--file <path>|--agent <name>|--decision <result>]`
4. `cub trace clean|doctor`
5. Optional bridge: `cub trace publish`

## 8. Data Model (Git-native)

Commit linkage:

1. `Cub-Checkpoint: <id>`
2. `Cub-Intent: <id>` (optional)
3. `Cub-Agent: <name>` (optional)

Checkpoint metadata branch:

1. `cub/checkpoints/v1`
2. Append-only writes (no force-push in normal operation)
3. Sharded object layout by checkpoint ID

The primary contract object is the Change Interaction Card, not raw transcript text.

## 9. Adapter Model

External agents are supported through Explorer Adapters that emit canonical records.

Adapter contract:

1. Must emit `ChangeIntent` fields
2. Must emit `ExecutionReport` fields (or explicit unknowns)
3. Must provide stable actor identity metadata
4. Must provide artifact pointers/digests when available

## 10. Governance Flow

1. Agent/human proposes change (`ChangeIntent`)
2. `cub-scout` normalizes evidence
3. `confighub-scan` runs pre-scan
4. `confighub` evaluates policy -> `ALLOW|ESCALATE|BLOCK`
5. On `ALLOW`, short-lived token is issued
6. `confighub-actions` executes apply with attested runtime metadata
7. Post-scan runs and outcome is attached
8. `ChangeInteractionCard` is persisted and linkable from commit

## 11. Security/Privacy Defaults

1. Summary-first capture by default
2. Full transcript retention disabled by default
3. Redaction and denylist filters before write
4. Telemetry off by default
5. Explicit opt-in for remote checkpoint branch push

## 12. Success Criteria

1. `explain --commit` resolves a full card locally in < 2s for normal repos
2. Every governed apply has a card with decision + token + outcome fields
3. Flux/Argo users adopt without pipeline/controller changes
4. Tier policy prevents out-of-scope apply attempts by default

## 13. Rollout

### Phase 0: Contract + Read Path

1. Freeze schemas (`change-intent.v1`, `execution-report.v1`, `change-interaction-card.v1`)
2. Implement card explain/search over Git storage

### Phase 1: Write Path + T0/T1

1. Add checkpoint writing and trailer linking
2. Tier 0/1 gating with policy decisions

### Phase 2: Tokened Runtime

1. Integrate `ALLOW` token issuance
2. Runtime attestation from `confighub-actions`

### Phase 3: T2/T3 Hardening

1. Human approval and dual approval flows
2. Strong attestation requirements for production domains

