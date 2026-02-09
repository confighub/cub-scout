# cub-scout Next Plan: v0.19.x and v1.x

> **Status:** Historical planning draft
> **Date:** 2026-02-06
> **Scope:** Three work areas across standalone and connected modes
>
> **Authoritative roadmap:** [`docs/roadmap.md`](roadmap.md)
>
> This document is retained as planning context and rationale. Where it conflicts with `docs/roadmap.md`, the roadmap document wins.

---

## Executive Summary

Three work areas, cleanly split between standalone (v0.19.x) and connected (v1.x):

| Area | v0.19.x (Standalone) | v1.x (Connected) |
|------|----------------------|------------------|
| **A. Docs Audit** | Archive stale, fix navigation | Integrate WHY_CONNECTED_MODE into roadmap |
| **B. Evidence Adjacency** | `bundle summarize --format ticket/pr/slack` | ConfigHub stores/indexes summaries |
| **C. GitOps Lifecycle Hazards** | Helm hook analyzer, `map hooks` view | Verifier, intent vs render vs observed |

---

## Area A: Documentation Audit

### Problem

The docs/ folder has 94+ files in various states:
- Stale design docs from v0.5–v0.14
- Deprecated demo scripts referenced everywhere
- Broken navigation links
- WHY_CONNECTED_MODE.md not integrated into roadmap

### v0.19.x Deliverables

#### A1. Archive Stale Docs

Move to `docs/archive/`:

| File | Reason |
|------|--------|
| `v0.14-json-schema.md` | Historical |
| `v0.15-design-checkpoint.md` | Historical |
| `codex-review-checklist.md` | Internal |
| `v0.19-experience-report.md` | Historical |
| `gitops-hierarchies.md` | Superseded by howto/ |
| Various release notes in `releases/` | Keep but link from one index |

#### A2. Fix Examples/Demos Navigation

The `EXAMPLES-OVERVIEW.md` and `demos/README.md` are full of:
- `# DEPRECATED: ./test/atk/demo` references
- Broken links to `test/expected-outputs/`
- References to non-existent files

**Action:** Rewrite to reflect current Go CLI:
```bash
# Replace deprecated bash demos with:
./cub-scout demo risk
./cub-scout demo healthy
./cub-scout demo hooks  # NEW
```

#### A3. Create Single Examples Index

Consolidate:
- `docs/EXAMPLES-OVERVIEW.md`
- `examples/README.md`
- `docs/demos/README.md`

Into one authoritative `examples/README.md` with clear categories.

### v1.x Integration

#### A4. Integrate WHY_CONNECTED_MODE.md

The `docs/WHY_CONNECTED_MODE.md` document defines:
- Intent awareness
- History & time
- Fleet & multi-cluster
- Impact analysis
- Git-aware navigation
- Governance context

This should feed directly into roadmap v1.x section with concrete issues.

---

## Area B: Evidence Adjacency

> "Put the explanation where humans already argue, decide, and get audited."

### Problem

cub-scout produces truth (bundles, diffs, timelines) but humans still copy/paste screenshots into:
- Jira tickets
- ServiceNow incidents
- PR reviews
- Slack channels

The gap is **placement**, not analysis.

### v0.19.x Deliverables (Standalone)

#### B1. `bundle summarize` Command

```bash
cub-scout bundle summarize ./bundle --format ticket --out jira.md
cub-scout bundle summarize ./bundle --format slack --out slack.json
```

**Output formats:**

| Format | Output | Use Case |
|--------|--------|----------|
| `ticket` | Markdown summary for Jira/ServiceNow | Incident documentation |
| `pr` | Markdown summary for PR description/comment | Code review |
| `slack` | Slack Block Kit JSON | Channel notification |
| `ascii` | Human-readable (default) | Terminal |
| `json` | Structured data | Tooling |

**Ticket format example:**
```markdown
## cub-scout Diagnostic Summary

**Cluster:** prod-eu-1
**Captured:** 2026-02-05T14:03Z
**Context:** Git commit a1b2c3 (infra repo)

### What changed
- Deployment `payments-api` image: 1.14.2 → 1.15.0
- ConfigMap `payments-env` modified

### Why it broke
- PostSync Job ran twice due to drift-triggered resync
- Job was not idempotent (API returned 409 on re-run)

### Scope / Impact
- Affected namespace: payments
- Other clusters: not affected

### Evidence
- Bundle hash: `sha256:abcd...`
```

#### B2. `bundle diff --format pr`

```bash
cub-scout bundle diff before/ after/ --format pr --out pr-summary.md
```

**PR format example:**
```markdown
## cub-scout Change Summary

### High-level
- 3 resources changed
- No drift introduced
- No new ownership ambiguity

### Resource changes
- Deployment/payments-api: image 1.14.2 → 1.15.0
- HPA/payments-api: max replicas 10 → 15

### Risk signals
- ⚠️ PostSync hook present (runs on every sync)
```

#### B3. Documentation

New doc: `docs/howto/evidence-export.md`
- Ticket export workflow
- PR summary workflow
- Slack notification workflow
- CI integration examples

### v1.x Deliverables (Connected)

#### B4. ConfigHub Summary Storage

ConfigHub stores summaries with:
- Durable retention
- Cross-incident search ("show me all PostSync failures")
- Audit-grade history

#### B5. ConfigHub Slack Integration

ConfigHub generates and sends notifications:
- Drift detected
- Sync failed
- Unusual diff

