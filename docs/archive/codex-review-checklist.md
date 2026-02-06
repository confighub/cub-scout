# Codex Review Checklist (v0.5+)

This checklist defines what "Codex review" means for cub-scout PRs.
It is designed for a small team where human line-by-line review is not available.

Rule of thumb:
- If a PR changes behavior, touches ownership/health/trace logic, or fixes a bug:
  **Codex review is mandatory before merge.**
- Docs-only or mechanical changes may be merged without Codex review if risk is low.

---

## 0) PR Hygiene (always)

- [ ] PR links to an issue (e.g. closes #NN)
- [ ] PR description includes the Review-by-evidence block and it is filled in
- [ ] Scope is tight: one concern per PR (no drive-by refactors)
- [ ] No unrelated formatting-only churn across many files

---

## 1) CI & Evidence (always)

- [ ] All required checks pass (Unit, Integration, GitOps E2E)
- [ ] Any "skipped" checks are not required by branch protection
- [ ] If PR claims runtime proof:
  - [ ] exact commands are included
  - [ ] environment/cluster details are stated (kind vs real cluster)
  - [ ] outcomes are copied verbatim (not paraphrased)

---

## 2) Safety & Trust (always for cub-scout)

### Read-only posture
- [ ] No kubectl apply/patch/delete paths introduced
- [ ] No writes to cluster state, even accidentally
- [ ] Any side effects are explicitly justified and documented

### "Parse, don't guess"
- [ ] No inference from names/namespaces/resource kinds
- [ ] Unknown/ambiguous states are surfaced as unknown (not forced into a false label)
- [ ] Any precedence rules are explicit and consistent with docs

### Avoid false certainty
- [ ] Changes do not make ownership/health claims more confident without stronger signals
- [ ] Weak/forgeable signals are treated with appropriate caution (doc + behavior aligned)

---

## 3) Crash/Panic Resistance (mandatory for runtime logic changes)

- [ ] No new unchecked indexing on slices (e.g. x[0]) without guards
- [ ] No nil deref risks added (pointers, maps, interfaces)
- [ ] Error handling is explicit: failures degrade gracefully
- [ ] For TUI/trace/map changes:
  - [ ] an empty/minimal cluster path is safe
  - [ ] missing CRDs path is safe

---

## 4) Ownership/Provenance Semantics (mandatory if touched)

- [ ] Detection order / precedence remains consistent with the ownership reference doc
- [ ] Conflicting signals behavior is explicitly defined and tested
- [ ] Any new signals are documented:
  - [ ] why they are authoritative
  - [ ] what can forge them
  - [ ] how cub-scout behaves if they are malformed/missing
- [ ] Unit tests cover at least:
  - [ ] Flux
  - [ ] Argo CD
  - [ ] Helm
  - [ ] conflict/overlap scenario
  - [ ] unknown scenario (no signals)

---

## 5) Failure/Health Semantics (mandatory if touched)

- [ ] Health/failure mapping matches the health reference doc
- [ ] "unknown" vs "unhealthy" boundaries are correct and conservative
- [ ] Tests cover representative failure states (or a waiver exists)

---

## 6) CLI / Output Contract (mandatory if touched)

- [ ] Help text remains accurate
- [ ] Command/flag behavior is stable or explicitly versioned
- [ ] JSON output fields are stable (schema changes are deliberate)
- [ ] Any output changes are either:
  - [ ] backed by golden tests, or
  - [ ] explicitly documented as breaking/behavioral

---

## 7) Security & Dependencies (mandatory for any dep changes)

- [ ] No new dependency without justification
- [ ] Any new dependency is minimal and reputable
- [ ] govulncheck results reviewed (findings resolved or documented)
- [ ] No secrets/credentials logged (kubeconfig, tokens, env vars)

---

## 8) Waivers (if any)

- [ ] Waiver issue exists and is approved
- [ ] Limitation is documented (user-facing if it affects behavior)
- [ ] Failure mode is safe (no false positives / misleading claims)
- [ ] Follow-up issue exists for post-v0.5 hardening

---

## Merge Decision Output (Codex)

Codex review must end with one of:

- **MERGE**: Safe to merge under v0.5 priorities
- **CHANGES REQUIRED**: Must fix X/Y/Z before merge
- **DO NOT MERGE**: Violates core principles or introduces unacceptable risk
