# cub-scout Handover for the Next AI Coder

Last updated: 2026-03-21

## Current repo state

- Branch: `main`
- Canonical roadmap: `docs/roadmap.md`
- Delivery rules: `docs/workflows/agent-milestone-plan.md`
- Latest relevant issue filed in this session: `#329`
- Note: local worktree currently has untracked `.claude/` and `LIVE`; treat them as user-owned and do not modify them unless explicitly asked

## Immediate TODOs

1. Check the current status of the GitOps import path end to end.
   Confirm what is actually working today across:
   - published docs
   - `examples/argo-import-confighub-demo/`
   - `examples/flux-import-confighub-demo/`
   - connected/import docs
   The main question is whether the import story is demo-ready, evidence-backed, and aligned with the current wedge.
2. Check the current status of scanning in the connected story.
   Confirm what can be shown immediately after import today:
   - native `cub-scout` scan surfaces
   - connected scan handoff
   - one real issue or policy result in the demos
   The main question is whether “import -> scan -> evidence” is actually present and convincing, or still mostly aspirational.

## Concrete next-step plan for this thread

Use `#329` as the immediate execution slice, but treat it as a status-and-proof audit first, not a wording pass.

### Slice 1: Claim-vs-proof matrix and structural alignment

1. Build a claim vs proof matrix across all current GitOps import surfaces.
   Include:
   - published docs: <https://docs.confighub.com/get-started/examples/gitops-import/>
   - `examples/argo-import-confighub-demo/`
   - `examples/flux-import-confighub-demo/`
   - `docs/howto/import-to-confighub.md`
   - `docs/howto/import-from-live.md`
   For each surface, record:
   - what import path is claimed
   - whether connected/readiness proof is claimed
   - whether scan/finding evidence is claimed
   - whether the story is AI-first or still human-demo-first

2. Check current command reality before changing docs.
   Separate local commands (verifiable from source) from external SDK commands:

   Local (`cub-scout`, verifiable from this repo):
   - `./cub-scout --help`
   - `./cub-scout import --help`
   - `./cub-scout scan --help`
   - `./cub-scout compare --help`

   External (`cub` SDK, separate binary):
   - `cub gitops --help`
   - `cub gitops discover --help`
   - `cub gitops import --help`

   Do not write generic "verify" or "scan" language unless it maps to a real current surface.

3. Separate what is already proved from what is only implied.
   Three proof layers exist today:
   - **Connected readiness** — workers/targets registered and active.
     Proved by `examples/scripts/verify-connected-demo.sh`.
   - **Import/render evidence** — dry/wet units created, renderer completed or failed visibly.
     Proved by `cub unit list` and `cub unit-action get` after `cub gitops import`.
   - **Post-import scan/finding evidence** — a concrete issue, policy result, or explicit "no finding" contract.
     Not proved today. Neither `demo.sh` (Argo or Flux) calls `cub-scout scan`.
     `scan` appears only in README "Exploring After the Demo" sidebars. No fixture is
     designed to produce a deterministic scan finding.

4. Align the Argo demo to the incubator AI-first structure (first implementation slice).
   The reference pattern is `confighub/examples/incubator/gitops-import-argo/`,
   which implements:
   - `AI_START_HERE.md` — safe entry point for AI agents
   - `prompts.md` — copyable prompts (orient, walkthrough, verify)
   - `contracts.md` — stable read-only and mutating inspection paths with evidence boundaries
   - `verify.sh` — three-surface verification (cluster, ConfigHub, cub-scout)
   - `setup.sh --explain` / `--explain-json` — progressive disclosure before mutation
   - `cleanup.sh` — explicit teardown

   AI-first is structural, not decorative. These files shape the example's script
   structure and evidence model. They are not packaging to bolt on afterward.

   Recommended first slice:
   - add `AI_START_HERE.md`, `prompts.md`, `contracts.md` to `examples/argo-import-confighub-demo/`
   - add a local `verify.sh` using the incubator's three-surface evidence model
     (factor from `examples/scripts/verify-connected-demo.sh` or build fresh)
   - define the evidence boundary contract: what cluster evidence, ConfigHub evidence,
     and cub-scout evidence each prove and do not prove
   - decide whether to keep `demo.sh` as the human-demo path alongside the new AI-first
     scripts, or refactor it into the `setup.sh`/`verify.sh`/`cleanup.sh` pattern

5. Sync docs after the structural alignment slice.
   Update local docs and the published-docs narrative together so they do not drift:
   - `docs/howto/import-to-confighub.md`
   - `docs/howto/import-from-live.md`
   - demo READMEs
   - published docs import example
   The goal is one honest story:
   - what import proves today
   - what connected mode proves today
   - that post-import scan/finding evidence is a follow-on slice, not yet demo-ready
   Note: the published doc is GUI-first while the local demos are CLI-first.
   The sync step must decide whether to bridge or keep them as separate audiences.

### Slice 2: Post-import scan/finding evidence (follow-on)

