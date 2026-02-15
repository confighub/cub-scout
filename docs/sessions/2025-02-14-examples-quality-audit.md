# Session: Examples Quality Audit & Documentation Overhaul

**Date:** 2025-02-14
**Branch:** `keen-galileo`
**Issues filed:** #154–#158

## Summary

Comprehensive review and rewrite of cub-scout's example documentation,
test coverage gaps, and terminology cleanup. Started from a full project
audit and ended with every example directory having a problem-first README
with expected output.

## What Changed

### 1. Core Code — CCVE → Risk Issue Terminology (#140)

Renamed user-facing "CCVE" references to "risk issue" across:
- `cmd/cub-scout/` — scan, remedy, demo, hierarchy, localcluster, map
- Golden TUI help files updated
- Integration test references updated

Internal struct fields and JSON schema kept unchanged (non-breaking).

### 2. New Tests

| Test | Coverage |
|------|----------|
| `pkg/queries/queries_test.go` | 25 tests for previously untested package |
| `pkg/agent/attribution_bench_test.go` | Benchmark for attribution graph |
| `test/scale/scale_smoke_test.go` | Scale smoke tests for large clusters |

### 3. Parser Taxonomy Alignment

`pkg/gitops/parser.go` — aligned skeleton IDs and pattern names with
`docs/reference/patterns-contract.md`.

### 4. Documentation Updates

- **CLAUDE.md** — added backlog tracking section with rules for roadmap checklist
- **CLI-GUIDE.md / README.md** — fixed ArgoCD detection docs, deployer scope
- **docs/roadmap.md** — added untracked backlog checklist
- **docs/reference/** — pattern taxonomy updates in gitops-patterns.md,
  patterns-contract.md
- **IITS → Enterprise** — replaced vendor-specific references across 12 active
  docs (archive files left as historical record)

### 5. Example READMEs — Comprehensive Rewrite

Every example directory now follows the pattern:
1. **The Problem** — a real question a developer would ask
2. **cub-scout answers this** — one command with expected output
3. **Directory tree** or ownership hierarchy
4. **How it works** — detection mechanism explained

#### Rewritten (existing)

| Directory | Key Change |
|-----------|------------|
| `rm-demos-argocd/repo-patterns/monorepo/` | ConfigHub commands → cub-scout trace/map/parse-repo |
| `rm-demos-argocd/repo-patterns/multi-repo/` | ConfigHub commands → cub-scout map/trace/tree |
| `rm-demos-argocd/repo-patterns/applicationsets/` | ConfigHub commands → cub-scout map/gitops-status/scan |
| `rm-demos-argocd/repo-patterns/helm-umbrella/` | ConfigHub commands → cub-scout map/trace/drift/scan |
| `rm-demos-argocd/README.md` | "ConfigHub Sees" → "cub-scout Shows" |
| `flux-boutique/README.md` | Problem-first narrative with ownership chain |
| `crossplane-system/README.md` | Expanded from 9-line stub to full walkthrough |
| `drift/env-var-drift/README.md` | Added comparison diagram (desired → live) |
| `drift/image-policy-drift/README.md` | Added comparison diagram + policy risk table |
| `drift/resource-drift/README.md` | Added comparison diagram with CRITICAL annotation |

#### Created (new)

| Directory | What |
|-----------|------|
| `apptique-examples/argo-app-of-apps/README.md` | Root → Child → Workload hierarchy |
| `apptique-examples/argo-applicationset/README.md` | Directory generator detection |
| `apptique-examples/flux-monorepo/README.md` | Kustomization chain, offline parse-repo |
| `drift/README.md` | Parent file tying 3 drift examples together |
| `workflows/README.md` | Artifact workflow + fleet demo coverage |
| `d2-control-plane/README.md` | Control Plane (controlplaneio-fluxcd) Flux reference architecture |
| `d2-control-plane/control-plane.yaml` | Full YAML fixture with Flux labels |

### 6. Issues Filed

| Issue | Title |
|-------|-------|
| #154 | Master tracking issue for pre-1.0 work |
| #155 | Scale smoke tests |
| #156 | Attribution benchmark |
| #157 | Queries test coverage |
| #158 | Demo-data example (connected mode, deferred) |

## Commits

```
acd0d82 refactor: CCVE→risk-issue terminology, parser taxonomy, new tests
ac48eb7 docs: backlog tracking, IITS→Enterprise cleanup, roadmap updates
6d448db examples: problem-first READMEs, new patterns, full coverage
```

## Verification

- `go build ./cmd/cub-scout` — passes
- `go test ./...` — all tests pass
- All 15 top-level example directories have READMEs
- No IITS references remain in active docs

## Follow-up

- Issue #158: Demo-data example requires connected mode (deferred)
- Pattern contract: skeleton IDs now canonical, future examples should reference them
- Archive docs (`docs/archive/`) left untouched as historical record
