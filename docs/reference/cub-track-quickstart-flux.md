# `cub track` Quickstart for Flux Teams (Preview)

> Preview note: `cub track` is proposed command surface and positioning material.
> This document is for rollout planning and customer messaging.

## What Flux Users Get

Without changing Flux controllers or reconciliation flow:

1. commit-linked AI change intent
2. policy decision trail (`ALLOW|ESCALATE|BLOCK`)
3. tokened execution outcome lineage
4. faster review + incident explainability

## 10-Minute Setup Story

1. Install `cub` CLI.
2. In your GitOps repo: `cub track enable`.
3. Keep normal branch + PR + Flux reconcile process.
4. Use `cub track explain --commit <sha>` during review/incident.

No controller replacement and no pipeline rewrite.

## First-Day Workflow

1. AI proposes a change in your Flux repo (`kustomization`, `helmrelease`, values, manifests).
2. Commit includes checkpoint linkage trailer.
3. `cub track` generates a change card (intent/evidence/decision/outcome linkage).
4. Flux reconciles as usual.
5. Team uses `explain` to answer “why did this happen?”

## 60-Second Demo Script

Use this script in a call/demo:

```bash
# 1) Enable tracking in repo (one time)
cub track enable

# 2) Make an AI-assisted GitOps change (example)
git checkout -b demo/flux-track
# (edit kustomization or values with your assistant)
git add .
git commit -m "chore: tune payment api resources"

# 3) Show explainability on that commit
cub track explain --commit HEAD

# 4) Show searchable history
cub track search --text "payment api" --agent codex
```

Demo narration:

1. “Flux is unchanged.”
2. “This commit now has a governed mutation trail.”
3. “I can explain and search by intent/outcome, not just diff.”

## Objection Handling

### “We already use Flux, why add this?”

Because Flux shows sync state, but not full governance context for AI-assisted mutations.

### “Will this break our pipeline?”

No. Start with observe-only mode (Tier 0) and keep apply behavior unchanged.

### “Is this platform lock-in?”

No. Core value is Git-native and local-first. ConfigHub integration is optional upgrade path.

