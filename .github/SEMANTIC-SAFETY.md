# Semantic Safety PR Checklist

**Scope**
This checklist enforces the cub-scout *Semantic Contract* (see: `docs/semantic-contract.md`).

It is designed to be used during PR review and CI gating. Each item is a **yes/no gate**. If any item fails, the PR must not merge until resolved.

---

## How to Use

* Reviewers should explicitly cite rule numbers (R1-R6) when requesting changes.
* Authors should reference this checklist in PR descriptions when touching renderers, models, drift detection, or outputs.
* This checklist complements existing golden-test practices; it does not replace them.

---

## A. Structural Facts (JSON authority)

**A1 — Single Fact Source (R1)**
- [ ] Are all *structural facts* introduced or modified by this PR represented explicitly in JSON?

**A2 — No Hidden Facts**
- [ ] Does ASCII avoid asserting any fact (identity, relationship, severity, classification, count) that is not present in JSON?

**A3 — Stable Identity (R5)**
- [ ] Does every referencable item in ASCII correspond to a stable ID or path in JSON?

---

## B. ASCII Rendering (Narrative authority)

**B1 — Lossless Structure (R2)**
- [ ] Can all structural facts shown in ASCII be traced directly back to JSON via a deterministic mapping?

**B2 — Narrative Is Additive (R3)**
- [ ] Are ordering, grouping, labels, icons, and prose clearly narrative (explanatory) rather than semantic?

**B3 — No Meaning by Placement (R4)**
- [ ] Would moving or removing a heading/group *not* change the meaning of any structural fact?

---

## C. Ordering & Grouping

**C1 — Ordering Is Narrative by Default (R6)**
- [ ] Is ASCII ordering used only for readability unless explicitly backed by JSON fields (e.g., `severity`, `priority`)?

**C2 — Explicit Semantics Where Needed**
- [ ] If ordering or grouping implies importance, risk, or causality, is that implication explicitly encoded in JSON?

---

## D. The Leak Test (Mandatory)

> **Leak Test:** If removing ASCII headings, grouping, or ordering would change how a machine *should* behave, then narrative semantics have leaked into structure.

**D1 — Leak Test Passes**
- [ ] If all ASCII narrative elements were removed, would CI, policy, alerts, or automation behave identically?

If **No**:

* Add the missing meaning to JSON **or**
* Revise ASCII so it no longer implies machine-relevant meaning

---

## E. Testing Implications

**E1 — JSON Tests (Truth)**
- [ ] Are JSON outputs covered by tests that assert correctness, invariants, or schema validity?

**E2 — ASCII Goldens (Clarity)**
- [ ] Are ASCII changes covered by golden tests that reflect intentional narrative improvements?

**E3 — Failure Classification**
- [ ] Is it clear whether a potential failure would be classified as a *semantic bug* (JSON) or a *UX bug* (ASCII)?

---

## Reviewer Notes (Optional)

* Rule(s) referenced: ____________
* Leak Test concern (if any): ___________________________
* Follow-up required: _________________________________

---

## Summary

This checklist exists to ensure that:

* JSON remains the authoritative record of structural facts
* ASCII remains free to evolve as a human explanation layer
* No change silently introduces a second source of truth

If in doubt, cite the contract and run the Leak Test.
