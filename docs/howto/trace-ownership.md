# Trace Ownership Chains

Trace any resource to its Git source — **one command for Flux, ArgoCD, or Helm**.

You don't need to know which tool manages a resource. Just run trace and cub-scout auto-detects the owner.

---

## Why This Matters

In mixed environments with multiple GitOps tools:
- **Without cub-scout:** Check labels → figure out owner → run `flux trace` or `argocd app get` or `helm status`
- **With cub-scout:** `cub-scout trace deploy/nginx -n prod` — done

---

## Basic Trace

```bash
cub-scout trace deploy/podinfo -n podinfo
```

**Output:**

```
┌─ TRACE: podinfo ─────────────────────────────────────────────────────────────┐
│                                                                              │
│  ┌─────────────────────────┐                                                │
│  │ GitRepository           │                                                │
│  │ flux-system/flux-system │                                                │
│  │ https://github.com/...  │                                                │
│  │ Revision: main@abc123   │                                                │
│  └───────────┬─────────────┘                                                │
│              │                                                               │
│              ▼                                                               │
│  ┌─────────────────────────┐                                                │
│  │ Kustomization           │                                                │
│  │ flux-system/apps        │                                                │
│  │ Path: ./apps/podinfo    │                                                │
│  │ Status: Applied         │                                                │
│  └───────────┬─────────────┘                                                │
│              │                                                               │
│              ▼                                                               │
│  ┌─────────────────────────┐                                                │
│  │ Deployment              │                                                │
│  │ podinfo/podinfo         │                                                │
│  │ Status: 2/2 Ready       │                                                │
│  └─────────────────────────┘                                                │
│                                                                              │
│  ✓ Full chain traced: Git → Flux → Kubernetes                               │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Trace with Diff

See what would change on next reconciliation:

```bash
cub-scout trace deploy/podinfo -n podinfo --diff
```

**Output:**

```
┌─ DIFF: podinfo ──────────────────────────────────────────────────────────────┐
│                                                                              │
│  spec.replicas:                                                              │
│    - live:    5     (kubectl edit)                                          │
│    + desired: 2     (from Git)                                              │
│                                                                              │
│  ⚠ This resource will revert on next Flux reconciliation                    │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## What Trace Shows

| Information | Example |
|-------------|---------|
| **Git source** | Repository URL, branch, revision |
| **GitOps controller** | Flux Kustomization, Argo Application |
| **Path in repo** | `./apps/podinfo/overlays/prod` |
| **Current state** | Pods ready, sync status |
| **Diff** | Live vs desired state |

---

## Tracing by Owner Type

### Flux Resources

```bash
# Flux Kustomization trace
cub-scout trace deploy/app -n namespace
```

Shows: GitRepository → Kustomization → Deployment

### ArgoCD Resources

```bash
# ArgoCD Application trace
cub-scout trace deploy/app -n namespace
```

Shows: Repository → Application → Deployment

### Helm Resources

```bash
# Helm release trace
cub-scout trace deploy/app -n namespace
```

Shows: HelmRepository → HelmRelease → Deployment

### Orphan Resources

```bash
# No GitOps owner
cub-scout trace deploy/debug-nginx -n default
```

Shows: "No GitOps owner found — created manually"

---

## Broken Trace Example

When something is wrong:

```
┌─ TRACE: broken-app ──────────────────────────────────────────────────────────┐
│                                                                              │
│  ┌─────────────────────────┐                                                │
│  │ GitRepository           │                                                │
│  │ ✓ Ready                 │                                                │
│  └───────────┬─────────────┘                                                │
│              │                                                               │
│              ▼                                                               │
│  ┌─────────────────────────┐                                                │
│  │ Kustomization           │  ◀── PROBLEM HERE                              │
│  │ ✗ ReconciliationFailed  │                                                │
│  │ Error: path not found   │                                                │
│  └───────────┬─────────────┘                                                │
│              │                                                               │
│              ▼                                                               │
│  ┌─────────────────────────┐                                                │
│  │ Deployment (stale)      │                                                │
│  │ Running old version     │                                                │
│  └─────────────────────────┘                                                │
│                                                                              │
│  ⚠ Chain broken at Kustomization — deployment is stale                      │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## See Also

- [concepts/clobbering-problem.md](../concepts/clobbering-problem.md) — Why diffs matter
- [diagrams/ownership-trace.d2](../diagrams/ownership-trace.d2) — Visual trace diagram
