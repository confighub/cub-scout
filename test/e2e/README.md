# TUI E2E Testing

End-to-end tests for the cub-agent TUI using real Kubernetes clusters with multiple GitOps tools.

## Quick Start

```bash
# Create test cluster with Flux + Argo CD + workloads
./setup-multi-tool-cluster.sh

# Run the TUI
cub-agent map

# Tear down when done
./teardown-cluster.sh
```

## What Gets Deployed

The setup script creates a kind cluster with:

| Namespace | Tool | Workloads |
|-----------|------|-----------|
| `flux-system` | Flux CD | Controllers |
| `flux-demo` | Flux | podinfo (from GitRepository) |
| `argocd` | Argo CD | Controllers |
| `argo-demo` | Argo CD | guestbook (from Application) |
| `helm-demo` | Helm | nginx (from bitnami chart) |
| `native-demo` | Native | mystery-app (kubectl apply) |
| `confighub-demo` | ConfigHub | payment-api (with labels) |

## Ownership Detection

After setup, `cub-agent map workloads` should show:

```
OWNER       COUNT   NAMESPACES
Flux        2       flux-demo
ArgoCD      1       argo-demo
Helm        1       helm-demo
Native      1       native-demo
ConfigHub   1       confighub-demo
```

## User Journey Tests

See [docs/planning/TUI-E2E-TESTING-PLAN.md](../../docs/planning/TUI-E2E-TESTING-PLAN.md) for:

1. "What's running?" - Dashboard exploration
2. "Find the problem" - Crash/issue investigation
3. "Audit GitOps" - Coverage analysis
4. "Import to ConfigHub" - Connected mode workflow
5. "Check drift" - Sync status verification
6. "Hub navigation" - Hierarchy exploration

## Adding Test Scenarios

### Degraded State (crashes)

```bash
# Scale down a deployment to cause issues
kubectl scale deployment/guestbook-ui -n argo-demo --replicas=0
```

### Drift State

```bash
# Manually edit a Flux-managed resource
kubectl annotate deployment/podinfo -n flux-demo manual-change=true
```

### More Orphans

```bash
kubectl create deployment orphan-app --image=nginx -n native-demo
```

## CI Integration

```yaml
# In GitHub Actions
- name: Setup test cluster
  run: ./test/e2e/setup-multi-tool-cluster.sh

- name: Run E2E tests
  run: go test ./cmd/cub-agent/... -tags=e2e -v

- name: Teardown
  if: always()
  run: ./test/e2e/teardown-cluster.sh
```

## Prerequisites

- Docker running
- kind, kubectl, flux, helm installed

```bash
brew install kind kubectl fluxcd/tap/flux helm
```

## Related

- [TESTING-GUIDE.md](../../docs/TESTING-GUIDE.md) - Full testing documentation
- [TUI-E2E-TESTING-PLAN.md](../../docs/planning/TUI-E2E-TESTING-PLAN.md) - Comprehensive plan
