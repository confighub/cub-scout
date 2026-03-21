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
   - connected validation/scan handoff
   - one real issue or policy result in the demos
   The main question is whether “import -> scan/validate -> evidence” is actually present and convincing, or still mostly aspirational.

## Concrete next-step plan for this thread

Use `#329` as the immediate execution slice, but treat it as a status-and-proof audit first, not a wording pass.

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
   - whether scan/validate/evidence is claimed
   - whether the story is AI-first or still human-demo-first

2. Check current command reality before changing docs.
   Use local CLI help/source as truth, not memory:
   - `CONFIGHUB_AGENT=1 cub --help-overview`
   - `cub gitops --help`
   - `cub gitops discover --help`
   - `cub gitops import --help`
   - `./cub-scout import --help`
   - `./cub-scout scan --help`
   Important: do not write generic "validate" language unless it maps to a real current surface. Today the stable `cub-scout` user-facing post-import surface appears to be `scan`.

3. Separate what is already proved from what is only implied.
   As of this handover:
   - the Argo and Flux demos do have a connected readiness check after `cub gitops import`
   - `examples/scripts/verify-connected-demo.sh` proves workers, targets, and connected workloads
   - the demos do not yet prove scan/evidence as part of the main scripted flow
   - `cub-scout scan` appears in demo README exploration sections, which is weaker than "demo-ready proof"

4. Take the smallest useful implementation slice after the audit.
   Recommended first slice:
   - extend one demo first (Argo is the best candidate) with a post-import scan/evidence check
   - prove one deterministic finding, policy result, or explicit "no finding" contract
   - add tests for the helper/script contract before broad docs rewrites
   Do not try to fix all wording drift before there is a real proof path.

5. Add AI-first packaging only after the proof path is real.
   The import demos still need:
   - `AI_START_HERE.md`
   - `prompts.md`
   - `contracts.md`
   Add those after the scan/evidence story is either genuinely demo-ready or explicitly scoped down.

6. Sync docs after the proof matrix and first proof slice.
   Update local docs and the published-docs narrative together so they do not drift:
   - `docs/howto/import-to-confighub.md`
   - `docs/howto/import-from-live.md`
   - demo READMEs
   - published docs import example
   The goal is one honest story:
   - what import proves today
   - what connected mode proves today
   - whether post-import scan/evidence is real, partial, or not yet demo-ready

## Current wedge

The near-term product wedge is:

`GitHub + Argo/Flux + AI/CLI + ConfigHub`

For this wedge, `cub-scout` should help prove:

1. existing GitOps-managed WET config can be imported from GitHub and/or observed from the cluster
2. the imported/connected view is organized clearly in ConfigHub terms
3. validation, scan, and evidence can be shown immediately
4. one real issue or policy result is visible quickly
5. cluster evidence can be compared with intended/imported state

Do not center the story on unreliable apply/status behavior.
Use direct cluster evidence and connected views side by side when runtime truth matters.

## Themes to preserve

1. AI-first, not docs-as-afterthought
   The examples and connected docs should be runnable by Claude/Codex with minimal guessing.
2. Additive to Argo/Flux, not a replacement
   `cub-scout` explains, imports, compares, and proves. It does not need to be the runtime authority.
3. App/Deployment/Target as the primary mental model
   Space/Unit remains the current storage/API vocabulary, but not the front-door explanation.
4. Trustworthy evidence over optimistic status
   If ConfigHub/runtime status is uncertain, say so and show cluster/controller evidence.
5. Import must pay off immediately
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

For the core connected demos, "AI-first" should mean:

1. each major example has:
   - `README.md`
   - `AI_START_HERE.md`
   - `prompts.md`
   - `contracts.md`
2. the docs answer:
   - what stack is this for?
   - what does it prove?
   - what does it read?
   - what does it write?
   - what is safe to do first?
   - what does success look like?
3. there is a read-only orientation path:
   - `--dry-run`
   - `--json`
   - `--explain`
   - `--explain-json`
   where available
4. trust boundaries are explicit:
   - repo/import result
   - ConfigHub intended state
   - controller evidence
   - direct cluster inspection
5. every demo shows at least one real issue or policy result after import

## File hotspots

These are the most likely files for the next slices:

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

## Suggested acceptance contract for the next slice

The next meaningful slice should prove this story end to end:

1. import existing Argo/Flux-managed WET config from GitHub
2. organize it into ConfigHub with app-centric language
3. run validation/scan immediately
4. show one concrete issue or policy result
5. compare imported WET with real cluster evidence
6. use GUI for tables/diffs/evidence/review, not as the primary operator surface

## Proof expectations

Follow the repo's proof-first rules. For the next connected/docs slice, that usually means:

```bash
go build ./cmd/cub-scout
go test ./...
```

And if example/demo behavior changes:

1. run the relevant example script in its non-destructive mode first
2. verify documented commands are copy-pasteable
3. check docs against current `cub` command reality
4. confirm the example still tells one coherent connected story

## Quick start for the next coder

1. Read `docs/roadmap.md`
2. Read `docs/workflows/agent-milestone-plan.md`
3. Read issues `#323`, `#321`, `#325`, `#329`, `#324`, `#326`
4. Inspect:
   - `examples/argo-import-confighub-demo/`
   - `examples/flux-import-confighub-demo/`
   - `docs/howto/import-to-confighub.md`
5. Choose one small slice and keep it proof-first

## One-sentence strategy anchor

For the current wedge, `cub-scout` should help an AI-first operator import and understand GitOps-managed WET config, show evidence and policy results quickly, and make ConfigHub feel additive to Argo/Flux rather than heavier than them.
