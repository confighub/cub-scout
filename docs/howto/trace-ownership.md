# Trace Ownership Chains

Trace any resource to its Git source — **one command for Flux, ArgoCD, or Helm**.

You don't need to know which tool manages a resource. Just run trace and cub-scout auto-detects the owner.

---

## Why This Matters

In mixed environments with multiple GitOps tools:
- **Without cub-scout:** Check labels → figure out owner → run `flux trace` or `argocd app get` or `helm status`
- **With cub-scout:** `cub-scout trace deploy/nginx -n prod` — done

---

## Owner Detection vs Trace Chain

`trace` has two steps:

1. Detect owner type from resource signals (same deterministic precedence used by `map list`).
2. Resolve the chain using the matching source resolver.

| Owner detected | Chain resolver |
|----------------|----------------|
| Flux | Flux source + Kustomization/HelmRelease chain |
| ArgoCD | Argo Application/ApplicationSet/App-of-Apps chain |
| Helm (standalone) | Helm release metadata from cluster secrets |
| Native/Unknown | Kubernetes ownerRef chain + orphan metadata |

This means Argo traces are resolved from Argo resources directly (not by reusing Flux semantics).

See also:
- [Ownership precedence rules](../reference/ownership-precedence.md)
- [Ownership detection guide](ownership-detection.md)

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

## Trace with Source Artifacts

Include source artifact provenance (read-only):

```bash
cub-scout trace deploy/podinfo -n podinfo --artifacts
```

For Flux sources (`GitRepository`, `OCIRepository`, `HelmRepository`, `Bucket`),
trace adds:

- artifact URL
- artifact revision
- artifact digest
- artifact last update time

JSON output places these under `summary.source.artifact`.
If provenance cannot be read, fields are still present with explicit `unknown` values.

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

### Flux Resources (Git or OCI)

```bash
# Flux Kustomization trace (Git source)
cub-scout trace deploy/app -n namespace
```

Shows: GitRepository → Kustomization → Deployment

```bash
# Flux with OCI source (container registry)
cub-scout trace deploy/app -n namespace
```

Shows: OCIRepository → Kustomization → Deployment

**Supported Flux sources:** GitRepository, OCIRepository, HelmRepository, Bucket

### ArgoCD Resources

```bash
# ArgoCD Application trace
cub-scout trace deploy/app -n namespace
```

Shows: Repository → Application → Deployment

### Helm Resources (Standalone)

For Helm releases **not managed by Flux HelmRelease** (standalone `helm install`):

```bash
# Standalone Helm release trace
cub-scout trace deploy/prometheus -n monitoring
```

Shows: HelmChart → Release → Deployment

**How it works:** cub-scout reads Helm release metadata from Kubernetes secrets (`sh.helm.release.v1.*`) to trace the full chain without requiring Flux.

### Flux HelmRelease

For Helm charts managed by Flux:

```bash
# Flux-managed Helm trace
cub-scout trace deploy/redis -n cache
```

Shows: HelmRepository → HelmRelease → Deployment

### Orphan Resources

```bash
# No GitOps owner
cub-scout trace deploy/debug-nginx -n default
```

Shows: "No GitOps owner found — created manually"

**Tip:** Use `--reverse` to see additional metadata for orphans:

```bash
cub-scout trace deploy/debug-nginx -n default --reverse
```

Shows:
- Creation timestamp
- Resource labels
- `kubectl.kubernetes.io/last-applied-configuration` (if created via `kubectl apply`)

---

## Reverse Trace

Walk **up** the ownership chain from any resource:

```bash
cub-scout trace pod/nginx-7d9b8c-x4k2p -n prod --reverse
```

**Output:**

```
REVERSE TRACE: Pod/nginx-7d9b8c-x4k2p

K8s Ownership Chain:
✓ Pod/nginx-7d9b8c-x4k2p (Running)
  └─▶ ✓ ReplicaSet/nginx-7d9b8c (3/3 ready)
      └─▶ ✓ Deployment/nginx (3/3 ready)

Detected Owner: FLUX (managed by apps)

💡 For full GitOps chain, run:
   cub-scout trace deployment/nginx -n prod
```

### Orphan Metadata

For unmanaged resources, `--reverse` extracts kubectl metadata:

```bash
cub-scout trace deploy/debug-nginx -n default --reverse
```

```
Detected Owner: NATIVE

⚠ This resource is NOT managed by GitOps
  • It will be lost if the cluster is rebuilt
  • No audit trail in Git
  • Consider importing to GitOps: cub-scout import

Orphan Metadata:
  Created: 2026-01-15 10:30:00 UTC
  Labels:
    app=debug-nginx
    team=platform

✓ last-applied-configuration found
  This resource was created via 'kubectl apply'.
  The original manifest is available in the annotation.

  💡 To see full manifest:
  kubectl get deployment debug-nginx -n default -o jsonpath='{.metadata.annotations.kubectl\.kubernetes\.io/last-applied-configuration}' | jq .
```

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
- [diagrams/README.md](../diagrams/README.md) — Diagram index and regeneration guide
