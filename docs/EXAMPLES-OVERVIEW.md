# Examples Overview

Central reference for all cub-scout examples, demos, and integrations.

> **Looking for examples?** All examples live in [examples/](../examples/).
> This document provides an overview and cross-references.

---

## Quick Links

| What You Want | Where It Is |
|---------------|-------------|
| Try it on your cluster | [examples/README.md](../examples/README.md) |
| **Full CLI reference** | [CLI-GUIDE.md](../CLI-GUIDE.md) |
| **Command matrix** | [COMMAND-MATRIX.md](COMMAND-MATRIX.md) |
| **Import workloads** | [map/howto/import-to-confighub.md](map/howto/import-to-confighub.md) |
| Step-by-step walkthrough | [examples/demos/walkthrough.md](../examples/demos/walkthrough.md) |
| Fleet query examples | [howto/fleet-queries.md](howto/fleet-queries.md) |
| UI mockups | [examples/integrations/MOCKUPS.md](../examples/integrations/MOCKUPS.md) |
| Integration scripts | [examples/scripts/](../examples/scripts/) |
| Conference demo | [examples/impressive-demo/](../examples/impressive-demo/) |

---

## Status Legend

| Status | Meaning |
|--------|---------|
| **Working** | Tested code that runs on your cluster |
| **Test Fixtures** | YAML with GitOps labels + placeholder images (nginx:alpine) |
| **Concept Demo** | Simulations/mockups showing future features (NOT functional) |
| **Mockup** | UI designs/mockups for discussion |
| **Proposal** | Architecture proposals, not yet implemented |

---

## Examples by Category

### Concept Demos (Future Features)

