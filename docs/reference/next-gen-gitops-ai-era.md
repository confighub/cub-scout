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

## score.dev Example (Apps + Agents)

Scenario:

1. Team maintains `score.yaml` for `checkout`.
2. Agent updates workload, probes, and env bindings.
3. Platform generator renders Kubernetes manifests and/or Helm values.
4. Git commit includes mutation linkage trailer.
5. Flux/Argo reconciles generated manifests.

Next-gen loop:

1. `ChangeIntent`: "increase checkout resilience and right-size resources."
2. Pre-scan evaluates policy (probes present, limits bounded, env restrictions).
3. Decision is recorded (`ALLOW` in staging, `ESCALATE` in prod).
4. Attested execution runs with scoped token.
5. Post-scan and runtime outcome are attached as receipts.

Outcome:

1. team can answer both "what changed in generated manifests" and "why/under what controls it changed."

## Why Skeptical Teams Adopt

1. Zero forced controller replacement.
2. Immediate incident/review value from `explain`.
3. Stepwise adoption: observe first, enforce later.
4. Clear boundary: OSS ledger first, connected governance optional.

## Related Docs

1. `docs/getting-started/app-and-ai-gitops-plain-english.md`
2. `docs/reference/cub-track-mvp-upsell-and-dual-store.md`
3. `docs/reference/stored-in-git-vs-confighub.md`
