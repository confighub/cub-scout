# D2 Control Plane Pattern

## The Problem

You've adopted the Flux reference architecture: `clusters/`, `infrastructure/`, and `apps/`
in separate directories (or repos). Your cluster has platform components (cert-manager, monitoring)
managed by the platform team, and tenant apps (payment-api, frontend) managed by dev teams.

During an incident you need to answer: *"Is the problem in infrastructure or in an app?
Who owns the broken resource?"*

With `kubectl` alone, a Deployment is just a Deployment. You can't tell whether it's platform
infrastructure or a tenant app without checking the Git repo.

**cub-scout detects the pattern and shows the layers:**

```
$ ./cub-scout tree patterns

  GITOPS PATTERNS DETECTED
  ════════════════════════════════════════════════════
  Primary Pattern: "Control Plane" (D2-style)

  Characteristics detected:
  ✓ clusters/ directory structure
  ✓ infrastructure/ for platform components
  ✓ apps/ for tenant applications
  ✓ Multi-environment overlays (prod, staging)
```

```
$ ./cub-scout tree ownership

  Flux (12 resources)
  ├── Infrastructure
  │   ├── Kustomization/cert-manager → cert-manager/cert-manager
  │   └── HelmRelease/monitoring → monitoring/prometheus, monitoring/grafana
  └── Applications
      ├── Kustomization/payment-api → payments/payment-api
      └── Kustomization/frontend → store/frontend
```

## Architecture

The D2 pattern (named after the Flux CD "Control Plane" reference architecture) separates
concerns into three layers:

```
platform-config/
├── clusters/                    # Per-cluster bootstrap
│   ├── prod/
│   │   ├── flux-system/        # Flux bootstrap
│   │   ├── infrastructure/     # Platform components for prod
│   │   └── apps/               # Tenant apps for prod
│   └── staging/
│       ├── flux-system/
│       ├── infrastructure/
│       └── apps/
├── infrastructure/              # Shared platform components
│   ├── cert-manager/
│   │   ├── namespace.yaml
│   │   └── helmrelease.yaml
│   └── monitoring/
│       ├── namespace.yaml
│       └── helmrelease.yaml
└── apps/                        # Tenant applications
    ├── payment-api/
    │   ├── base/
    │   └── overlays/
    │       ├── staging/
    │       └── prod/
    └── frontend/
        ├── base/
        └── overlays/
            ├── staging/
            └── prod/
```

**Layer separation:**
| Layer | Owner | Contains | Example |
|-------|-------|----------|---------|
| `clusters/` | Platform team | Bootstrap + per-cluster Kustomizations | `clusters/prod/infrastructure/` |
| `infrastructure/` | Platform team | Shared add-ons (cert-manager, monitoring) | HelmReleases, namespaces |
| `apps/` | Dev teams | Tenant applications | Deployments, Services |

## What It Demonstrates

| What you'll see | Why it matters |
|-----------------|----------------|
| Pattern auto-detection ("D2-style") | cub-scout recognizes the Flux reference architecture |
| Infrastructure vs app separation in ownership tree | Answers "is it platform or app?" instantly |
| Flux Kustomization chains | GitRepository → Kustomization → HelmRelease → Deployment |
| Mixed deployer types (Kustomization + HelmRelease) | Real clusters use both |

## Quick Start

```bash
# Apply the fixture (Flux must be installed)
kubectl apply -f examples/d2-control-plane/control-plane.yaml

# Wait for reconciliation
flux get kustomizations --watch

# Detect the pattern
./cub-scout tree patterns

# See ownership by layer
./cub-scout tree ownership

# Trace an infrastructure component
./cub-scout trace deployment/prometheus -n monitoring

# Trace a tenant app
./cub-scout trace deployment/payment-api -n payments

# See everything
./cub-scout map list
```

## Offline Use

```bash
# Scan the fixture without a cluster
./cub-scout scan --file examples/d2-control-plane/control-plane.yaml

# Parse the repo structure
./cub-scout parse-repo --path examples/d2-control-plane
```

## Pattern Variants

The D2 pattern has several real-world variants:

| Variant | Description | Example |
|---------|-------------|---------|
| **Single-repo D2** | All layers in one repo | This example |
| **Multi-repo D2** (Fluxy) | fleet-repo + infra-repo + apps-repo | See [Multi-Repo Fleet](../rm-demos-argocd/repo-patterns/multi-repo/) |
| **D2 + OCI** | Git → CI → OCI artifact → Flux | Enterprise pattern |

## See Also

- [Platform Example](../platform-example/) — Full D2 learning environment with orphans
- [Flux Boutique](../flux-boutique/) — Simpler Flux-only example
- [Flux Monorepo](../apptique-examples/flux-monorepo/) — Monorepo variant with overlays
- [GitOps Repo Structures](../../docs/reference/gitops-repo-structures.md) — All patterns documented
- [Tree Hierarchies](../../docs/howto/tree-hierarchies.md) — How `tree patterns` works
