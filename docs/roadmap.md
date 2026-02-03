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

**Status:** v0.14.1 released (2026-02-03)
**Audience:** Individual engineers, SREs, platform teams

### v0.14.1 — Delegated Apply Observability (Released)

**Delivered:**
- `cub-scout gitops status` command
- Delegated apply backend detection (Flux/Argo/Worker)
- Failure stage classification (source, build, apply, sync)
- OCI GitOps fixtures for testing
- SourceRef parsing for Flux deployers
- Flux and Argo failure details extraction

### v0.14.2 — Debug/Trace (Planned)

**Theme:** Guided GitOps debugging

| # | Title | Description |
|---|-------|-------------|
| #2 | Kustomize overlay layer attribution | Trace through overlay layers |
| #37 | Guided GitOps Debug Mode | Interactive debugging workflow |
| #39 | Container logs in debug mode | View crash logs with pattern detection |
| #40 | Event timeline | See what happened recently with explanations |

### v0.14.3 — Drift Detection (Planned)

**Theme:** Detect when live state differs from desired

| # | Title | Description |
|---|-------|-------------|
| #33 | Detect GitOps drift | kubectl smell detection |
| #34 | Drift UI badges and CLI | Surface drift in TUI and CLI |

---

## Version 0.15 — Graph & Export

**Status:** Planned
**Theme:** Unified graph schema and shareable artifacts

| # | Title | Description |
|---|-------|-------------|
| #35 | Define unified internal graph schema | Foundation for all graph operations |
| #36 | Export ownership graph (JSON + DOT) | Graphviz visualization |
| #38 | Shareable hierarchy map snapshots | Bundled JSON + metadata |

### Capabilities

- Unified graph schema for all resource relationships
- Export to JSON and DOT (Graphviz) formats
- Shareable diagnostic snapshots
- **Offline replay** of snapshots (first-class workflow)

### Offline Mode

Offline replay of snapshots is a **first-class supported workflow**:

- Incident review without cluster access
- Onboarding with real examples
- Security-restricted environments
- Use `cub-scout snapshot view <file>` to replay any snapshot

---

## Version 0.16 — Crossplane & Platform Composition

**Status:** Planned
**Theme:** First-class support for platform composition tools

| # | Title | Description |
|---|-------|-------------|
| #3 | Support platform composition tools | Crossplane, kro |
| #8 | First-class Crossplane ownership & lineage | XR-first, system resource classification |
| #21 | Platform composition beyond Crossplane | kro support |
| #22 | Performance & scale guardrails | 1000+ resource handling |
| #23 | Crossplane walkthrough demo | Documentation |
| #24 | Document resolver pattern | Generated resources |
| #25 | v0.5 Epic closure | GitOps Explorer and Debugger complete |

### Capabilities

- Crossplane XR ownership detection
- Claim → XR → Managed Resource lineage
- System resource classification
- Performance guardrails for large clusters
- kro support (exploratory)

---

## Version 0.17 — TUI Polish

**Status:** Planned
**Theme:** Consistent, polished terminal experience

| # | Title | Description |
|---|-------|-------------|
| #88 | TUI snapshot golden tests | Lock TUI output |
| #90 | TUI polish: consistent symbols/ordering | Match CLI output |
| #91 | CLI ↔ TUI symmetry flags | --owner, --depth, --from |
| #92 | Context-aware command suggestions | Read-only panel |
| #93 | Shell-out with cub completion | Integration with cub CLI |

---

## Version 0.18 — Documentation

**Status:** Planned
**Theme:** Complete, consistent, navigable documentation

### Scope

| Area | Work |
|------|------|
| **CLI Reference** | Ensure all commands documented |
| **Contract Docs** | Document all version contracts |
| **Golden Tests** | All user-facing output covered |
| **Examples** | Update with current syntax |
| **Demos** | Verify all demos work |
| **Navigation** | Cross-references between docs |
| **Consistency** | Align terminology |

---

## Connected Mode (Future)

**Status:** Planned (post-v0.18)
**Audience:** Teams using ConfigHub

### Capabilities

- ConfigHub authentication and connection
- Target / space / revision context
- Intended vs actual comparisons
- History-aware debugging
- Fleet-wide health views
- Cross-cluster comparisons
- Impact analysis before changes

### ConfigHub Backend

These features are powered by **existing ConfigHub engines**:

| Engine | Powers |
|--------|--------|
| **ChangeSets API** | Revision-aware views, "what changed" queries |
| **Views API** | Composable filters/projections |
| **Dependency Graph Engine** | Impact analysis, blast radius |
| **Bridge/Worker Framework** | Fleet-wide visibility |

cub-scout surfaces results from these engines — it does not reimplement them.

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
