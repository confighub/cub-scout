# View Hierarchies with tree

The `cub-scout tree` command shows different hierarchical perspectives on your infrastructure.

## Quick Start

```bash
# Default: Runtime hierarchy (Deployment → ReplicaSet → Pod)
cub-scout tree

# Resources grouped by owner
cub-scout tree ownership

# Suggested Hub/AppSpace organization
cub-scout tree suggest
```

## Available Views

### Runtime Hierarchy (default)

Shows the Kubernetes runtime tree: Deployment → ReplicaSet → Pod.

```bash
cub-scout tree
cub-scout tree runtime
```

Output:
```
Runtime Hierarchy (51 Deployments)
────────────────────────────────────────────────────────────
├── boutique/cart [Flux] 2/2 ready
│   └── ReplicaSet cart-86f68db776 [2/2]
│       ├── Pod cart-86f68db776-hzqgf ✓ Running
│       └── Pod cart-86f68db776-mp8kz ✓ Running
├── cert-manager/cert-manager [ArgoCD] 2/2 ready
│   └── ReplicaSet cert-manager-8bdb658c7 [2/2]
│       └── Pod cert-manager-8bdb658c7-cx7cn ✓ Running
```

### Ownership Hierarchy

Groups resources by GitOps owner (Flux, ArgoCD, Helm, ConfigHub, Native).

```bash
cub-scout tree ownership
```

Output:
```
Ownership Hierarchy
────────────────────────────────────────────────────────────
Flux (28)
  ├── boutique/cart
  ├── boutique/checkout
  └── ...

ArgoCD (12)
  ├── cert-manager/cert-manager
  └── ...

Native (7)
  └── temp-test/debug-nginx
```

### Git Source Hierarchy

Shows Git repository structure from Flux GitRepositories and ArgoCD Applications.

```bash
cub-scout tree git
```

### Patterns Hierarchy

Detects named GitOps patterns (D2, Arnie, Banko, Fluxy).

```bash
cub-scout tree patterns
```

### ConfigHub Hierarchy

Wraps `cub unit tree` to show Unit relationships.

```bash
# Clone relationships (configuration inheritance)
cub-scout tree config --space my-space

# Link relationships (dependencies)
cub-scout tree config --space my-space --edge link

# Across all spaces
cub-scout tree config --space "*"
```

### Suggested Organization

Analyzes cluster workloads and suggests Hub/AppSpace organization.

```bash
cub-scout tree suggest
```

Output:
```
Hub/AppSpace Suggestion
────────────────────────────────────────────────────────────

Suggested structure (Hub/App Space model):
  App Space: payment-team

    ├── app=payment-api
    │   ├── Unit: payment-api (variant=default, 1 workload(s))
    │   └── Unit: payment-api-prod (variant=prod, 1 workload(s))
    └── Unit: order-processor (app=order-processor, 1 workload(s))

Next steps:
  1. Review the suggested structure above
  2. Import workloads: cub-scout import -n <namespace>
  3. View in ConfigHub: cub unit tree --space <space>
```

## Relationship to cub unit tree

These commands are complementary:

| Command | Perspective | Shows |
|---------|-------------|-------|
| `cub-scout tree` | Cluster | What's deployed in THIS cluster |
| `cub unit tree` | ConfigHub | How Units relate ACROSS your fleet |

Use `cub-scout tree` to understand your cluster, then `cub unit tree` to see cross-cluster relationships after importing to ConfigHub.

## Options

| Option | Description |
|--------|-------------|
| `-n, --namespace` | Filter by namespace |
| `-A, --all` | Include system namespaces |
| `--space` | ConfigHub space (for config view) |
| `--edge` | clone (inheritance) or link (dependencies) |
| `--json` | JSON output |

## See Also

- [Fleet Queries](fleet-queries.md) - Multi-cluster queries with ConfigHub
- [Import to ConfigHub](import-to-confighub.md) - Import workloads
