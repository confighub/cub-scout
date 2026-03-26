# Why Connect cub-scout to ConfigHub

> **Authoritative reference** for the standalone vs connected boundary and interface ownership split.
> See also: [`docs/roadmap.md`](../roadmap.md) for the full roadmap context.

## TL;DR

`cub-scout` is a free, read-only cluster explorer.

- Standalone mode answers: what exists now, who owns it, and what looks risky.
- Connected mode answers: what should exist, what changed over time, and what this affects across environments.

Connected mode is optional. Standalone remains fully usable without ConfigHub.

## What Standalone Gives You (Free)

With only Kubernetes access (`kubectl` context), `cub-scout` provides:

- live ownership and provenance tracing (Flux, Argo CD, Helm, native)
- health/issue/risk scanning in the current cluster
- relationship exports (JSON/graph) for debugging and handoff
- deterministic local workflows that do not mutate cluster state

## Why Standalone Has a Hard Ceiling

A cluster API can only show current observed state. It cannot reliably answer:

- what changed last week and why
- whether one cluster is an outlier vs the rest of the fleet
- what should happen in another environment before a rollout
- how imported state maps to durable org structure

Those require durable history, indexing, and cross-environment context outside a single cluster.

## What Connected Mode Unlocks

When connected to ConfigHub, `cub-scout` can use:

- intent context (DRY/WET/LIVE)
- change history and timeline correlations
- fleet comparison and outlier detection
- import/adoption workflows (break-glass to managed)
- dependency-aware impact analysis
- governance context and approvals metadata

`cub-scout` still remains read-only. Connected mode adds context, not control.

## Interface Boundaries (Authoritative)

- `cub` CLI is the external interface contract for connected workflows.
- `confighub-agent` depends on `cub` command behavior (arguments, exit codes, stdout/JSON shape), documented in `/Users/alexis/Public/github-repos/confighub-agent/README.md:16`.
- `cub-scout` connected mode depends on `cub auth login` semantics (credential/session creation and context resolution), documented in `/Users/alexis/Public/github-repos/cub-scout/README.md:80`.
- Standalone `cub-scout` must continue to function without `cub`.

This is the ownership split:

- `cub-scout`: deterministic discovery, explanation, evidence export
- ConfigHub: system of record, lifecycle state, migration semantics

## Paid Value Boundary

Connected value is paid because it requires hosted platform capabilities:

- durable multi-tenant storage
- cross-cluster indexing and query infrastructure
- fleet/governance APIs
- retention, auditability, and operations at scale

The CLI remains free and safe to run offline.

## Roadmap Alignment

For sequencing and boundaries:

- `docs/roadmap.md`
- `docs/roadmap-1x-connected-upsell.md`
