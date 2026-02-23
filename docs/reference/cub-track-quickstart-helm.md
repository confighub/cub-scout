# `cub track` Quickstart for Helm-Centric Teams (Preview)

> Preview note: `cub track` is proposed command surface and positioning material.
> This document is for rollout planning and customer messaging.

## What Helm Users Get

Without replacing Helm workflows:

1. commit-linked AI change context for chart/values updates
2. policy and risk signals tied to each mutation
3. execution outcome receipts for audit/operations
4. faster handoff and rollback decisions

## 10-Minute Setup Story

1. Install `cub` CLI.
2. In your chart/config repo: `cub track enable`.
3. Keep normal Helm packaging/release process.
4. Use `cub track explain --commit <sha>` for release troubleshooting.

## First-Day Workflow

1. AI updates chart template or values.
2. Commit creates mutation linkage metadata.
3. Team reviews intent + decision context before release.
4. Helm release process runs as usual.
5. Post-release outcome is searchable by commit/intent.

## 60-Second Demo Script

```bash
# 1) Enable tracking in repo
cub track enable

# 2) Commit AI-assisted chart change
git checkout -b demo/helm-track
# (edit chart templates or values with your assistant)
git add .
git commit -m "chore: tighten checkout chart probes"

# 3) Explain decision and outcome lineage
cub track explain --commit HEAD

# 4) Search historical changes by chart path
cub track search --file charts/checkout --text "probes"
```

Demo narration:

1. “Helm release mechanics are unchanged.”
2. “We add governance-grade provenance for AI-assisted chart mutations.”
3. “This is Git-native and reversible.”

## Objection Handling

### “We already use Helm history.”

Helm history shows release revisions, not full intent/policy/AI actor context tied to Git commits.

### “We don’t want platform lock-in.”

Core mode is local-first + Git-native. ConfigHub enriches later if you choose.

### “We’re too busy to migrate.”

No migration required. Start with one repo in observe-only mode and prove value in one week.

