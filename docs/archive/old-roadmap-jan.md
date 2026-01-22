# Completed Roadmap (January 2026)

**Archived:** 2026-01-22

This document contains completed roadmap items from the January 2026 development cycle.

---

## Completed: Documentation & Diagrams

| Item | Description | Status |
|------|-------------|--------|
| README positioning | Navigation-first "Demystify GitOps" tagline | ✅ Done |
| Problem framing | What's obscure about GitOps | ✅ Done |
| SCALE-DEMO | Navigation focus | ✅ Done |
| D2: Flux architecture | `docs/diagrams/flux-architecture.d2` | ✅ Done |
| D2: Ownership trace | `docs/diagrams/ownership-trace.d2` | ✅ Done |
| D2: Kustomize overlays | `docs/diagrams/kustomize-overlays.d2` | ✅ Done |
| D2: Ownership detection | `docs/diagrams/ownership-detection.d2` | ✅ Done |
| D2: Clobbering problem | `docs/diagrams/clobbering-problem.d2` | ✅ Done |
| D2: Upgrade tracing | `docs/diagrams/upgrade-tracing.d2` | ✅ Done |
| SVG renders | All D2 diagrams rendered to SVG | ✅ Done |

---

## Completed: Phase 1 - CLI UX Polish

### 1.1 `map orphans` — Context Header ✅

Added header explaining what orphans are, plus next steps:

```
ORPHAN RESOURCES
════════════════════════════════════════════════════════════════════
Resources not managed by GitOps (Flux, ArgoCD, Helm, ConfigHub).
These may be: legacy systems, manual hotfixes, debug pods, or shadow IT.

NAMESPACE           KIND           NAME                    OWNER
...

→ To import into ConfigHub: cub-scout import --wizard
→ To trace ownership: cub-scout trace <kind>/<name> -n <namespace>
```

### 1.2 `map issues` — Header, Breakdown, Next Steps ✅

Separated deployer issues from workload issues with breakdown summary.

### 1.3 Differentiate `map crashes` from `map issues` ✅

| Command | Focus | Shows |
|---------|-------|-------|
| `map crashes` | Pod health only | CrashLoopBackOff, ImagePullBackOff, OOMKilled, Error |
| `map issues` | GitOps health | All: deployers + workloads |

### 1.4 Summary Lines for All Commands ✅

Added summaries to `map workloads` and `map deployers`.

### 1.5 Link D2 Diagrams from Output ✅

Commands now link to relevant SVG diagrams for visual learning.

---

## Completed: Phase 2 - Learning Mode

### 2.1 `--explain` Flag for `map list` ✅

Shows ownership detection explanation and what each owner type means.

### 2.2 `--explain` Flag for `trace` ✅

Shows ownership chain explanation (Git → Kustomization → Deployment → Pods).

### 2.3 `--explain` Flag for `scan` ✅

Shows risk category explanations (stuck reconciliations, Kyverno violations, timing bombs, dangling resources).

---

## Completed: Phase 3 - Meaningful Example

### 3.1 `platform-example` ✅

Created complete GitOps platform example with ~50 resources:

```
examples/platform-example/
├── infrastructure/           # GitRepos, HelmRepos, RBAC, monitoring
├── apps/
│   ├── frontend/            # Base + dev/staging/prod overlays
│   ├── backend/             # Base + dev/staging/prod overlays
│   └── database/            # PostgreSQL HelmRelease + overlays
└── clusters/                # Dev, staging, prod Kustomizations
```

### 3.2 Clobbering Scenario ✅

Documented in example README - shows what happens when someone kubectl patches a GitOps resource.

### 3.3 Upgrade Tracing Scenario ✅

`trace --diff` command shows what changed between chart versions.

---

## Completed: Phase 4 - Documentation Restructure

### 4.1 Diataxis Structure ✅

Restructured docs following Diataxis framework:

```
docs/
├── getting-started/         # TUTORIALS
│   ├── install.md
│   └── first-map.md
├── howto/                   # HOW-TO GUIDES
│   ├── find-orphans.md
│   ├── trace-ownership.md
│   └── ...
├── reference/               # REFERENCE
│   ├── commands.md
│   ├── query-syntax.md
│   └── gsf-schema.md
├── concepts/                # EXPLANATION
│   ├── gitops-overview.md
│   └── clobbering-problem.md
└── diagrams/                # Visual guides
```

---

## Completed: Phase 5.6 - Diff & Upgrade Tracing

| Feature | Status |
|---------|--------|
| `trace --diff` | ✅ Done |

---

## Implementation Summary

| Phase | Items | Status |
|-------|-------|--------|
| Documentation & Diagrams | 11 | ✅ Complete |
| Phase 1: CLI UX Polish | 5 | ✅ Complete |
| Phase 2: Learning Mode | 3 | ✅ Complete |
| Phase 3: Meaningful Example | 3 | ✅ Complete |
| Phase 4: Documentation Restructure | 1 | ✅ Complete |
| Phase 5.6: trace --diff | 1 | ✅ Complete |
| **Total Completed** | **24** | |
