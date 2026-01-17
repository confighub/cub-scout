# Break Glass Scenarios: When GitOps Isn't Fast Enough

## The Problem

GitOps is great until it isn't:
- Production is down NOW
- GitOps sync takes 5 minutes
- PR review takes 30 minutes
- You need to fix it NOW

**The forbidden command:**
```bash
kubectl apply -f hotfix.yaml -n prod
```

## What Happens After Break Glass

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         BREAK GLASS FLOW                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. EMERGENCY                                                               │
│     │   kubectl apply -f hotfix.yaml -n prod                                │
│     │   Resource deployed immediately                                       │
│     │   GitOps doesn't know about it                                        │
│     │                                                                       │
│     ▼                                                                       │
│  2. DETECTION                                                               │
│     │   cub-scout map orphans                                               │
│     │   Shows resource as "Native" (orphan)                                 │
│     │   "Who deployed this? When? Why?"                                     │
│     │                                                                       │
│     ▼                                                                       │
│  3. DECISION (ConfigHub)                                                    │
│     │                                                                       │
│     ├──▶ ACCEPT                        ├──▶ REJECT                          │
│     │    "This is good, keep it"       │    "This shouldn't exist"          │
│     │    Creates Unit in ConfigHub     │    Deletes from cluster            │
│     │                                  │    GitOps state restored           │
│     ▼                                  │                                    │
│  4. MERGE (if accepted)                                                     │
│         ConfigHub becomes source of truth                                   │
│         ──▶ OCI artifact created                                            │
│         ──▶ Git updated (PR or direct)                                      │
│         ──▶ Other stores notified                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## The Three Outcomes

### Outcome A: Accept → Adopt into GitOps

The break-glass change was correct. Make it permanent.

```bash
# 1. Detect the orphan
cub-scout map orphans
# Shows: deploy/hotfix-api (Native)

# 2. Import to ConfigHub
cub-scout import deploy/hotfix-api -n prod
# Creates Unit in ConfigHub

# 3. ConfigHub updates stores
# - OCI artifact published
# - Git PR created (or direct commit)
# - Resource now managed by GitOps
```

**Result:** Break-glass change is now in Git, managed by GitOps.

### Outcome B: Reject → Restore GitOps State

The break-glass change was temporary or wrong. Remove it.

```bash
# 1. Detect the orphan
cub-scout map orphans

# 2. Delete from cluster
kubectl delete deploy/hotfix-api -n prod

# 3. GitOps restores correct state
# (Flux/ArgoCD reconciles from Git)
```

**Result:** Cluster matches Git again.

### Outcome C: Defer → Track but Don't Merge

You need more time to decide.

```bash
# 1. Tag the orphan for tracking
cub-scout tag deploy/hotfix-api -n prod --label="break-glass=2026-01-14"

# 2. Review later
cub-scout map orphans --label="break-glass"
```

**Result:** Orphan is tracked, decision deferred.

## How Map Helps

### During the Incident

```bash
# See what's actually running (not what Git says)
cub-scout map list -n prod

# Quick health check
cub-scout map issues
```

### After the Incident

```bash
# Find all break-glass resources
cub-scout map orphans

# See what was kubectl-applied
cub-scout map list -q "owner=Native AND namespace=prod"

# Trace what changed
cub-scout trace deploy/hotfix-api -n prod
```

### Weekly Audit

```bash
# Monday morning check
cub-scout map orphans -n prod

# Alert if orphans exist
if [[ $(cub-scout map orphans --count) -gt 0 ]]; then
  echo "WARNING: Orphan resources in production"
fi
```

## The ConfigHub Advantage

Without ConfigHub:
- Break-glass → manual Git PR → hope nobody forgets
- No tracking of what was changed
- No audit trail

With ConfigHub:
- Break-glass → map detects → accept/reject in ConfigHub
- ConfigHub updates Git automatically
- Full audit trail in ConfigHub

## The Merge Flow (RM Pattern)

When you ACCEPT a break-glass change:

```
LIVE (cluster)     CONFIGUB (store)     OCI (transport)     FLUX/ARGO
     │                   │                    │                  │
     │   import          │                    │                  │
     ├──────────────────▶│                    │                  │
     │                   │                    │                  │
     │                   │    publish         │                  │
     │                   ├───────────────────▶│                  │
     │                   │                    │                  │
     │                   │                    │    pull          │
     │                   │                    │◀─────────────────┤
     │                   │                    │                  │
     │◀──────────────────┼────────────────────┼──── reconcile ───┤
     │                   │                    │                  │
     │                   │                    │                  │
     │               (optional)               │                  │
     │                   ├── Git PR ─────────────────────────────▶
     │                   │                    │                  │
```

**Primary flow:** ConfigHub → OCI → Flux/ArgoCD → Cluster
**Optional:** Git PR for audit/review (not required for deployment)

**Key insight:** ConfigHub becomes the decision point. It can:
- Accept live changes → push via OCI → Flux/ArgoCD reconciles
- Reject live changes → restore from Git
- Track changes → defer decision

**Why OCI, not Git?**
- OCI artifacts are immutable (tags don't change)
- No merge conflicts (rendered manifests)
- Faster distribution (pull only what's needed)
- Git PR is for humans, OCI is for machines

## Demo

```bash
# Simulate break-glass scenario
# DEPRECATED: ./test/atk/demo scenario orphan-hunt

# Shows:
# 1. Orphan resources created
# 2. Detection with map
# 3. Options for remediation
```

## See Also

- [Ownership Visibility](ownership-visibility.md) — The Native bucket insight
- [Find Orphans](../map/howto/find-orphans.md) — How to find unmanaged resources
- [Import to ConfigHub](../map/howto/import-to-confighub.md) — Adopting resources
