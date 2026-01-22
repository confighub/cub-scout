# Running Demos

How to run cub-scout demos to explore features.

## Prerequisites

```bash
# Build cub-scout
go build ./cmd/cub-scout

# Ensure kubectl access
kubectl cluster-info
```

---

## Quick Demo Commands

```bash
# Ownership detection
cub-scout map                     # Interactive TUI
cub-scout map list                # List all resources with owners

# Find orphans (shadow IT)
cub-scout map list -q "owner=Native"

# CCVE scanning
cub-scout scan

# Trace ownership
cub-scout trace deploy/nginx -n default

# Query resources
cub-scout map list -q "owner=Flux"
cub-scout map list -q "namespace=prod*"
```

---

## Demo Scenarios

### Standalone (No ConfigHub Required)

| Demo | Duration | What It Shows |
|------|----------|---------------|
| **Ownership Detection** | ~30 sec | Resources grouped by owner (Flux, ArgoCD, Native) |
| **CCVE Scanning** | ~2 min | CCVE-2025-0027: Grafana whitespace bug |
| **Query Language** | ~1 min | Filter with `owner!=Native`, `namespace=prod*` |
| **Orphan Hunt** | ~2 min | Find mystery Native resources |

### Connected Mode (Requires ConfigHub)

```bash
# First, authenticate
cub auth login
cub context get

# Run ConfigHub TUI
cub-scout map --hub
```

---

## Demo 1: Ownership Detection

**Goal:** See how cub-scout identifies who manages each resource.

```bash
# Deploy demo fixtures
kubectl apply -f examples/demos/ownership-demo/

# Run TUI
cub-scout map

# What to look for:
# - Resources grouped by owner (Flux, ArgoCD, Helm, Native)
# - Native resources highlighted as orphans
# - Color-coded status indicators
```

**TUI Navigation:**
- `j/k` - Move up/down
- `Tab` - Switch views
- `?` - Help
- `q` - Quit

---

## Demo 2: Find Orphans

**Goal:** Find unmanaged resources (shadow IT).

```bash
# List all Native (unmanaged) resources
cub-scout map list -q "owner=Native"

# In TUI, press '3' to see Problems view
cub-scout map
```

**What orphans indicate:**
- kubectl apply without GitOps
- Leftover resources from deleted deployments
- Manual debugging pods

---

## Demo 3: CCVE Scanning

**Goal:** Detect misconfigurations like CCVE-2025-0027 (Grafana sidecar whitespace).

```bash
# Scan current cluster
cub-scout scan

# Scan specific namespace
cub-scout scan -n production

# JSON output for CI/CD
cub-scout scan --json
```

**Common detections:**
- Trailing whitespace in annotations
- Missing resource limits
- Privileged containers
- Drift indicators

---

## Demo 4: Trace Ownership

**Goal:** Trace the full ownership chain for a resource.

```bash
# Trace a deployment
cub-scout trace deploy/frontend -n myapp

# Trace any resource
cub-scout trace service/api -n production
```

**What you'll see:**
```
GitRepository → Kustomization → Deployment → ReplicaSet → Pod
```

---

## Demo 5: Query Resources

**Goal:** Filter resources using the query language.

```bash
# By owner
cub-scout map list -q "owner=Flux"
cub-scout map list -q "owner=ArgoCD"
cub-scout map list -q "owner!=Native"

# By namespace
cub-scout map list -q "namespace=prod*"

# Combined
cub-scout map list -q "owner=Flux AND namespace=prod*"

# By labels
cub-scout map list -q "labels[team]=payments"
```

See [Query Library Reference](../reference/query-library.md) for all query options.

---

## Demo 6: ConfigHub Hierarchy

**Goal:** Navigate the ConfigHub hierarchy (Org → Space → Unit).

```bash
# Authenticate
cub auth login

# Start worker (in separate terminal)
cub worker run

# Run hierarchy TUI
cub-scout map --hub
```

**TUI Navigation:**
- `Enter` - Drill down into item
- `Backspace` - Go back up
- `H` - Toggle Hub view

---

## Example Fixtures

Pre-built fixtures for demos are in `examples/demos/`:

| Fixture | Description |
|---------|-------------|
| `ownership-demo/` | Flux + ArgoCD + Native resources |
| `ccve-demo/` | Misconfigured resources for scanning |
| `drift-demo/` | Resources with drift |

```bash
# Apply a fixture
kubectl apply -f examples/demos/ownership-demo/

# Clean up
kubectl delete -f examples/demos/ownership-demo/
```

---

## Working Examples

Beyond demos, see working reference architectures:

| Example | Directory | Pattern |
|---------|-----------|---------|
| **Flux Monorepo** | `examples/apptique-examples/source/` | Kustomize + HelmRelease |
| **ArgoCD ApplicationSet** | `examples/apptique-examples/argo-applicationset/` | Directory generator |
| **ArgoCD App-of-Apps** | `examples/apptique-examples/argo-app-of-apps/` | Parent→children |

---

## Troubleshooting

### Demo fails to start

```bash
# Check kubectl access
kubectl cluster-info

# Check cub-scout built
./cub-scout version
```

### Clean up demo resources

```bash
# Delete demo namespaces
kubectl delete namespace demo-flux demo-argo demo-native 2>/dev/null || true
```

### Connected demo fails

```bash
# Check authentication
cub context get

# Check worker
cub worker list
```

---

## See Also

- [Install Guide](../getting-started/install.md) — First-time setup
- [First Map](../getting-started/first-map.md) — Your first map command
- [Query Library](../reference/query-library.md) — Query syntax
- [Commands Reference](../reference/commands.md) — All CLI commands
