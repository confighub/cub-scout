# cub-scout Unified Roadmap

> **Status:** Authoritative
>
> This document **replaces and subsumes**:
>
> * `/ROADMAP.md`
> * `/docs/roadmap.md`
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

**Status:** Released (v0.5.0)

Theme: *Trustworthy, script-safe CLI*

* Canonical CLI surfaces: `trace`, `map status`, `scan --file`, `map list --json`, `map deployers --json`
* Deterministic output, stable exit codes, locked JSON schemas

> v0.5 contracts are sealed. Changes require a future minor.

---

### v0.6 — Graph Foundation

**Status:** Released

Theme: *First-class resource graph*

* Unified graph model (nodes, edges, evidence)
* Ownership chain collectors
* GitOps CRDs as graph nodes
* `graph export --json`, `graph explain`

---

### v0.7 — Pattern Detection & Explanation

**Status:** Released (v0.7.0)

Theme: *Explainable inference*

* Pattern engine + list/detect/explain
* Deterministic output with golden tests
* Stable pattern IDs

---

### v0.8 — Finding Enrichment

**Status:** Released (via v0.9.0)

Theme: *Richer findings without breaking contracts*

* Optional additive fields: confidence, refs, remediation
* Backwards-compatible JSON

---

### v0.9 — Pattern Prerequisites

**Status:** Released (v0.9.0–v0.9.2)

Theme: *Correctness before cleverness*

* Structured prerequisite system
* Skip semantics with explicit reasons

---

### v0.10 — Git-Aware Inference

**Status:** Released

Theme: *Optional git context as evidence*

* `--git-root` support
* Hybrid patterns (graph-only vs git-aware)
* Deterministic repo scanning

---

## v0.14 Line — Sharable Artifacts & Explainable Debugging

The v0.14 line marks a **second major arc** of the project: from static inspection to *explainable, shareable diagnostics*.

---

### v0.14.0 — Portable Outputs

**Status:** Released

* JSON + Markdown formats
* Deterministic v0.14 schema

---

### v0.14.1 — Delegated Apply Observability

**Status:** Released

* `gitops status` command
* Delegated apply backend detection
* Failure stage classification

---

### v0.14.2 — Guided Debug UX

**Status:** Released

