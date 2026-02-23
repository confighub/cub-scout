# cub-scout Unified Roadmap

> **Status:** Authoritative
>
> This document **replaces and subsumes**:
>
> * `/ROADMAP.md`
> * `/docs/NEXT-PLAN.md` (historical planning draft)
> * `/SESSION.md` roadmap/status snapshots
> * scattered milestone notes and implicit issue groupings
>
> It reconciles *historical releases*, *current shipped reality*, and the *forward execution plan*.

---

## Untracked Backlog Checklist

The planning docs below contain future ideas not yet tracked as GitHub issues.
This checklist prevents ideas from being forgotten during sprint planning.
When an item graduates to real work, file a dedicated issue and remove it here.

Tracking issue: **#154** (master backlog tracking)

### Rendered Manifest + Argo (`roadmap-rendered-manifest-and-argo.md`)

- [ ] Workstream A: Distinguish "ConfigHub via OCI" ownership in trace outputs
- [ ] Workstream A: Show explicit `source → render → OCI → deployer → workload` chain
- [ ] Workstream A: Source staleness/sync signals where evidence is available
- [ ] Workstream B: Source inventory view — broader vision beyond `map sources` (#148)
- [ ] Workstream C: Schema extension for `rendered_from` / `original_source` (beyond #150)
- [ ] Workstream D: Bridge pattern — Git→Flux→cluster (beyond #151)
- [ ] Workstream D: Bridge pattern — Git→Argo→cluster
- [ ] Workstream D: Bridge pattern — Git→ConfigHub→OCI→deployer
- [ ] Workstream D: Bridge pattern — Live import→ConfigHub→OCI→deployer
- [ ] Workstream E: Orphan detection for broken ApplicationSet generator links
- [ ] Workstream F: Fleet query ergonomics and provenance readability
- [ ] Workstream F: Impact analysis ergonomics and multi-cluster context clarity
- [ ] Workstream G: Platform-only surfaces (Functions, Actions, ChangeSets, saved queries, alert triggers, dependency graphing, time-travel UX, three-state drift resolution, bulk operations)

### Connected Views + Launch (`roadmap-connected-views-and-launch.md`)

- [ ] Workstream A: Navigation-first messaging and "aha in seconds" walkthrough
- [ ] Workstream B: Scale-real demo assets (realistic many-workload examples)
- [ ] Workstream B: Expected-output snapshots for core navigation flows
- [ ] Workstream C: WET-LIVE panel clarity and causality messaging
- [ ] Workstream C: Break-glass decision ergonomics and audit visibility
- [ ] Workstream C: Hierarchy consistency across map/tree/unit surfaces
- [ ] Workstream D: Fleet view delivery sequence (by-cluster → compare → matrix → revisions → rollout/deps/incident/security)
- [ ] Workstream E: Claim→demo→status proof matrix
- [ ] Workstream F: Testing gate contract (coverage matrix, CI-enforced gate, per-run proof artifact)

### Pattern Contract (`reference/patterns-contract.md`)

- [x] `gitops.argocd.applicationset_generators` pattern (implemented; doc ID drift fixed in #153)
- [ ] `gitops.flux_kustomization_paths` pattern (planned, not implemented)

### Connected Mode Ideas (`NEXT-PLAN.md`, `WHY_CONNECTED_MODE.md`)

- [ ] ConfigHub summary storage and Slack integration for drift/sync notifications
- [ ] Connected import flows (`cub-scout connected import bundle/git/cluster`)
- [ ] Intent vs render vs observed three-way comparison
- [ ] Hook compatibility verifier

### Scale and Testing

- [x] Production-scale E2E testing (500+ resources, mixed ownership, deep hierarchies) — resolved by #155, #156
- [x] TUI performance profiling at scale (500+ resources) — resolved by #157
- [ ] CI-enforced coverage metrics

### 1.x Connected Upsell (`roadmap-1x-connected-upsell.md`)

- [ ] Connected hierarchy navigation defaults (cluster-aware filtering)
- [x] Mode state visibility in TUI (QuickMode for instant display, Offline/Standalone/Connected distinction)
- [x] Canonical migration guide from Argo/Helm to ConfigHub (`docs/howto/migration-playbook.md`)
- [ ] Trace context diagnostics and documented reset path for connected workflows
- [ ] "Break-glass to managed" flow documentation and guided testing
- [ ] User-facing explainer for ASCII vs JSON model (canonical data vs presentation)

### Documentation Gaps (`NEXT-PLAN.md`)

- [ ] Unified examples index (consolidate EXAMPLES-OVERVIEW.md, examples/README.md, demos/README.md)
- [ ] Integrate WHY_CONNECTED_MODE.md content into roadmap v1.x section with concrete issues

### Patterns (`reference/patterns-contract.md`)

- [ ] Connected mode `--git-url` / `--git-ref` / `--git-subpath` flags for remote Git pattern enrichment

---

## How to Read This Roadmap

* **Released** sections are sealed unless explicitly reopened.
* **Planned** sections describe *intent*, not promises.
* Each version has a **theme** explaining why it exists.
* Issues are grouped where they belong *now*, not where they were first imagined.

Semantic contracts, determinism guarantees, and the ASCII = f(JSON) + g model are assumed and not re-litigated here.

---

## Current Planning and Backlog Docs

`docs/roadmap.md` is the canonical roadmap. The files below are active planning/backlog inputs and must not override this document.

| File | Status | Role |
|------|--------|------|
| `docs/roadmap-rendered-manifest-and-argo.md` | Planning only (non-authoritative) | Workstream backlog for rendered-manifest + Argo hierarchy |
| `docs/roadmap-connected-views-and-launch.md` | Planning only (non-authoritative) | Workstream backlog for connected views and launch narrative |
| `docs/roadmap-1x-connected-upsell.md` | Draft for review (non-authoritative) | Candidate sequencing for 1.x connected packaging/upsell |
| `docs/releases/v0.20.0-slice-plan.md` | Delivered historical plan (non-authoritative) | Scoped implementation plan used for shipped v0.20.0 slice |

Graduation rule: move an item from these files into this roadmap (or a release plan) before treating it as committed execution scope.

---

## Released History (Locked)

### v0.5 — Contract-Locked CLI

**Status:** Released

Theme: *Trustworthy, script-safe CLI*

* Canonical CLI surfaces
* Deterministic output
* Stable exit codes and schemas

Contracts are sealed.

---

### v0.6 — Graph Foundation

**Status:** Released

Theme: *First-class resource graph*

* Unified graph model
* Ownership chains
* Graph export and explain

---

### v0.7–v0.10 — Explainable Inference

**Status:** Released

Theme: *Correctness before cleverness*

* Pattern detection and explanation
* Deterministic inference
* Git-aware evidence (optional, additive)

---

## v0.14 Line — Explainable Debugging

**Status:** Released

Theme: *Share explanations, not just output*

Delivered across v0.14.0–v0.14.6:

* Guided GitOps debug UX
* Drift detection + expansion
* Drift ↔ debug correlation
* Debug Bundle v1 (capture, replay, docs)

This arc is complete and sealed.

---

## v0.15 — Replay & Time-Series Reasoning

**Status:** Released

Theme: *What changed over time?*

* Catalog v1
* Bundle diff v1
* Bundle timeline v1
* Offline, read-only, deterministic

All semantics complete.

---

## v0.16 — Platform Composition & Attribution

**Status:** Released

Theme: *Why this change exists*

* Attribution Graph v1
* Attribution Report v1
* Crossplane lineage (XR/MR/Composition)
* Kustomize overlay attribution
* Deterministic scoring and replay

### Open (Future)

* Platform composition beyond Crossplane (kro) — #3, #21

---

## v0.17 — Stabilization

**Status:** Released (Non-semantic)

Theme: *Harden before workflows*

Delivered:

* CI workflows
* Benchmark harness
* Contract audit (schema immutability + golden determinism)
* Epic cleanup

---

## v0.18 — Connected Workflows

**Status:** Released

Theme: *Bring cub-scout to where decisions happen*

This release fulfills the **original v0.2–v0.3 "why connected" intent** *without* introducing controllers or mutation.

### Delivered

* Artifact-first workflows (CI → bundle → replay)
* Git-aware, read-only workflows (context captured, not authoritative)
* Explicit fleet & environment views (catalog + diff)
* CI-backed demos and examples
* Zero new schemas or inference

cub-scout is now **operationally connected** while remaining offline-capable.

---

## v0.19 — TUI Polish & Evidence Workflows

**Status:** Released (v0.19.0–v0.19.6)

Theme: *Delight on top of substance*

### v0.19.0–v0.19.3: TUI Polish

* Canonical visual vocabulary
* TUI snapshot golden tests
* CLI ↔ TUI symmetry flags
* Context-aware suggestions
* Shell completion
* Resolver pattern documentation (#24) ✅
* Crossplane walkthrough (#23) ✅
* Performance & scale guardrails (#22) ✅

### v0.19.4: Evidence Adjacency

* `bundle summarize` command — export for tickets, PRs, Slack
* Docs audit — archived stale docs, fixed DEPRECATED references

### v0.19.5: GitOps Lifecycle Hazards

* `scan --lifecycle-hazards` — detect Helm hook risks under ArgoCD
* `map hooks` — list resources with lifecycle hook annotations
* Helm-to-ArgoCD phase mapping (pre-install→PreSync, post-install→PostSync)
* `examples/lifecycle-hazards/` — documentation and example manifests

### v0.19.6: 1.0 Readiness

* Real connected mode auth (LoadAuth/SaveAuth with JSON persistence)
* Real connectivity check (HTTP HEAD to ConfigHub with timeout)
* CLI contract v1.0 frozen (`docs/reference/cli-contract.md`)
* Hub package tests added
* Removed "experimental" messaging from README
* GOTOOLCHAIN=local for air-gapped build compatibility
* Release-readiness Argo regression audit command + fixtures added (`test/regression/argo-version-audit.sh`, issue #125)

The UX surface is now **stable and locked**.

---

## v0.20.0 — Flux Operator Interop Slice

**Status:** Released (2026-02-12)
**Theme:** *Read-only operator workflows with stronger runtime evidence*

Delivered:

* `map cronjobs` and `map jobs` for schedule/run visibility
* `map actions` for read-only runbook/action preview
* `trace --artifacts` for source artifact provenance
* `map activity` for normalized GitOps action timeline
* `map previews` for ephemeral PR environment detection

See details:

* `docs/releases/v0.20.0-release-notes.md`
* `docs/releases/v0.20.0-slice-plan.md` (historical planning record)

---

## v1.0.0 — Stable Contract Baseline

**Status:** Released (2026-02-20)
**Theme:** *General availability for the standalone contract surface*

Delivered:

* First stable major tag: `v1.0.0`
* CLI contract v1.0 is now GA (`docs/reference/cli-contract.md`)
* Standalone map/tree/trace/scan and JSON contract surfaces remain read-only and deterministic
* Standard GitHub Release + Homebrew + container artifacts via existing release automation

See details:

* `docs/releases/v1.0.0.md`

---

# What Is Left on the Roadmap

Everything below is **new work**.
Nothing here represents unfinished 0.x promises.

---

## v1.1+ — Connected Mode (ConfigHub)

**Status:** Planned
**Theme:** *Shared intent, history, and fleets*

Connected Mode integrates cub-scout with **ConfigHub**, the system of record for configuration intent, history, and fleets.

This work is explicitly motivated by `WHY_CONNECTED_MODE.md`.

### Definition

Connected Mode means:

* cub-scout participates in **importing state into ConfigHub**
* ConfigHub owns **storage, indexing, and lifecycle**
* cub-scout remains:

  * read-only
  * bundle-first
  * deterministic
  * non-controller

---

### Archive MAP Doc Reconciliation

The archived MAP series under `docs/archive/from-confighub-agent/2026-02-09/map/` is historical planning input.
Current canonical behavior and scope are defined by:

* `docs/reference/connected-tiers-and-views-product-guide.md`
* `docs/reference/gitops-repo-structures.md`
* `docs/reference/hub-appspace-examples.md`
* `docs/reference/import-docs-crosswalk.md`

Coverage currently represented in this roadmap:

* Standalone -> discovery -> connected adoption path (read-only exploration + import context)
* Git/source visibility and comparison direction
* Fleet topology/history surfaces as connected/fleet features

Not in scope for `cub-scout` implementation (by design):

* Worker/controller lifecycle commands in `cub-scout`
* Continuous reconciliation or policy enforcement in `cub-scout`
* Mutating platform workflows (`drift accept`, `mutate`, `promote`) implemented directly in `cub-scout`

Legacy `cub-agent` command examples in archive docs are non-canonical; current command contract is `docs/reference/commands.md`.

---

### Remaining Capabilities (Authoritative)

#### 1. Connected Import into ConfigHub

* Import bundles into ConfigHub
* Import cluster-captured state via cub-scout
* Explicit, one-shot, auditable imports
* Intentionally deferred from late 0.x to keep read-only-first stabilization tight; now staged as 1.1+ Connected Mode scope

#### 2. Git as a First-Class Source

* Import Git intent (not just metadata)
* Compare:

  * Git ↔ cluster
  * Git ↔ Git
* No mutation, no continuous sync

#### 3. Shared Fleet Definitions

* Fleets stored durably in ConfigHub
* Fleets spanning clusters, git sources, history
* cub-scout as the exploration and explanation surface

#### 4. Surfacing ConfigHub Engines

* History (ChangeSets)
* Views / projections
* Impact & blast radius
* Policy context (explanatory only)

cub-scout surfaces results; it does not reimplement engines.

---

### Explicit Non-Goals

Connected Mode does **not** introduce:

* Controllers or agents
* Continuous reconciliation
* Policy enforcement
* Silent semantic expansion

---

## Guiding Principles (Still True)

* **Explainability first**
* **No silent contract expansion**
* **Determinism over convenience**
* **Artifacts over live dependencies**

---

## Summary

* v0.x is **complete**
* v0.18 fulfilled the original "why connected" *operationally*
* v0.19 locked UX, behavior, and performance
* Open issues #3/#21 (kro), #149-#151 (delivery chain), #158 (connected example), #163-#166 (experiments/evidence) remain for future work
* The remaining roadmap is **Connected Mode with ConfigHub**:

  * import
  * git as source
  * fleets
  * shared history

There is no other hidden work.
