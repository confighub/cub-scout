# cub-scout Roadmap

## Explorer and Debugger for GitOps Systems

**Last Updated:** 2026-02-01

This roadmap describes the planned evolution of cub-scout across standalone and connected modes.

cub-scout follows three guiding principles:
- Read-only by default
- Explorer and debugger, not controller
- Clear separation between live reality and external intent

For archived roadmap items, see [archive/old-roadmap-jan.md](archive/old-roadmap-jan.md).

---

## Version 0.5 — Standalone Explorer and Debugger (Current Focus)

**Status:** In progress
**Epic:** [#25](https://github.com/confighub/cub-scout/issues/25)
**Audience:** Individual engineers, SREs, platform teams

### Core Capabilities

- Single-cluster exploration
- Ownership and provenance tracing
- Delegated apply visibility (Flux / Argo via OCI)
- Failure-stage explanation (source vs apply)
- GitOps drift detection (controller-based)
- Guided GitOps Debug Mode
- Exportable ownership/dependency graph (JSON, DOT)
- Shareable diagnostic snapshots
- TUI with `:` shell-out and CLI awareness

### v0.5 Issues

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
| #35 | Ownership graph schema | Export |
| #36 | Graph export (JSON + DOT) | Export |
| #37 | Guided GitOps Debug Mode | Education |
| #38 | Shareable snapshots | Education |

### Non-goals for v0.5

- No desired-state rendering
- No Git diffs
- No policy enforcement
- No fleet views

v0.5 establishes cub-scout as a trustworthy, production-safe GitOps explorer and debugger.

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

## Summary

cub-scout evolves along two deliberate axes:

- **Depth (Standalone):** Better exploration and debugging of live state
- **Breadth (Connected):** Intent, history, fleets, and impact via ConfigHub

This separation keeps cub-scout safe, focused, and valuable at every stage.
