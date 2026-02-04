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

* Guided GitOps Debug Mode (#37)
* Container logs (#39)
* Event timeline (#40)

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

* Correlation helpers (#98)
* Narrative correlation in debug flows (#99)

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

**Status:** Planned

Theme: *What changed over time?*

This release **subsumes earlier "Graph & Export" ideas** (#35, #36, #38).

* Replay debug bundles
* Compare snapshots (before/after)
* Time-series reasoning

Graphs become *views over artefacts*, not new semantic layers.

---

## v0.16 — Platform Composition & Attribution

**Status:** Planned

Theme: *Why this change exists*

* Kustomize overlay attribution (#2)
* Crossplane ownership and lineage (#8, #21)
* Platform composition tools (kro) (#22)

---

## v0.17 — Stabilization Window

**Status:** Planned

Theme: *Harden before workflows*

* Performance and scale guardrails (#23)
* Resolver pattern documentation (#24)
* Epic cleanup and closure (#25)

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

* v0.14.5: #98, #99
* v0.14.6: #100–#102
* v0.16: #2, #8, #21–#22
* v0.19: #88–#93

Issues remain the execution unit; versions define intent.
