# Platform Example: Learn GitOps with cub-scout

**A realistic GitOps environment for learning and demoing cub-scout.**

This example combines:
- **flux2-kustomize-helm-example** — Official Flux reference architecture (~28 resources)
- **Orphan resources** — Realistic "shadow IT" scenarios (~7 resources)

Total: ~35 resources across multiple namespaces

---

## What You'll Learn

| Concept | What cub-scout shows |
|---------|---------------------|
| **Ownership detection** | Flux vs Helm vs Native (orphan) |
| **Trace chains** | GitRepository → Kustomization → Deployment → Pod |
| **Orphan danger** | Resources that will be lost on cluster rebuild |
| **Multi-layer Kustomize** | Base + overlays pattern |
| **Helm via GitOps** | HelmRelease managing charts |
| **The clobbering problem** | What happens when you kubectl edit a GitOps resource |

---

## Quick Start

```bash
# 1. Create a kind cluster (or use existing)
kind create cluster --name platform-demo

# 2. Run the setup script
./setup.sh

# 3. Explore with cub-scout
cub-scout map              # Interactive TUI
cub-scout map workloads    # See ownership
cub-scout map orphans      # Find the shadow IT
cub-scout trace deploy/podinfo -n podinfo  # Trace to Git source
```

---

## What Gets Deployed

### From flux2-kustomize-helm-example

| Namespace | Resources | Owner |
|-----------|-----------|-------|
| `flux-system` | Flux controllers, GitRepository, Kustomizations | Flux |
| `podinfo` | podinfo deployment, service, HPA | Flux (Kustomization) |
| `kube-prometheus-stack` | Prometheus, Grafana, AlertManager | Flux (HelmRelease) |

### Orphan Resources (for demo)

| Namespace | Resource | Simulates |
|-----------|----------|-----------|
| `default` | `debug-nginx` Deployment | Left from debugging session |
| `default` | `manual-config` ConfigMap | kubectl apply during incident |
| `default` | `hotfix-secret` Secret | Manual hotfix |
| `kube-system` | `legacy-monitor` DaemonSet | Pre-GitOps monitoring |

---

## Learning Journeys

### Journey 1: "What's in my cluster?"

```bash
# See everything
cub-scout map

# Press these keys:
# s - Status dashboard (health overview)
# w - Workloads by owner
# o - Orphans only
# 4 - Deep dive (Deployment → ReplicaSet → Pod trees)
```

### Journey 2: "Where did this come from?"

```bash
# Pick any deployment and trace it
cub-scout trace deploy/podinfo -n podinfo

# Output shows:
# GitRepository → Kustomization → Deployment → ReplicaSet → Pods
```

### Journey 3: "What's NOT in Git?"

```bash
# Find all orphan resources
cub-scout map orphans

# These are risks:
# - No audit trail
# - Lost on cluster rebuild
# - May conflict with GitOps
```

### Journey 4: "What would change?"

```bash
# See diff between live and Git
cub-scout trace deploy/podinfo -n podinfo --diff

# Useful for:
# - "Why isn't my change applying?"
# - "What will the next reconciliation do?"
```

### Journey 5: "The Clobbering Demo"

Demonstrate what happens when someone uses kubectl on a GitOps resource:

```bash
# 1. Check current state
kubectl get deploy podinfo -n podinfo -o jsonpath='{.spec.replicas}'
# Output: 2

# 2. "Break glass" - manual change
kubectl scale deploy podinfo -n podinfo --replicas=5

# 3. Watch Flux clobber it back
watch kubectl get deploy podinfo -n podinfo
# Within 5 minutes, replicas will reset to 2

# 4. cub-scout shows the danger
cub-scout trace deploy/podinfo -n podinfo --diff
# Shows: live=5, desired=2, will reconcile!
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Git Repository                           │
│  github.com/fluxcd/flux2-kustomize-helm-example             │
└─────────────────────┬───────────────────────────────────────┘
                      │ Flux watches
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                   Flux Controllers                           │
│  source-controller, kustomize-controller, helm-controller   │
└───────┬─────────────────────────┬───────────────────────────┘
        │                         │
        ▼                         ▼
┌───────────────────┐    ┌────────────────────┐
│  Kustomization    │    │   HelmRelease      │
│  apps/podinfo     │    │   monitoring       │
└───────┬───────────┘    └────────┬───────────┘
        │                         │
        ▼                         ▼
┌───────────────────┐    ┌────────────────────┐
│  podinfo NS       │    │  monitoring NS     │
│  - Deployment     │    │  - Prometheus      │
│  - Service        │    │  - Grafana         │
│  - HPA            │    │  - AlertManager    │
└───────────────────┘    └────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                    ORPHAN RESOURCES                          │
│  (No GitOps owner - deployed via kubectl)                   │
│                                                              │
│  default/debug-nginx     ← Left from debugging              │
│  default/manual-config   ← kubectl apply during incident    │
│  kube-system/legacy-mon  ← Pre-GitOps monitoring            │
└─────────────────────────────────────────────────────────────┘
```

---

## Cleanup

```bash
# Remove the demo
./cleanup.sh

# Or manually:
flux uninstall
kubectl delete -f orphans.yaml
kind delete cluster --name platform-demo
```

---

## Troubleshooting

### "Flux not bootstrapping"

```bash
# Check flux prerequisites
flux check --pre

# Common issues:
# - GitHub token not set
# - Cluster not accessible
```

### "Resources not appearing"

```bash
# Check Kustomization status
flux get kustomizations -A

# Check for reconciliation errors
flux logs
```

### "cub-scout shows nothing"

```bash
# Verify kubectl access
kubectl get pods -A

# Check cub-scout can connect
cub-scout map list
```

---

## See Also

- [docs/SCALE-DEMO.md](../../docs/SCALE-DEMO.md) - Scale testing guide
- [docs/diagrams/](../../docs/diagrams/) - Visual explanations
- [flux2-kustomize-helm-example](https://github.com/fluxcd/flux2-kustomize-helm-example) - Upstream repo
