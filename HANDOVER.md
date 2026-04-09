# cub-scout Handover for the Next AI Coder

Last updated: 2026-04-09

## Current repo state

- Branch: `main`
- Canonical roadmap: `docs/roadmap.md`
- Delivery rules: `docs/workflows/agent-milestone-plan.md`
- First repo-specific AI entrypoint: `AI-README-FIRST.md`

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
  - Three modes: `human`, `ai`, `paired`
  - Opt-in only: omit the flag to keep the legacy/default render path
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
| #370 | Structured next-step hints for JSON + MCP | Highest-leverage AI/MCP follow-on, builds on #369 |
| #368 | Beat Argo CD GUI as the first stop for troubleshooting | Broader umbrella, likely after #370 |
| #364 | Investigate integration with `cub gitops import` rendering for manifest preview | Investigated already; only resume for explicit render-readiness follow-on |
| #359 | Extend `--presentation` to additional read-only commands | Useful polish, lower priority |
| #360 | Carry secret evidence through the legacy trace v0.14 JSON converter | Low-priority contract cleanup |
| #362 | Stabilize intermittent `TestContextPack_FormatJSON` kill in `test/ascii` | Test-stability follow-on from v1.9 release verification |

Recent closures:
- #369 — Expose doctor as first-class MCP tool (Apr 9)
  - Added `doctor` to MCP gateway standalone tool set
  - Parameters: namespace (optional), top (optional integer)
  - First in tool list, matching its role as first troubleshooting command
  - Wraps `cub-scout doctor --format json`
- #371 — Three-way agreement/convergence summary (Apr 9)
  - New `AgreementSummary` in `compare three-way` output
  - States: agreed, converging, diverged, partial
  - Derived deterministically from per-resource patterns
  - Shows "Agreement: ✓ AGREED - All 5 resources agree" in ASCII
  - JSON: `summary.agreement.{state,summary,reasons,sources}`
- #363 — Enhanced Git parser for ArgoCD ApplicationSet git generators (Apr 9)
  - Extracts `directories[].path` patterns from git generators
  - Supports matrix generators with nested git generators
  - Exclude pattern support (paths starting with `!`)
  - Path-centric model for unique identification (handles duplicate basenames)
  - Full-path slugs in import proposals (`apps-team-a-api` vs `services-team-a-api`)

Investigation status:
- #364 — Render integration investigation (Apr 9)
  - Finding: Keep tools separate (`cub-scout` for structure, `cub gitops` for rendering)
  - Recommendation: Add optional "render-readiness" metadata (renderableType, kustomizePath)
  - See issue comment for detailed findings
- #366 — Three-way disagreement surfacing for connected mode (Apr 9)
  - `explain` shows THREE-WAY STATUS section when ConfigHub/Argo/cluster disagree
  - Patterns: change-in-progress, sync-stale, rollout-pending, multi-change
  - `doctor` hints to run `compare three-way` for full conformance check
- #367 — Phase-aware next-step hints for Argo-managed resources (Apr 9)
  - Three phases: incident (investigate), verify (confirm), closeout (read-only)
  - Phase detection is deterministic from ExplainSummary facts (health, drift, risks)
  - Hints are tailored to each phase - no new commands, just better guidance
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

Also important: the Apr 9 Argo truth-and-guidance track is now complete:
- `#365` (closed) — ownership truth for ApplicationSet-managed resources
- `#367` (closed) — phase-aware hints for Argo incidents/verification/closeout
- `#366` (closed) — three-way disagreement surfacing in connected mode

The Git import track is now investigated (see #364 comment):
- #363 complete: parser supports ApplicationSet git generators with path-centric model
- #364 investigated: recommendation is to keep tools separate, optionally add render-readiness metadata

Recommended next steps (optional enhancements):
1. Add `RenderableType` detection to parser (infer from kustomization.yaml vs Chart.yaml)
2. Add `KustomizePath` to JSON output for kustomize apps
3. Document combined workflow (`cub-scout` scouts, `cub gitops` renders)

Lower-priority follow-ons remain: #359, #360, #362

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

### ArgoCD ApplicationSet Git Generator Support (Complete)

The parser now fully supports ArgoCD ApplicationSets with git generators:

```yaml
# applicationsets/apps.yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
spec:
  generators:
    - git:
        repoURL: https://github.com/org/repo
        directories:
          - path: apps/*
          - path: "!apps/excluded"  # Exclude patterns supported
```

**Implementation in cub-scout (#363 complete):**
- `pkg/gitops/parser.go`:
  - `extractGitGeneratorPatternsWithExcludes()` extracts include/exclude patterns
  - `extractMatrixGitPatternsWithExcludes()` handles nested git generators in matrix
  - full-path scanning preserves matched relative paths instead of guessing
  - path-centric target tracking preserves duplicate basenames safely
- `cmd/cub-scout/import.go` / `suggest.go`:
  - import proposals use path-derived unique slugs for duplicate-basename apps
- `cmd/cub-scout/import_git_test.go`: End-to-end ApplicationSet integration tests

`cub-scout import --git-path` now correctly discovers apps from ApplicationSet
git generators like those in `jesperfj/gitops-argocd`.

### Files Across Repos

| Repo | File | Purpose |
|------|------|---------|
| cub-scout | `cmd/cub-scout/import.go` | `--git-path` flag, Git preview flow |
| cub-scout | `cmd/cub-scout/import_git_test.go` | Git import tests |
| cub-scout | `pkg/gitops/parser.go` | `ParseRepo()` with ApplicationSet git generator support |
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

1. Milestone 1: workflow-first machine surfaces (#370, #369)
   - `#370`: structured action-typed hints in JSON and MCP
   - `#369`: expose `doctor` / `observe.scope_summary` as a first-class MCP tool
2. Milestone 2: broader GitOps troubleshooting wedge (#368)
   - make `cub-scout` the first stop before the Argo CD GUI
   - likely builds on the new three-way + hinting work rather than replacing it
3. Milestone 3: optional Git import polish / compat
   - `#364`: render-readiness metadata only if explicitly resumed
   - `#359`, `#360`, `#362`: presentation polish, legacy JSON gap, test stability

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
- `pkg/gitops/parser.go` — `ParseRepo()` with ApplicationSet git-generator + path-centric discovery support

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
2. Read `AI-README-FIRST.md` for tool boundaries and current queue
3. Read `docs/roadmap.md` for strategic context
4. Check open issues: `gh issue list --state open`
5. Inspect AI-first examples:
   - `examples/argo-import-confighub-demo/`
   - `examples/flux-import-confighub-demo/`
6. Choose one small slice and keep it proof-first

## One-sentence strategy anchor

For the current wedge, `cub-scout` should help an AI-first operator import and understand GitOps-managed WET config, verify the import with three-surface evidence, show scan/policy results quickly, and make ConfigHub feel additive to Argo/Flux rather than heavier than them.
