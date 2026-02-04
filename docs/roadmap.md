# cub-scout Roadmap

> **Audience:** humans and AI contributors (Claude, Codex, reviewers)
> **Purpose:** single, authoritative forward plan that subsumes prior roadmap documents and scattered issues.

This roadmap reflects the current *shipped reality* (v0.14.3), the locked semantic contract, and an aggressive but disciplined execution plan through v0.16.

**Last Updated:** 2026-02-04

---

## Guiding Principles (Locked)

These principles are not up for renegotiation unless explicitly revised.

1. **Single semantic model**
   All outputs derive from one internal model.

2. **Complementary authorities**
   - JSON → structural facts (machine authority)
   - ASCII → f(JSON) + g (human authority)

3. **Leak Test invariant**
   If removing ASCII narrative would change machine behavior, meaning has leaked and must be fixed.

4. **Determinism first**
   Outputs must be replayable, diffable, and golden-tested.

5. **Capability before polish**
   UI/TUI polish only happens once artefacts and correlations are rich.

These principles govern *all* work below.

---

## Current State (Ground Truth)

### v0.14.3 — Drift Detection (RELEASED)

**Status:** Shipped (2026-02-04)

**Delivered:**
- Drift JSON schema + deterministic ordering
- Drift comparator engine (replicas, images)
- CI semantics via `--fail-on` (JSON-driven)
- ASCII drift renderer as f(JSON)+g

**Contracts enforced:**
- R1–R6 semantic rules
- Leak Test (exit behavior independent of ASCII)

**Value unlocked:**
- First-class drift detection
- Automation-ready facts
- Human-readable explanations

---

## v0.14.x Theme — Drift as Diagnostic & Shareable Truth

v0.14.x is about *exploiting* drift detection, not redesigning it.

Focus:
- Expand drift **coverage**
- Correlate drift with **debugging signals**
- Make drift and debugging **shareable artefacts**
- Finish with complete docs, tests, examples, and demos

No new semantic authorities. No architectural churn.

---

## v0.14.4 — Drift Coverage Expansion

**Theme:** More signal, same rails

### Scope (high leverage, mechanical)

Add new drift types using the existing comparator → JSON → ASCII pipeline.

Planned drift types:

1. **Environment variable drift**
   - name → desired/live
   - classification: config
   - severity: warning

2. **Resource requests / limits drift**
   - cpu / memory
   - classification: capacity
   - severity: warning / critical (invalid configs)

3. **Image pull policy drift**
   - Always / IfNotPresent / Never
   - classification: rollout

4. *(Optional)* **Restart / probe configuration drift**

### Non-goals

- No new flags
- No new exit semantics
- No TUI changes

### Deliverables

- Comparator extensions + tests
- JSON schema updates
- ASCII renderer automatically reflects new drift types
- Docs + examples updated in lockstep

### PR Plan

1. **PR1** — Drift comparator extensions (env vars, resources)
2. **PR2** — Drift comparator extensions (image pull policy)
3. **PR3** — ASCII renderer auto-reflects new drift (goldens)
4. **PR4** — JSON schema + reference docs
5. **PR5** — User docs + examples

Each PR must pass Semantic Safety Checklist.

---

## v0.14.5 — Drift ↔ Debug Correlation

**Theme:** Why this broke

This release connects drift facts to existing debugging signals (logs, events).

### Core idea

Every drift finding can be correlated with logs and events via shared object identity.

### Scope

- Link drift findings to:
  - container logs
  - Kubernetes events
- Debug flows can answer:
  - "show drift for this object"
  - "show logs/events for objects with critical drift"
  - "this failure occurred with no drift" (important signal)

### Semantics

- Correlation statements are **narrative only** (ASCII/TUI)
- No new JSON meaning required
- Correlation helpers are join utilities over existing JSON facts; they introduce no new semantic meaning

### Correlation Helpers (Implementation)

Pure functions in `pkg/agent/` that:
- Join `DriftFinding.object_id` with log entries and events
- Return **references**, not new facts

Examples:
- `FindingsForObject(objectID string) []DriftFinding`
- `EventsForFindings(findings []DriftFinding) []Event`
- `LogsForFindings(findings []DriftFinding) []LogEntry`

Constraints:
- No new JSON schema
- No new fields added to findings
- Correlation results are render-time only
- All conclusions remain narrative (ASCII/TUI)

### Deliverables

- Correlation helpers (joins on object_id)
- Narrative explanations in debug output
- Tests proving correlation does not invent facts
- Docs explaining correlation semantics

---

## v0.14.6 — Shared Debug Artefacts (Debug Bundle v1)

**Theme:** Debugging across time and people

### Goal

Make debugging reproducible, portable, and asynchronous.

### Concept: Debug Bundle

