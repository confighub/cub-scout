# Import from LIVE: How Detection Works Without Git

**Technical explainer** — How we detect ownership, variant, and structure from the cluster without parsing Git repos.

---

## The Surprise: You Don't Need Git

Most people assume you need to parse Git repos to understand GitOps structure. **Wrong.**

The GitOps deployers (Flux, Argo CD) store everything we need in the cluster itself:

| What We Need | Where It Lives | Example |
|--------------|----------------|---------|
| Which app is this? | Workload labels | `app.kubernetes.io/name: payment-api` |
| Who deploys it? | Workload labels | `kustomize.toolkit.fluxcd.io/name: apps` |
| What path in the repo? | Deployer object | `Kustomization.spec.path: ./staging` |
| What revision? | Deployer status | `status.lastAppliedRevision: main@sha1:abc123` |

**The path is the key.** When Flux deploys from `./staging`, it stores that path. We read it.

---

## What TUI Can Do (From LIVE Cluster)

### 1. Detect Ownership

```bash
cub-scout import --dry-run
```

Output:
```
┌─────────────────────────────────────────────────────────────┐
│ DISCOVERED                                                  │
└─────────────────────────────────────────────────────────────┘
  myapp (4 workloads)
    payment-api (owner: Flux)
    payment-worker (owner: Flux)
    redis (owner: Helm)
    debug-pod (owner: Native)
```

**Problem solved:** "Who manages what?" — answered instantly from cluster labels.

### 2. Infer Variant from Path

The deployer stores its source path. We extract variant from it:

| Deployer Path | Inferred Variant |
|---------------|------------------|
| `./staging` | `staging` |
| `./production` | `prod` |
| `apps/prod/payment` | `prod` |
| `tenants/checkout/overlays/dev` | `dev` |

**Problem solved:** "What environment is this?" — no Git parsing needed.

### 3. Suggest ConfigHub Structure

```bash
cub-scout import -n myapp-prod --dry-run
```

Output:
```
┌─────────────────────────────────────────────────────────────┐
│ WILL CREATE                                                 │
└─────────────────────────────────────────────────────────────┘
  App Space: myapp-team

  • payment-prod
    labels: app=payment, variant=prod
    workloads: 2
```

**Problem solved:** "How should I organize this in ConfigHub?" — smart suggestions from what's deployed.

### 4. Map Any Repo Structure

Different teams use different Git layouts. Doesn't matter:

| Their Repo Structure | We Read From LIVE |
|---------------------|-------------------|
| `apps/{env}/{app}/` | Kustomization path |
| `clusters/{env}/` | Kustomization path |
| `tenants/{team}/{app}/overlays/{env}/` | Application path |
| Flat structure | Fall back to namespace/labels |

**Problem solved:** "Our repo is weird" — we adapt to whatever structure exists.

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

## Demo

```bash
# Preview import
cub-scout import --dry-run

# Import a specific namespace
cub-scout import -n <your-ns> --dry-run

# Actually import
cub-scout import
```

---

## Summary

| Capability | TUI (LIVE) |
|------------|------------|
| Detect ownership | Yes |
| Infer variant from path | Yes |
| Suggest App Space structure | Yes |
| Handle any repo layout | Yes |
| See other clusters | No |
| See undeployed variants | No |
| See Git history | No |

**TUI handles single-cluster import completely from LIVE data. No Git access needed.**
