# Connected Views and Launch Backlog

Status: Planning only (non-authoritative)
Last updated: 2026-02-09

This document tracks rollout sequencing for tiered views and launch narrative work.
Authoritative behavior is in:

- `docs/reference/connected-tiers-and-views-product-guide.md`

## Planning Intent

- Keep OSS value immediate and deterministic.
- Make connected mode the next clear operational step.
- Reserve multi-cluster/history-heavy surfaces for fleet milestones.

## Workstream A: Launch Narrative Quality

Goal: keep positioning tied to real operator workflows.

Candidate tasks:

- Navigation-first messaging (structure, trace, deep-dive)
- Problem-led README/story flow
- "Aha in seconds" walkthrough alignment with actual commands

Exit criteria:

- New user understands core value in under a minute without feature education overhead.

## Workstream B: Scale-Real Demo Assets

Goal: demos that reflect real cluster complexity.

Candidate tasks:

- Maintain realistic examples with many workloads/namespaces
- Include orphan/break-glass scenarios as optional but reproducible cases
- Publish expected-output snapshots for core navigation flows

Exit criteria:

- Demo value is obvious on realistic scale, not only toy clusters.

## Workstream C: Connected View Maturity

Goal: stabilize connected-tier views before fleet expansion.

Candidate tasks:

- Panel/WET-LIVE clarity and causality messaging
- Break-glass decision ergonomics and audit visibility
- Hierarchy consistency across map/tree/unit surfaces

Exit criteria:

- Connected users can resolve state ambiguity without leaving the workflow.

## Workstream D: Fleet View Delivery

Goal: ship fleet-only views in a coherent sequence.

Candidate sequence:

1. By-cluster summary
2. Compare
3. Matrix
4. Revisions and timeline
5. Rollout, dependencies, incident, fleet security

Exit criteria:

- Fleet views are integrated around operational decisions, not isolated screens.

## Workstream E: Proof and Conversion

Goal: ensure capability claims are demonstrable.

Candidate tasks:

- Maintain claim -> demo -> status matrix
- Keep preview/upsell surfaces tied to real limitations
- Track onboarding-to-connected conversion moments

Exit criteria:

- Every claim in top-level docs maps to runnable proof or marked planned status.

## Promotion Rule

Promote backlog items into release roadmap only when:

1. Contract impact is known.
2. Test/fixture strategy is defined.
3. Tier boundary (OSS/Connected/Fleet) is explicit.

## Source Lineage

Roadmap extraction source docs:

- `/Users/alexis/Public/github-repos/confighub-agent/planning/VIEW-TIERS.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/RM-MOCKUPS.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/PRODUCT-PLAN-LAUNCH.md`

