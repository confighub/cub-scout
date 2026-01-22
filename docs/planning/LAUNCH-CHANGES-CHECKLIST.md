# cub-scout Launch Changes Checklist

**Status:** RALPH Review Complete ✅
**Date:** 2026-01-22

---

## Summary

Make cub-scout the best tool for demystifying GitOps — not just viewing data, but understanding what's happening.

---

## Change List

### A. Documentation Changes (Already Done)

| # | Change | File | Status |
|---|--------|------|--------|
| A1 | README: Navigation-first positioning | `README.md` | ✅ Done |
| A2 | README: "Demystify GitOps" tagline | `README.md` | ✅ Done |
| A3 | README: Problem framing (what's obscure) | `README.md` | ✅ Done |
| A4 | SCALE-DEMO: Navigation focus | `docs/SCALE-DEMO.md` | ✅ Done |
| A5 | Product plan documented | `planning/PRODUCT-PLAN-LAUNCH.md` | ✅ Done |
| A6 | D2: Flux architecture diagram | `docs/diagrams/flux-architecture.d2` | ✅ Done |
| A7 | D2: Ownership trace diagram | `docs/diagrams/ownership-trace.d2` | ✅ Done |
| A8 | D2: Kustomize overlays diagram | `docs/diagrams/kustomize-overlays.d2` | ✅ Done |
| A9 | D2: Ownership detection diagram | `docs/diagrams/ownership-detection.d2` | ✅ Done |

### B. CLI UX Improvements (Proposed)

| # | Change | File | Priority |
|---|--------|------|----------|
| B1 | `map orphans`: Add header explaining what orphans are | `cmd/cub-scout/map.go` | P1 |
| B2 | `map orphans`: Add summary count at end | `cmd/cub-scout/map.go` | P1 |
| B3 | `map orphans`: Add "next steps" suggestions | `cmd/cub-scout/map.go` | P1 |
| B4 | `map issues`: Differentiate from `map crashes` | `cmd/cub-scout/map.go` | P2 |
| B5 | `map crashes`: Focus on pod health only | `cmd/cub-scout/map.go` | P2 |
| B6 | All commands: Consistent summary lines | `cmd/cub-scout/map.go` | P2 |
| B7 | Exit codes for scripting (0/1/2) | `cmd/cub-scout/*.go` | P3 |

### C. New User Learning Features (Proposed)

| # | Change | File | Priority |
|---|--------|------|----------|
| C1 | `--explain` flag for `map list` | `cmd/cub-scout/map.go` | P1 |
| C2 | `--explain` flag for `trace` | `cmd/cub-scout/trace.go` | P1 |
| C3 | `--explain` flag for `scan` | `cmd/cub-scout/scan.go` | P2 |
| C4 | `cub-scout learn` command | `cmd/cub-scout/learn.go` (new) | P3 |
| C5 | Contextual help in TUI (press ? on resource) | `cmd/cub-scout/localcluster.go` | P3 |

### D. Meaningful Examples (Proposed)

| # | Change | Location | Priority |
|---|--------|----------|----------|
| D1 | Create `platform-example/` with 50+ resources | `examples/platform-example/` | P1 |
| D2 | Multi-layer Kustomize (base + 3 overlays) | `examples/platform-example/apps/` | P1 |
| D3 | Helm charts via Flux HelmRelease | `examples/platform-example/apps/database/` | P1 |
| D4 | Infrastructure layer (monitoring, ingress) | `examples/platform-example/infrastructure/` | P2 |
| D5 | Full documentation with learning journey | `examples/platform-example/README.md` | P1 |
| D6 | Deploy script for kind cluster | `examples/platform-example/setup.sh` | P1 |

### E. Enhanced Import (Proposed)

| # | Change | File | Priority |
|---|--------|------|----------|
| E1 | Import wizard structure detection | `cmd/cub-scout/import_wizard.go` | P2 |
| E2 | Multi-layer repo understanding | `cmd/cub-scout/import.go` | P2 |
| E3 | Dependency inference | `cmd/cub-scout/import.go` | P3 |
| E4 | Hub/Space/Unit mapping suggestions | `cmd/cub-scout/import_wizard.go` | P2 |

---

### F. Gaps Identified in RALPH Review (New)

| # | Gap | File | Priority |
|---|-----|------|----------|
| F1 | Link D2 diagrams from `--explain` output | `cmd/cub-scout/map.go` | P1 |
| F2 | Add "quick wins" timing claims to README | `README.md` | P1 |
| F3 | First-run welcome message | `cmd/cub-scout/map.go` | P2 |
| F4 | Error recovery suggestions | `cmd/cub-scout/*.go` | P3 |
| F5 | GitHub issues link in help | `cmd/cub-scout/root.go` | P3 |

### G. Diff & Upgrade Tracing (New)

| # | Feature | Description | Priority |
|---|---------|-------------|----------|
| G1 | `trace --diff` | Show live vs git differences | P1 |
| G2 | Chart version diff | Show what changed between helm chart versions | P2 |
| G3 | Layer-by-layer trace | Show which layer (chart/values/helmrelease/kustomize) caused a change | P2 |
| G4 | Upgrade impact preview | Before upgrading, show what will change | P3 |

**User pain:** When something breaks after an upgrade, users do git archaeology through repo tree + overlay + chart mix. 30-60 minutes.

**cub-scout solution:** Layer-by-layer diff showing exactly what changed. 5 seconds.

See: `docs/diagrams/upgrade-tracing.d2`

---

## Priority Summary (Updated)

| Priority | Count | Focus |
|----------|-------|-------|
| **P1** | 12 | Core demystification: orphans UX, --explain, platform-example, D2 links, quick wins |
| **P2** | 9 | Polish: crashes/issues differentiation, import wizard, first-run |
| **P3** | 7 | Nice-to-have: learn command, exit codes, error recovery |

---

## RALPH Review Results

### Q1: Is the "demystify" positioning correct? ✅ YES

Web search confirmed:
- Flux docs acknowledge "steep learning curve"
- Users frequently ask "Where did this resource come from?"
- No competitor uses "demystify" - it's differentiated
- Resonates because GitOps IS genuinely obscure to newcomers

### Q2: Are the P1 changes the right focus? ✅ YES, with ordering

Recommended order:
1. **Platform-example first** - gives users something to explore
2. **`--explain` flag** - teaches as they explore
3. **CLI UX (orphans header)** - polish for power users

### Q3: Is `platform-example` the right approach? ✅ HYBRID

Decision: Use official `flux2-kustomize-helm-example` (28+ resources) + add orphans
- Provides real complexity users will recognize
- Already documented and maintained by FluxCD team
- Add 5-7 native resources to demonstrate orphan detection
- Total: ~35 resources, good demo scale

### Q4: Is import enhancement needed for launch? ⚠️ DEFER

Current import works. Enhanced wizard is P2.

### Q5: What's missing? - GAPS IDENTIFIED

| Gap | Description | Priority | Recommendation |
|-----|-------------|----------|----------------|
| **First-run experience** | No welcome/onboarding for new users | P2 | Add brief intro on first `map` run |
| **D2 diagrams not linked** | Created but not referenced from help | P1 | Link from `--explain` output |
| **Error recovery guidance** | No help when commands fail | P3 | Add troubleshooting suggestions |
| **Quick wins not visible** | Learning focus buries immediate value | P1 | Add "in 5 seconds" claims to README |
| **Offline experience** | No guidance when cluster unreachable | P3 | Defer - edge case |
| **Feedback mechanism** | No way for users to report confusion | P3 | Add GitHub issues link in help |

---

## Validation Criteria

For each change, verify:
- [ ] Solves a real user problem
- [ ] Teaches, not just shows
- [ ] Works with realistic scale (50+ resources)
- [ ] No breaking changes to existing behavior
- [ ] Can be tested/demoed

---

## Files Modified/Created

### Already Modified:
- `README.md` - Demystify positioning
- `docs/SCALE-DEMO.md` - Navigation focus
- `docs/planning/PRODUCT-PLAN-LAUNCH.md` - Full plan
- `docs/planning/CLI-UX-IMPROVEMENTS.md` - CLI UX plan
- `docs/planning/NEW-USER-JOURNEY.md` - Learning journey plan

### To Be Created:
- `examples/platform-example/` - Meaningful example
- `cmd/cub-scout/learn.go` - Learn command (P3)

### To Be Modified:
- `cmd/cub-scout/map.go` - Orphans UX, --explain
- `cmd/cub-scout/trace.go` - --explain flag
- `cmd/cub-scout/import_wizard.go` - Structure detection
