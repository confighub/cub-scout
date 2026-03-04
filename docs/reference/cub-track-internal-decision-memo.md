# Internal Decision Memo: `cub-track` (Labs Wedge)

**Status:** Draft for review  
**Date:** 2026-02-15  
**Audience:** Brian, Jesper, Product, Engineering, GTM  
**Decision owner:** ConfigHub leadership

## Decision Ask

Approve a **separate OSS Labs project** named `cub-track` with a strict MVP:

1. `enable`
2. `explain`
3. `search`

No Flux/Argo replacement claims. No required ConfigHub backend dependency for OSS mode.

## Why This Decision Now

1. AI-assisted config mutations are increasing faster than current review/governance tooling.
2. Teams need a simple Git-native first step, not a platform migration pitch.
3. We need an adoption wedge that proves value before connected/runtime expansion.

## Proposed Positioning

`cub-track` is a **Git-native mutation ledger** for AI-assisted GitOps changes.

It answers:

1. What was changed?
2. Why was it proposed?
3. What was decided/executed?

It is explicitly positioned as:

1. OSS
2. Free
3. Community-adoptable
4. Compatible with Flux/Argo/Helm workflows

## Scope Guard (What We Are Not Doing)

1. Not replacing Flux/Argo controllers.
2. Not shipping a full hosted governance platform inside `cub-track`.
3. Not writing full WET telemetry/transcripts to Git by default.
4. Not forcing product coupling in MVP.

## Architecture Boundary (DRY vs WET)

1. **Git (DRY):** immutable linkage + compact receipts.
2. **ConfigHub (WET):** policy traces, approvals, execution telemetry, audit search.

MVP writes to Git only. Connected mode later adds optional ConfigHub ingestion and write-back digests.

## Why This Does Not Undermine Product

1. Clear boundary: `cub-track` is capture/explain/search, not governance authority.
2. Upsell is capability-based, not lock-in-based.
3. Connected value remains in ConfigHub portfolio:
   - `confighub-scan` (policy/risk),
   - `confighub` (decision/attestation),
   - `confighub-actions` (tokened runtime),
   - `cub-scout` (GitOps Explorer views).

## MVP Deliverables (30 Days)

1. Git contract:
   - trailers (`Cub-Checkpoint`, `Cub-Intent`, `Cub-Agent`)
   - metadata branch (`cub/mutations/v1`, append-only)
2. Commands:
   - `cub-track enable`
   - `cub-track explain --commit <sha>`
   - `cub-track search --text|--file|--agent`
3. Docs:
   - Flux, Argo, Helm quickstart pages
   - “stored in Git vs ConfigHub” boundary note
4. Safety defaults:
   - no secret writes
   - summary-first evidence
   - optional remote push behavior

## Success Metrics (Pilot)

1. Time to first value: under 10 minutes from install to first `explain`.
2. Explainability utility: reduction in “why did this deploy?” triage time.
3. Review utility: reduction in PR clarification loops for AI-authored GitOps changes.
4. Adoption: repeat usage across at least two real repos (Flux + Argo/Helm).

## Risks and Mitigations

1. **Confusion risk:** “Is this the product?”
   - Mitigation: Labs branding, explicit non-goals, boundary docs in every quickstart.
2. **Distraction risk:** parallel track steals core focus.
   - Mitigation: strict three-command MVP; no platform-runtime scope in this slice.
3. **Culture risk:** cool OSS vs hard product tension.
   - Mitigation: define OSS as top-of-funnel + evidence generator for connected product.

## Go/No-Go Gate After Pilot

Go forward only if:

1. users can adopt without workflow breakage,
2. evidence quality is high enough for governance linkage,
3. connected upsell moments are real (not forced prompts).

If go:

1. add optional connected evidence ingest,
2. keep `cub-track` separate until naming/packaging consolidation is justified,
3. revisit merge path into `cub` CLI later (`cub track`).

## Related Docs

1. `docs/reference/cub-track-mvp-upsell-and-dual-store.md`
2. `docs/reference/stored-in-git-vs-confighub.md`
3. `docs/reference/gitops-checkpoint-prd.md`

# Next-Gen GitOps in the AI Era

**Status:** Draft explainer  
**Date:** 2026-02-15

## One-Sentence Thesis

Next-gen GitOps keeps Git as the durable source of desired state, but upgrades change control from "commit + sync" to **intent + policy decision + attested execution + outcome**.

