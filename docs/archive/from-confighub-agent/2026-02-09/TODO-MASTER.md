# ConfigHub Agent — Master TODO List

**Updated:** 2026-01-12 | **Source:** Consolidated from session notes, design docs, GitHub issues

---

## Priority 1: Rendered Manifest Pattern ("Cutting GitOps in Half")

From Issue #3339 design discussion. ConfigHub as the WET source of truth between rendering and deployment.

### 1A. Extend Trace for OCI Sources

| Task | Effort | Files |
|------|--------|-------|
| Detect "ConfigHub via OCI" as distinct ownership type | Medium | `pkg/agent/ownership.go` |
| Add OCI source detection to trace command | Medium | `pkg/agent/trace.go` |
| Show full chain: Source → ConfigHub Unit → OCI → Flux/Argo → Resource | Medium | `cmd/cub-agent/trace.go` |

### 1B. New `map sources` View

| Task | Effort | Files |
|------|--------|-------|
| Create `map sources` command showing all config sources | Medium | `cmd/cub-agent/map_sources.go` (new) |
| Show GitRepo, OCIRepo, HelmRepo, ConfigHub sources | Medium | TUI in `test/atk/map` |
| Display source staleness and sync status | Low | — |

### 1C. GSF Schema Extension

| Task | Effort | Files |
|------|--------|-------|
| Add `source` field to GSF entries | Low | `pkg/gsf/types.go` |
| Include `rendered_from` and `original_source` metadata | Low | `docs/GSF-SCHEMA.md` |

### 1D. Bridge Detection

| Task | Effort | Files |
|------|--------|-------|
| Detect which bridge pattern is in use | High | `pkg/agent/bridge.go` (new) |
| Git → Flux → Cluster (Flux direct) | — | — |
| Git → Argo → Cluster (Argo direct) | — | — |
| Git → ConfigHub → OCI → Flux (ConfigHub rendered) | — | — |
| Live → ConfigHub → OCI → Flux (ConfigHub imported) | — | — |

---

## Priority 2: Trace Integration (GitHub Issue #3)

From `docs/TUI-TRACE-FLUX-notes.md`. Full ownership chain tracing.

### 2A. Flux Trace

| Task | Status | Files |
|------|--------|-------|
| Basic `flux trace` CLI integration | Done | `pkg/agent/flux_trace.go` |
| Parse flux trace output into TraceResult | Done | — |
| TUI trace view with `t` key | Done | `test/atk/map` |

### 2B. Argo Trace

| Task | Status | Files |
|------|--------|-------|
| `argocd app get --output json` integration | Done | `pkg/agent/argo_trace.go` |
| Parse app status into TraceResult | Done | — |
| Unified trace output for both tools | Done | — |

### 2C. Extended Trace Sources

| Task | Effort | Files |
|------|--------|-------|
| ConfigHub Unit → Target → Cluster tracing | Medium | `pkg/agent/confighub_trace.go` (new) |
| Standalone Helm releases (not via Flux/Argo) | Medium | `pkg/agent/helm_trace.go` (new) |
| kubectl apply orphans (last-applied-configuration) | Low | `pkg/agent/orphan_trace.go` |
| Full provenance: ConfigHub → Git → Deployer → K8s | High | Integrate all |

### 2E. Source Verification (ConfigHub Main Product)

Extend trace to verify source integrity all the way back to origin.

| Task | Effort | Product | Notes |
|------|--------|---------|-------|
| Git commit signature verification | Medium | Main | Verify GPG/SSH signatures on source commits |
| OCI artifact signature verification (cosign/sigstore) | Medium | Main | Verify signatures on OCI images/artifacts |
| SLSA provenance attestation | High | Main | Full supply chain verification |
| Show signature status in trace output | Low | Both | `✓ signed` vs `⚠ unsigned` in chain |
| Alert on unsigned sources in production | Low | Main | Policy enforcement |

**Why this matters:**
- Trace currently stops at "GitRepository fetched from X"
- Should verify: Was that commit signed? By whom?
- For OCI: Was that artifact signed? Attestation valid?
- Complete chain: Developer → Signed Commit → Signed OCI → Verified Deploy

### 2D. Trace Performance

| Task | Effort | Files |
|------|--------|-------|
| Add trace caching (5min TTL) | Medium | `pkg/agent/trace_cache.go` |
| Batch tracing for overview (trace deployers only) | Medium | — |
| Consider flux-operator FluxReport CRD as data source | Low | — |

---

## Priority 3: Testing & Quality

### 3A. Unit Tests

| Task | Status | Files |
|------|--------|-------|
| Unit tests for `pkg/agent/flux_trace.go` | Pending | `pkg/agent/flux_trace_test.go` |
| Unit tests for `pkg/agent/argo_trace.go` | Pending | `pkg/agent/argo_trace_test.go` |
| Unit tests for `pkg/agent/ownership.go` | Pending | `pkg/agent/ownership_test.go` |
| Create mock k8s client for testing without cluster | Pending | `pkg/agent/mock_client.go` |
| Integration tests for connected mode (workers, targets) | Pending | `test/integration/` |
| Automate ATK script testing in CI | Pending | `.github/workflows/` |

