# TUI Hub View Enhancements — TODO

**Created:** 2026-01-14 | **Status:** Planning
**Source:** GLOSSARY-OF-CONCEPTS.md gap analysis

---

## Overview

The ConfigHub-connected view (Hub mode, `H` key) needs to fully support all concepts from GLOSSARY-OF-CONCEPTS.md. Currently "partially supported" items must become "100% supported."

---

## Priority 1: Core Concept Views

### 1A. App View (NEW)

**Problem:** No dedicated "App" view. Apps are just label values (`app=payment-api`) but users think in terms of apps.

**Solution:** Add App view that queries across Units by `app` label.

| Task | Effort | Notes |
|------|--------|-------|
| Add `a` key binding for App view | Low | New tab in Hub mode |
| Query Units grouped by `app` label value | Medium | `cub unit list --where "Labels['app'] = 'X'"` |
| Show all variants of each app | Medium | Group by `variant` label |
| Show deployment status per variant | Medium | Include target/cluster status |

**Expected Output:**
```
APPS
─────────────────────────────────────────────────────────────────
payment-api
  ├── prod     → prod-east (healthy)
  ├── staging  → staging-cluster (healthy)
  └── dev      → dev-cluster (syncing)

order-service
  ├── prod     → prod-east (healthy), prod-west (healthy)
  └── staging  → staging-cluster (degraded)

redis
  └── prod     → prod-east (healthy)
```

### 1B. Variant Display

**Problem:** Variants (`prod`, `staging`, `dev`) not prominently displayed in Unit views.

**Solution:** Show variant label prominently when viewing Units.

| Task | Effort | Notes |
|------|--------|-------|
| Add `variant` column to Unit list | Low | Extract from Labels |
| Color-code variants (prod=green, staging=yellow, dev=blue) | Low | Visual distinction |
| Group Units by variant option | Medium | Alternative view mode |
| Show variant in Unit detail panel | Low | Already have detail panel |

### 1C. Platform Hub / Base Catalog View

**Problem:** Hub's Base Catalog (templates) not browsable in TUI.

**Solution:** Add Base Catalog browsing under Hub node.

| Task | Effort | Notes |
|------|--------|-------|
| Query Hub's base templates | Medium | New API call needed |
| Show which Units derive from which base | Medium | Relationship display |
| Display template parameters/variables | High | Schema parsing |

**Note:** May require ConfigHub API support for base catalog queries.

---

## Priority 2: Map Subviews (from INTRODUCTION.md)

### 2A. Problems View

**Problem:** Users ask "What's broken right now?" — no dedicated view.

**Solution:** Add Problems view filtering to broken/degraded resources.

| Task | Effort | Notes |
|------|--------|-------|
| Add `p` key for Problems view | Low | Filter to unhealthy resources |
| Show Flux/Argo sync failures | Low | Already have status |
| Show Pod crashes, restarts | Medium | Need pod status |
| Group by severity | Low | Critical first |

### 2B. Suspended View

**Problem:** Users ask "What's suspended/forgotten?" — hard to find.

**Solution:** Add Suspended view for paused reconciliations.

| Task | Effort | Notes |
|------|--------|-------|
| Add `u` key for sUspended view | Low | Filter to suspended |
| Show Flux `suspend: true` | Low | Parse Kustomization/HelmRelease |
| Show Argo `operation.sync.suspended` | Low | Parse Application |
| Show age since suspended | Low | Flag long-dormant items |

### 2C. Pipelines View

**Problem:** Users ask "What are my GitOps pipelines?" — no overview.

**Solution:** Add Pipelines view showing deployment chains.

| Task | Effort | Notes |
|------|--------|-------|
| Add `P` (shift-p) for Pipelines | Low | Dedicated view |
| Show Source → Kustomization → Target | Medium | Trace dependencies |
| Show sync status per stage | Low | Already have status |
| Show last sync time | Low | Freshness indicator |

---

## Priority 3: Enhanced Status Views

### 3A. Worker Status in Hub Mode

**Problem:** Worker status shown but not prominently featured.

| Task | Effort | Notes |
|------|--------|-------|
| Show worker health indicator at Space level | Low | Already have data |
| Alert when no workers connected | Low | Warning banner |
| Show worker → target mapping | Medium | Which worker serves which target |

### 3B. Risk Categories Tab

**Problem:** Scan shows findings but no category browser.

