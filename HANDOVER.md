# cub-scout Handover for the Next AI Coder

Last updated: 2026-05-09

## Current repo state

- Branch: `main`
- Canonical roadmap: `docs/roadmap.md`
- Delivery rules: `docs/workflows/agent-milestone-plan.md`
- First repo-specific AI entrypoint: `AI-README-FIRST.md`
- Canonical CLI doc layout:
  - `README.md` = overview + Fast Path
  - `CLI-GUIDE.md` = workflow-first guide
  - `docs/reference/cli-reference.md` = A-Z command catalog
  - `docs/reference/commands.md` = detailed usage and examples
  - `docs/reference/cli-contract.md` = stable flags, exit codes, and schemas

## May 2026 completions — session 2026-05-09

| PR | Issue(s) | Subject |
|----|----------|---------|
| [#414](https://github.com/confighub/cub-scout/pull/414) | #408, #391 scope #1 | `--view` flag on `compare three-way`; `cubRunner` subprocess-injection seam; 7 new tests |
| [#411](https://github.com/confighub/cub-scout/pull/411) | #388 | Flake fix: startup grace `0.2s → 1s` in demo-worker lifecycle script |
| [#412](https://github.com/confighub/cub-scout/pull/412) | #385 | GitHub Actions Node 24 bump (checkout v6, setup-go v6, upload-artifact v7, download-artifact v8, golangci-lint v9, docker/login v4, goreleaser v7) |
| [#413](https://github.com/confighub/cub-scout/pull/413) | #389 | GoReleaser `brews:` → `homebrew_casks:` deprecation migration |

### Key design decisions locked in #414

- **Testability seam**: `viewCubRunner cubRunner` func var in `views.go` replaces all direct `exec.Command("cub", ...)` calls. Tests inject a fake runner returning prefab JSON; production delegates to the real binary. This seam also covers `views resolve` and any future `cub`-shelling code in the views layer.
- **`report.Scope` for views**: always `view/<uuid>` regardless of whether the input was a UUID or URL. Stable, compact, discriminable from `resource:`, `namespace/`, `cluster`.
- **Mutual exclusion**: `--scope` and `--view` produce a clear error if both are passed. `--view` requires connected mode; error message mirrors `views resolve`.
- **Triad audit finding**: `cmd/cub-scout/remedy.go` contains a genuine architectural triad violation — the `remedy` command describes itself as "executing remediation" and "fixing issues". Flagged for cleanup; a task chip has been filed.

## May 2026 completions — council source-truth track

The council (April 2026) prescribed the **truth-floor → source-truth contract → fixtures** sequence. The full sequence is now in main:

| PR | Issue(s) | Subject |
|----|----------|---------|
| [#397](https://github.com/confighub/cub-scout/pull/397) | #387, #390, narrow #394 | Truth-floor: Flux false-pessimism fix, tree-patterns nil panic, kstatus narrow slice |
| [#398](https://github.com/confighub/cub-scout/pull/398) | #384 | Pre-existing main lint debt cleanup (5 dead funcs + 1 ineffassign) |
| [#399](https://github.com/confighub/cub-scout/pull/399) | — | Parity script reads canonical CLI reference (post-#374 doc move) |
| [#400](https://github.com/confighub/cub-scout/pull/400) | #393 | Source-truth evidence contract v0.1 (`compare source-truth`) |
| [#401](https://github.com/confighub/cub-scout/pull/401) | #396 | Direct integration coverage for `discoverWorkloads` / `getManagedResources` |
| [#402](https://github.com/confighub/cub-scout/pull/402) | #394 (rest) | kstatus migration complete (state_scanner.go) |
| [#403](https://github.com/confighub/cub-scout/pull/403) | #391 v0.1 | Views resolver with URL-as-positional convention (`views resolve`) |
| [#404](https://github.com/confighub/cub-scout/pull/404) | #395 v0.1 | Source-truth producer fixture suite (6 fixtures, byte-equal goldens) |

### Architectural triad locked in code (not just intent)

- **cub-scout** = read-only evidence provider for source truth
- **Pilot** = acceptance judge (lives in `confighubai/confighub-ai-demo`)
- **ConfigHub** = authority and workflow engine

cub-scout never mutates, repairs, approves, or infers authority. The source-truth contract enforces this surface in code.

### Strict rules locked in tests

- **Strategy-relative correctness.** Identical observations get opposite verdicts under different declared strategies. Argo reading Git is `PASS` under `git-argo` and `BLOCK` (controller outlier) under `confighub-oci-argo`. Locked by `TestDerive_StrategyMismatch_ControllerOutlier` + `TestDerive_VanillaGitOps_PASS`.
- **Missing proof never produces PASS.** Any blank source/digest/runtime field forces at least `WATCH` / `INCOMPLETE`. Locked by `TestDerive_NeverPASS_OnAnyMissingProof` sweep.
- **Connected-mode gate.** `compare source-truth` and `views resolve` refuse to run without `cub` auth — both contracts are meaningless without the ConfigHub surface.

### Pre-existing main CI breakage (cleared)

Discovered while landing the truth-floor: main CI had been red since 2026-04-19, broken by two pre-existing issues unrelated to this work — `#384` lint debt and a parity-script path drift introduced by the `#374` doc consolidation. Both fixed in `#398` / `#399` so subsequent PRs inherit a green baseline.

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

Current tracked follow-ons (verified 2026-05-09):

- **#391 (two scopes remain)** — Views integration. Scope #1 (`--view` on `compare three-way`) shipped in #414. Remaining:
  2. View column projection in TUI Hub view
  3. Reality overlay composing View columns with #393 source-truth verdicts (depends on scope #1 — now unblocked)
- **#409** — source-truth v0.2 cross-surface revision equality. Design pre-baked in the issue body, plus a [strategy-shape comment](https://github.com/confighub/cub-scout/issues/409#issuecomment-4411862418) with the full per-strategy data shape table, runtime-anchor extractability per strategy (notably: Helm runtime anchor is **already extractable** via `helm.sh/chart` labels in `pkg/agent/ownership.go`), the multi-source Argo gap, and a concrete Phase 1/2/3 implementation plan. Phase 1 = existing four strategies; Phase 2 = enum expansion (`helm-flux`, `helm-argo`, `kustomize-flux`, `oci-flux`, `oci-argo`); Phase 3 = multi-source Argo. Verify ConfigHub-side rendered-digest exposure before starting Phase 1. `confighubai/confighub#4356` interacts with `confighub-oci-argo` equality.
- **#410** — Triad-compliance audit (HIGH severity). Discussion ticket framing the architectural decision on `cmd/cub-scout/remedy.go`, which actually executes cluster mutations via `kubectl apply`/`delete` (real triad violation, not just a wording issue). Three options: remove / rename to analysis-only / hide execution behind a flag. Decision needed before code change. Lower-severity findings on `import apply` wording and a contributor-side hint-command lint rule are in the same issue.
- **#392** — Initiatives compliance overlay. **Still deferred.** ConfigHub side has no backend primitive yet. Design doc at [`docs/howto/initiatives-integration-when-ready.md`](docs/howto/initiatives-integration-when-ready.md) holds the integration spec.
- **confighubai/confighub#4356** — cross-repo dependency for ArgoCDOCI Helm-source shape. Blocks accurate `confighub-oci-argo` symptom classification in `compare source-truth`.
- **confighub-ai-demo#264** — Pilot consumer-side fixtures pairing with cub-scout #395 + future #409 fixtures.

## Current checkpoint

`go test ./...` is green as of 2026-05-09 with all 37 packages passing. Main CI is green. GitHub Actions workflows now run on Node 24 (PRs #412, #413).

The practical state heading into the `cub scout` plugin switchover is:

- M0 decision lock: done
- M2 trust/proof polish: materially advanced
- M3 AI/MCP gateway readiness: materially advanced
- M1 plugin packaging: still not started, and still the real `v2.0.0` blocker
- M4 migration/install docs: still partial

Recent connected trust-surface work now covers:
- canonical ConfigHub unit/revisions URLs in compare/trace/explain/history/MCP
- revision-aware hints that tell AI/operators when to review revision history before sign-off
- connected `history` JSON trust guidance
- release-gate cleanup that brought the full test suite back to green

## Next milestone

The next major milestone is now the `cub scout` plugin switchover for `v2.0.0`.

Canonical planning doc:

- `docs/releases/v2.0.0-plugin-plan.md`

Core direction:

- `cub scout` becomes the preferred invocation
- `cub scout mcp serve` becomes the preferred explorer/investigation gateway
- `cub` remains the authority and governed-execution host

### CLI migration table (`#375`)

| Old top-level | New canonical path | Compatibility |
|-------|-------|-------|
| `discover` | `map workloads` | Hidden deprecated alias kept for one release |
| `health` | `map issues` | Hidden deprecated alias kept for one release |
| `combined` | `compare` | Alias kept; `compare` is the primary name |
| `connect` | `setup connect` | Hidden deprecated alias kept for one release |
| `completion` | `setup completion` | Hidden deprecated alias kept for one release |
| `apply` | `import apply` | Hidden deprecated alias kept for one release |
| `parse-repo` | `import parse-repo` | Hidden deprecated alias kept for one release |
| `import-argocd` | `import argocd` | Hidden deprecated alias kept for one release |
| `import-cluster-aggregator` | `import cluster-aggregator` | Hidden deprecated alias kept for one release |
| `drift` | `compare drift` | Hidden deprecated alias kept for one release |
| `demo` | `quickstart demo` | Hidden deprecated alias kept for one release |

Recent closures:
- #372 — Trim and restructure README (Apr 11)
  - Rewrote `README.md` from 845 lines down to 293 lines
  - Kept one signature trace example and moved deeper command detail to the canonical reference docs
  - Tightened the top-down flow to overview -> install -> fast path -> interfaces -> docs map
- #373 — Consolidate import commands under `import` (Apr 11)
  - `import argocd`, `import cluster-aggregator`, and `import parse-repo` are the canonical command paths
  - Hidden deprecated aliases remain available for one release: `import-argocd`, `import-cluster-aggregator`, and `parse-repo`
  - Active examples and docs now point at the canonical subcommand paths
- #374 — Consolidate overlapping CLI reference docs (Apr 11)
  - Recast `CLI-GUIDE.md` as a workflow-first guide instead of a second command encyclopedia
  - Updated `cli-reference.md` to match the current post-#375 command tree
  - Added `scripts/check-cli-docs/main.go` to verify canonical links and prevent README command tables from drifting back
- #375 — Reduce top-level command sprawl (Apr 11)
  - Reduced visible top-level command count to 29 while keeping legacy entrypoints as hidden deprecated aliases
  - Canonicalized `compare` as the primary name with `combined` kept as an alias
  - Reparented commands under `setup`, `import`, `compare`, and `quickstart`
- #377 — Audit `cub-scout mcp serve` tool descriptions against the cold-test sharpening lessons (Apr 11)
  - Sharpened MCP tool descriptions around first-tool identity, chain boundaries, and fallbacks
  - Added connected `compare_three_way` MCP tool as a thin read-only wrapper over `cub-scout compare three-way --format json`
  - Cold-test gap for “compare governed state to live state” is now covered by the MCP surface itself
- #368 — Beat Argo CD GUI as first stop for troubleshooting (Apr 9) — v1.10 wedge
  - Added recent K8s events to `explain` and `trace` commands
  - Bounded (top 5), prioritized (errors/warnings first), readable age format
  - JSON contract: `events` field with events[], totalCount, warningCount, errorCount
- #362 — Stabilize intermittent TestContextPack_FormatJSON kill (Apr 9)
  - Root cause: 30-second timeout was too tight for `go run ./cmd/cub-scout`
  - Fix: increased test runner timeout to 90 seconds
- #359 — Extend `--presentation` to additional read-only commands (Apr 9)
  - Added `--presentation=ai|human|paired` to trace command
  - AI mode: bracket notation, uppercase markers, `[end trace]` outro
  - Human/Paired mode: title-case headings, owner info, ownership chain heading
  - Legacy (no flag): preserves original formatting
- #370 — Structured action-typed next-step hints for JSON + MCP (Apr 9)
  - Added `nextSteps` field to doctor and explain JSON output
  - Each hint has actionType (read-only, mutating, waiting, human-decision), reason, nextCommand/nextSurface
  - MCP clients get structured hints for AI-driven workflows
- #360 — Carry secret evidence through v0.14 trace JSON converter (Apr 9)
  - v0.14 trace JSON now includes `secrets` field with full safe metadata
  - Fields: name, namespace, refType, refPath, status, secretType, createdAt, owner
  - Safe metadata only — no secret data exposed
- #364 — Render integration investigation (Apr 9)
  - Finding: Keep tools separate (`cub-scout` for structure, `cub gitops` for rendering)
  - `cub-scout import --git-path` for local preview, `cub gitops import` for actual rendering
  - Optional future enhancements: RenderableType detection, KustomizePath metadata
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
- #364 — Render integration investigation (Apr 9) — **CLOSED**
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

1. v1.10 shipped: Events wedge for #368 complete
   - Recent K8s events in `explain` and `trace`
   - Closes the main P0 gap in cub-scout vs Argo GUI comparison

2. Future directions (no open issues):
   - Sync status detail for deeper reconciliation visibility
   - Resource conditions surfacing (#348 follow-on)
   - Application-level grouping
   - Standalone desired-vs-live diff

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

### Presentation mode implementation (#352, #359 complete)

- `cmd/cub-scout/presentation.go` — mode types and helper functions (including Trace*)
- `cmd/cub-scout/doctor.go` — `--presentation` flag and rendering
- `cmd/cub-scout/explain.go` — `--presentation` flag and rendering
- `cmd/cub-scout/trace.go` — `--presentation` flag and mode-aware output

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
