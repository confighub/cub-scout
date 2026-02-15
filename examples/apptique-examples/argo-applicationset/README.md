# Argo CD ApplicationSet Pattern

## The Problem

You want ArgoCD to auto-discover environments from your directory structure.
An ApplicationSet with a directory generator creates Applications for `dev/` and `prod/`.
But when something breaks, you need to know:
*"Did the ApplicationSet actually generate the prod Application? Is it synced?"*

**cub-scout shows what's actually deployed:**

```
$ ./cub-scout map list -q "owner=ArgoCD" -q "namespace=apptique-*"

  STATUS  NAMESPACE      NAME      OWNER   MANAGED-BY
  ✓       apptique-dev   frontend  ArgoCD  apptique-dev
  ✓       apptique-prod  frontend  ArgoCD  apptique-prod
```

## Architecture

```
ApplicationSet/apptique (argocd)
├── generates → Application/apptique-dev  → Deployment/frontend (apptique-dev)
└── generates → Application/apptique-prod → Deployment/frontend (apptique-prod)
```

```
argo-applicationset/
├── bootstrap/
│   └── applicationset.yaml         # Directory generator
└── apps/apptique/
    ├── dev/deployment.yaml         # Auto-discovered
    └── prod/deployment.yaml        # Auto-discovered
```

**How it works:**
1. ApplicationSet scans `apps/apptique/*` directories
2. Generates one Application per directory (`dev`, `prod`)
3. Each Application deploys to its own namespace

## How cub-scout Sees It

```
./cub-scout tree ownership

OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════

ArgoCD (4 resources)
────────────────────────────────────────────────────────────────────
  ApplicationSet/apptique (argocd)
  │
  ├── generates → Application/apptique-dev (argocd)
  │              └── Deployment/frontend (apptique-dev)     ✓ 1/1
  │
  └── generates → Application/apptique-prod (argocd)
                 └── Deployment/frontend (apptique-prod)    ✓ 2/2

════════════════════════════════════════════════════════════════════

  Generator                 Discovered           Deployed
  ─────────                 ──────────           ────────
  apps/apptique/ scanner →  dev/ directory  →    apptique-dev/frontend
                         →  prod/ directory →    apptique-prod/frontend

→ Add a directory = add an environment (no YAML changes needed)
→ Trace any app:   cub-scout trace deploy/frontend -n apptique-prod
```

## What It Demonstrates

| What you'll see | Why it matters |
|-----------------|----------------|
| Auto-generated Applications detected | Ownership detection works with ApplicationSets |
| Each generated app traced to its source | Answers "where does this deployment come from?" |
| Directory → namespace mapping visible | See how generator templates produce real apps |
| `gitops status` covers all generated apps | One health check for the whole set |

## Quick Start

```bash
# Deploy the ApplicationSet
kubectl apply -f examples/apptique-examples/argo-applicationset/bootstrap/applicationset.yaml

# Watch Applications get created
kubectl get applications -n argocd --watch
# Expected: apptique-dev, apptique-prod created automatically

# See all ArgoCD-managed workloads
./cub-scout map list -q "owner=ArgoCD"

# Trace a generated app back to source
./cub-scout trace deployment/frontend -n apptique-prod

# Check sync health
./cub-scout gitops status
```

## Ownership Labels

The ApplicationSet template adds these labels:

```yaml
labels:
  app.kubernetes.io/name: apptique
  app.kubernetes.io/instance: 'apptique-{{path.basename}}'
  environment: '{{path.basename}}'
```

cub-scout detects `argocd.argoproj.io/instance` or `app.kubernetes.io/instance` to identify the owning Application.

## When to Use ApplicationSets

- Dynamic environment discovery (add a directory = add an environment)
- Cluster-based generators (deploy to all clusters matching a label)
- Reducing boilerplate (one template instead of N Application CRs)
- Teams that add environments frequently

## Cleanup

```bash
kubectl delete applicationset apptique -n argocd
kubectl delete ns apptique-dev apptique-prod
```

## See Also

- [Argo App-of-Apps](../argo-app-of-apps/) — Alternative: explicit parent-child hierarchy
- [Flux Monorepo](../flux-monorepo/) — Same app, Flux instead of ArgoCD
- [ApplicationSet Pattern](../../rm-demos-argocd/repo-patterns/applicationsets/) — Pattern comparison