### 3B. Working Examples

| Example | Status | Needs |
|---------|--------|-------|
| `jesper-argocd` | Working | Document outputs |
| `jesper-fluxcd` | Untested | Verify and document |
| `vm-fleet` | Untested | Verify with cub CLI |
| `flux-bridge` | Untested | Test import flow |
| Enterprise unhealthy | Demo only | Make runnable |

| Task | Effort |
|------|--------|
| Create `./test/atk/examples --verify-all` | Medium |
| Add example for Argo ApplicationSet pattern | Medium |
| Add example for Argo App-of-Apps pattern | Medium |
| Add example for Flux multi-tenancy pattern | Medium |

---

## Priority 4: Import Architecture

### 4A. Import from LIVE vs GIT

| Task | Effort | Files |
|------|--------|-------|
| Document LIVE import flow (what TUI does) | Low | `docs/IMPORT-FROM-LIVE.md` ✓ |
| Document GIT import flow (what GUI does) | Low | `docs/IMPORT-FROM-SOURCES.md` ✓ |
| Create reference repo structure for Argo App-of-Apps | Medium | — |
| Create reference repo structure for Argo ApplicationSet | Medium | — |
| Create reference repo structure for Flux multi-tenancy | Medium | — |
| Show how each maps to Hub → App Space → Unit | Low | `docs/IMPORT-GIT-REFERENCE-ARCHITECTURES.md` ✓ |

### 4B. Import Detection

| Task | Effort | Files |
|------|--------|-------|
| Detect OCI sources in import | Medium | `pkg/agent/import.go` |
| Detect Argo ApplicationSet generator patterns | Medium | `pkg/agent/import.go` |
| Add existing unit detection (if unit already in ConfigHub) | Medium | — |

---

## Priority 5: TUI Enhancements

### 5A. Session State Persistence

From Brian Grant / Jesper Joergensen feedback.

| Task | Effort | Files |
|------|--------|-------|
| Implement `test/atk/lib/session.sh` session manager | Medium | — |
| Add `--resume` / `--new` flags to `map-import` | Low | — |
| Auto-save on each step completion | Low | — |
| Show "Resume session?" prompt on TUI start | Low | — |
| Add session expiry (24h warning, 7d archive) | Low | — |

