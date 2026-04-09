# cub-scout Handover for the Next AI Coder

Last updated: 2026-04-09

## Current repo state

- Branch: `main`
- Canonical roadmap: `docs/roadmap.md`
- Delivery rules: `docs/workflows/agent-milestone-plan.md`
- Note: local worktree currently has untracked `.claude/` and `LIVE`; treat them as user-owned and do not modify them unless explicitly asked

## Recent completions

The connected/AI-first issue cluster from March 2026 is now closed:

| Issue | Title | Status |
|-------|-------|--------|
| #348 | Preserve status fields in scan/export path | Closed (Apr 2) |
| #329 | Make GitOps import and connected demos AI-first | Closed |
| #327 | Kubara-oriented Argo debugging guide | Closed |
| #326 | Refresh ADT docs with connected commands | Closed |
| #325 | Treat OSS SDK cmd/cub as source of truth | Closed |
| #324 | Align import proposals with app-centric model | Closed |
| #323 | Define canonical App/Deployment/Target mapping | Closed |
| #321 | Make ADT primary model across docs | Closed |
| #343-347 | Archive stale branch ideas | Closed (Apr 2) |

Key deliverables now in place:
- AI-first demo structure in `examples/argo-import-confighub-demo/` and `examples/flux-import-confighub-demo/`
  - `AI_START_HERE.md`, `prompts.md`, `contracts.md`, `verify.sh`