**Solution:** Add risk categories panel in Hub mode.

| Task | Effort | Notes |
|------|--------|-------|
| Add Risk tab/panel in Hub view | Medium | New panel |
| Show findings grouped by category | Medium | SOURCE, RENDER, APPLY, etc. |
| Link findings to affected Units | Medium | Click to navigate |
| Show severity distribution | Low | Critical/High/Medium/Low counts |

**Expected Output:**
```
Risk FINDINGS BY CATEGORY
─────────────────────────────────────────────────────────────────
STATE (3)
  [C] RISK-2025-0166  HelmRelease/redis-cluster stuck
  [W] RISK-2025-0012  Kustomization/monitoring BuildFailed
  [W] RISK-2025-0169  Application/frontend Degraded

ORPHAN (2)
  [W] RISK-2025-0638  Deployment/debug-pod not tracked
  [W] RISK-2025-0638  ConfigMap/temp-config not tracked

DRIFT (1)
  [I] RISK-2025-0639  Service/api spec differs from Git
```

---

## Priority 4: Navigation Enhancements

### 4A. Cross-Reference Navigation

| Task | Effort | Notes |
|------|--------|-------|
| From Unit → show owning App | Low | Link back to App view |
| From Unit → show Base template (if derived) | Medium | Source lineage |
| From Risk finding → affected Unit | Medium | Direct navigation |
| From Target → Units deployed there | Medium | Reverse lookup |

### 4B. Search Improvements

| Task | Effort | Notes |
|------|--------|-------|
| Search across Units by name/label | Low | Already have search |
| Search by variant | Low | Filter option |
| Search by app name | Low | Label search |
| Search by Risk ID | Medium | Link to Risk database |

---

## Priority 5: Three-State Model (from INTERNAL-PITCH)

### 5A. Three-State View

**Problem:** Users can't see Git intent vs ConfigHub state vs Cluster reality in one view.

**Source:** "ConfigHub is the operational source of truth. It knows both what Git says and what the cluster says."

| Task | Effort | Notes |
|------|--------|-------|
| Add three-column view: Git → ConfigHub → Cluster | High | Visual diff |
| Show sync status between each state | Medium | Arrows with status |
| Highlight discrepancies (drift) | Medium | Color-coded diffs |
| Click to expand diff details | Medium | Inline diff view |

**Expected Output:**
```
THREE-STATE VIEW: payment-service
─────────────────────────────────────────────────────────────────
GIT (intent)           CONFIGHUB (state)      CLUSTER (reality)
─────────────────────────────────────────────────────────────────
replicas: 3      ✓──▶  replicas: 3      ⚠──▶  replicas: 2
image: v1.2.3    ✓──▶  image: v1.2.3    ✓──▶  image: v1.2.3
limits: 512Mi    ✓──▶  limits: 512Mi    ✓──▶  limits: 512Mi

⚠ Drift detected: Cluster has 2 replicas, expected 3
```

### 5B. Drift Merge Action

**Problem:** "Merge a hotfix back to Git cleanly" — no TUI action.

**Source:** `cub drift merge` — "10 sec vs 30 min"

| Task | Effort | Notes |
|------|--------|-------|
| Add `M` key for Merge action | Low | When drift detected |
| Show preview of merge changes | Medium | Diff view |
| Call `cub drift merge` | Low | CLI integration |
| Show MR/PR link after merge | Low | Actionable result |

### 5C. History View

**Problem:** Users ask "What changed?" — no history browser.

**Source:** "What changed (history)" — Map should show this.

| Task | Effort | Notes |
|------|--------|-------|
| Add `h` key for History view | Low | New tab |
| Show revision history per Unit | Medium | ConfigHub API |
| Show who changed what, when | Medium | Audit trail |
| Diff between revisions | High | Side-by-side diff |

### 5D. Dependencies View

**Problem:** Users ask "What depends on what?" — no relation view.

**Source:** "What depends on what (relations)" — Map should know this.

| Task | Effort | Notes |
|------|--------|-------|
| Add `d` key for Dependencies | Low | New tab |
| Show dependency graph | High | Visual graph |
| Show upstream (what this depends on) | Medium | Parent links |
| Show downstream (what depends on this) | Medium | Child links |

---

## Priority 6: Journey Doc Features (Verify Implementation)

**Source:** JOURNEY-*.md docs describe features that should exist. Verify these work.

### 6A. Help Screen (`?` key)