cub-scout surfaces these; ConfigHub decides when to send.

---

## Area C: GitOps Lifecycle Hazards

> "Helm works, ArgoCD fails" — the pain is semantic drift between tooling models.

### Problem

Users assume Helm lifecycle semantics; ArgoCD templates + applies differently:
- Hooks ignored or mapped differently
- PreSync dependency races
- PostSync reruns causing non-idempotent jobs to fail

### v0.19.x Deliverables (Standalone)

#### C1. GitOps Lifecycle Hazards Analyzer

New findings pack detecting:

| Rule | Detection |
|------|-----------|
| **Helm hook ambiguity** | `helm.sh/hook` with comma-separated values (lossy mapping) |
| **PreSync dependency race** | Hook references ConfigMap/Secret not in same phase |
| **PostSync idempotency risk** | Job with `hook-delete-policy: before-hook-creation` |
| **Missing hook resource** | Present in intent, absent in cluster |

**Output (JSON finding):**
```json
{
  "rule": "helm-hook-ambiguity",
  "resource": "Job/db-migrate",
  "severity": "warning",
  "hooks": ["post-install", "post-upgrade"],
  "mapped_phase": "PostSync",
  "risk": "Argo maps comma hooks to single phase; may differ from Helm behavior",
  "remediation": "Split into separate resources or convert to ArgoCD Sync hook"
}
```

#### C2. Hook/Lifecycle Lens

New CLI command:
```bash
cub-scout map hooks [--format ascii|json|md]
```

Shows:
- Hook annotations (Helm + ArgoCD)
- Sync-wave
- Phase classification
- Ordering view

TUI integration:
- New "Hooks & Ordering" panel (key: `h`)
- Suggestions: "show dependencies for this hook"

#### C3. Repro Bundle Example

New example: `examples/gitops/argocd-helm-hooks/`
- Helm chart with hooks
- ArgoCD Application
- Expected cub-scout output
- CI-backed golden tests

#### C4. Documentation

New doc: `docs/howto/helm-hooks-argocd.md`
- Explain: Argo templates + applies; doesn't run Helm lifecycle
- Explain: Phases vs waves
- Explain: Why `post-install,post-upgrade` is tricky
- Show: cub-scout output

### v1.x Deliverables (Connected)

#### C5. Connected Import Flows

```bash
cub-scout connected import bundle <path> --env <name>
cub-scout connected import git <repo>@<ref>
cub-scout connected import cluster
```

#### C6. Intent vs Render vs Observed

ConfigHub computes and stores comparisons:
- intent vs render → templating/mapping problem
- render vs observed → apply/prune/hook semantics problem

Proves "where disappearance occurs."

#### C7. Hook Compatibility Verifier

ConfigHub verifier outputs:
- "Helm hook annotation set is ambiguous under Argo mapping"
- "Job likely mapped to PostSync; will rerun on drift"
- "PreSync hook depends on Secret created in main phase"

cub-scout surfaces results; ConfigHub owns the rules.

---

## Roadmap Updates

### v0.19.x (Standalone Polish)

| Issue | Title | Area |
|-------|-------|------|
| NEW | Docs audit and archive | A |
| NEW | Fix examples/demos navigation | A |
| NEW | `bundle summarize --format ticket/pr/slack` | B |
| NEW | GitOps Lifecycle Hazards analyzer | C |
| NEW | `map hooks` command and TUI view | C |
| NEW | Helm hooks example and documentation | C |

### v1.x (Connected Mode)

| Issue | Title | Area |
|-------|-------|------|
| NEW | Integrate WHY_CONNECTED_MODE into roadmap | A |
| NEW | ConfigHub summary storage and indexing | B |
| NEW | ConfigHub Slack notification integration | B |
| NEW | `connected import bundle/git/cluster` | C |
| NEW | Intent vs render vs observed comparisons | C |
| NEW | Hook compatibility verifier | C |
| #3 | Platform composition tools (kro) | Existing |
| #21 | kro support | Existing |

---

## Documents to Create

| Doc | Purpose | Area |
|-----|---------|------|
| `docs/howto/evidence-export.md` | Ticket/PR/Slack export workflows | B |
| `docs/howto/helm-hooks-argocd.md` | Helm hooks under ArgoCD | C |
| `docs/reference/lifecycle-hazards.md` | Hazard rules reference | C |
| `examples/gitops/argocd-helm-hooks/README.md` | Repro bundle example | C |

---

## Guiding Principles (Unchanged)

- **Read-only** — cub-scout never mutates
- **Deterministic** — same input = same output
- **Explainable** — every inference has evidence
- **Human-centered** — put explanations where humans decide
- **ASCII = f(JSON) + g** — human output derives from machine output

---

## Acceptance Criteria

### v0.19.x Done When

- [ ] Docs archived, navigation fixed, examples consolidated
- [ ] `bundle summarize` works with ticket/pr/slack formats
- [ ] Lifecycle hazards detected and surfaced in scan
- [ ] `map hooks` shows hook metadata in CLI and TUI
- [ ] Example and docs for Helm hooks under ArgoCD
- [ ] All existing tests pass, new tests added

### v1.x Done When

- [ ] `connected import` flows work (bundle, git, cluster)
- [ ] ConfigHub stores and indexes summaries
- [ ] Intent vs render vs observed comparisons available
- [ ] Verifier results surfaced in cub-scout
- [ ] Offline still works (graceful degradation)