> **⚠️ Important:** Concept demos print simulated output or show TUI mockups. They do NOT connect
> to real clusters or ConfigHub. Use [Test Fixtures](#demos-test-fixtures--not-real-apps) or
> [Real-World Examples](#real-world-examples-github) for actual functionality.

| Demo | Type | Status | Shows |
|------|------|--------|-------|
| [rm-demos-argocd/](../examples/rm-demos-argocd/) | **Simulation** | Concept Demo | Rendered Manifest pattern — fleet-wide queries, drift detection, bulk patching |
| [app-config-rtmsg/](../examples/app-config-rtmsg/) | **TUI Mockup** | Concept Demo | Non-K8s config management — DynamoDB/Consul style with Hub/Space model |

**Rendered Manifest demos (rm-demos-argocd/):**
```bash
./examples/rm-demos-argocd/scenarios/monday-panic/demo.sh    # Find problem across 47 clusters
./examples/rm-demos-argocd/scenarios/2am-kubectl/demo.sh     # Catch and fix drift
./examples/rm-demos-argocd/scenarios/security-patch/demo.sh  # Patch 847 services in one command
```

**App Config demo (app-config-rtmsg/):**
```bash
./examples/app-config-rtmsg/demo.sh    # TUI mockup showing Hub, Spaces, Units
```

Key concepts in app-config-rtmsg:
- `hub.yaml` — Config catalog + constraints
- `spaces/*.yaml` — Team + customer self-serve spaces
- `units/**/*.yaml` — Templates, instances, customer overrides

### Demos (Test Fixtures — NOT Real Apps)

> **Important:** Demos are **test fixtures**, not real applications. They create Kubernetes
> resources with GitOps labels (Flux, Argo, Helm) to demonstrate ownership detection,
> but run `nginx:alpine` as a placeholder. For real GitOps apps, see [Real-World Examples](#real-world-examples-github).

| Demo | Status | Time | Description |
|------|--------|------|-------------|
| [demos/](../examples/demos/) | Test Fixtures | 30s-2m | YAML with ownership labels + nginx:alpine |
| [demos/cross-owner-demo](../examples/demos/cross-owner-demo.yaml) | **Working** | 1m | Crossplane, cross-owner refs, elapsed time |
| [demos/walkthrough.md](../examples/demos/walkthrough.md) | Working | 5-10m | Step-by-step demo walkthrough |
| [impressive-demo/](../examples/impressive-demo/) | Test Fixtures | 5m | Conference demo with scan scenarios |

**Quick start:**
```bash
# DEPRECATED: ./test/atk/demo quick            # 30-second demo
# DEPRECATED: ./test/atk/demo healthy          # Enterprise healthy pattern
# DEPRECATED: ./test/atk/demo unhealthy        # Common GitOps problems
# DEPRECATED: ./test/atk/demo connected        # ConfigHub connected mode (requires cub auth)
# DEPRECATED: ./test/atk/demo scenario clobber # Platform updates vs app overlays
```

**Cross-Owner Reference Demo (NEW in v0.3.3):**
```bash
# Visual demo (no cluster required)
./examples/demos/cross-owner-demo.sh

# Real cluster demo
kubectl apply -f examples/demos/cross-owner-demo.yaml
./cub-scout trace deploy/api-server -n ecommerce
```

Shows:
- **Crossplane detection** — Cloud infrastructure ownership (RDS, ElastiCache claims)
- **Cross-owner warnings** — Flux deployment → Terraform secret references
- **Elapsed time** — Time since last reconciliation with stuck resource highlighting

Use case: Platform teams using Crossplane/Terraform for infrastructure while app teams use Flux/ArgoCD for workloads.

**To convert demos to real apps:** See [examples/README.md — Converting Demos to Real Apps](../examples/README.md#converting-demos-to-real-apps)

### Fleet Queries

Real examples answering real questions. See [howto/fleet-queries.md](howto/fleet-queries.md).

**Interactive demo:**
```bash
./examples/demos/fleet-queries-demo.sh    # Shows query syntax and examples
# DEPRECATED: ./test/atk/demo query                     # Live queries against cluster
```

| Question | Command |
|----------|---------|
| What's running? | `cub-scout map` |
| Who owns each workload? | `cub-scout map workloads` |
| What's crashing? | `cub-scout map crashes` |
| What has issues? | `cub-scout map issues` |
| What's drifted? | `cub-scout map drift` |
| What's orphaned? | `cub-scout map orphans` |
| Which clusters are behind? | `cub-scout map fleet` (Connected mode) |
| What config bugs exist? | `cub-scout scan` |

### Integration Scripts

Copy-paste scripts for common use cases. See [examples/scripts/](../examples/scripts/).

| Script | Use Case |
|--------|----------|
| k9s-plugin.yaml | Add map/scan to k9s |
| slack-alerting.sh | Alert on drift/configuration issues |
| github-workflow.yaml | CI/CD gate for configuration issues |
| prometheus-metrics.sh | Export metrics |

### Third-Party Integrations

| Integration | Status | Description |
|-------------|--------|-------------|
| [argocd-extension/](../examples/integrations/argocd-extension/) | Working | Scan tab in Argo CD UI |
| [flux-operator/](../examples/integrations/flux-operator/) | Working | Metrics exporter |
| [flux9s/](../examples/integrations/flux9s/) | Proposal | K9s-style TUI for Flux |

UI mockups for these integrations: [examples/integrations/MOCKUPS.md](../examples/integrations/MOCKUPS.md)

### TUI Showcase Demos

Multi-service demos designed to showcase all 17 TUI views:

| Demo | Services | Status | Best For |
|------|----------|--------|----------|
| [flux-boutique/](../examples/flux-boutique/) | 5 | **Working** | TUI view showcase, trace demo |

**Quick start:**
```bash
kubectl apply -f examples/flux-boutique/boutique.yaml
kubectl wait --for=condition=available deployment --all -n boutique --timeout=120s
cub-scout map   # Press 's' status, 'w' workloads, 'p' pipelines, 'o' orphans, 'T' trace, '?' help
```

#### External Multi-Service Demos

| Demo | Services | Complexity | Notes |
|------|----------|------------|-------|
| [fluxcd-community/microservices-demo](https://github.com/fluxcd-community/microservices-demo) | 20 | Medium | Full Flux showcase, requires newer K8s |
| [GoogleCloudPlatform/microservices-demo](https://github.com/GoogleCloudPlatform/microservices-demo) | 11 | High | Real e-commerce app (Online Boutique) |
| [fluxcd/flux2-kustomize-helm-example](https://github.com/fluxcd/flux2-kustomize-helm-example) | Multi-env | Medium | Official Flux multi-environment reference |

### Real-World Examples

#### Apptique Examples (In This Repo)

Multiple GitOps patterns using Google's Online Boutique app:

| Pattern | Tool | Shows |
|---------|------|-------|
| [apptique-examples/flux-monorepo](../examples/apptique-examples/flux-monorepo/) | **Flux** | Monorepo with Kustomize overlays |
| [apptique-examples/argo-applicationset](../examples/apptique-examples/argo-applicationset/) | **Argo CD** | ApplicationSet with directory generator |
| [apptique-examples/argo-app-of-apps](../examples/apptique-examples/argo-app-of-apps/) | **Argo CD** | Parent Application managing children |

See [apptique-examples/README.md](../examples/apptique-examples/README.md) for deployment instructions.

#### External Examples (GitHub)

| Example | Repo | Shows |
|---------|------|-------|
| **ArgoCD Setup** | [confighubai/examples-internal/argocd](https://github.com/confighubai/examples-internal/tree/main/argocd) | Argo CD Applications, failing deployers |
| **FluxCD Setup** | [confighubai/examples-internal/fluxcd](https://github.com/confighubai/examples-internal/tree/main/fluxcd) | Flux Kustomizations, HelmReleases |
| **Global App** | [confighub/examples/global-app](https://github.com/confighub/examples/tree/main/global-app) | ConfigHub multi-region deployment |
| **Helm Platform** | [confighub/examples/helm-platform-components](https://github.com/confighub/examples/tree/main/helm-platform-components) | Helm ownership detection |
| **VM Fleet** | [confighub/examples/vm-fleet](https://github.com/confighub/examples/tree/main/vm-fleet) | Non-Kubernetes fleet management |
| **Flux Bridge** | [confighubai/flux-bridge](https://github.com/confighubai/flux-bridge) | Flux-to-ConfigHub integration |

**Try an example:**
```bash
# Deploy an example to your cluster and see the map output
# DEPRECATED: ./test/atk/examples --capture jesper_argocd

# Or test that all examples are accessible
# DEPRECATED: ./test/atk/examples

# Filter by type
# DEPRECATED: ./test/atk/examples jesper    # Internal examples
# DEPRECATED: ./test/atk/examples public    # Public confighub/examples
```

Expected output for each example is in `test/fixtures/expected-output/examples/`.

---

## Map Subcommands (17)

The map command supports multiple subcommands:

```bash
# Interactive TUI
cub-scout map              # Full dashboard (interactive TUI)
cub-scout map --hub        # ConfigHub hierarchy TUI (Connected mode)

# CLI Output (Standalone mode)
cub-scout map list         # Plain text resource list
cub-scout map status       # One-line health check
cub-scout map workloads    # List workloads by owner
cub-scout map deployers    # List GitOps deployers
cub-scout map orphans      # Unmanaged (Native) resources
cub-scout map crashes      # Failing pods/deployments
cub-scout map issues       # Resources with problems
cub-scout map drift        # Desired vs actual state
cub-scout map bypass       # Factory bypass detection
cub-scout map sprawl       # Configuration sprawl
cub-scout map deep-dive    # ALL cluster data with LiveTree
cub-scout map app-hierarchy # Inferred ConfigHub Units
cub-scout map dashboard    # Unified health dashboard
cub-scout map queries      # Saved queries management

# Connected mode
cub-scout map fleet        # Multi-cluster fleet view
cub-scout map hub          # ConfigHub hierarchy

# Output formats
cub-scout map --json       # JSON output for tooling
```

### TUI Views (17)

Press these keys in the interactive TUI to switch views:

| Key | View | Description |
|-----|------|-------------|
| `s` | Status | Dashboard overview |
| `w` | Workloads | Workloads by owner |
| `a` | Apps | Grouped by app label + variant |
| `p` | Pipelines | GitOps deployers (Flux, ArgoCD) |
| `d` | Drift | Resources diverged from desired state |
| `o` | Orphans | Native resources (not GitOps-managed) |
| `c` | Crashes | Failing pods |
| `i` | Issues | Unhealthy resources |
| `u` | sUspended | Paused/forgotten resources |
| `b` | Bypass | Factory bypass detection |
| `x` | Sprawl | Config sprawl analysis |
| `D` | Dependencies | Upstream/downstream relationships |
| `G` | Git sources | Forward trace from Git |
| `4` | Cluster Data | All data sources TUI reads |
| `5`/`A` | App Hierarchy | Inferred ConfigHub model |
| `M` | Maps | Three Maps view |
| `H` | Hub | ConfigHub hierarchy (Connected mode) |

### Command Palette (`:`)

Press `:` in the TUI to run shell commands:

```
:kubectl get pods
:cub-scout scan
:flux get kustomizations
```

- `↑`/`↓` — Navigate command history (last 20 commands)
- `Enter` — Execute command
- `Esc` — Cancel

---

## JSON Output

All commands support `--json` for tooling:

```bash
cub-scout map --json | jq '.workloads[] | select(.owner == "ConfigHub")'
cub-scout scan --json | jq '.findings[] | select(.severity == "critical")'
```

---

## Maintainer Notes

When updating examples, ensure you update this overview if:
- Adding new demos or scripts
- Changing example status
- Adding new integration types
- Modifying the examples directory structure

Each examples file should include this maintainer note:
```markdown
> **Maintainer note:** When updating this file, also update [docs/EXAMPLES-OVERVIEW.md](../../docs/EXAMPLES-OVERVIEW.md).
```

---

## See Also

| Doc | Content |
|-----|---------|
| [README.md](../README.md) | Project overview |
| [CLI-GUIDE.md](../CLI-GUIDE.md) | Complete CLI reference |
| [COMMAND-MATRIX.md](COMMAND-MATRIX.md) | Full command/option matrix |
| [ARCHITECTURE.md](ARCHITECTURE.md) | How it works, GSF protocol |
| [SCAN-GUIDE.md](SCAN-GUIDE.md) | CCVE scanning deep dive |
