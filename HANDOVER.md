# cub-scout Handover for the Next AI Coder

Last updated: 2026-04-05

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
- Deterministic `TRY NEXT` hints are stronger and more contextual on `main` (`cec20f8`, `#349`)
- `explain` can now suggest ConfigHub GUI deep-links when connected context provides a unit URL (`a5016ad`, `#350`)

## Open issues

| Issue | Title | Notes |
|-------|-------|-------|
| #349 | Improve next-step hints while keeping them deterministic and testable | Implemented on `main` in `cec20f8`; GitHub issue still open pending manual close. |
| #350 | When connected, suggest relevant ConfigHub URLs as standard behavior | Implemented on `main` in `a5016ad`; GitHub issue still open pending manual close. |
| #351 | Make CLI outputs more colorful (not just TUI) | Next likely CLI slice after `#349`/`#350` issue closure. Improve scanability without breaking `NO_COLOR` or ASCII/test contracts. |
| #352 | Detect AI usage and frame responses accordingly | Needs reframing before coding. Prefer explicit, testable modes over brittle auto-detection. |
| #342 | Bidirectional snapshot and conformance workflow | Future product direction; out of scope for the completed March connected/docs slice |
| #328 | Secrets in cub-scout | Separate feature track; out of scope for the completed March connected/docs slice |

No immediate follow-on from the March connected/docs cluster remains open. The
next slice should now be chosen intentionally from the current open set. As of
`a5016ad`, the code for `#349` and `#350` is already on `main`, so the next
coding slice is most likely `#351` unless someone first wants to close out the
tracking state on GitHub.

Important: the current deterministic hints are already good and should not be
diminished. `#349` strengthened them on `main`; any follow-on hint work should
keep moving in that direction rather than flattening the current system.

## Suggested next milestones

1. Milestone 1: close out landed hint work (`#349`, `#350`)
   The code is already on `main` in `cec20f8` and `a5016ad`. The remaining work
   is tracking hygiene: close the GitHub issues if the shipped behavior matches
   expectations and capture any small follow-on notes separately.
2. Milestone 2: CLI color polish (`#351`)
   Improve scanability in normal CLI output without regressing `NO_COLOR`,
   accessibility, or golden-test stability.
3. Milestone 3: reframe AI-mode output explicitly (`#352`)
   If pursued, make this an explicit and testable mode choice rather than
   brittle environment auto-detection.
4. Milestone 4: return to larger feature tracks (`#328`, `#342`)
   Secrets and bidirectional conformance remain important, but they are not the
   best next incremental slice after the March cluster closure.

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

### Likely file hotspots for the next slice

- `cmd/cub-scout/navigation_hints.go`
- `cmd/cub-scout/navigation_hints_test.go`
- `pkg/hub/config.go`

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
