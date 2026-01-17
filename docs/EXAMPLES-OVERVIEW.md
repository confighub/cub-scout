# Examples Overview

Central reference for all ConfigHub Agent examples, demos, and integrations.

> **Looking for examples?** All examples live in [examples/](../examples/).
> This document provides an overview and cross-references.

---

## Quick Links

| What You Want | Where It Is |
|---------------|-------------|
| Try it on your cluster | [examples/README.md](../examples/README.md) |
| **Import workloads** | [docs/IMPORTING-WORKLOADS.md](IMPORTING-WORKLOADS.md) |
| Step-by-step walkthrough | [examples/demos/walkthrough.md](../examples/demos/walkthrough.md) |
| Fleet query examples | [JOURNEY-QUERY.md](JOURNEY-QUERY.md) |
| **Expected output reference** | [docs/CLI-EXPECTED-OUTPUT.md](CLI-EXPECTED-OUTPUT.md) |
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

**To convert demos to real apps:** See [examples/README.md — Converting Demos to Real Apps](../examples/README.md#converting-demos-to-real-apps)

### Fleet Queries

Real examples answering real questions. See [JOURNEY-QUERY.md](JOURNEY-QUERY.md).

**Interactive demo:**
```bash
./examples/demos/fleet-queries-demo.sh    # Shows query syntax and examples
# DEPRECATED: ./test/atk/demo query                     # Live queries against cluster
```

| Question | Command |
|----------|---------|
| What's running? | `cub-scout map` |
| Who owns each workload? | `cub-scout map workloads` |
| What's broken? | `cub-scout map problems` |
| Which clusters are behind? | `cub-scout map` (with ConfigHub auth) |
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

Multi-service demos designed to showcase all TUI views (`s`, `w`, `p`, `T`, `G`, etc.):

| Demo | Services | Status | Best For |
|------|----------|--------|----------|
| [flux-boutique/](../examples/flux-boutique/) | 5 | **Working** | TUI view showcase, trace demo |

**Quick start:**
```bash
kubectl apply -f examples/flux-boutique/boutique.yaml
kubectl wait --for=condition=available deployment --all -n boutique --timeout=120s
cub-scout map   # Press 's' for status, 'w' for workloads, 'p' for pipelines, 'T' to trace
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

## Dashboard Modes

The map command supports multiple output modes:

```bash
cub-scout map              # Full dashboard (interactive TUI)
cub-scout map status       # One-line health check
cub-scout map workloads    # List workloads by owner
cub-scout map problems     # Show only problems
cub-scout map deployers    # List GitOps deployers
cub-scout map suspended    # List suspended resources
cub-scout map confighub    # ConfigHub hierarchy (requires auth)
cub-scout map deep-dive    # ALL cluster data with LiveTree
cub-scout map app-hierarchy # Inferred ConfigHub Units
cub-scout map --json       # JSON output for tooling
```

### TUI Tab Views (Interactive Mode)

| Key | Tab | Description |
|-----|-----|-------------|
| `1` | Dashboard | Health overview, problems summary |
| `2` | Workloads | Deployments by owner |
| `3` | Deployers | GitOps controllers |
| `4` | Cluster Data | All data sources (Flux, Argo, Helm) |
| `5/A` | App Hierarchy | Inferred ConfigHub model |
| `H` | Hub | ConfigHub hierarchy (connected mode) |

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
| [ARCHITECTURE.md](ARCHITECTURE.md) | How it works, GSF protocol |
| [CCVE-GUIDE.md](CCVE-GUIDE.md) | Config CVE scanning |
| [CLI-REFERENCE.md](CLI-REFERENCE.md) | Full CLI reference |
