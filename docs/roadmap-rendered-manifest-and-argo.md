# Rendered Manifest and Argo Hierarchy Roadmap Backlog

Status: Planning only (non-authoritative)
Last updated: 2026-02-09

> Canonical roadmap: `docs/roadmap.md`
> This file is a supplemental backlog/workstream plan. If any content conflicts, `docs/roadmap.md` wins.

This file is the roadmap group for Rendered Manifest + Argo hierarchy work.
Product semantics are defined in:

- `docs/reference/rendered-manifest-and-argo-product-guide.md`

## How to Read This File

- This is sequencing and execution intent, not a shipping guarantee.
- Items here should graduate into `docs/roadmap.md` / release plans when scoped.
- Anything unclear in this file must not override product-guide behavior.

## Workstream A: Trace and Source Enrichment

Goal: strengthen provenance for rendered-manifest workflows.

Candidate tasks:

- Distinguish "ConfigHub via OCI" ownership in trace outputs.
- Show explicit chain: source -> render -> OCI artifact -> deployer -> workload.
- Add source staleness/sync signals where evidence is available.

Exit criteria:

- One-command trace explains rendered provenance without manual cross-checking.

## Workstream B: Source Inventory View

Goal: make source-of-truth visibility explicit.

Candidate tasks:

- Add/extend source-oriented map view.
- Surface Git, OCI, Helm, and connected-source evidence consistently.

Exit criteria:

- Operators can list active config sources and identify stale/unhealthy sources quickly.

## Workstream C: Data Contract Extensions

Goal: encode rendered/source metadata in stable contracts.

Candidate tasks:

- Extend state schema with source metadata fields.
- Represent `rendered_from` / `original_source` semantics in documented contract.

Exit criteria:

- Schema + docs + fixtures stay deterministic and backward-compatible.

## Workstream D: Bridge Pattern Detection

Goal: detect delivery bridge patterns without ambiguous inference.

Candidate patterns:

- Git -> Flux -> cluster
- Git -> Argo -> cluster
- Git -> ConfigHub -> OCI -> deployer
- Live import -> ConfigHub -> OCI -> deployer

Exit criteria:

- Pattern classification is test-backed and explainable in output.

## Workstream E: Argo Hierarchy Lineage Hardening

Goal: make App-of-Apps and ApplicationSet lineage trustworthy at scale.

Candidate tasks:

- Parent/child lineage clarity for App-of-Apps.
- Generator/instance lineage clarity for ApplicationSet.
- Orphan detection for broken generator links.

Exit criteria:

- Users can answer "who generated this app?" and "what depends on this generator?" without ambiguity.

## Workstream F: Connected and Fleet UX

Goal: ship useful connected workflows before platform-heavy surfaces.

Candidate focus:

- Fleet query ergonomics
- Provenance readability
- Impact analysis ergonomics
- Multi-cluster context clarity

Exit criteria:

- Connected mode is materially better than standalone for real fleet debugging.

## Workstream G: Platform-Only Surfaces (Later)

These are explicitly platform roadmap items, not cub-scout-only deliverables:

- Functions
- Actions
- ChangeSets
- Saved queries and alert triggers
- Dependency graphing
- Revision/time-travel UX
- Three-state drift resolution UX
- Bulk operations UX

Exit criteria:

- Ownership boundaries remain clear: cub-scout as evidence input, platform as lifecycle system.

## Promotion Rule

Only move an item from this backlog into release roadmap when:

1. Contract impact is known.
2. Fixture/test strategy is defined.
3. Ownership boundary (`cub-scout` vs platform) is explicit.

## Source Lineage

Roadmap extraction source docs:

- `/Users/alexis/Public/github-repos/confighub-agent/planning/RENDERED-MANIFEST-PATTERN.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/RENDERED-MANIFEST-PATTERN-FULL-PRODUCT.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/REPO-SKELETON-TAXONOMY.md`
