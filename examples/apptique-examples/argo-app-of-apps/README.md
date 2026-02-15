# Argo CD App-of-Apps Pattern

## The Problem

Your enterprise ArgoCD setup uses a parent Application to manage child Applications.
When a child app fails to sync, you need to answer:
*"Is the root app healthy? Which child is broken? Where does it pull from?"*

ArgoCD's UI shows the tree, but you need kubectl + ArgoCD UI + repo browsing to get the full picture.

**cub-scout traces the full hierarchy in one command:**

```
$ ./cub-scout trace deployment/frontend -n apptique-dev

  Deployment/frontend (apptique-dev)
  ├── Owner: ArgoCD
  ├── Application: apptique-dev (argocd)
  │   └── Managed by: apptique-apps (root)
  └── Source: confighubai/confighub-agent → manifests/apptique/dev@HEAD
```

## Architecture

```
Root Application (apptique-apps)
├── Child: apptique-dev  → Deployment/frontend (apptique-dev)
└── Child: apptique-prod → Deployment/frontend (apptique-prod)
```

```
argo-app-of-apps/
├── root/
│   └── root-app.yaml           # Parent Application
├── apps/
│   ├── apptique-dev.yaml       # Child Application CR
│   └── apptique-prod.yaml      # Child Application CR
└── manifests/apptique/
    ├── dev/deployment.yaml     # Actual K8s manifests
    └── prod/deployment.yaml
```

**How it works:**
1. Root app (`apptique-apps`) syncs the `apps/` directory → creates child Application CRs
2. Each child app syncs its respective manifests directory
3. Two-level hierarchy: Root → Child App → Workloads

## How cub-scout Sees It

```
./cub-scout tree ownership

OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════

ArgoCD (5 resources)
────────────────────────────────────────────────────────────────────
  Application/apptique-apps (root)
  ├── Application/apptique-dev (child)
  │   └── Deployment/frontend (apptique-dev)      ✓ 1/1
  └── Application/apptique-prod (child)
      └── Deployment/frontend (apptique-prod)      ✓ 2/2

════════════════════════════════════════════════════════════════════

  Root App           Child Apps          Workloads
  ──────────         ──────────          ─────────
  apptique-apps ──→  apptique-dev  ──→  frontend (1 replica)
                └──→ apptique-prod ──→  frontend (2 replicas)

→ Trace any level: cub-scout trace deploy/frontend -n apptique-dev
→ Check all sync:  cub-scout gitops status
```

## What It Demonstrates

| What you'll see | Why it matters |
|-----------------|----------------|
| Root → Child → Workload hierarchy | Full app-of-apps lineage |
| Both child apps detected as ArgoCD-managed | Ownership detection works through the hierarchy |
| Trace shows parent Application | Answers "who manages this Application?" |
| `gitops status` shows sync state for all | Pipeline health at a glance |

## Quick Start

```bash
# Deploy the root Application
kubectl apply -f examples/apptique-examples/argo-app-of-apps/root/root-app.yaml

# Watch the hierarchy unfold
kubectl get applications -n argocd --watch
# Expected: apptique-apps (root), apptique-dev, apptique-prod

# See ownership
./cub-scout map list -q "owner=ArgoCD"

# Trace any workload through the hierarchy
./cub-scout trace deployment/frontend -n apptique-dev

# Check pipeline health
./cub-scout gitops status
```

## Ownership Labels

ArgoCD adds these labels to managed resources:

```yaml
labels:
  app.kubernetes.io/instance: apptique-dev
  argocd.argoproj.io/instance: apptique-dev
```

cub-scout uses these to detect ArgoCD ownership and trace back to the Application CR.

## When to Use App-of-Apps

- Multi-team environments where teams own their Application CRs
- Need to version Application configurations separately from workloads
- Want to use ArgoCD RBAC at the Application level
- Enterprise setups with approval workflows per environment

## Cleanup

```bash
kubectl delete application apptique-apps -n argocd
kubectl delete ns apptique-dev apptique-prod
```

## See Also

- [Argo ApplicationSet](../argo-applicationset/) — Alternative: auto-discover environments
- [Flux Monorepo](../flux-monorepo/) — Same app, Flux instead of ArgoCD
- [ApplicationSet Pattern](../../rm-demos-argocd/repo-patterns/applicationsets/) — Pattern comparison
