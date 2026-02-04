# cub-scout Semantic Contract

**Purpose**
This document defines the meaning contract between cub-scout's internal semantic model and its external outputs (ASCII, JSON, and optional YAML). It is written to be understandable by both humans *and* machines, and to remain evolvable over time.

This contract exists to prevent semantic drift, accidental dual sources of truth, and brittle automation as cub-scout grows (e.g., drift detection, CI integration, replay).

---

## Core Model

cub-scout produces a **single semantic model** describing system structure and state.

From that model, multiple renderings are produced for different obligations.

We name this explicitly:

> **ASCII = f(JSON) + g**

Where:

* **JSON** records all *structural facts*.
* **f** is a deterministic, 1-to-1 structural mapping from those facts into an ASCII skeleton.
* **g** is *narrative semantics* layered on top for human comprehension.

JSON and ASCII are *not competing truths*; they are complementary views with different responsibilities.

---

## Definitions

### Structural Facts (machine-meaning)

Structural facts are assertions about the system that affect correctness, comparison, automation, or policy.

Examples:

* object identities and stable IDs
* parent/child or ownership relationships
* desired vs live values
* counts, states, classifications, severities
* references and paths
* event types and timestamps (if present)

**Properties:**

* explicit
* deterministic
* lossless
* comparable across runs

Structural facts **must** appear in JSON.

---

### Narrative Semantics (human-meaning)

Narrative semantics exist to help humans understand *why* something matters.

Examples:

* ordering for readability or emphasis
* grouping under headings
* labels such as "Warning: Capacity drift"
* collapsing or expanding repetition (e.g., "x5")
* icons, glyphs, explanatory prose

**Properties:**

* additive
* human-facing
* allowed to evolve
* not inputs to automation

Narrative semantics live only in ASCII (and TUI views).

---

## Output Responsibilities

### JSON — Factual Authority

JSON is authoritative for **structural facts**.

JSON exists to:

* support drift detection and comparison
* enable automation and policy
* provide portable, replayable snapshots
* validate invariants via schema

JSON must:

* be deterministic
* be explicit (no implied meaning)
* avoid narrative, emphasis, or storytelling

---

### ASCII — Narrative Authority

ASCII is authoritative for **explanation**.

ASCII exists to:

* explain tree + trace structure
* guide human attention
* tell the debugging story
* make causality legible

ASCII may:

* reorder facts for clarity
* group related facts
* add headings, labels, and emphasis

ASCII must **not**:

* invent structural facts
* hide structural facts
* contradict JSON

---

## Contributor Rules (Normative)

These rules are intentionally numbered so they can be referenced, amended, or extended in the future.

### R1 — Single Fact Source

All structural facts must be present in JSON. ASCII must not introduce any structural fact absent from JSON.

### R2 — Lossless Structure

For every ASCII view, there exists a deterministic mapping **f** such that all structural facts rendered in ASCII originate from JSON.

### R3 — Narrative Is Additive

ASCII may add narrative semantics (**g**) but must not change the interpretation of any structural fact.

### R4 — No Meaning by Placement

Placement under a heading, section, or group must not alter identity, scope, or semantics. Headings are visual only unless backed by explicit JSON fields.

### R5 — Stable Identity

Anything referencable in ASCII must correspond to a stable identifier in JSON. Friendly names may appear, but identity resolves via JSON.

### R6 — Ordering Is Narrative by Default

Ordering in ASCII is narrative unless explicitly declared as semantic via a JSON field (e.g., `severity`, `priority`, `classification`).

---

## The Leak Test (Mandatory)

The **leak test** is used to detect when narrative semantics accidentally become structural facts.

> **Leak Test:**
> If removing ASCII headings, grouping, or ordering would change how a machine *should* behave, then narrative semantics have leaked into structure.

If the leak test fails:

* the missing meaning must be added to JSON as an explicit field, or
* the ASCII presentation must be revised so it no longer implies machine-relevant meaning.

This test applies to:

* drift severity
* CI exit behavior
* alerting
* policy decisions

---

## Example: Drift Detection

### JSON (structural facts)

```json
{
  "kind": "drift_report",
  "target": {"kind": "Deployment", "name": "web", "namespace": "prod", "id": "dep:prod/web"},
  "findings": [
    {
      "id": "drift:replicas",
      "path": "spec.replicas",
      "desired": 3,
      "live": 5,
      "classification": "capacity",
      "severity": "warning"
    }
  ]
}
```

### ASCII (narrative semantics layered on top)

```
Deployment prod/web

[!] Capacity drift
    replicas: desired 3, live 5
```

Classification and severity are structural (JSON). Grouping and emphasis are narrative (ASCII).

---

## Testing Implications

* JSON tests assert **truth and invariants**.
* ASCII golden tests assert **clarity and narrative quality**.
* Discrepancies are classified:

  * JSON incorrect → semantic bug
  * ASCII misleading → UX bug

---

## Breadcrumbs for Future Evolution

* Rules are intentionally numbered (R1-R6) to allow targeted discussion and revision.
* New rule categories may be added (e.g., Rx for performance, Ry for security).
* The f(JSON) + g model should remain stable even as renderers or formats evolve.

---

## Summary

cub-scout separates **facts** from **explanation** deliberately:

* JSON records what *is true*.
* ASCII explains *why it matters*.

Maintaining this separation is critical to determinism, trust, and long-term evolution.