Design doc: [`docs/TUI-SESSION-STATE.md`](https://github.com/confighubai/confighub-agent/search?q=TUI-SESSION-STATE.md)

### 5B. TUI Features

| Task | Status | Files |
|------|--------|-------|
| Import Wizard TUI (`cub-agent import --wizard`) | Done ✓ | `cmd/cub-agent/import_wizard.go` |
| Add screenshots/recordings for each journey | Pending | `docs/images/` |

**Import Wizard Features (PR #5, merged 2026-01-10):**
- 5-step guided flow: Namespace → Workloads → Structure → Apply → ArgoCD Cleanup
- Owner detection (Flux/ArgoCD/Helm/Native) with color coding
- Interactive treeview with edit mode (rename, merge, labels)
- E2E verification test (`t` key) after worker starts
- Help overlay (`?`), search/filter (`/`)

---

## Priority 6: Documentation

### 6A. Journey Docs (Completed ✓)

- [x] `docs/JOURNEY-FIRST-SETUP.md`
- [x] `docs/JOURNEY-IMPORT.md`
- [x] `docs/JOURNEY-MAP.md`
- [x] `docs/JOURNEY-SCAN.md`
- [x] `docs/JOURNEY-QUERY.md`

### 6B. Reference Docs

| Task | Status |
|------|--------|
| CLI-REFERENCE.md (all commands) | Done ✓ |
| GSF-SCHEMA.md (JSON output format) | Done ✓ |
| GLOSSARY-OF-CONCEPTS.md | Done ✓ |

### 6C. New Docs Needed

| Doc | Status | Purpose |
|-----|--------|---------|
| `docs/planning/RENDERED-MANIFEST-PATTERN.md` | Done ✓ | CLI-focused RM pattern (11 tasks, 100-cluster scenario) |
| `docs/planning/RENDERED-MANIFEST-PATTERN-FULL-PRODUCT.md` | Done ✓ | Full SaaS + GUI mockups (15 features) |
| `docs/planning/SOURCES-CONCEPT.md` | Pending | Capture Issue #3339 design (Config Source) |

### 6D. Terminology Standardization (TBC)

| Task | Status | Notes |
|------|--------|-------|
| Standardize "App Space" vs "Space" terminology | TBC | Currently mixed usage (67 vs 899 occurrences). Consider adding "Space = shorthand for App Space" to GLOSSARY |

---

## Priority 7: Session Backlog

### From Session 01-05
- [x] Create glossary (Hub, App Space, Unit, WET/DRY) ✓

### From Session 01-06
- [ ] Migration path: Org → Space to Hub model
- [ ] Validate Hub model with IITS — match their mental model?

### From Session 01-08
- [ ] Add trace to Risk scanner for automated detection

### From Session 01-09
- [ ] Review if PROBLEMS.md needs updates

### From Session 01-10
- [x] ~~Ask Charlie to review and merge [PR #5: Add bubbletea-based Import Wizard TUI](https://github.com/confighubai/confighub-agent/pull/5)~~ — Merged 2026-01-10
- [x] Created `RENDERED-MANIFEST-PATTERN.md` and `RENDERED-MANIFEST-PATTERN-FULL-PRODUCT.md`
- [x] Added Boris Mode (verification-driven development) to CLAUDE.md
- [x] Added Chrome extension setup instructions to CLAUDE.md
- [x] ~~RISK-BOW POC Session 001 (dry-run)~~ — Completed 2026-01-10, 200 attacks generated
- [ ] **RISK-BOW Session 002 (live)** — Apply top attacks to test cluster, validate predictions
- [ ] [Issue #3: Extend trace to include ConfigHub sources, Helm, and raw kubectl apply](https://github.com/confighubai/confighub-agent/issues/3) — see P2 Trace Integration
- [ ] Upgrade Claude Code to v2.0.73+ for Chrome integration

---

## Priority 8: Apps + Actions + AI Architecture

**New framing doc:** `docs/planning/APPS-ACTIONS-AI-ARCHITECTURE.md`

### 8A. Architecture Layers (Session 2026-01-12)

| Layer | Status | Notes |
|-------|--------|-------|
| Source Workers | Existing | Workers connect to Sources (Git, DynamoDB, Helm, OCI) |
| Map | Existing | Hubs → App Spaces → Units + Labels + Decision Traces |
| Releases | **Design needed** | Versioned Map snapshots, "ready to run" |
| Apps Layer | Partially exists | Persistent apps + Actions + Triggers + ActiveQueries |
| AI Layer | **Design needed** | AI queries Map + Decision Traces for context |

### 8B. Releases Concept (New)

| Task | Effort | Notes |
|------|--------|-------|
| Design Release resource schema | Medium | What does a Release contain? |
| `cub release create` / `deploy` / `rollback` CLI | Medium | — |
| Release → App deployment flow | Medium | How Apps reference Releases |
| IITS "deploy entire platform" workflow | High | End-to-end validation |

### 8C. AI Integration

| Task | Effort | Notes |
|------|--------|-------|
| `confighub/ai-diagnose@v1` Action spec | Medium | Diagnosis from error + context |
| `confighub/ai-fix@v1` Action spec | Medium | Generate fix from diagnosis |
| Decision Trace capture for AI reasoning | Medium | Flywheel: exceptions → precedent → policy |
| AI context graph queries | High | Query Map + Traces for precedent |

### 8D. Documentation

| Task | Status |
|------|--------|
| `APPS-ACTIONS-AI-ARCHITECTURE.md` (framing) | Done ✓ |
| Review `global-app` example | Done ✓ |
| AI Action packaging question | Done ✓ (primitives + Apps) |
| DevOps as Apps (monadic) integration | Done ✓ |
| `use-case-devops-as-apps.md` (5 proposed Apps) | Done ✓ |
| Document Source types (supported vs planned) | Pending |
| Connect framing to Actions PRD | Pending |

---

## Blocked / Needs Discussion

| Item | Blocker |
|------|---------|
| Hub model migration path | Needs product decision |
| LIVE vs GIT import UX | Needs Jesper feedback |
| Which reference architectures to prioritize | Needs IITS input |
| **Release schema design** | Needs architecture review |

---

## Summary by Priority

| Priority | Category | Count |
|----------|----------|-------|
| P1 | Rendered Manifest Pattern | 11 |
| P2 | Trace Integration | 12 |
| P3 | Testing & Quality | 10 |
| P4 | Import Architecture | 8 |
| P5 | TUI Enhancements | 6 |
| P6 | Documentation | 3 |
| P7 | Session Backlog | 4 |
| P8 | Apps + Actions + AI Architecture | 12 |
| **Total** | | **66** |

---

## Quick Reference: Key Files

| Area | File |
|------|------|
| Ownership detection | `pkg/agent/ownership.go` |
| Flux trace | `pkg/agent/flux_trace.go` |
| Argo trace | `pkg/agent/argo_trace.go` |
| Import logic | `cmd/cub-agent/import.go` |
| **Import Wizard TUI** | `cmd/cub-agent/import_wizard.go` |
| Hierarchy TUI | `cmd/cub-agent/hierarchy.go` |
| GSF types | `pkg/gsf/types.go` |
| TUI map | `test/atk/map` |
| TUI scan | `test/atk/scan` |
| RM Pattern docs | `docs/planning/RENDERED-MANIFEST-PATTERN*.md` |
| **Apps+Actions+AI Architecture** | `docs/planning/APPS-ACTIONS-AI-ARCHITECTURE.md` |
| Actions PRD | `docs/planning/actions/confighub-actions-prd.md` |
| AI Context use case | `docs/planning/actions/use-case-confighub-ai-context.md` |
