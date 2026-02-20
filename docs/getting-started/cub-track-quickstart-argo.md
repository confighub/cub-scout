# `cub track` Quickstart for Argo CD Teams (Preview)

> Preview note: `cub track` is proposed command surface and positioning material.
> This document is for rollout planning and customer messaging.

## What Argo Users Get

Without changing Argo Applications or ApplicationSets:

1. commit-linked change intent and actor identity
2. policy decision trail before execution
3. execution and post-scan outcome receipts
4. searchable “why” context for app sync events

## 10-Minute Setup Story

1. Install `cub` CLI.
2. In your Argo Git repo: `cub track enable`.
3. Keep normal PR and Argo sync workflow.
4. Use `cub track explain --commit <sha>` on deployment changes.

## First-Day Workflow

1. AI proposes an Argo app change (Application, ApplicationSet, chart values, manifests).
2. Commit is linked to checkpoint/mutation metadata.
3. Team reviews intent + decision context in PR.
4. Argo sync continues as usual.
5. When a rollout drifts/fails, `explain` gives fast provenance.

## 60-Second Demo Script

```bash
# 1) Enable tracking in repo
cub track enable

# 2) Commit AI-assisted Argo change
git checkout -b demo/argo-track
# (edit applicationset or values with your assistant)
git add .
git commit -m "feat: adjust checkout app sync policy"

# 3) Explain this commit in governance terms
cub track explain --commit HEAD

# 4) Search by app/file context
cub track search --file clusters/prod/checkout --text "sync policy"
```

Demo narration:

1. “Argo remains our deploy engine.”
2. “We add intent/decision/outcome provenance for AI-generated mutations.”
3. “This reduces incident and review cycle time.”

## Objection Handling

### “Argo already has history.”

Argo history is deployment-centric. `cub track` adds policy + intent + AI actor lineage tied to Git commits.

### “We can’t add risk right now.”

Run observe-only first. No enforced apply path changes are required to get value.

### “Too much process.”

Start with one repo and only `enable + explain + search`. Expand only if review/incident metrics improve.

