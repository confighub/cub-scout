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

## How to Read This Roadmap

* **Released** sections are sealed unless explicitly reopened.
* **Planned** sections describe *intent*, not promises.
* Each version has a **theme** explaining why it exists.
* Issues are grouped where they belong *now*, not where they were first imagined.

Semantic contracts, determinism guarantees, and the ASCII = f(JSON) + g model are assumed and not re-litigated here.

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

# What Is Left on the Roadmap

Everything below is **new work**.
Nothing here represents unfinished 0.x promises.

---

## v1.x — Connected Mode (ConfigHub)

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

### Remaining Capabilities (Authoritative)

#### 1. Connected Import into ConfigHub

* Import bundles into ConfigHub
* Import cluster-captured state via cub-scout
* Explicit, one-shot, auditable imports

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
* Open issues #3/#21 (kro) remain for future work
* The remaining roadmap is **Connected Mode with ConfigHub**:

  * import
  * git as source
  * fleets
  * shared history

There is no other hidden work.
