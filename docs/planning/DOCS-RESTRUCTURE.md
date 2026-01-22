# Documentation Restructure Plan

**Status:** PROPOSAL
**Date:** 2026-01-22
**Goal:** BRILLIANT, concise, clear, complete docs

---

## Current Problems

| Problem | Impact |
|---------|--------|
| **Scattered structure** | 5+ locations: `/docs`, `/docs/map`, `/docs/archive`, `/docs/outcomes`, `/docs/planning` |
| **Multiple entry points** | README.md, CLI-GUIDE.md, docs/README.md, docs/map/QUICKSTART.md all claim to be "start here" |
| **Duplicated content** | Quick start repeated in 3+ places |
| **Archive bloat** | 14 files in `/docs/archive` cluttering repo |
| **No learning path** | New user has no clear "start here → next → done" journey |
| **D2 diagrams orphaned** | Beautiful diagrams exist but nothing links to them |
| **Inconsistent naming** | CAPS vs lowercase vs hyphenated |

---

## Proposed Structure

Following the [Diataxis framework](https://diataxis.fr/) (tutorials, how-to, reference, explanation):

```
cub-scout/
├── README.md                    # Project intro + install (keep as entry point)
├── CLI-GUIDE.md                 # DELETE - merge into reference/commands.md
│
├── docs/
│   ├── README.md                # Docs index (single page, links to sections)
│   │
│   ├── getting-started/         # TUTORIALS (learning-oriented)
│   │   ├── install.md           # All install methods
│   │   ├── first-map.md         # Your first 5 minutes
│   │   └── understand-gitops.md # For GitOps newcomers (uses D2 diagrams)
│   │
│   ├── howto/                   # HOW-TO GUIDES (task-oriented)
│   │   ├── find-orphans.md
│   │   ├── trace-ownership.md
│   │   ├── scan-for-risks.md
│   │   ├── query-resources.md
│   │   └── import-to-confighub.md
│   │
│   ├── reference/               # REFERENCE (information-oriented)
│   │   ├── commands.md          # All CLI commands
│   │   ├── keybindings.md       # TUI shortcuts
│   │   ├── query-syntax.md      # Query language
│   │   ├── gsf-schema.md        # JSON schema
│   │   └── ownership-labels.md  # How detection works
│   │
│   ├── concepts/                # EXPLANATION (understanding-oriented)
│   │   ├── gitops-overview.md   # What is GitOps?
│   │   ├── ownership-detection.md
│   │   ├── clobbering-problem.md  # The PDF content!
│   │   └── flux-vs-argo.md
│   │
│   ├── diagrams/                # D2 source files (keep)
│   │   ├── flux-architecture.d2
│   │   ├── ownership-trace.d2
│   │   ├── kustomize-overlays.d2
│   │   ├── ownership-detection.d2
│   │   └── clobbering-problem.d2
│   │
│   └── planning/                # Internal (not user-facing)
│
├── examples/                    # Examples (simplify)
│   ├── README.md                # Overview + which to use when
│   ├── platform-example/        # THE example (new)
│   └── flux-boutique/           # Keep for simple demos
│
└── archive/                     # Move from docs/archive to root (or delete)
```

---

## Content Migration Plan

### ARCHIVE GOLD — Content to Extract

These archive docs have excellent ASCII art and content to migrate:

| Archive File | Gold Content | Migrate To |
|--------------|--------------|------------|
| `JOURNEY-MAP.md` | TUI screenshots: health bars, panels, trace boxes, keyboard shortcuts | `getting-started/first-map.md`, README |
| `JOURNEY-QUERY.md` | Query syntax, examples with expected output, cheat sheet | `reference/query-syntax.md` |
| `EXAMPLES-TUI-MAP-FLEET-IITS-STUDIES.md` | IITS pain points, "before/after" comparisons, real enterprise problems | `concepts/why-cub-scout.md` (new) |
| `IMPORT-GIT-REFERENCE-ARCHITECTURES.md` | GitOps pattern diagrams, repo structures, mapping rules | `concepts/gitops-patterns.md` |
| `TUI-TRACE.md` | Trace output examples | `howto/trace-ownership.md` |

### ASCII Art to Preserve

**Health dashboard:**
```
┌─ CLUSTER HEALTH ─────────────────────────────────────┐
│  ████████████████████░░░░  85%  (17/20 ready)       │
└──────────────────────────────────────────────────────┘
```

**Trace visualization:**
```
┌─ TRACE: payment-api ─────────────────────────────────┐
│  ┌─────────────────────────┐                        │
│  │ GitRepository           │                        │
│  │ flux-system/platform    │                        │
│  └───────────┬─────────────┘                        │
│              ▼                                       │
│  ┌─────────────────────────┐                        │
│  │ Kustomization           │                        │
│  └───────────┬─────────────┘                        │
│              ▼                                       │
│  ┌─────────────────────────┐                        │
│  │ Deployment              │                        │
│  └─────────────────────────┘                        │
└──────────────────────────────────────────────────────┘
```

**Fleet hierarchy:**
```
  payment-api
  |-- variant: prod
  |   |-- cluster-east @ rev 89
  |   |-- cluster-west @ rev 89
  |   |-- cluster-eu @ rev 87    <- behind!
  |-- variant: staging
      |-- cluster-staging @ rev 92
```

**Side-by-side panels:**
```
┌─ RESOURCES ────────────────┬─ PIPELINES ────────────┐
│  Flux        8  ████████   │  ✓ GitRepo → Kust → D  │
│  ArgoCD      5  █████      │  ✓ GitRepo → App → D   │
│  Helm        4  ████       │  ⚠ HelmRelease pending │
│  Native      3  ███        │                        │
└────────────────────────────┴────────────────────────┘
```

These visuals make users WANT to use cub-scout. Preserve them prominently.

### CONFIGHUB-AGENT Gold — Content to Migrate

The confighub-agent README.md has additional excellent content:

| Content | What | Migrate To |
|---------|------|------------|
| Trace boxes with emoji colors | 🟢🔴🟡🟣🔵 colored trace output | `howto/trace-ownership.md`, README |
| Saved queries display | Color-coded query list with counts | `reference/query-syntax.md` |
| "Three Sources of Truth" | Git=WHAT, ConfigHub=HOW, Cluster=NOW | `concepts/gitops-overview.md` |
| "For AI Agents" section | Structured context for LLMs | `concepts/ai-integration.md` (new) |
| Broken trace example | Shows "PROBLEM HERE" marker | `howto/trace-ownership.md` |
| Color legend | Emoji meanings explained | Reference docs |

**Trace with problem marker (compelling!):**
```
┌─────────────────────────────────────────────────────────────────────┐
│ TRACE: Deployment/broken-app                                        │
├─────────────────────────────────────────────────────────────────────┤
│   🟢 ✓ 🟣 GitRepository/infra-repo                                  │
│       └─▶ 🔴 ✗ 🔵 Kustomization/apps        ◀── PROBLEM HERE        │
│               │ 🔴 Error: path './clusters/prod/apps' not found     │
│               └─▶ Deployment/broken-app (stale)                     │
├─────────────────────────────────────────────────────────────────────┤
│ 🟡 ⚠ Chain broken at Kustomization/apps                            │
└─────────────────────────────────────────────────────────────────────┘
```

**Three Sources of Truth (core concept):**
- **Git says WHAT** — desired state (DRY: templates, Kustomizations)
- **ConfigHub says HOW** — which tool deploys what, who owns it (WET: rendered)
- **Cluster says NOW** — actual running state, may have drifted

This is the foundation of the "demystify" message.

### New Docs to Create (with archive content)

**`concepts/why-cub-scout.md`** — The compelling "why" page
```markdown
# Why cub-scout?

## The Problem (from IITS research)

> "What you see in the Git repository isn't what actually gets deployed...
> you need to mentally compile all these layers"

[ASCII art showing the problem]

## Without cub-scout
- SSH into each cluster
- Check each Argo/Flux dashboard
- Grep through repos
- **Time: Hours**

## With cub-scout
[ASCII art of TUI]
- One command
- **Time: Seconds**

## Real Questions Answered

| Question | Command | Time |
|----------|---------|------|
| What's deployed? | `cub-scout map` | 2 sec |
| Who owns what? | `cub-scout map workloads` | 2 sec |
| What's broken? | `cub-scout map issues` | 2 sec |
```

**`concepts/gitops-patterns.md`** — Pattern reference
- App-of-Apps
- ApplicationSet
- Flux Tenancy
- Mono-repo
- Each with: Repo structure, what TUI detects, ConfigHub mapping

**`getting-started/first-map.md`** — 5-minute quickstart with ASCII screenshots
- Install
- Run `cub-scout map`
- See health dashboard (with ASCII art)
- Navigate views
- Trace a resource (with trace box art)

---

### DELETE (after migration)

### CONSOLIDATE
| Old Files | New File |
|-----------|----------|
| README quick start + QUICKSTART.md | `getting-started/first-map.md` |
| `docs/map/reference/commands.md` + CLI-GUIDE.md | `reference/commands.md` |
| `docs/map/reference/keybindings.md` | `reference/keybindings.md` (keep) |
| Multiple ownership docs | `concepts/ownership-detection.md` |

### CREATE NEW
| New File | Content |
|----------|---------|
| `getting-started/understand-gitops.md` | For newcomers, links D2 diagrams |
| `concepts/clobbering-problem.md` | From the PDF |
| `concepts/gitops-overview.md` | What is GitOps? (for newcomers) |
| `howto/scan-for-risks.md` | CCVE scanning guide |

### KEEP AS-IS
- `docs/diagrams/*` (D2 files)
- `docs/planning/*` (internal)
- `examples/flux-boutique/` (simple example)

---

## New docs/README.md

```markdown
# cub-scout Documentation

**Demystify GitOps. See what's really happening in your cluster.**

## Getting Started

New to cub-scout? Start here:

1. **[Install](getting-started/install.md)** - Get cub-scout running
2. **[First Map](getting-started/first-map.md)** - See your cluster in 5 minutes
3. **[Understand GitOps](getting-started/understand-gitops.md)** - New to GitOps? Start here

## How-To Guides

Task-based guides:

- [Find orphan resources](howto/find-orphans.md) - Resources not in Git
- [Trace ownership chains](howto/trace-ownership.md) - Git → Deployment → Pod
- [Scan for risks](howto/scan-for-risks.md) - 46 configuration patterns
- [Query resources](howto/query-resources.md) - Filter and search
- [Import to ConfigHub](howto/import-to-confighub.md) - Connect for fleet visibility

## Reference

Complete reference:

- [Commands](reference/commands.md) - All CLI commands
- [Keybindings](reference/keybindings.md) - TUI shortcuts
- [Query Syntax](reference/query-syntax.md) - Filter language
- [Ownership Labels](reference/ownership-labels.md) - How detection works
- [GSF Schema](reference/gsf-schema.md) - JSON output format

## Concepts

Understand the "why":

- [GitOps Overview](concepts/gitops-overview.md) - What is GitOps?
- [Ownership Detection](concepts/ownership-detection.md) - How cub-scout knows who owns what
- [The Clobbering Problem](concepts/clobbering-problem.md) - Why direct changes are risky
- [Flux vs ArgoCD](concepts/flux-vs-argo.md) - Comparing GitOps tools

## Visual Guides

See the [diagrams](diagrams/) for visual explanations:

- [Flux Architecture](diagrams/flux-architecture.d2) - How Flux GitOps works
- [Ownership Trace](diagrams/ownership-trace.d2) - What cub-scout reveals
- [Kustomize Overlays](diagrams/kustomize-overlays.d2) - Multi-environment pattern
- [Clobbering Problem](diagrams/clobbering-problem.d2) - Hidden layer dangers
```

---

## Naming Conventions

| Type | Convention | Example |
|------|------------|---------|
| **Directories** | lowercase, hyphenated | `getting-started/` |
| **Files** | lowercase, hyphenated | `find-orphans.md` |
| **Headers** | Title Case | `# Find Orphan Resources` |
| **Code** | backticks | `cub-scout map` |

---

## Writing Style

1. **Concise** - Say it in 10 words, not 50
2. **Task-focused** - "To find orphans, run..." not "Orphans are resources that..."
3. **Code first** - Show the command, then explain
4. **No fluff** - No "In this guide, we will..." — just do it

**Example - BAD:**
> In this section, we will explore how to use the cub-scout map command to discover resources in your Kubernetes cluster that are not currently being managed by any GitOps tooling.

**Example - GOOD:**
> ```bash
> cub-scout map orphans
> ```
> Shows all resources not managed by Flux, ArgoCD, or Helm.

---

## Success Criteria

After restructure:

| Criteria | Measure |
|----------|---------|
| **One entry point** | README → docs/README → sections |
| **No duplicates** | Content exists in ONE place |
| **Clear paths** | "New user? Start here." / "Task? How-to." / "Details? Reference." |
| **Diagrams linked** | Every D2 diagram referenced from relevant docs |
| **Archive clean** | Old docs moved or deleted |
| **Consistent style** | All docs follow naming + writing conventions |

---

## Implementation Order

1. **Create new structure** - Make directories, stub files
2. **Migrate content** - Move existing good content
3. **Write new docs** - `understand-gitops.md`, `clobbering-problem.md`
4. **Delete old** - Remove archive, duplicates
5. **Update links** - Fix all cross-references
6. **Review** - Read through as new user

**Estimated effort:** 4-6 hours

---

## Questions for Review

1. Should `archive/` be deleted entirely or kept at repo root?
2. Should examples be consolidated further?
3. Should planning docs move out of `docs/`?
