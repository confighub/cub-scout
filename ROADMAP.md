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

## v0.6 — Graph Foundation (Active)

**Status:** In progress (Track G)

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

## v0.7 — Pattern Detection & Explanation

**Status:** Planned

* Detect common GitOps patterns (App-of-Apps, ApplicationSet, Flux tenancy, mono-repo, Helm umbrella)
* Evidence-backed, confidence-scored explanations
* New CLI surfaces:
  * `patterns list`
  * `patterns detect --json`
  * `patterns explain`

---

## v0.8 — Git-Aware Inference

**Status:** Planned

* Optional Git context to improve explanations
* Overlay/variant inference
* ApplicationSet generator interpretation

---

## v0.9 — ConfigHub Collaboration

**Status:** Planned

* Connected mode enrichment via ConfigHub
* Cluster ↔ Git intent correlation
* Foundation for fleet-scale analysis

---

## v1.0 — Fleet Intelligence & Stability

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
* [docs/reference/gitops-patterns.md](docs/reference/gitops-patterns.md) — GitOps patterns conceptual reference
