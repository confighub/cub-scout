# Alternatives & Related Tools

This document helps you understand when to use cub-scout vs other tools.

## Comparison Matrix

| Tool | What It Does | cub-scout Difference |
|------|--------------|---------------------|
| **kubectl** | Raw K8s API access | cub-scout adds ownership detection, queries |
| **bash scripts** | Custom queries | cub-scout is structured, tested, maintained |
| **ArgoCD CLI** | ArgoCD-specific operations | cub-scout is GitOps-agnostic (Flux, Argo, Helm) |
| **Flux CLI** | Flux-specific operations | cub-scout is GitOps-agnostic |
| **k9s** | Terminal UI for K8s | cub-scout focuses on GitOps ownership, not general K8s |
| **Lens/OpenLens** | Desktop K8s IDE | cub-scout is CLI-first, GitOps-focused |
| **CloudQuery** | Data pipeline to databases | cub-scout is real-time, no DB needed, GitOps-aware |
| **Karpor** | Multi-cluster search with AI | cub-scout has native GitOps ownership detection, deterministic |
| **Headlamp** | Web UI for K8s | cub-scout is CLI/TUI, GitOps-focused |

## cub-scout Unique Value

### GitOps Ownership Detection

cub-scout knows who manages each resource:
- Flux (Kustomize or Helm)
- ArgoCD
- Helm (standalone)
- Terraform
- ConfigHub
- Native (kubectl-applied)

Other tools show you resources but don't tell you *who owns them*.

### Trace Command

Walk the full delivery chain from Git to cluster:

```bash
cub-scout trace deployment/api -n production
# Shows: Git repo → Flux Kustomization → Deployment → ReplicaSet → Pod
```

### Orphan Detection

Find resources not managed by any GitOps tool:

```bash
cub-scout orphans
# Shows: Resources with no ownership labels
```

### Deterministic, Not AI

All calculations are 100% deterministic:
- Same input = same output
- No ML models, no API keys
- Works fully offline
- Auditable and explainable

### Works Standalone

No server, no database, just a binary:
- Download and run
- No installation wizard
- No external dependencies

## When to Use Each Tool

| Use Case | Best Tool |
|----------|-----------|
| General K8s exploration | k9s, Lens |
| Flux-specific operations | Flux CLI |
| ArgoCD-specific operations | ArgoCD CLI |
| GitOps ownership visibility | **cub-scout** |
| Multi-tool GitOps environments | **cub-scout** |
| Finding orphaned resources | **cub-scout** |
| Tracing delivery chains | **cub-scout** |
| Data pipeline to SQL | CloudQuery |
| AI-powered search | Karpor |

## Integration

cub-scout works alongside other tools:

```bash
# Use with kubectl
kubectl get pods -o json | cub-scout analyze -

# Export for CloudQuery
cub-scout snapshot --json > cluster-state.json

# Pipe to jq
cub-scout list --json | jq '.[] | select(.owner == "Native")'
```
