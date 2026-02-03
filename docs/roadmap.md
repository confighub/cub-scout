# cub-scout Roadmap

## Explorer and Debugger for GitOps Systems

**Last Updated:** 2026-02-03

This roadmap describes the planned evolution of cub-scout across standalone and connected modes.

cub-scout follows three guiding principles:
- Read-only by default
- Explorer and debugger, not controller
- Clear separation between live reality and external intent

For archived roadmap items, see [archive/old-roadmap-jan.md](archive/old-roadmap-jan.md).

---

## Recent Releases

### Version 0.14 — Sharable Artifacts & Portable Outputs (Current)

**Status:** Released (2026-02-03)
**Theme:** JSON is the complete truth; ASCII/MD are projections

**Delivered:**
- `--format json` for tree, trace, and map list commands
- `--format md` (Markdown) for all output commands
- v0.14 JSON schema with determinism guarantees
- Golden tests locking all output formats

**Schema guarantees:**
- Deterministic (same input = same output)
- Lossless (display limits are metadata, not data loss)
- Joinable (canonical `id` objects for cross-reference)
- No timestamps by default

### Version 0.13 — CLI ↔ TUI Symmetry

**Status:** Released
**Theme:** Polished interaction layer

**Delivered:**
- TUI polish and keyboard navigation
- CLI/TUI feature parity
- Improved user experience

### Version 0.12 — Tree & Trace Contracts (ASCII Locked)

**Status:** Released
**Theme:** Lock user-facing ASCII output

**Delivered:**
- Golden tests for tree, trace, map list, map status, scan, orphans
- Test hook infrastructure for deterministic testing
- GitOps hierarchies documentation

---

## Version 0.14.x — Incremental Improvements (Current Focus)

**Status:** In progress
**Audience:** Individual engineers, SREs, platform teams

Future 0.14.x releases may include:

| Feature | Description |
|---------|-------------|
| `--format yaml` | kubectl-compatible output |
| `graph export --format dot` | Graphviz visualization |
| Snapshot format | Bundled JSON + metadata |
| TUI snapshot goldens | #88 from v0.12 backlog |

---

## Version 0.15+ — Standalone Explorer and Debugger