6. Extend one demo (Argo first) with a post-import scan/finding check.
   This is the import -> scan -> evidence path the wedge cares about.
   Neither cub-scout's demos nor the incubator reference currently prove it.
   - first, run `cub-scout scan` against the existing `--keep` demo cluster and record
     what it produces today with the current fixtures
   - if it returns zero findings, the first code task is to add a fixture that triggers
     a known finding (Kyverno policy + violation, or a stuck reconciliation state)
   - prove one deterministic finding, policy result, or explicit "no finding" contract
   - add the scan step to `verify.sh` and `contracts.md`
   - add tests for the scan contract before broad docs rewrites

## Current wedge

The near-term product wedge is:

`GitHub + Argo/Flux + AI/CLI + ConfigHub`

## Command vs Watch

For AI-first GitOps work, keep the interaction model explicit:

- `command` mode:
  write intended config or command intent into ConfigHub, then let the live
  delivery path act on it
- `watch` mode:
  observe runtime state and status from the live systems directly
  (`kubectl`, ArgoCD, Flux, controller APIs)

Authority boundary:

- ConfigHub is authoritative for intended config, imported config, and command
  intent
- live systems are authoritative for runtime state and status

Do not teach Claude/Codex to work around a broken apply path by pulling config
from ConfigHub and applying it directly. If ConfigHub apply/refusal is broken,
that is a bug to fix in the command path, not a reason to bypass the model.

For this wedge, `cub-scout` should help prove:

1. existing GitOps-managed WET config can be imported from GitHub and/or observed from the cluster
2. the imported/connected view is organized clearly in ConfigHub terms
3. verification, scan, and evidence can be shown immediately
4. one real issue or policy result is visible quickly
5. cluster evidence can be compared with intended/imported state

Do not center the story on unreliable apply/status behavior.
Use direct cluster evidence and connected views side by side when runtime truth matters.

## Themes to preserve

1. AI-first is structural, not decorative
   The examples should follow the incubator pattern: four-file bundle, progressive disclosure,
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

## Roadmap and milestone alignment

Use `docs/roadmap.md` as the source of truth. As of this handover, there are no active GitHub milestones assigned to the connected issues, so treat the roadmap workstreams and the delivery plan as the effective milestones.

The most relevant roadmap areas are:

- `Connected Views + Launch`
  - Workstream C: WET-LIVE panel clarity and causality messaging
  - Workstream E: claim -> demo -> status proof matrix
- `Connected Mode Ideas`
  - connected import flows
  - import wizard with auto-detection
- `1.x Connected Upsell`
  - already completed, but useful as guardrails for packaging and connected positioning

Use `docs/workflows/agent-milestone-plan.md` for proof-first execution. Even for docs/example work, keep the same discipline:

1. define contract and non-goals
2. define proof path before editing
3. update example/docs in small slices
4. verify after each slice

## Active issue cluster

These are the issues that currently define the connected/AI-first work:

1. `#323` `connected: define the canonical App/Deployment/Target to Space/Unit mapping`
   Foundation issue. This is the main semantic decision.
2. `#321` `connected: make App/Deployment/Target the primary model across docs`
   Docs follow-through once the mapping is fixed.
3. `#324` `connected: align import proposals and connected views with the chosen app-centric model`
   Code/docs/output alignment after `#323`.
4. `#325` `connected/docs: treat OSS SDK cmd/cub as source of truth for connected cub workflows`
   Important for docs parity and not inventing commands.
5. `#326` `connected/examples: refresh ADT docs and label contract for import-to-promotion workflows`
   Keeps the App/Deployment/Target example path current and tied to real SDK flows.
6. `#329` `connected/examples: make GitOps import and connected demos AI-first`
   New issue from this session. This is the docs/example packaging slice for the current wedge.
7. `#327` `documentation: add a Kubara-oriented guide for Argo/ApplicationSet platform debugging with cub-scout`
   Adjacent and useful, but not the first blocker.
8. `#328` `Secrets in cub-scout`
   Separate concern. Do not mix into the AI-first connected example work unless needed.

## Recommended execution order

If you are picking up this thread cold, do the work in this order:

1. `#323`
   Decide and document the canonical App/Deployment/Target to Space/Unit mapping.
   This prevents more docs drift.
2. `#321`
   Update the connected docs to lead with the chosen model.
3. `#325`
   Tighten connected docs against current OSS SDK `cub` commands.
4. `#329`
   Make the Argo and Flux import demos AI-first.
   This is the clearest short-term payoff.
5. `#324`
   Align import proposals, help text, and connected views with the same model.
6. `#326`
   Refresh the ADT example and promotion handoff once the import story is coherent.
7. `#327`
   Add Kubara/ApplicationSet-specific guidance after the core import surfaces are stable.

Important: parts of `#329` can start before `#323` is closed, but avoid hard-coding wording that could conflict with the final mapping decision.

## What "AI-first" means here

AI-first is structural, not decorative. It shapes the example's script structure,
evidence model, and file layout. It is not packaging to add after the fact.

The canonical reference is `confighub/examples/incubator/gitops-import-argo/`,
which implements the full pattern. cub-scout demos should converge on that structure.

For the core connected demos, "AI-first" means:

