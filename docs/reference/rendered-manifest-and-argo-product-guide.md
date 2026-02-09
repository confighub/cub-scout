# Rendered Manifest and Argo Hierarchy Product Guide

Status: Authoritative product guide
Last updated: 2026-02-09

This guide defines product behavior and boundaries for:

- Rendered Manifest pattern (DRY -> WET -> LIVE)
- Argo App-of-Apps and ApplicationSet hierarchy semantics
- Single-cluster-first adoption for real repos

This is not a delivery plan. Execution sequencing lives in roadmap docs.

## Scope and Boundaries

This document is about product semantics, not implementation schedule.

- `cub-scout`: deterministic discovery, explanation, and evidence export
- `cub`: connected interface contract for ConfigHub workflows
- ConfigHub platform: durable storage, indexing, lifecycle, migration semantics

Standalone `cub-scout` remains usable without `cub` or ConfigHub.

## Rendered Manifest Pattern (Product Model)

Core flow:

1. DRY config (Helm/Kustomize/CUE) is rendered into WET manifests.
2. WET manifests are distributed via OCI artifacts.
3. Deployer (Argo/Flux) applies WET manifests to clusters.
4. `cub-scout` observes LIVE state and explains ownership/trace/risk.

Why OCI for WET:

- WET is machine-generated output, not collaborative source code.
- OCI gives immutable artifact semantics for rendered snapshots.
- Distribution and caching are better aligned with deployment artifacts.

## Argo Hierarchy Semantics

### App-of-Apps

Behavior model:

- Root Application is a generator-level object.
- Child Applications map to workload-bearing units.
- Child relationships preserve generator linkage (`generated_by` semantics).

Practical outcome:

- You can answer both "what is deployed?" and "who generated it?"
- Generator metadata is visible without pretending the root directly owns workloads.

### ApplicationSet

Behavior model:

- ApplicationSet is a template/generator object.
- Generated Applications map to instance units.
- Instance linkage preserves generator context (`instance_of` semantics).

Practical outcome:

- Generated lineage is queryable.
- Orphaned instances (generator removed, instance remains) can be surfaced.

## Single-Cluster-First Verification

Before fleet claims, verify one cluster end-to-end:

1. Map what exists and who owns it.
2. Trace one known workload to its deployer/source evidence.
3. Run risk scan and validate findings are explainable.
4. Verify App-of-Apps or ApplicationSet resources are represented with clear lineage.
5. Validate expected hierarchy shape for one app family.
6. Repeat in one more cluster only after step 1-5 are clean.

If one cluster is ambiguous, fleet behavior will be ambiguous at scale.

## Repo Skeleton Taxonomy (Guide-Level)

Dimensions:

1. Tool: Argo, Flux, Helm, hybrid
2. Repo count: mono, multi, poly
3. Environment strategy: overlays, folders, branches, values files
4. Orchestration: App-of-Apps, ApplicationSet, tenancy, flat

Canonical skeleton IDs:

- `argo-aoa-mono`
- `argo-appset-mono`
- `argo-appset-multi`
- `flux-tenant-mono`
- `flux-helm-mono`
- `helm-umbrella`

Use skeleton IDs for classification and test coverage, not marketing names.

## Tiered Value Story

- OSS (`cub-scout` standalone): single-cluster map/trace/scan, read-only evidence
- Connected: fleet-aware context, hierarchy-aware queries, import correlation
- Full platform: governance/approval/automation/compliance surfaces

This value progression is product positioning, not a promise of immediate feature availability.

## What Does Not Belong Here

- Milestone dates
- Effort estimates
- Phase sequencing
- Issue triage or sprint tasks

Those belong in roadmap documents.

## Source Lineage

This guide consolidates product content from:

- `/Users/alexis/Public/github-repos/confighub-agent/planning/RENDERED-MANIFEST-PATTERN.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/RENDERED-MANIFEST-PATTERN-FULL-PRODUCT.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/REPO-SKELETON-TAXONOMY.md`