## What "Post-Flux / Post-Argo" Actually Means

It does **not** mean removing Flux or Argo.

It means Flux/Argo stop being the only operational story. They remain strong deployment engines inside a larger control loop:

`commit -> intent/evidence -> decision -> tokened execution -> outcome`

So:

1. controllers still reconcile,
2. but governance and provenance move up a layer.

## Why This Change Is Needed

Classical GitOps answers:

1. what changed in Git,
2. whether reconcile succeeded.

AI-era operations also require:

1. who/what proposed the mutation,
2. why policy allowed/blocked it,
3. what risk checks were run pre/post apply,
4. whether execution was attested and bounded by trust tier.

## App GitOps + AI GitOps Model

### App GitOps (desired app behavior)

Focus:

1. app rollout intent,
2. environment policy and guardrails,
3. runtime health outcomes.

Typical unit:

1. `ChangeIntent` (app + env target)

### AI GitOps (how mutations are produced safely)

Focus:

1. agent-generated proposals,
2. normalized evidence,
3. policy decision (`ALLOW|ESCALATE|BLOCK`),
4. attested execution and receipts.

Typical unit:

1. governed mutation card (`intent + decision + execution + outcome`)

Combined result:

1. app intent and agent behavior are linked in one audit path.

## Practical Boundary by Component

1. `cub-track`: Git-native mutation ledger (`enable`, `explain`, `search`)
2. `cub-scout`: GitOps Explorer and evidence normalizer
3. `confighub-scan`: risk/policy signal engine
4. `confighub`: decision + attestation + approval authority
5. `confighub-actions`: tokened execution runtime

## DRY/WET Storage Split

1. **Git is DRY:** compact immutable linkage for review and history.
2. **ConfigHub is WET:** full policy graph, approvals, execution telemetry, analytics.

This avoids Git bloat while preserving portability and verifiability.

## Flux User Story (Fast Pitch)

1. Keep Flux exactly as deploy engine.
2. Add `cub-track` in repo.
3. For each AI-assisted mutation, get commit-linked explainability and searchable provenance.
4. Optionally connect to ConfigHub for policy decisions and attested apply flows.

Message:

"No migration first. Higher trust immediately. Connected governance when ready."

## Argo User Story (Fast Pitch)

1. Keep Argo Applications/ApplicationSets and sync workflow.
2. Add mutation ledger records linked to each AI-assisted commit.
3. Use `explain` during incident/review to answer "why this was allowed and what executed."
4. Add connected policy/approval/tier enforcement later.

## Helm-Centric User Story (Fast Pitch)

1. Keep chart and values workflows.
2. Track AI-assisted chart mutations in Git with searchable intent/outcome history.
3. Add optional connected controls for risk and enterprise audit.

## Traefik Helm Example (Base + App Units)

Use the complete reference:

1. `docs/reference/traefik-helm-dry-wet-unit-worked-example.md`

That walkthrough includes:

1. upstream Traefik chart pinning as platform DRY base unit,
2. app-owned DRY overlays with bounded field ownership,
3. renderer-unit dry->wet composition with OCI delivery,
4. app change -> platform approval -> upstream DRY promotion path.

## score.dev Example (Apps + Agents)

Use the complete end-to-end reference:

1. `docs/reference/scoredev-dry-wet-unit-worked-example.md`

That walkthrough includes:

1. Score DRY intent -> WET render pipeline.
2. Flux and Argo controller-equivalent wiring.
3. `cub gitops import` dry/wet pair creation with MergeUnits linkage.
4. AI mutation + explainability overlay using `cub track`.

## Why Skeptical Teams Adopt

1. Zero forced controller replacement.
2. Immediate incident/review value from `explain`.
3. Stepwise adoption: observe first, enforce later.
4. Clear boundary: OSS ledger first, connected governance optional.

## Related Docs

1. `docs/reference/app-and-ai-gitops-plain-english.md`
2. `docs/reference/cub-track-mvp-upsell-and-dual-store.md`
3. `docs/reference/stored-in-git-vs-confighub.md`
4. `docs/reference/scoredev-dry-wet-unit-worked-example.md`
5. `docs/reference/dual-approval-gitops-gh-pr-and-ch-mr.md`
6. `docs/reference/traefik-helm-dry-wet-unit-worked-example.md`