**From:** JOURNEY-MAP.md

| Task | Effort | Notes |
|------|--------|-------|
| Add `?` key for help overlay | Low | Show all keybindings |
| Dynamic help based on mode | Low | Different help per view |

### 6B. Auto-Fix Action (`f` key)

**From:** JOURNEY-SCAN.md — "Press [f] to fix (if remediation available)"

| Task | Effort | Notes |
|------|--------|-------|
| Add `f` key in scan results | Low | When Risk has remediation |
| Show fix preview before apply | Medium | Dry-run first |
| Apply fix and re-scan | Medium | Verify fix worked |

### 6C. Namespace Navigation

**From:** JOURNEY-MAP.md — `[n] next namespace, [p] previous namespace`

| Task | Effort | Notes |
|------|--------|-------|
| Add `n`/`N` keys for namespace nav | Low | Quick namespace jump |
| Show namespace count/position | Low | "3/12 namespaces" |

### 6D. Output Formats

**From:** JOURNEY-QUERY.md — `--json`, `--count`, `--names-only`

| Task | Effort | Notes |
|------|--------|-------|
| Verify `--json` works for all commands | Low | GSF output |
| Add `--count` option | Low | Just count, no list |
| Add `--names-only` option | Low | For scripting |

### 6E. Query Management

**From:** JOURNEY-QUERY.md — `--save`, `--query`, `--list-queries`

| Task | Effort | Notes |
|------|--------|-------|
| Verify `--save <name>` works | Low | Save current query |
| Verify `--query <name>` works | Low | Run saved query |
| Verify `--list-queries` works | Low | List all saved |
| Persist queries to `~/.confighub/queries/` | Medium | File-based storage |

### 6F. Import Log & Resume

**From:** JOURNEY-IMPORT.md — logs at `~/.confighub/logs/import-*.log`

| Task | Effort | Notes |
|------|--------|-------|
| Verify import logs are written | Low | Check location |
| Add `--resume` flag | Medium | Continue interrupted import |
| Show log path after import | Low | User can review |

---

## Priority 7: Session & State

### 7A. Upgrade Call-to-Action (CTA)

**Problem:** Users in standalone mode don't know what they're missing with ConfigHub.

**Solution:** Show contextual upgrade prompts at bottom of each tab.

| Tab | CTA Message |
|-----|-------------|
| Map | "Want fleet-wide visibility? → `cub-agent map confighub`" |
| Scan | "Want to track findings over time? → Connect to ConfigHub" |
| Trace | "Want ownership across all clusters? → Connect to ConfigHub" |
| Query | "Want to query across your fleet? → Connect to ConfigHub" |

| Task | Effort | Notes |
|------|--------|-------|
| Add CTA component to tab footer | Low | Subtle, non-intrusive |
| Make CTA contextual per tab | Low | Different message per view |
| Link to docs/getting-started | Low | Actionable next step |
| Hide CTA when already connected | Low | Don't show if in Hub mode |

### 7B. Session State Persistence

**Status:** Design spec only (TUI-SESSION-STATE.md)

| Task | Effort | Notes |
|------|--------|-------|
| Implement local session file | Medium | `~/.confighub/sessions/` |
| Resume import workflow | High | Multi-step state |
| Save view preferences | Low | Last tab, expanded nodes |
| Cloud sync (Phase 2) | High | ConfigHub API integration |

---

## Open Questions / Issues to File

### Issue: How to Query Apps?

**Question:** Should we:
1. Query all Units and group client-side by `app` label?
2. Add a ConfigHub API endpoint for app aggregation?
3. Use `cub unit list --group-by app`?

