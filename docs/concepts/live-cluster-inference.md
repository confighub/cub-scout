# Live Cluster Inference: Detection Without Git

How cub-scout detects ownership, variant, and structure from your cluster without parsing Git repos.

## The Insight: Git Access Not Required

Most people assume you need to parse Git repos to understand GitOps structure. **Not true.**

GitOps deployers (Flux, Argo CD) store everything we need in the cluster itself:

| What We Need | Where It Lives | Example |
|--------------|----------------|---------|
| Which app is this? | Workload labels | `app.kubernetes.io/name: payment-api` |
| Who deploys it? | Workload labels | `kustomize.toolkit.fluxcd.io/name: apps` |
| What path in the repo? | Deployer object | `Kustomization.spec.path: ./staging` |
| What revision? | Deployer status | `status.lastAppliedRevision: main@sha1:abc123` |

**The path is the key.** When Flux deploys from `./staging`, it stores that path. We read it.

---

## What We Can Infer from LIVE

### 1. Ownership Detection

```bash
cub-scout map list
```

```
NAME            NAMESPACE    OWNER      STATUS
payment-api     prod         Flux       ✓ Synced
payment-worker  prod         Flux       ✓ Synced
redis           prod         Helm       ✓ Deployed
debug-pod       prod         Native     ⚠ Orphan
```

**Solved:** "Who manages what?" — answered instantly from cluster labels.

### 2. Variant Inference from Path

The deployer stores its source path. We extract variant from it:

| Deployer Path | Inferred Variant |
|---------------|------------------|
| `./staging` | `staging` |
| `./production` | `prod` |
| `apps/prod/payment` | `prod` |
| `tenants/checkout/overlays/dev` | `dev` |

**Solved:** "What environment is this?" — no Git parsing needed.

### 3. Structure Suggestion

```bash
cub-scout tree suggest
```

```
SUGGESTED STRUCTURE
───────────────────────────────────────────────────
Space: myapp-team

  Unit: payment-prod
    labels: app=payment, variant=prod
    workloads: 2
```

**Solved:** "How should I organize this?" — smart suggestions from what's deployed.

### 4. Any Repo Layout

Different teams use different Git layouts. Doesn't matter:

| Their Repo Structure | We Read From LIVE |
|---------------------|-------------------|
| `apps/{env}/{app}/` | Kustomization path |
| `clusters/{env}/` | Kustomization path |
| `tenants/{team}/{app}/overlays/{env}/` | Application path |
| Flat structure | Fall back to namespace/labels |

**Solved:** "Our repo is weird" — we adapt to whatever exists.

---

## Inference Priority

When determining variant, we check in order:

| Priority | Source | Confidence |
|----------|--------|------------|
| 0 | Flux `Kustomization.spec.path` | High |
| 0 | Argo `Application.spec.source.path` | High |
| 1 | Label `app.kubernetes.io/instance` | High |
| 2 | Label `environment` or `env` | High |
| 3 | Namespace pattern (`myapp-prod`) | Medium |
| 4 | Workload name | Low |

GitOps paths win because the deployer explicitly stores them.

---

## The Limit: What LIVE Can't Tell Us

| Question | From LIVE? | Why Not |
|----------|------------|---------|
| What other variants exist? | No | Only see what's deployed HERE |
| What's in `apps/base/`? | No | Base templates aren't deployed |
| What changed in Git but isn't deployed? | No | Pending commits invisible |
| Who approved this change? | No | Git history not in cluster |
| What SHOULD exist but doesn't? | No | Drift by deletion |

**Rule:** LIVE tells us what IS. Git tells us what SHOULD BE.

---

## Summary

| Capability | From LIVE Cluster |
|------------|-------------------|
| Detect ownership | ✓ Yes |
| Infer variant from path | ✓ Yes |
| Suggest structure | ✓ Yes |
| Handle any repo layout | ✓ Yes |
| See other clusters | ✗ No (need fleet) |
| See undeployed variants | ✗ No (need Git) |
| See Git history | ✗ No (need Git) |

**Single-cluster inference works completely from LIVE data. No Git access needed.**

---

## See Also

- [Ownership Detection](../howto/ownership-detection.md) — How to use ownership detection
- [Architecture](architecture.md) — System design overview
