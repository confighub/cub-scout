# Connected Tiers and Views Product Guide

Status: Authoritative product guide
Last updated: 2026-02-09

This document defines product behavior for tiered views and navigation outcomes.
It does not define delivery timing.

## Tier Model

- Tier 1 (OSS standalone): single-cluster map, trace, local risk/issue discovery
- Tier 2 (Connected): single-cluster plus ConfigHub context (WET <-> LIVE, hierarchy, break-glass acceptance)
- Tier 3 (Fleet): multi-cluster and history-heavy views

Connected mode adds context and lifecycle correlation; standalone remains fully usable.

## View Ownership by Tier

### OSS

- Ownership detection and resource grouping
- Orphan detection
- Trace for local ownership chain
- Local query, issues, and risk scan

### Connected

- Hierarchy context (Org -> App -> Deployment -> Unit + labels)
- WET <-> LIVE panel semantics
- Import/adoption and break-glass accept/reject workflows
- Causality surfaces for "what should be" vs "what is"

### Fleet

- Matrix (Unit x Cluster)
- Compare (cluster/environment diff)
- Revision history and rollout progression
- Dependency, incident, timeline, and fleet security posture views

## Hierarchy Semantics

Hierarchy path:

`Org -> App -> Deployment -> Unit -> Labels`

Rules:

- Labels are facets for grouping/filtering, not another hierarchy layer.
- Unit is the deployable lifecycle object.
- Connected/Fleet views should preserve this shape consistently.

## WET <-> LIVE Panel Semantics

Connected panel views should make three things explicit:

1. Expected state from ConfigHub (WET)
2. Observed cluster state (LIVE)
3. Causality between them (sync/drift/break-glass)

Key expectation:

- Users can identify drift and unmanaged resources without switching tools.

## Break-Glass Semantics

OSS:

- Detect unmanaged/native resources

Connected:

- Accept -> versioned Unit and repeatable lifecycle path
- Reject -> explicit rollback/removal path

Product requirement:

- Break-glass decisions must be auditable and visible in context.

## Core Navigation Views

Core navigation set:

- Fleet overview
- Unit detail
- Drift and break-glass queue
- Revision history
- Trace chain

Fleet extension set:

- By cluster
- Compare
- Matrix
- Rollout
- Dependency
- Security
- Incident
- Timeline

## Upsell and Preview Rules

- Upsell only at real capability boundaries.
- Never block core OSS workflows.
- Preview surfaces must be concrete and tied to immediate user value.
- Capability messaging must map to an observable limitation in current context.

## Product Positioning Guardrails

- Prioritize "understand and navigate cluster complexity quickly."
- Avoid feature-list-first framing.
- Keep trace/deep-dive/structure clarity ahead of generic claim language.

## Source Lineage

Consolidated from:

- `/Users/alexis/Public/github-repos/confighub-agent/planning/VIEW-TIERS.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/RM-MOCKUPS.md`
- `/Users/alexis/Public/github-repos/confighub-agent/planning/PRODUCT-PLAN-LAUNCH.md` (reusable product sections)

