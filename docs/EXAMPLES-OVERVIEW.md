# Examples Overview

Central reference for all cub-scout examples, demos, and integrations.

> **Looking for examples?** All examples live in [examples/](../examples/).

---

## Quick Links

| What You Want | Where It Is |
|---------------|-------------|
| Run a demo | `./cub-scout demo list` |
| Full CLI reference | [CLI-GUIDE.md](../CLI-GUIDE.md) |
| Try on your cluster | [examples/README.md](../examples/README.md) |
| Fleet query examples | [howto/fleet-queries.md](howto/fleet-queries.md) |
| Conference demo | [examples/impressive-demo/](../examples/impressive-demo/) |

---

## Demos (Built-in)

cub-scout includes built-in demos. Run `./cub-scout demo list` to see all available.

```bash
# Quick demos
./cub-scout demo quick              # Ownership detection (~30 sec)
./cub-scout demo ccve               # CCVE-2025-0027 (~2 min)
./cub-scout demo healthy            # Enterprise healthy pattern
./cub-scout demo unhealthy          # Common GitOps problems

# Scenarios
./cub-scout demo scenario bigbank   # BIGBANK incident
./cub-scout demo scenario orphan    # Find shadow IT

# Cleanup
./cub-scout demo quick --cleanup
```

See [demos/README.md](demos/README.md) for detailed walkthroughs.

---

## Examples by Category

### TUI Showcase Demos

Multi-service demos designed to showcase TUI views:

| Demo | Services | Status | Best For |
|------|----------|--------|----------|
| [flux-boutique/](../examples/flux-boutique/) | 5 | Working | TUI view showcase, trace demo |
| [platform-example/](../examples/platform-example/) | ~35 | Working | Full GitOps learning environment |
| [orphans/](../examples/orphans/) | ~20 | Working | Orphan detection demo |

**Platform Example (Recommended for learning):**
```bash
cd examples/platform-example
./setup.sh                              # Deploy Flux + orphans to kind cluster
./cub-scout map                         # Explore with TUI
./cub-scout trace deploy/podinfo -n podinfo  # Trace to Git source
./cleanup.sh                            # Remove everything
```

### Drift Detection Examples

| Example | Shows |
|---------|-------|
| [drift/env-var-drift/](../examples/drift/env-var-drift/) | Environment variable drift |
| [drift/resource-drift/](../examples/drift/resource-drift/) | Resource requests/limits drift |
| [drift/image-policy-drift/](../examples/drift/image-policy-drift/) | Image pull policy drift |

### Crossplane Examples

| Example | Shows |
|---------|-------|
| [crossplane-system/](../examples/crossplane-system/) | Crossplane ownership detection |

### Integration Examples

| Integration | Status | Description |
|-------------|--------|-------------|
| [argocd-extension/](../examples/integrations/argocd-extension/) | Working | Scan tab in Argo CD UI |
| [flux-operator/](../examples/integrations/flux-operator/) | Working | Metrics exporter |
| [flux9s/](../examples/integrations/flux9s/) | Proposal | K9s-style TUI for Flux |
| [confighub-oci/](../examples/integrations/confighub-oci/) | Working | ConfigHub OCI integration |

### Real-World Examples

#### Apptique (Google Online Boutique)

Multiple GitOps patterns using the Online Boutique app:

| Pattern | Directory | Shows |
|---------|-----------|-------|
| Flux Monorepo | [apptique-examples/](../examples/apptique-examples/) | Kustomize + HelmRelease |
| Drift Detection | [apptique-examples/scenarios/drift-detection/](../examples/apptique-examples/scenarios/drift-detection/) | Drift scenarios |

#### ArgoCD Patterns (rm-demos-argocd)

Repository patterns for ArgoCD:

| Pattern | Directory |
|---------|-----------|
| Monorepo | [rm-demos-argocd/repo-patterns/monorepo/](../examples/rm-demos-argocd/repo-patterns/monorepo/) |
| Multi-repo | [rm-demos-argocd/repo-patterns/multi-repo/](../examples/rm-demos-argocd/repo-patterns/multi-repo/) |
| ApplicationSets | [rm-demos-argocd/repo-patterns/applicationsets/](../examples/rm-demos-argocd/repo-patterns/applicationsets/) |
| Helm Umbrella | [rm-demos-argocd/repo-patterns/helm-umbrella/](../examples/rm-demos-argocd/repo-patterns/helm-umbrella/) |

---

## Map Subcommands

The map command supports multiple subcommands:

```bash
# Interactive TUI
./cub-scout map              # Full dashboard (interactive TUI)
./cub-scout map --hub        # ConfigHub hierarchy TUI (Connected mode)

# CLI Output
./cub-scout map list         # Plain text resource list
./cub-scout map status       # One-line health check
./cub-scout map workloads    # List workloads by owner
./cub-scout map orphans      # Unmanaged (Native) resources
./cub-scout map drift        # Desired vs actual state
```

### TUI Views

Press these keys in the interactive TUI to switch views:

| Key | View | Description |
|-----|------|-------------|
| `s` | Status | Dashboard overview |
| `w` | Workloads | Workloads by owner |
| `p` | Pipelines | Deployers |
| `d` | Drift | Resources diverged from desired state |
| `o` | Orphans | Native resources (not GitOps-managed) |
| `c` | Crashes | Failing pods |
| `i` | Issues | Unhealthy resources |
| `T` | Trace | GitOps trace |
| `?` | Help | Keybindings |

---

## JSON Output

All commands support `--json` for tooling:

```bash
./cub-scout map list --json | jq '.[] | select(.owner == "Flux")'
./cub-scout scan --json | jq '.findings[] | select(.severity == "critical")'
```

---

## See Also

| Doc | Content |
|-----|---------|
| [README.md](../README.md) | Project overview |
| [CLI-GUIDE.md](../CLI-GUIDE.md) | Complete CLI reference |
| [demos/README.md](demos/README.md) | Demo walkthroughs |
| [FAQ.md](FAQ.md) | Frequently asked questions |
