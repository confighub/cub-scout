# Connected Views and Launch Backlog

Status: Planning only (non-authoritative)
Last updated: 2026-02-09

> Canonical roadmap: `docs/roadmap.md`
> This file is a supplemental backlog/workstream plan. If any content conflicts, `docs/roadmap.md` wins.

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
- Maintain adoption-pattern proof coverage for Fluxy/Banko/Arnie-style structures
- Collaborate with operators on real domain->platform mapping patterns and feed validated mappings back into examples/docs

Exit criteria:

- Demo value is obvious on realistic scale, not only toy clusters.

## Workstream C: Connected View Maturity

Goal: stabilize connected-tier views before fleet expansion.

Candidate tasks:

- Panel/WET-LIVE clarity and causality messaging
- Break-glass decision ergonomics and audit visibility
- Hierarchy consistency across map/tree/unit surfaces
- Document label-based mapping decisions (and tradeoffs) for common repo/domain projection patterns

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
- Define and maintain a testing gate contract (feature x mode x tool matrix, expected-output snapshots, per-run proof artifact)

Exit criteria:

- Every claim in top-level docs maps to runnable proof or marked planned status.

## Workstream F: Testing Gate Contract

Goal: make release-readiness measurable, repeatable, and auditable.

Candidate tasks:

- Maintain required coverage matrix for core surfaces by feature x mode x tool.
- Enforce expected-output snapshot/golden comparisons for key map/trace/navigation flows.
- Publish a per-run proof artifact in CI summarizing matrix coverage and gate outcomes.

Exit criteria:

- CI fails when required gate coverage regresses.
- A proof artifact exists for each run covering promoted capability claims.

## Promotion Rule

Promote backlog items into release roadmap only when:

1. Contract impact is known.
2. Test/fixture strategy is defined.
3. Tier boundary (OSS/Connected/Fleet) is explicit.
4. Testing gate definition exists (matrix coverage, expected-output checks, proof artifact expectations).

## Source Lineage

Roadmap extraction source docs:

- `/Users/alexis/Public/github-repos/confighub-agent/planning/VIEW-TIERS.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/RM-MOCKUPS.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/PRODUCT-PLAN-LAUNCH.md`
