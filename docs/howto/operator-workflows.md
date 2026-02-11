# Operator Workflows (Read-Only)

`cub-scout` v0.20 adds operator-facing workflows that improve day-to-day evidence
without introducing cluster mutation.

## Scope

- Visibility for scheduled workloads (`CronJob`, `Job`)
- Runbook-style action previews
- Normalized activity timeline
- Preview environment detection
- Trace artifact provenance

All commands are read-only.

## 1. Scheduled Workload Visibility

```bash
cub-scout map cronjobs
cub-scout map jobs
```

Use this to verify:
- schedule and suspension state
- active run count
- derived run status (`success|failed|running|unknown`)
- owner attribution (Flux/Native/etc.)

## 2. Action Preview (No Mutation)

```bash
cub-scout map actions deployment/payment-api -n ecommerce
```

Use this to generate runbook guidance for:
- rollout restarts (Deployment/StatefulSet/DaemonSet)
- pod recycle guidance
- one-off job execution from a CronJob template

Output includes rationale, impact, and risk level for each suggestion.

## 3. Activity Timeline

```bash
cub-scout map activity --since 24h
```

Optional filters:

```bash
cub-scout map activity --owner Flux --since 24h
cub-scout map activity --namespace prod --format json
```

The timeline normalizes signals from:
- Flux reconcile conditions
- Argo app sync/health snapshots
- Helm release secrets
- Kubernetes events

## 4. Preview Environment Hygiene

```bash
cub-scout map previews --stale-after 72h
```

Detects likely PR environments via labels/annotations (including Forgejo/Gitea hints),
then marks stale candidates for cleanup review.

## 5. Artifact Provenance During Trace

```bash
cub-scout trace deployment/payment-api -n ecommerce --artifacts
```

Adds source artifact metadata when available:
- `url`
- `revision`
- `digest`
- `lastUpdateTime`
- `sourceKind`

When unavailable, values are explicitly reported as `unknown`.

## ASCII vs JSON

- `--format json` is the canonical machine contract.
- ASCII/Markdown are human renderings of the same underlying fields.
- Prefer JSON for CI, bots, and diff-based regression checks.