A Debug Bundle is a structured snapshot containing drift and debugging context.

It introduces **no new semantics** — only packaging and replay.

---

## Debug Bundle v1 — Specification

### Bundle Layout

```
debug-bundle/
├─ metadata.json
├─ drift.json
├─ events.json
├─ logs.json
└─ README.md
```

### Contents

- **metadata.json**
  - cub-scout version
  - bundle format version
  - optional label (env, incident id)

- **drift.json**
  - v0.14.3+ drift report (canonical facts)

- **events.json**
  - structured Kubernetes events (existing schema)

- **logs.json**
  - structured container logs + detected patterns

- **README.md** (generated)
  - what this bundle contains
  - how to inspect it
  - semantic guarantees

### Target Metadata

Debug Bundles may include existing target metadata (e.g., ConfigHubTarget) if already emitted by cub-scout. Bundles do not introduce new target semantics.

### Commands

- `cub-scout debug bundle --out <dir|tar>`
- `cub-scout debug inspect <bundle>`

ASCII and TUI views render *from the bundle*, not the cluster.

### Renderer Source Abstraction

ASCII/TUI renderers will accept a generic data source (cluster or bundle). This is a mechanical refactor; no rendering logic or semantics change.

### Guarantees

- Replayable
- Deterministic
- Shareable
- No hidden meaning

---

## Documentation Skeleton

To be filled as features land:

```
docs/
├─ semantic-contract.md        # DONE (anchor)
├─ roadmap.md                  # THIS DOCUMENT
├─ drift.md                    # what drift is, how to use it
├─ ci-integration.md           # JSON + --fail-on
├─ debugging.md                # logs, events, correlation
├─ debug-bundles.md            # bundle spec + walkthrough
└─ reference/
   ├─ json-schema.md
   ├─ exit-codes.md
   ├─ severity-taxonomy.md
   └─ bundle-schema.md         # Debug Bundle v1 schema reference
```

Examples / demos live alongside docs.

---

## v0.15 — Replay & Time-Series Reasoning

**Status:** Planned (after v0.14.x complete)
**Theme:** Compare debug bundles over time

### Scope

- Compare debug bundles over time
- Before/after drift analysis
- Time-series reasoning

### Graph & Export Issues (#35, #36, #38)

These older "Graph & Export" issues are **not separate initiatives**.

They are **realized through replayable Debug Bundles** and time-series comparison built on those bundles. Visualization (graphs) becomes a *view* over replayed artefacts, not a new semantic layer.

**Status:** Deferred → subsumed by v0.15 Replay & Comparison

---

## v0.16 — Kustomize Overlay Attribution (#2)

**Status:** Planned
**Theme:** Attribution answers "this overlay caused this drift"

Reintroduced with real value context now that drift detection is complete.

### Parked Issues

| # | Title | Rationale |
|---|-------|-----------|
| #2 | Kustomize overlay layer attribution | Needs drift as foundation; now unblocked |
| #3 | Platform composition (kro) | API not stable; park until v0.17+ |

---

## v0.18 — Connected Workflows

**Status:** Planned (after v0.16)
**Theme:** Operate with cub-scout

This is the "operate with cub-scout" release — workflows that connect cub-scout to external systems.

### Scope

- **Import / inspect artefacts** — load external debug bundles, snapshots
- **Write / export outputs** — generate patches, reports, structured exports
- **Git workflows** — PR context, patch generation, commit attribution
- **Fleet view** — multi-cluster / multi-namespace rollups

### Rationale

Connected workflows only make sense after:
- Artefacts exist (v0.14.6 bundles)
- Replay works (v0.15)
- Attribution is available (v0.16)

---

## v0.19 — TUI Polish

**Status:** Planned (after v0.18)
**Theme:** The TUI becomes delightful because the underlying model is already powerful

### Rationale

TUI polish is **presentation leverage**, not capability leverage.

Only after v0.14–v0.18 does TUI polish make sense, because then:
- The TUI has **rich artefacts to browse**
- It can navigate **fleet, history, attribution**
- It becomes a *window over substance*, not a substitute for it

UI serves substance, not the other way around.

### Parked Issues

| # | Title |
|---|-------|
| #34 | Drift UI badges (optional polish) |
| #88 | TUI snapshot golden tests |
| #90 | TUI polish: consistent symbols/ordering |
| #91 | CLI ↔ TUI symmetry flags |
| #92 | Context-aware command suggestions |
| #93 | Shell-out with cub completion |

---

## Summary

cub-scout is transitioning from *capability creation* to *capability exploitation*.

v0.14.x completes drift as:
- factual (JSON)
- automatable (CI)
- explainable (ASCII)
- debuggable (correlation)
- shareable (debug bundles)

Future releases build on this foundation without reopening semantics.
