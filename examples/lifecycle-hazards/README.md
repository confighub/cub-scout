# Lifecycle Hazards Example

## The Problem

You're migrating a Helm chart to ArgoCD. The chart has `post-install` and `post-upgrade`
hooks — but ArgoCD doesn't distinguish between install and upgrade. Every sync is an "upgrade."

Your `db-migrate` Job runs on `post-install,post-upgrade`. Under Helm, it ran once on install
and once per upgrade. Under ArgoCD, it runs on **every single sync** — including manual
retries and auto-sync.

**cub-scout detects these hazards:**

```
./cub-scout scan --file helm-hooks.yaml --lifecycle-hazards

LIFECYCLE HAZARD SCAN
════════════════════════════════════════════════════════════════════

┌─────────────────────────────────────────────────────────────────┐
│  Helm Chart Migration → ArgoCD                                   │
│                                                                   │
│  helm.sh/hook annotation         ArgoCD Phase    Hazard?         │
│  ─────────────────────           ────────────    ───────         │
│  pre-install            ──→      PreSync         ⚠ ambiguous    │
│  pre-upgrade            ──→      PreSync         ⚠ ambiguous    │
│  post-install           ──→      PostSync        ⚠ ambiguous    │
│  post-upgrade           ──→      PostSync        ⚠ ambiguous    │
│  test                   ──→      PostSync        ⚠ runs on sync │
│  argocd.argoproj.io/hook ──→     (explicit)      ✓ safe         │
└─────────────────────────────────────────────────────────────────┘

Hazards (2):
  ├── [WARNING] Job/db-migrate
  │   Rule: helm-hook-ambiguity
  │   Risk: ArgoCD treats every sync as upgrade;
  │         post-install may run unexpectedly
  │
  └── [WARNING] Job/notify-deploy
      Rule: postsync-idempotency-risk
      Risk: Job runs on every sync;
            ensure operation is idempotent

Safe (1):
  └── [OK] Job/schema-validate
      argocd.argoproj.io/hook: PreSync  ← explicit, no ambiguity

════════════════════════════════════════════════════════════════════
Summary: 2 hazards │ 1 safe │ 3 hooks scanned
```

## Quick Start

```bash
# List all hooks and their phase mappings
./cub-scout map hooks --file examples/lifecycle-hazards/helm-hooks.yaml

# Scan for lifecycle hazards
./cub-scout scan --file examples/lifecycle-hazards/helm-hooks.yaml --lifecycle-hazards
```

## What This Example Shows

### 1. Helm Hook Ambiguity

When Helm hooks like `post-install,post-upgrade` are deployed via ArgoCD, the distinction
between "install" and "upgrade" is lost. ArgoCD treats every sync as equivalent to an upgrade.

**Example in file:** `db-migrate` Job

**Detection:**
```
RULE: helm-hook-ambiguity
RISK: ArgoCD treats every sync as upgrade; post-install may run unexpectedly
```

### 2. PostSync Idempotency

Jobs that run as PostSync hooks execute on every sync, not just when something changes.
Non-idempotent operations (like sending notifications) will fire repeatedly.

**Example in file:** `notify-deploy` Job

**Detection:**
```
RULE: postsync-idempotency-risk
RISK: Job runs on every sync; ensure operation is idempotent
```

### 3. Safe ArgoCD Patterns

The file also includes examples of safe patterns:
- Explicit `argocd.argoproj.io/hook` annotations (no ambiguity)
- Idempotent PostSync jobs (safe to run repeatedly)

## Hook Phase Mapping

| Helm Hook | ArgoCD Phase | Notes |
|-----------|--------------|-------|
| `pre-install` | PreSync | Runs before resources applied |
| `pre-upgrade` | PreSync | Same as pre-install in ArgoCD |
| `post-install` | PostSync | Runs after resources healthy |
| `post-upgrade` | PostSync | Same as post-install in ArgoCD |
| `test` | PostSync | Runs after sync |

## Remediation

1. **For notification hooks:** Add change detection logic or use Argo Events instead
2. **For migration hooks:** Ensure idempotency (use schema versioning, not one-time scripts)
3. **For mixed hooks:** Split into separate resources with explicit ArgoCD annotations

## Related Commands

```bash
# View hooks in a live cluster
./cub-scout map hooks

# View hooks in a specific namespace
./cub-scout map hooks --namespace prod

# JSON output for programmatic use
./cub-scout map hooks --format json
```