1. each major example has:
   - `README.md` — human-oriented, answers the six reader questions
   - `AI_START_HERE.md` — safe entry point for AI agents with mutation boundaries
   - `prompts.md` — copyable prompts (orient, walkthrough, verify)
   - `contracts.md` — stable inspection paths with evidence boundaries
   - `verify.sh` — three-surface verification script
2. the docs answer the six non-negotiable reader questions
   (from `confighub/examples/incubator/ai-example-playbook.md`):
   - what stack is this for?
   - what does it require?
   - what does it read?
   - what does it write?
   - what does success look like?
   - what is safe to do first from an AI assistant?
3. there is a progressive-disclosure path:
   - `--explain` / `--explain-json` (preview what will happen, no mutation)
   - `--dry-run` (execute logic but do not mutate)
   - `--json` (machine-readable output)
   where available
4. evidence boundaries are explicit and three-surfaced:
   - cluster evidence (kubectl, controller API)
   - ConfigHub evidence (cub unit list, cub target list, unit-action results)
   - cub-scout evidence (gitops status, map list, ownership classification)
   Import and renderer evidence do not, by themselves, prove live workload
   reconciliation. If runtime truth matters, compare all three surfaces.
5. scan/finding evidence (follow-on slice):
   once the structural alignment is done, each demo should show at least
   one real issue or policy result after import via `cub-scout scan`

## File hotspots

### Incubator reference (read, do not modify from this repo)

- `confighub/examples/incubator/gitops-import-argo/README.md`
- `confighub/examples/incubator/gitops-import-argo/AI_START_HERE.md`
- `confighub/examples/incubator/gitops-import-argo/contracts.md`
- `confighub/examples/incubator/gitops-import-argo/prompts.md`
- `confighub/examples/incubator/gitops-import-argo/verify.sh`
- `confighub/examples/incubator/ai-example-playbook.md`
- `confighub/examples/incubator/ai-example-template.md`

### cub-scout files for the next slices

- `examples/argo-import-confighub-demo/README.md`
- `examples/argo-import-confighub-demo/demo.sh`
- `examples/flux-import-confighub-demo/README.md`
- `examples/flux-import-confighub-demo/demo.sh`
- `examples/demo-data-adt/README.md`
- `docs/howto/import-to-confighub.md`
- `docs/howto/import-from-live.md`
- `docs/howto/using-cub-scout-from-ai-tool.md`
- `docs/reference/import-docs-crosswalk.md`
- `docs/reference/hub-appspace-examples.md`
- `docs/reference/connected-tiers-and-views-product-guide.md`
- `docs/reference/commands.md`
- `docs/reference/json-contracts.md`
- `docs/README.md`

## Suggested acceptance contracts

### Slice 1 acceptance (structural alignment)

The Argo demo is structurally AI-first:

1. `AI_START_HERE.md`, `prompts.md`, `contracts.md` exist alongside `README.md`
2. `verify.sh` runs a three-surface evidence check (cluster, ConfigHub, cub-scout)
3. `contracts.md` documents stable read-only and mutating inspection paths
4. evidence boundaries are explicit: what each surface proves and does not prove
5. the demo can be driven by an AI agent using only the files in the example directory

### Slice 2 acceptance (scan/finding evidence)

The Argo demo proves import -> scan -> evidence:

1. import existing Argo-managed WET config from GitHub
2. run `cub-scout scan` immediately after import
3. show one concrete finding, policy result, or explicit "no finding" contract
4. compare imported WET with real cluster evidence
5. scan step is in `verify.sh` and `contracts.md`

## Proof expectations

Follow the repo's proof-first rules. For the next connected/docs slice, that usually means:

```bash
go build ./cmd/cub-scout
go test ./...
```

And if example/demo behavior changes:

1. run the relevant example script in its non-destructive mode first (`--explain`, `--dry-run`)
2. run `verify.sh` and confirm it passes
3. verify documented commands are copy-pasteable
4. check docs against current `cub` and `cub-scout` command reality (see step 2 above for the split)
5. confirm the example still tells one coherent connected story

## Quick start for the next coder

1. Read `docs/roadmap.md`
2. Read `docs/workflows/agent-milestone-plan.md`
3. Read the incubator reference pattern:
   - `confighub/examples/incubator/gitops-import-argo/` (README, AI_START_HERE, contracts, prompts, verify.sh)
   - `confighub/examples/incubator/ai-example-playbook.md` (methodology)
   - `confighub/examples/incubator/ai-example-template.md` (four-file bundle template)
4. Read issues `#323`, `#321`, `#325`, `#329`, `#324`, `#326`
5. Inspect:
   - `examples/argo-import-confighub-demo/`
   - `examples/flux-import-confighub-demo/`
   - `docs/howto/import-to-confighub.md`
6. Choose one small slice and keep it proof-first

## One-sentence strategy anchor

For the current wedge, `cub-scout` should help an AI-first operator import and understand GitOps-managed WET config, verify the import with three-surface evidence, show scan/policy results quickly, and make ConfigHub feel additive to Argo/Flux rather than heavier than them.