**Recommendation:** Start with client-side grouping (#1), file API enhancement request for #2.

### Issue: Base Catalog API

**Question:** Is there a `cub` CLI command to list base templates in a Hub's catalog?

**Action:** Check `cub --help` for catalog commands. May need API enhancement.

### Issue: Variant Color Scheme

**Question:** What colors for variants?
- `prod` → Green (production)
- `staging` → Yellow/Amber (pre-prod)
- `dev` → Blue (development)
- `canary` → Purple (experimental)
- Other → Gray

**Action:** Confirm with design guidelines.

---

## Implementation Order

**Phase 1: Quick Wins (Low effort, high impact)** ✅ COMPLETE
1. ✅ **App View** (1A) — `a` key, groups by app label
2. ✅ **Variant Display** (1B) — Color-coded prod/staging/dev badges
3. ✅ **Problems View** (2A) — Dashboard shows problems first
4. ✅ **Suspended View** (2B) — `u` key for suspended resources
5. ✅ **Help Screen** (6A) — `?` key (already existed)
6. ✅ **Upgrade CTA** (7A) — Footer hints in dashboard

**Phase 2: Map Enhancements** ✅ COMPLETE
7. ✅ **Pipelines View** (2C) — `P` key, visual flow with type grouping
8. ✅ **Risk Categories Tab** (3B) — Scan results grouped by category
9. ✅ **Worker Status** (3A) — Header indicators + disconnect warnings
10. ✅ **Namespace Navigation** (6C) — `n`/`N` keys with indicator

**Phase 3: Verify Journey Doc Features** ✅ COMPLETE
11. ✅ **Auto-Fix Action** (6B) — `f` dry-run, `F` apply
12. ✅ **Output Formats** (6D) — Added `--count`, `--names-only`
13. ✅ **Query Management** (6E) — Working: `map queries`, save, delete
14. ✅ **Import Logging** (6F) — Logs to `.confighub/logs/` (resume deferred)

**Phase 4: Three-State Model** — ConfigHub Features
15. 🔗 **Three-State View** (5A) — ConfigHub web UI feature
16. ✅ **Drift Panel Enhanced** — Groups by Flux/ArgoCD with sync hints
17. 🔗 **History View** (5C) — ConfigHub web UI (`cub revision list`)

**Phase 5: Advanced Features** ✅ COMPLETE
18. ✅ **Dependencies View** (5D) — `55ef0b0` D key, upstream/downstream relations
19. ✅ **Cross-Reference Navigation** (4A) — `b86f3c1` Enter key in panel mode
20. 🔗 **Base Catalog** (1C) — ConfigHub platform feature
21. ✅ **Session State** (7B) — `48ae9d2` + `1e0af98` Local + Hub snapshots

**Phase 6: TUI Unification** — [Issue #10](https://github.com/confighubai/confighub-agent/issues/10)
22. 📋 **Unified TUI** — Merge localcluster.go and hierarchy.go

---

## Related Docs

**Source documents for this TODO:**
- [INTRODUCTION.md](historical/2026-01-07-before-reorg/INTRODUCTION.md) — Problems by stage, Map subviews
- [INTERNAL-PITCH-WHY-WE-NEED-THIS.md](https://github.com/confighubai/confighub-agent/search?q=INTERNAL-PITCH-WHY-WE-NEED-THIS.md) — Three-State Model, drift merge

**Journey docs (verify features exist):**
- [JOURNEY-FIRST-SETUP.md](https://github.com/confighubai/confighub-agent/search?q=JOURNEY-FIRST-SETUP.md) — First-time setup flow
- [JOURNEY-MAP.md](https://github.com/confighubai/confighub-agent/search?q=JOURNEY-MAP.md) — Map TUI navigation, keybindings
- [JOURNEY-SCAN.md](https://github.com/confighubai/confighub-agent/search?q=JOURNEY-SCAN.md) — Scan TUI, auto-fix
- [JOURNEY-QUERY.md](https://github.com/confighubai/confighub-agent/search?q=JOURNEY-QUERY.md) — Query syntax, save/load queries
- [JOURNEY-IMPORT.md](https://github.com/confighubai/confighub-agent/search?q=JOURNEY-IMPORT.md) — Import wizard, logs, resume
- [04-MAP-USER-JOURNEY-TO-FULL-CONFIGHUB.md](map/04-MAP-USER-JOURNEY-TO-FULL-CONFIGHUB.md) — Adoption stages

**Concept references:**
- [GLOSSARY-OF-CONCEPTS.md](https://github.com/confighubai/confighub-agent/search?q=GLOSSARY-OF-CONCEPTS.md) — Concept definitions
- [TUI-SESSION-STATE.md](https://github.com/confighubai/confighub-agent/search?q=TUI-SESSION-STATE.md) — Session state design
- [02-HUB-APPSPACE-MODEL.md](map/02-HUB-APPSPACE-MODEL.md) — Hub/Space model

**Implementation:**
- [hierarchy.go](../archive/cub-agent/hierarchy.go) — Current Hub TUI implementation
- [localcluster.go](../archive/cub-agent/localcluster.go) — Local cluster TUI