**Status:** Planned
**Epic:** [#25](https://github.com/confighub/cub-scout/issues/25)
**Audience:** Individual engineers, SREs, platform teams

### Core Capabilities (from original v0.5 epic)

- Delegated apply visibility (Flux / Argo via OCI)
- Failure-stage explanation (source vs apply)
- GitOps drift detection (controller-based)
- Guided GitOps Debug Mode
- Shareable diagnostic snapshots
- **Offline replay** of snapshots (first-class workflow)

### Offline Mode

Offline replay of snapshots is a **first-class supported workflow**:

- Incident review without cluster access
- Onboarding with real examples
- Security-restricted environments
- Use `cub-scout snapshot view <file>` to replay any snapshot

### Open Issues

| # | Title | Scope |
|---|-------|-------|
| #26 | OCI GitOps fixtures | Foundation |
| #27 | Flux sourceRef parsing | Foundation |
| #28 | Delegated apply detector | Foundation |
| #29 | Flux source failure visibility | Failure Explanation |
| #30 | Flux apply failure visibility | Failure Explanation |
| #31 | Argo operation visibility | Failure Explanation |
| #32 | Delegated Apply summary panel | Failure Explanation |
| #33 | Drift detection | Drift |
| #34 | Drift UI + CLI | Drift |
| #37 | Guided GitOps Debug Mode | Education |
| #38 | Shareable snapshots | Education |

### Non-goals

- No desired-state rendering
- No Git diffs
- No policy enforcement
- No fleet views

**Why OCI-first:** ConfigHub treats OCI publishing/consumption as a core transport. Git transport is intentionally deferred.

---

## Version 0.6 — Deep Debugging + Connected Mode Foundations

**Status:** Planned
**Audience:** Teams using ConfigHub

### Deep Debugging (Standalone)

Extends v0.5 debug mode with Kubernetes-native insights:

| # | Title | Description |
|---|-------|-------------|
| #39 | Container logs in debug mode | View crash logs with pattern detection |
| #40 | Event timeline | See what happened recently with explanations |

### Connected Mode Foundations

First integration with ConfigHub:

- ConfigHub authentication and connection
- Target / space / revision context
- Intended vs actual comparisons
- History-aware debugging:
  - "When did this break?"
  - "What changed since last healthy?"
- Intent-aware CLI suggestions in TUI

### ConfigHub Backend (Already Exists)

These Connected Mode features are powered by **existing ConfigHub engines**:

| Engine | Powers |
|--------|--------|
| **ChangeSets API** | Revision-aware views, "what changed" queries |
| **Views API** | Composable filters/projections (matches cub-scout lenses) |

cub-scout surfaces results from these engines — it does not reimplement them.

### Notes

- cub-scout remains read-only
- All intent and history lives in ConfigHub
- Graceful degradation when disconnected

---

## Version 0.7 — Fleet & Impact Intelligence (Connected)

**Status:** Planned
**Audience:** Platform and infrastructure teams

### Capabilities

- Fleet-wide health views
- Cross-cluster comparisons
- Version skew detection
- Outlier identification ("this cluster is the weird one")
- Impact analysis before changes
- Dependency blast radius analysis

### ConfigHub Backend (Already Exists)

| Engine | Powers |
|--------|--------|
| **Dependency Graph Engine** | Impact analysis, blast radius, topological ordering |
| **Bridge/Worker Framework** | Fleet-wide visibility across targets |

cub-scout queries these engines — it does not reimplement dependency resolution.

---

## Version 0.8 — Governance & Collaboration Context (Connected)

**Status:** Exploratory

### Capabilities

- Policy evaluation context (read-only)
- Approval and gate visibility
- Audit-friendly timelines
- Shared, persistent debugging artifacts

### ConfigHub Backend (Already Exists)

| Engine | Powers |
|--------|--------|
| **Verifier Component** | Policy evaluation, validation outcomes |
| **Helm Rendering** | Worker-side HelmRelease logic (Flux-oriented) |

cub-scout surfaces governance outcomes — validation/policy belongs in ConfigHub.

---

## Ongoing: UX & Performance Improvements

These improvements may land in any release:

| Feature | Description |
|---------|-------------|
| Split panes | View multiple things simultaneously |
| Command palette | Fuzzy-search all actions |
| Quick terminal | Quake-style overlay for quick lookups |
| Inspector panel | Raw API responses and timing |
| Large-cluster performance | 1000+ resource handling |
| Session persistence | Save view preferences between sessions |
| `cub-scout learn` | Interactive GitOps learning with live examples |
| JSON output consistency | `--json` flag for all commands |
| Exit codes | Consistent codes for CI/CD scripting |

UX improvements must preserve:
- Read-only guarantees
- CLI discoverability
- Explorer + debugger identity

---

## Free vs Connected

| Capability | Standalone (Free) | Connected (Paid) |
|------------|-------------------|------------------|
| Single-cluster exploration | Yes | Yes |
| Ownership tracing | Yes | Yes |
| Failure explanation | Yes | Yes |
| Drift detection | Yes | Yes |
| Debug mode | Yes | Enhanced |
| Graph export | Yes | Yes |
| Snapshots | Yes | Yes |
| Intent awareness | — | Yes |
| History & time | — | Yes |
| Fleet views | — | Yes |
| Impact analysis | — | Yes |
| Git-aware navigation | — | Yes |
| Governance context | — | Yes |

See [WHY_CONNECTED_MODE.md](WHY_CONNECTED_MODE.md) for detailed value proposition.

---

## Future: CSV DICS (Declarative Intent via Flat Data)

**Status:** Exploratory / Post-v0.5
**Audience:** Small teams, early GitOps adopters, migration use cases

CSV DICS is a proposed future feature that allows users to manage **simple, literal configuration intent** using flat data formats (CSV), without introducing a full templating or reconciliation system.

This feature is intentionally positioned as:
- a **discovery and simplification tool**
- an **on-ramp to ConfigHub**
- not a replacement for Helm, GitOps controllers, or ConfigHub itself

---

### Problem It Addresses

Many clusters start with:
- ad-hoc manifests
- copy-pasted YAML
- light variations across namespaces or environments

For these cases:
- Helm can feel too heavy
- full ConfigHub adoption may feel premature
- yet teams still want configuration expressed as *data*, not duplicated YAML

---

### Proposed Capabilities

CSV DICS would allow users to:

- **Export observed cluster state to CSV**
  - One row per logical resource
  - Flat, literal fields only (no templating)
- **Diff CSV vs live cluster**
  - Identify additions, removals, and value changes
- **Render CSV back to literal Kubernetes manifests**
  - Deterministic output
  - No loops, conditionals, or inheritance
- **Manage CSV in Git**
  - Treat CSV as a simple declarative source of truth

All apply actions remain external (kubectl, Flux, Argo, or ConfigHub worker).

---

### Explicit Non-Goals

CSV DICS must **not**:
- Become a templating language
- Introduce reconciliation or controllers
- Apply changes automatically
- Replace Helm or Kustomize
- Compete with ConfigHub's intent model

CSV values are literal and explicit by design.

---

### Relationship to Existing cub-scout Features

- **Complementary to Shareable Views**
  - Shareable Views explain *what happened*
  - CSV DICS helps simplify *what exists*
- **Built on hierarchy maps**
  - CSV export is derived from resource / ownership views
- **Standalone by default**
  - No ConfigHub required to use CSV DICS

---

### Graduation Path to ConfigHub

CSV DICS is explicitly a **Level-1 intent system**.

When teams need:
- richer validation
- history and timelines
- fleet-wide consistency
- impact analysis
- governance and policy

...they should graduate from CSV DICS to **ConfigHub**, which provides a full system of record for intent.

CSV DICS should make this transition easier, not harder.

---

### Roadmap Placement

CSV DICS is intentionally **out of scope for v0.5**.

Potential placement:
- v0.6+ as an experimental feature, or
- gated behind an explicit `--experimental` flag

Implementation should proceed only after:
- Shareable Views are stable
- Standalone explorer/debugger workflows are proven

---

## Summary

cub-scout evolves along two deliberate axes:

- **Depth (Standalone):** Better exploration and debugging of live state
- **Breadth (Connected):** Intent, history, fleets, and impact via ConfigHub

This separation keeps cub-scout safe, focused, and valuable at every stage.
