# cub-scout 1.x Roadmap and Connected Upsell Plan

Status: Draft for review
Date: 2026-02-08

## Why this plan

This plan synthesizes:
- Artem feedback themes (Argo hierarchy trust, context clarity, workload scope, migration path)
- Existing cub-scout roadmap direction (`docs/roadmap.md`)
- Connected/fleet value framing from local planning docs

The goal is to sequence 1.x so we:
- protect 1.0 trust first
- ship connected value in clear steps
- create strong, honest upsell points

## Planning principles

- Single-Cluster-First: if it does not work cleanly on one cluster, it is not fleet-ready.
- Deterministic first: no fuzzy behavior in map/tree/trace contracts.
- Explain before upsell: every paid prompt must correspond to a real user pain in current flow.
- Additive adoption: keep Flux/Argo/Helm workflows; avoid replacement framing.
- Contract discipline: changes to tree/map behavior must be covered by fixtures and golden outputs.

## Current reality snapshot

- 1.0 baseline is stable, but Argo hierarchy trust must be validated and hardened.
- Existing docs show tension:
  - some guidance says App-of-Apps/ApplicationSet parents are not modeled as Units
  - other planning/docs describe explicit parent/child visibility as desired
- This is acceptable for 1.0 only if we make current behavior explicit and document near-term intent.

## 1.x roadmap (execution order)

### Phase A: 1.0.x trust hardening (now)

Scope:
- Close P0s before broad connected expansion.

Primary issues:
- #125 regression audit for Argo tree/appset/app-of-apps
- #126 pipeline semantics (`unknown -> ...`)
- #127 trace context hardening
- #129 workload scope definition

Exit criteria:
- Reproducible v0.4.0 vs v0.19.6 comparison report exists and is linked from docs.
- Known behavior gaps are classified as intentional or regression.
- Map/trace UX has no unresolved “what does this mean?” dead ends.

### Phase B: 1.1 connected foundation (next)

Scope:
- Make connected mode clearly better than OSS for single-cluster operations.

Deliverables:
- Better connected hierarchy navigation defaults (cluster-aware filtering and clear mode state).
- First-class trace context diagnostics and reset path in connected workflows.
- Canonical migration guide from Argo/Helm to ConfigHub (#130), promoted out of archive.

Exit criteria:
- A user can import one real cluster and understand Hub/App Space/Unit mapping without external guidance.
- “Break-glass to managed” flow is documented and testable in one guided path.

### Phase C: 1.2 Argo hierarchy intelligence (next)

Scope:
- Resolve App-of-Apps/ApplicationSet visibility gap and unify tree mental model.

Deliverables:
- Ownership/tree lineage support for:
  - parent Application -> child Applications
  - ApplicationSet -> generated Applications
- Fixture-backed coverage for both patterns (#128, #132).
- Explicit inferred-vs-explicit relationship markers in output and docs.

Exit criteria:
- Argo users can answer “who generated this app?” and “what is parent of this app?” in one command/view.

### Phase D: 1.3 fleet and governance expansion (later)

Scope:
- Fleet-level workflows and governance surfaces that justify paid tiers.

Deliverables:
- Multi-cluster topology view and scoped roadmap implementation (#131).
- Archive normalization and source-of-truth docs index (#133).
- Progressive rollout, revision history, incident-centered views (as connected/fleet features).

Exit criteria:
- Fleet value is visible in under 2 minutes from connected onboarding.
- Tier boundaries are clear and non-contradictory in product/docs.

## Connected upsell strategy

## Tier story

- OSS (single cluster, no account):
  - map/trace/scan/orphans/issues
  - value: immediate cluster understanding and risk detection
- Connected (single cluster + ConfigHub):
  - import, hierarchy, WET vs LIVE causality, break-glass acceptance path
  - value: durable operational context and managed state
- Fleet/Enterprise (multi-cluster):
  - fleet queries, matrix/compare, rollout/revisions/timeline/security
  - value: cross-cluster control and audit/compliance scale

## Upsell triggers (command-level)

- `map orphans`:
  - trigger: user sees unmanaged resources
  - upsell: “Connect to classify, import, and track these with revisioned state.”
- `trace`:
  - trigger: user can trace local chain but lacks cross-cluster history
  - upsell: “Connect for full DRY->WET->LIVE provenance and change history.”
- `scan`:
  - trigger: user finds local risk
  - upsell: “Upgrade to fleet-wide risk posture and trend tracking.”
- `map` / `tree`:
  - trigger: local view is useful but context is fragmented
  - upsell: “Connect to see org/hub/app-space hierarchy and cluster scoping.”

## Upsell quality bar

- Prompts must be tied to an immediate, visible limitation.
- Prompts must never block core OSS workflows.
- Every prompt should map to one concrete action, not generic marketing.

## Metrics to validate roadmap and upsell

- Time to first trust signal:
  - user confirms output correctness for one known resource in <2 minutes.
- Time to first connected value:
  - successful dry-run import understanding in <5 minutes.
- Argo hierarchy confidence:
  - % of App-of-Apps/ApplicationSet fixture assertions passing in CI.
- Upsell usefulness:
  - conversion events after “limitation moments” (orphans, trace depth, fleet query need).

## Risks and mitigations

- Risk: Feature drift between docs and runtime behavior.
  - Mitigation: promote one canonical “current behavior vs planned behavior” section in docs.
- Risk: Over-promising lineage semantics before robust fixtures.
  - Mitigation: ship inferred markers and confidence labels early.
- Risk: Complex connected story dilutes OSS value.
  - Mitigation: keep OSS reflexes sharp (map, trace, scan) and additive messaging.

## Near-term actions (next 2 sprints)

1. Complete #125 and publish comparison report.
2. Land #126 + #127 with tests and docs.
3. Finalize workload scope contract (#129).
4. Draft and publish canonical migration path (#130).
5. Start lineage fixture implementation for #128.

## Source references

- `docs/roadmap.md`
- `docs/archive/IMPORT-FROM-LIVE.md`
- `docs/archive/IMPORT-FROM-SOURCES.md`
- `docs/archive/IMPORT-GIT-REFERENCE-ARCHITECTURES.md`
- `docs/archive/IMPORTING-WORKLOADS.md`
- `docs/archive/JOURNEY-IMPORT.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/REPO-SKELETON-TAXONOMY.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/RENDERED-MANIFEST-PATTERN.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/RENDERED-MANIFEST-PATTERN-FULL-PRODUCT.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/VIEW-TIERS.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/RM-DEMOS-ARGOCD.md`