- Status-dependent scanner rules work correctly (#348 regression test added)
- App/Deployment/Target is the primary mental model across connected docs
- Deterministic `TRY NEXT` hints are stronger and more contextual (`cec20f8`, `#349`)
- `explain` suggests ConfigHub GUI deep-links when connected context provides a unit URL (`a5016ad`, `#350`)
- CLI color output with `NO_COLOR` support for doctor and explain commands (`2768e0e`, `#351`)
- Explicit `--presentation` flag for doctor and explain commands (`a5a5e59`, `#352`)
  - Three modes: `human` (default), `ai`, `paired`
  - Opt-in only: default behavior unchanged without flag
  - AI mode uses uppercase markers, bracket notation, `RECOMMENDED ACTIONS` heading
- #342 bidirectional snapshot and conformance workflow (both slices)
  - Conformance reporting for import proposals
  - Curated import selection with include/exclude filtering
- #328 secrets — complete (`2346598`, `a6b5128`, `e527bdd`, `53e4ca0`)
  - Slice 1: Secret evidence in `trace` for workloads and Flux sources
  - Slice 2: Crossplane ProviderConfig with cross-namespace resolution
  - Slice 3: Secret issues in `map issues` output
  - Slice 4: TUI integration (secret panel in trace view)
  - Dynamic CRD discovery for any *.crossplane.io / *.upbound.io provider
  - Status classification: present, missing, unreadable, unresolved
  - RBAC-aware error detection (Forbidden vs NotFound)
  - Safe metadata only (never exposes .data or .stringData)
  - HelmRepository and Bucket coverage for Flux source secrets
  - Optional secrets correctly excluded from issues

## Open issues

Current tracked follow-ons:

| Issue | Title | Notes |
|-------|-------|-------|
| #359 | Extend `--presentation` to additional read-only commands | Useful polish, lower priority |
| #360 | Carry secret evidence through the legacy trace v0.14 JSON converter | Low-priority contract cleanup |
| #362 | Stabilize intermittent `TestContextPack_FormatJSON` kill in `test/ascii` | Test-stability follow-on from v1.9 release verification |
| #366 | Connected mode: surface three-way disagreement between ConfigHub, Argo, and cluster state | Important connected evidence follow-on |
| #367 | Phase-aware next-step hints for Argo-managed incidents and closeout | Highest-leverage next slice; builds on #365 ownership truth |
| #363 | Enhance Git parser to support ArgoCD ApplicationSet git generators | Active continuation of the Git import track after #357 |
| #364 | Investigate integration with `cub gitops import` rendering for manifest preview | Active continuation of the Git import track after #357 |

Recent closures:
- #365 — `explain` ownership for ArgoCD ApplicationSet-managed resources (Apr 9)
  - Two commits: ownership-preserving fallback + negative mismatch candidate filtering
  - `explain` now correctly reports ArgoCD for tracking-id annotated resources
- #357 — Git as a first-class source: initial local `--git-path` preview slice complete
- #356, #358, #361 — Docs/example sync cluster complete (Apr 6)
- #328 — Secrets in cub-scout: all slices complete (trace + Crossplane + map issues + TUI)
- #342 — Bidirectional snapshot and conformance workflow: both slices shipped
- #349, #350, #351, #352 — CLI polish cluster complete

Important: the current deterministic hints are already good and should not be
diminished. `#349` strengthened them; any follow-on hint work should keep moving
in that direction rather than flattening the current system.

Also important: the Apr 9 issues split into two distinct tracks:
- Argo/AI/operator truth-and-guidance: `#365` (closed), `#366`, `#367`
- Git import continuation: `#363`, `#364`

The recommended next slice is `#367` because:
- `#365` just restored ownership truth in `explain` (closed Apr 9)
- `#367` builds directly on that truth with better next-step guidance for Argo incident/closeout flows
- It's narrower and easier to ship cleanly than the broader connected-state surface in `#366`

## Git Import Architecture (Critical Context for #363 / #364)

This section documents the relationship between different import tools across
multiple repos. Understanding both `cub-scout import` and the broader `cub`
CLI import/rendering path is essential for continuing the Git import track.

### The Three Import Surfaces

1. **`cub gitops import`** (confighub/sdk - `cmd/cub/gitops_import.go`)
   - Discovers ArgoCD/Flux resources **from live K8s cluster** (not Git directly)
   - Uses render targets to get manifests (ArgoCD API or Flux renderer)
   - Creates ConfigHub units: -dry (renderer), -crds, -wet (rendered output)
   - Requires: running ArgoCD/Flux controller, K8s target, render target

2. **`cub gitops discover`** (confighub/sdk - `cmd/cub/gitops_discover.go`)
   - Finds ArgoCD Applications, Flux HelmReleases/Kustomizations in cluster
   - Prerequisite step for `cub gitops import`
   - Query: `import.include_custom = true AND kind IN ('Application','HelmRelease','Kustomization')`

3. **`cub-scout import --git-path`** (this repo - `cmd/cub-scout/import.go`)
   - Parses Git repo structure **locally** without cluster
   - Shows what SHOULD be deployed (Git source of truth)
   - Enables Git↔cluster comparison for verification
   - Initial implementation: uses `gitops.ParseRepo()` for Flux-style patterns

### The Full ConfigHub Loop (from Slack discussion)

```
Git repo → ArgoCD syncs to cluster
        → cub gitops discover (finds ArgoCD apps)
        → cub gitops import (renders via ArgoCD API)
        → ConfigHub units created
        → Edit in ConfigHub
        → Apply back via Argo+OCI
```

Sample repo demonstrating this: `jesperfj/gitops-argocd`

### Rendering Implementations (confighub/sdk - bridge-impl/)

**ArgoCD Renderer** (`argocd-renderer/renderer.go`):
- Calls ArgoCD API to render Applications to manifests
- Requires running ArgoCD controller in cluster
- Creates/updates Application, waits for sync, fetches rendered manifests

**Flux Renderer** (`flux-renderer/`):
- `kustomize.go`: Fetches artifact URL, runs `kustomize build` locally
- `helm.go`: Loads chart from URL, uses Helm template engine locally
- Can render without cluster controller (just needs artifact URLs)

### Current Gap: ArgoCD ApplicationSet Git Generators

The closed #357 initial slice uses `gitops.ParseRepo()` which supports
Flux-style patterns (`apps/base/`, `apps/staging/`). However, ArgoCD repos
often use **ApplicationSets with git generators**:

```yaml
# applicationsets/apps.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
spec:
  generators:
    - git:
        repoURL: https://github.com/org/repo
        directories:
          - path: apps/*    # ← This pattern defines what apps exist
```

**What exists in cub-scout:**
- `pkg/gitops/parser.go`: Has `ApplicationSetDef` type with `TargetApps` field
- `internal/patterns/pattern_git_aware.go`: `extractGeneratorTypes()` gets type names

**What's missing:**
- Extracting `directories[].path` patterns from git generators
- Scanning matching directories in repo
- Populating `TargetApps` with discovered apps

This gap means `cub-scout import --git-path` on `jesperfj/gitops-argocd` would
not find the apps defined by the ApplicationSet.

### Files Across Repos

| Repo | File | Purpose |
|------|------|---------|
| cub-scout | `cmd/cub-scout/import.go` | `--git-path` flag, Git preview flow |
| cub-scout | `cmd/cub-scout/import_git_test.go` | Git import tests |
| cub-scout | `pkg/gitops/parser.go` | `ParseRepo()`, needs ApplicationSet enhancement |
| confighub/sdk | `cmd/cub/gitops_import.go` | Import from K8s cluster |
| confighub/sdk | `cmd/cub/gitops_discover.go` | Discover GitOps resources in cluster |
| confighub/sdk | `bridge-impl/argocd-renderer/` | ArgoCD manifest rendering |
| confighub/sdk | `bridge-impl/flux-renderer/` | Flux manifest rendering (kustomize, helm) |

### Decision: Comparison is Automatic

When both Git and cluster sources are provided, comparison happens automatically:
- `import --git-path ./repo` → Git-only preview
- `import --git-path ./repo -n prod` → Git↔cluster comparison
- `import --git-path ./repo --from-bundle ./bundle` → Git↔bundle comparison

No separate `--compare` flag needed.

## Suggested next milestones

1. Milestone 1: #367 — phase-aware next-step hints for Argo-managed incidents and closeout
   - Builds directly on the #365 ownership truth fix (now closed)
   - Narrower and easier to ship than #366
   - High leverage for AI+human Argo debugging workflows
2. Milestone 2: #366 — connected three-way disagreement surface
   - Surface disagreement between ConfigHub, Argo/Flux, and cluster state
   - Important for connected mode evidence, but broader scope than #367
3. Milestone 3: Continue the Git import track via #363 and #364
   - #357 is CLOSED as the initial local `--git-path` preview slice
   - #363: enhance parser for ArgoCD ApplicationSet git generators
   - #364: investigate rendering integration between `cub-scout import` and `cub gitops import`
4. Milestone 4: polish/compat follow-ons (#359, #360, #362)
   Extend `--presentation` incrementally to other read-only commands if useful,
   close the legacy v0.14 trace JSON converter gap when worth it, and stabilize
   the `context-pack` test kill path.

## External publication boundary

Do not publish work externally without explicit user approval.

That includes:
- pushing to `confighubai/confighub`
- opening PRs or issue comments there
- creating public or secret gists for the work
- posting branch status externally

## Current wedge

The near-term product wedge is:

`GitHub + Argo/Flux + AI/CLI + ConfigHub`

## Themes to preserve

1. AI-first is structural, not decorative
   The examples follow the incubator pattern: four-file bundle, progressive disclosure,
   three-surface evidence boundaries. Runnable by Claude/Codex with minimal guessing.
2. Additive to Argo/Flux, not a replacement
   `cub-scout` explains, imports, compares, and proves. It does not need to be the runtime authority.
3. Command vs watch must stay explicit
   ConfigHub is for intended config and command intent. Runtime state/status
   comes from live systems and is authoritative there.
4. App/Deployment/Target as the primary mental model
   Space/Unit remains the current storage/API vocabulary, but not the front-door explanation.
5. Trustworthy evidence over optimistic status
   If ConfigHub/runtime status is uncertain, say so and show cluster/controller evidence.
6. Import must pay off immediately
   If a demo only shows import success and not value after import, it will feel academic.

## What "AI-first" means here

AI-first is structural, not decorative. It shapes the example's script structure,
evidence model, and file layout.

For the core connected demos, "AI-first" means:

1. each major example has:
   - `README.md` — human-oriented, answers the six reader questions
   - `AI_START_HERE.md` — safe entry point for AI agents with mutation boundaries
   - `prompts.md` — copyable prompts (orient, walkthrough, verify)
   - `contracts.md` — stable inspection paths with evidence boundaries
   - `verify.sh` — three-surface verification script
2. evidence boundaries are explicit and three-surfaced:
   - cluster evidence (kubectl, controller API)
   - ConfigHub evidence (cub unit list, cub target list, unit-action results)
   - cub-scout evidence (gitops status, map list, ownership classification)

## File hotspots

### AI-first demo examples (now complete)

- `examples/argo-import-confighub-demo/` — full AI-first structure
- `examples/flux-import-confighub-demo/` — full AI-first structure

### Key docs

- `docs/howto/import-to-confighub.md`
- `docs/howto/import-from-live.md`
- `docs/howto/using-cub-scout-from-ai-tool.md`
- `docs/reference/commands.md`
- `docs/reference/json-contracts.md`

### Presentation mode implementation (#352 complete)

- `cmd/cub-scout/presentation.go` — mode types and helper functions
- `cmd/cub-scout/doctor.go` — `--presentation` flag and rendering
- `cmd/cub-scout/explain.go` — `--presentation` flag and rendering

### Secret evidence implementation (#328 complete)

- `pkg/agent/secret_evidence.go` — core model and collector
- `pkg/agent/secret_evidence_test.go` — unit tests
- `cmd/cub-scout/trace.go` — wiring, Crossplane discovery, ASCII output
- `cmd/cub-scout/trace_providerconfig_test.go` — ProviderConfig-specific tests
- `cmd/cub-scout/map.go` — `map issues` secret collection and formatting
- `cmd/cub-scout/map_secret_issues_test.go` — map issues secret tests
- `cmd/cub-scout/localcluster.go` — TUI trace secret panel (renderTrace, runTrace)

### Git import implementation (#357 initial slice complete)

- `cmd/cub-scout/import.go` — `--git-path` flag, `runImportFromGit()`, `buildImportFromGitPreview()`
- `cmd/cub-scout/import_git_test.go` — Git path tests, evidence JSON tests
- `pkg/gitops/parser.go` — `ParseRepo()` (needs enhancement for ApplicationSet git generators)

## Proof expectations

Follow the repo's proof-first rules:

```bash
go build ./cmd/cub-scout
go test ./...
```

And if example/demo behavior changes:

1. run the relevant example script in its non-destructive mode first (`--explain`, `--dry-run`)
2. run `verify.sh` and confirm it passes
3. verify documented commands are copy-pasteable
4. check docs against current `cub` and `cub-scout` command reality
5. confirm the example still tells one coherent connected story

## Quick start for the next coder

1. Read `CLAUDE.md` for build/run/test commands
2. Read `docs/roadmap.md` for strategic context
3. Check open issues: `gh issue list --state open`
4. Inspect AI-first examples:
   - `examples/argo-import-confighub-demo/`
   - `examples/flux-import-confighub-demo/`
5. Choose one small slice and keep it proof-first

## One-sentence strategy anchor

For the current wedge, `cub-scout` should help an AI-first operator import and understand GitOps-managed WET config, verify the import with three-surface evidence, show scan/policy results quickly, and make ConfigHub feel additive to Argo/Flux rather than heavier than them.