* Guided GitOps Debug Mode (#37) ✅
* Container logs (#39) ✅
* Event timeline (#40) ✅

---

### v0.14.3 — Drift Detection (Core)

**Status:** Released

Theme: *What changed?*

* Drift JSON schema
* Comparator engine
* CI semantics (`--fail-on`)
* ASCII renderer as f(JSON) + g

---

### v0.14.4 — Drift Coverage Expansion

**Status:** Released

Theme: *More signal, same rails*

* Env var drift (#94)
* Resource requests/limits drift (#95)
* Image pull policy drift (#96)
* Complete docs and examples (#97)

---

### v0.14.5 — Drift ↔ Debug Correlation

**Status:** Released

Theme: *Why this broke*

* Correlation helpers (#98) ✅
* Narrative correlation in debug flows (#99) ✅

Correlation is explanatory only; no new JSON facts.

---

### v0.14.6 — Debug Bundle v1

**Status:** Released

Theme: *Debugging across time and people*

* Debug Bundle v1 packaging (#100) ✅
* Bundle inspect/replay (#101) ✅
* Bundle documentation (#102) ✅

Bundles are pure packaging of existing facts.

---

### Explainable Debugging Arc — Complete

**v0.14.3–v0.14.6** delivers the complete Explainable Debugging arc:

| Version | Capability |
|---------|------------|
| v0.14.3 | Drift detection (what changed?) |
| v0.14.4 | Drift coverage expansion |
| v0.14.5 | Drift ↔ debug correlation (why it broke) |
| v0.14.6 | Portable debug bundles (share and replay) |

All semantic contracts preserved throughout.

---

## v0.15 — Replay & Time-Series Reasoning

**Status:** Released

Theme: *What changed over time?*

This release **subsumes earlier "Graph & Export" ideas** (#35 ✅, #36 ✅, #38 ✅).

### v0.15.0 — Multi-Bundle Operations

**Status:** Released

* **Catalog v1** (`catalog.v1`): File-backed manifest for indexing multiple bundles
  * Explicit ordering (manifest/created_at/sequence)
  * Deterministic tie-break by ID
  * Portable directory-based structure
* **Pairwise Diff** (`bundle-diff.v1`): Compare two bundles
  * Join mode: object_id (composite/none deferred)
  * Per-object status: added/removed/changed/unchanged/ambiguous/unjoinable
  * Per-section summaries: drift, correlation
* **Timeline** (`bundle-timeline.v1`): N-bundle time-series view
  * Aligned points per bundle order
  * Gap detection (missing points are first-class)
  * Presence + per-point summaries (not full diffs)

Commands:
* `cub-scout catalog init/add/list/validate`
* `cub-scout bundle diff <A> <B> [--join ...] [--format ...]`
* `cub-scout bundle timeline <catalog> [--order ...] [--join ...] [--format ...]`

Key constraints:
* Read-only computation (no cluster/git access)
* New meaning = new schema (bundle-diff.v1, bundle-timeline.v1)
* No implicit ordering (never filesystem-based)
* ASCII = f(JSON) + g

Graphs become *views over artefacts*, not new semantic layers.

---

## v0.16 — Platform Composition & Attribution

**Status:** Released

Theme: *Why this change exists*

### v0.16.0 — Attribution Graph Foundation (PR1)

**Status:** Released

* **Attribution Graph v1** (`attribution-graph.v1`): Composition lineage schema
  * Node types: xr, mr, claim, composition, composition_revision, k8s_object, kustomize_overlay
  * Edge types: owns, selected_composition, selected_composition_revision
  * Evidence types: owner_reference, composite_label, claim_label, kustomize_overlay, spec refs
* Bundle integration: `attribution.json` as optional section
* Replay support: `bundle replay --section attribution`

### v0.16.1 — Debug Bundle Capture (PR2)

**Status:** Released

* `cub-scout debug ... --save-bundle <dir>` writes debug bundles
* Crossplane lineage automatically captured as attribution
* Test hooks for CI-safe testing (`CUB_SCOUT_TEST_TARGET_OBJECT`)
* Deterministic bundle directory naming

### v0.16.2 — Attribution Report (PR3)

**Status:** Released

* `attribution-report.v1` schema with scoring and rankings
* Reason codes: owned_via_owner_ref, owned_via_label, unattributed, ambiguous
* Replay support: `bundle replay --section attribution-report`

### v0.16.3 — Kustomize Overlay Attribution (PR4)

**Status:** Released

* `--kustomize <path>` flag for explicit overlay provenance
* `kustomize_overlay` evidence type (score=75)
* `owned_via_kustomize` reason code
* Deterministic graph merge for multiple attribution sources
* Non-Crossplane targets supported via `k8s_object` node type

### Completed Issues

* Crossplane ownership and lineage (#8) ✅
* Kustomize overlay attribution (#2) ✅

### Deferred

* Platform composition beyond Crossplane (kro) (#21)

---

## v0.17 — Stabilization Window

**Status:** Released (Non-semantic)

Theme: *Harden before workflows*

### Completed

* CI workflows (gofmt, lint, tests) ✅
* Performance benchmark scaffolding ✅
* Contract audit (schema immutability + golden determinism) ✅
* Epic cleanup and closure (#25) ✅

### Docs / Guardrails (Non-blocking)

* Resolver pattern documentation (#24)
* Performance & scale expectations (#22)
* Crossplane walkthrough demo (#23)

These are **polish**, not missing functionality.

---

## v0.18 — Connected Workflows

**Status:** Released

**Theme:** *Bring cub-scout to where decisions happen*

> This release fulfills the **original v0.2–v0.3 "why connected" intent**:
> not as an active controller, but as a **context-aware, read-only companion** embedded in real workflows.

### What "Connected" Means (and Does *Not* Mean)

**Connected means:**

* cub-scout fits naturally into **CI, PR review, and local debugging**
* outputs are **exportable, shareable, and inspectable**
* context travels with artefacts, not with running systems

**Connected does NOT mean:**

* mutating clusters
* applying changes
* acting as a controller or policy engine

cub-scout remains **read-only, explainable, and deterministic**.

### Delivered

* Artifact-first workflows (CI → bundle → replay)
* Git-aware, read-only workflows (context captured, not authoritative)
* Explicit fleet & environment views (catalog + diff)
* CI-backed demos and examples
* Zero new schemas or inference

cub-scout is now **operationally connected** while remaining offline-capable.

---

## v0.19 — TUI Polish

**Status:** Released

Theme: *Delight on top of substance*

* Canonical visual vocabulary (`symbols.go`) ✅
* TUI snapshot golden tests (#88) ✅
* Visual consistency and ordering (#90) ✅
* CLI ↔ TUI symmetry flags (#91) ✅
* Context-aware suggestions (#92) ✅
* Shell completion (#93) ✅

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

* **Explainability first** — every inference must be attributable
* **No silent contract expansion** — new meaning requires new surfaces
* **Determinism over convenience** — replay and diff always work
* **Artifacts over live dependencies** — bundles travel, clusters don't

---

## Issue Alignment Snapshot

* v0.14.5: #98 ✅, #99 ✅
* v0.14.6: #100 ✅, #101 ✅, #102 ✅
* v0.15.0: #35 ✅, #36 ✅, #38 ✅, #40 ✅
* v0.16.0–v0.16.3: #2 ✅, #8 ✅
* v0.17: #25 ✅ | Deferred: #21, #22, #23, #24
* v0.19: #88 ✅, #90 ✅, #91 ✅, #92 ✅, #93 ✅

Issues remain the execution unit; versions define intent.
