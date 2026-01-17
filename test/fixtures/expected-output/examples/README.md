# Example Output Fixtures

This directory contains expected map output for standard ConfigHub examples.

---

## Quick Reference: What You Should See

### jesper_argocd (Argo CD Example)

**Scenario:** Multi-namespace Argo CD deployment with intentional failures for demo purposes.

```
[s]tatus
┌────────────────────────────────────────────────────────────────────────┐
│  ██████████████████████░░░░░░░░  76%   13/17 workloads healthy         │
│                                                                        │
│  ✗ Deployers  0/5      ✗ Sources  0/1      ⚠ Workloads  13/17          │
└────────────────────────────────────────────────────────────────────────┘
```

**Key observations:**
- 76% healthy (13/17 workloads) — intentional failures for demo
- Deployers failing (0/5) — shows unhealthy Argo apps
- 4 drifted resources — shows drift detection working
- 29% GitOps coverage — 12 Native (orphan) resources for demo

**Namespaces created:**
- `example-jesper-argocd` — Argo CD resources
- `demo-payments` — Payment service (with failures)
- `demo-orders` — Order service (with failures)
- `confighub` — Flux bridge (with failure)

### jesper_fluxcd (Flux CD Example)

**Scenario:** Multi-namespace Flux CD deployment with Kustomizations and HelmReleases.

**Key observations:**
- Flux sources and Kustomizations visible
- HelmRelease pipeline shown
- Ownership detection shows Flux-managed resources

### Connected Mode (jesper_argocd-connected.txt)

When connected to ConfigHub, you see additional context:

```
┌─ CONFIGHUB ────────────────────────────────────────────────────────────┐
│  Org: ConfigHub                                                        │
│  └─ Hub: alexis@confighub.com                                          │
│     └─ AppSpace: example-jesper-argocd-team                            │
│        └─ Unit: example-jesper-argocd ✓                                │
│           ├─ Status: Ready                                             │
│           └─ Target: dev-kubernetes-yaml-kind-atk                      │
└────────────────────────────────────────────────────────────────────────┘
```

---

## How These Are Generated

```bash
./test/atk/examples --capture              # Capture all examples
./test/atk/examples --capture jesper_argocd   # Capture specific example
```

The capture process:
1. Clones the example repo from GitHub
2. Deploys to current Kubernetes cluster
3. Runs `./test/atk/map` and captures output
4. Saves to `{example_name}.txt`
5. Cleans up deployed resources

---

## Available Examples

| Example | GitHub Repo | What It Shows |
|---------|-------------|---------------|
| **jesper_argocd** | confighubai/examples-internal/argocd | Argo CD with intentional failures |
| **jesper_fluxcd** | confighubai/examples-internal/fluxcd | Flux CD with Kustomizations |
| **global_app** | confighub/examples/global-app | Multi-cluster global app |
| **helm_platform** | confighub/examples/helm-platform-components | Helm platform components |
| **vm_fleet** | confighub/examples/vm-fleet | VM fleet management |

---

## Interpreting the Output

### Status Section

| Indicator | Meaning |
|-----------|---------|
| `██████████████████░░░░░` 80% | Health bar — percentage of healthy workloads |
| `✓ Deployers 5/5` | All deployers (Flux/Argo sources) healthy |
| `✗ Deployers 0/5` | Deployer issues detected |
| `⚠ Workloads 13/17` | Some workloads not ready |

### Problems Section

Shows resources with issues:
- `✗ namespace/name 0/N pods` — Deployment with no ready pods
- Pod crash reasons when available

### Pipelines Section

Shows GitOps pipelines:
```
✓ repo@branch ──▶ app-name ──▶ N resources   # Healthy
✗ repo@branch ──▶ app-name ──▶ N resources   # Unhealthy
⏸ repo@branch ──▶ app-name ──▶ N resources   # Suspended
```

### Workloads Section

Shows ownership breakdown:
```
Flux     ████████ 8      # Flux-managed
ArgoCD   █████    5      # Argo-managed
Helm     ████     4      # Helm-managed
Native   ██████   12     # No GitOps owner (orphans)

GitOps Coverage   ███░░░░░░░  29%   # Percentage managed by GitOps
```

### Drift Section

```
✓ 0 synced    # In sync with Git
⚠ 4 drifted   # Live differs from Git
```

---

## Regenerating Fixtures

Fixtures may need regeneration when:
- Example repos are updated
- Map output format changes
- New ownership detection is added

```bash
# Regenerate all
./test/atk/examples --capture

# Regenerate one
./test/atk/examples --capture jesper_argocd
```

---

## Notes

- **Jesper examples** require access to `confighubai/examples-internal` (private)
- **Public examples** work with any GitHub account
- Fixtures are cluster-specific (your results may vary slightly)
- Connected mode fixtures require ConfigHub authentication
