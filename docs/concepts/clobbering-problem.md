# The Clobbering Problem

Why setting the wrong value in a GitOps pipeline is so easy — and how to prevent it.

---

## The Problem

In GitOps, your deployment passes through multiple layers:

```
Upstream Chart → Chart Defaults → values.yaml → HelmRelease → Kustomize Overlay → Live Cluster
     ↓               ↓                ↓              ↓               ↓              ↓
  v14.0.1        replicas: 2      replicas: 3    patches...    patchesJson6902   ACTUAL
```

Each layer can override the previous one. This creates **clobbering scenarios** — where changes get silently overwritten.

---

## Types of Clobbering

### 1. Default Value Clobbering

Chart maintainer changes a default you were relying on.

```yaml
# Chart v14.0.1 (old)
replicaCount: 2

# Chart v15.0.0 (new) - maintainer changed default
replicaCount: 1   # ← Surprise! Your production now has 1 replica
```

**Result:** Your production deployment silently drops to 1 replica.

### 2. Hardcoded Template Clobbering

Chart template ignores your values entirely.

```yaml
# Your values.yaml
resources:
  limits:
    memory: "2Gi"

# Chart template (v15.0.0)
resources:
  limits:
    memory: "512Mi"  # ← Hardcoded, ignores your values
```

**Result:** Your memory limit is ignored, pods OOM crash.

### 3. Break-Glass Clobbering

Someone `kubectl edit`ed a resource directly.

```bash
# During incident at 3 AM
kubectl scale deployment payment-api --replicas=5

# 5 minutes later, Flux reconciles
# replicas: 5 → replicas: 2 (from Git)
```

**Result:** Your emergency scale-up gets reverted.

### 4. Layer Complexity

Kustomize overlays add another layer.

```yaml
# base/deployment.yaml
replicas: 2

# overlays/prod/patch.yaml
replicas: 5

# overlays/prod/kustomization.yaml  ← typo in path
patches:
  - path: patches/patch.yaml  # WRONG - should be patch.yaml
```

**Result:** Patch never applies, prod runs with 2 replicas.

---

## Hyrum's Law

> "With a sufficient number of users of an API, it does not matter what you promise in the contract: all observable behaviors of your system will be depended on by somebody."

Chart maintainers can't see all the ways people use their charts. A "safe" change on their side can break your deployment.

---

## How cub-scout Helps

### 1. Trace the Full Chain

```bash
cub-scout trace deploy/payment-api -n payments
```

Shows every layer from Git to live cluster.

### 2. Diff Live vs Desired

```bash
cub-scout trace deploy/payment-api -n payments --diff
```

Shows what would change on next reconciliation.

```
┌─ DIFF: payment-api ──────────────────────────────────────────────────────────┐
│                                                                              │
│  spec.replicas:                                                              │
│    - live:    5     (kubectl edit)                                          │
│    + desired: 2     (from Git)                                              │
│                                                                              │
│  ⚠ This resource will revert on next Flux reconciliation                    │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 3. Find Orphans (Break-Glass Leftovers)

```bash
cub-scout map orphans
```

Shows resources created outside GitOps — often from incident response.

---

## Prevention Strategies

| Strategy | How |
|----------|-----|
| **Pin chart versions** | Always specify exact version in HelmRelease |
| **Override all critical values** | Don't rely on chart defaults |
| **Use trace --diff** | Before upgrading, see what will change |
| **Check for orphans** | After incidents, clean up manual changes |
| **Test upgrades in staging** | Catch clobbering before production |

---

## The Demo

See the clobbering problem in action:

```bash
# 1. Check current replicas
kubectl get deploy podinfo -n podinfo -o jsonpath='{.spec.replicas}'
# Output: 2

# 2. "Break glass" - manual change
kubectl scale deploy podinfo -n podinfo --replicas=5

# 3. Watch Flux clobber it back
watch kubectl get deploy podinfo -n podinfo
# Within 5 minutes: replicas goes back to 2

# 4. cub-scout shows the danger
cub-scout trace deploy/podinfo -n podinfo --diff
# Shows: live=5, desired=2, will reconcile!
```

---

## See Also

- [diagrams/clobbering-problem.d2](../diagrams/clobbering-problem.d2) — Visual explanation
- [howto/trace-ownership.md](../howto/trace-ownership.md) — Tracing guide
- [examples/platform-example/](../../examples/platform-example/) — Live demo
