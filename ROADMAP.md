# cub-scout Roadmap

## v0.5 — Contract-Locked CLI (Released)

**Status:** Released (`v0.5.0`)

v0.5 establishes cub-scout as a **trustworthy, script-safe CLI** with explicit contracts backed by golden tests.

### Guaranteed CLI surfaces

* `trace`
* `map status`
* `scan --file`
* `map list --json`
* `map deployers --json`

For these commands:

* Output formats are stable and deterministic
* Exit codes are consistent
* JSON schemas are locked
* Documentation matches behavior exactly

> v0.5 is sealed. Changes to these contracts require a future minor version.

---

## v0.6 — Graph Foundation (Complete)

**Status:** Complete (Track G)

### Completed
- `graph export --json` with schema v1 (#59)
- K8s ownership chain collectors (#60, PR #63)
- GitOps CRDs as nodes (#61, PR #64)
- `graph explain` command (#62, PR #65)

### Goal

Introduce a first-class **resource graph** that captures cluster-visible relationships and supports GitOps-aware reasoning without changing v0.5 contracts.

### Scope

* Graph data model (nodes, edges, evidence)
* Cluster-first ingestion:
  * Core Kubernetes resources
  * GitOps CRDs (Argo CD, Flux) *when present*
* New CLI surfaces (versioned):
  * `graph export --json`
  * `graph explain <resource>`

### Non-goals

* No changes to existing v0.5 CLI commands
* No pattern classification yet
* No Git parsing or external service integration

---

## v0.7 — Pattern Detection & Explanation (Released)

**Status:** Released (`v0.7.0`) (Track H)

### Completed
- Pattern engine foundation (`internal/patterns` package)
- `patterns list` command
- `patterns detect` command with text and JSON output
- `patterns explain` command
- MVP patterns:
  - `k8s.ownership_chain_complete`: Checks Deployment → ReplicaSet → Pod chains
  - `gitops.controller_presence`: Detects Argo CD and Flux controller CRDs
- Deterministic output with golden tests
- Contract documentation (`docs/reference/patterns-contract.md`)

> v0.7 pattern IDs and core output formats are stable. Additive fields may be added in future versions.

---

## v0.8 — Finding Enrichment (Released)

**Status:** Released (`v0.8.0` content in `v0.9.0`) (Track I)

### Completed

- Additive optional fields on findings:
  - `confidence`: deterministic score (0.0–1.0)
  - `refs`: stable resource identifiers for correlation
  - `remediation`: structured guidance (summary, steps, links)
- Text rendering of remediation blocks
- Backwards-compatible JSON (omitempty)
- Remediation wired to MVP patterns:
  - `gitops.controller_presence`: Guidance for installing Argo CD or Flux
  - `k8s.ownership_chain_complete`: Steps for investigating orphaned resources

> v0.8 features shipped as part of v0.9.0 release due to combined merge.

---

## v0.9 — Pattern Prerequisites (Released)

**Status:** Released (`v0.9.0`) (Track J)

### Completed

- Pattern prerequisite system:
  - `requires_node_kind`: Graph must contain specific node kind
  - `requires_any_of_kinds`: Graph must contain any of listed kinds
  - `skip_reason`: Structured reason when prerequisites unmet
- Prerequisites wired to `k8s.ownership_chain_complete` pattern
- Contract documentation updated

> v0.9 pattern prerequisites are stable. Additive prerequisite types may be added in future versions.

---

## v0.9.x — Stabilization Window (Complete)

**Status:** Complete (`v0.9.1`, `v0.9.2`)

### Completed

- v0.9.1: Roadmap alignment and release notes cleanup
- v0.9.2: GitOps interpretive patterns:
  - `gitops.argocd.resources_present`: Argo CD resource counts
  - `gitops.flux.resources_present`: Flux resource counts

### Goal

Polish and harden v0.9 before expanding scope. No new features beyond minor pattern coverage gaps.

### Scope

* Wording polish and documentation alignment
* Pattern coverage gaps (1–2 GitOps interpretive patterns)
* Contract clarity improvements
* Release notes cleanup

### Non-goals

* No new prerequisite types
* No hierarchical GitOps patterns
* No Git parsing or external service integration

---

## v0.10 — Git-Aware Inference (Complete)

**Status:** Complete (Track K)

### Completed

- `--git-root <path>` flag for `patterns detect` and `patterns explain` (PR #76, #77)
- `internal/gitctx` package for deterministic repo scanning
- Determinism constraints: bounded scan, no network, lexicographic ordering, max-file cap
- Pattern type system: Graph-only, Hybrid, Git-aware
- Skip semantics for git-aware patterns when git-root absent/invalid
- Hybrid patterns: reduced evidence without `--git-root`, enriched with valid `--git-root`
- MVP Hybrid patterns (PR #78):
  * `gitops.argocd.applicationset_generators`: ApplicationSet generator summary
  * `gitops.flux.kustomization_paths`: Flux Kustomization path correlation
- Testdata fixtures (`testdata/repo-argocd/`, `testdata/repo-flux/`)
- Updated goldens for patterns list/detect/explain

### Key Principle

**Git context is optional evidence.** All existing patterns continue to work without `--git-root`.
Hybrid patterns run with reduced evidence when `--git-root` omitted, SKIP when provided but invalid.

> v0.10 git-aware pattern semantics are stable. Additional hybrid/git-aware patterns may be added in future versions.

---

## v0.11 — ConfigHub Collaboration

**Status:** Planned

* Connected mode enrichment via ConfigHub
* Cluster ↔ Git intent correlation
* Foundation for fleet-scale analysis

---

## v1.0+ — Fleet Intelligence & Stability

**Status:** Future

* Cross-cluster correlation
* Drift grouping and rollout insights
* Stable long-lived schemas and deprecation policy

---

## Guiding Principles

* **Signal ≠ Contract**: cub-scout may understand more than it exposes at any given version
* **Explainability first**: every inference must be attributable
* **No silent contract expansion**: new capabilities require new surfaces

---

## See Also

* [docs/roadmap.md](docs/roadmap.md) — Detailed v0.5 epic and issue tracking
* [docs/reference/cli-contract.md](docs/reference/cli-contract.md) — v0.5 CLI contract reference
* [docs/reference/graph-contract.md](docs/reference/graph-contract.md) — v0.6 graph contract reference
* [docs/reference/patterns-contract.md](docs/reference/patterns-contract.md) — v0.7+ patterns contract reference
* [docs/reference/gitops-patterns.md](docs/reference/gitops-patterns.md) — GitOps patterns conceptual reference
