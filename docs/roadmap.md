# cub-scout Unified Roadmap

> **Status:** Authoritative
>
> This document **replaces and subsumes**:
>
> * `/ROADMAP.md`
> * `/docs/roadmap.md`
> * scattered milestone notes and implicit issue groupings
>
> It reconciles *historical releases*, *current shipped reality*, and the *forward execution plan* through v0.19.

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

**Status:** Complete

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

**Status:** Complete

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

All semantic contracts preserved throughout. Ready for v0.15.

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
* Performance & scale guardrails (#22)

---

## v0.17 — Stabilization Window

**Status:** In Progress

Theme: *Harden before workflows*

### Completed

* CI workflows (gofmt, lint, tests) ✅
* Performance benchmark scaffolding ✅
* Contract audit (schema immutability + golden determinism) ✅
* Epic cleanup and closure (#25) ✅

### Remaining

* Performance and scale guardrails (#22)
* Resolver pattern documentation (#24)
* Crossplane walkthrough demo (#23)

---

## v0.18 — Connected Workflows

**Status:** Planned

Theme: *Operate with cub-scout*

This release restores and completes the **workflow layer** that was always implied:

* Import / inspect artefacts
* Write / export outputs
* Git workflows (patches, PR context)
* Fleet view (multi-cluster / multi-namespace)

---

## v0.19 — TUI Polish

**Status:** Planned

Theme: *Delight on top of substance*

* TUI snapshot goldens (#88)
* Visual consistency and ordering (#90)
* CLI ↔ TUI symmetry flags (#91)
* Context-aware suggestions (#92)
* Shell-out completion (#93)

TUI polish is intentionally last.

---

## v1.0+ — Fleet Intelligence & Stability

Theme: *Long-lived trust*

* Cross-cluster correlation
* Rollout intelligence
* Stable schemas and deprecation policy

---

## Guiding Principles (Still True)

* **Explainability first** — every inference must be attributable
* **No silent contract expansion** — new meaning requires new surfaces
* **Determinism over convenience** — replay and diff always work

---

## Issue Alignment Snapshot

* v0.14.5: #98 ✅, #99 ✅
* v0.14.6: #100 ✅, #101 ✅, #102 ✅
* v0.15.0: #35 ✅, #36 ✅, #38 ✅, #40 ✅
* v0.16.0–v0.16.3: #2 ✅, #8 ✅
* v0.17: #25 ✅, #21, #22, #23, #24
* v0.19: #88, #90, #91, #92, #93

Issues remain the execution unit; versions define intent.
