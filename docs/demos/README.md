# Demo Suite

> **Note:** The bash demo scripts (`# DEPRECATED: ./test/atk/demo`) are deprecated. Use the Go CLI instead:

## Quick Reference (Go CLI)

```bash
# Build first
go build ./cmd/cub-scout

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

## Legacy Bash Demos (Deprecated)

The following demos used bash scripts and are preserved for reference. Use the Go CLI commands above instead.

```bash
# These commands are DEPRECATED:
# DEPRECATED: ./test/atk/demo quick           # Use: cub-scout map list
# DEPRECATED: ./test/atk/demo ccve            # Use: cub-scout scan
# DEPRECATED: ./test/atk/demo healthy         # Use: cub-scout map
# DEPRECATED: ./test/atk/demo unhealthy       # Use: cub-scout map list -q "status=..."
```

---

## Demo Inventory

### Quick Demos (Standalone)

These work without ConfigHub connection.

| Demo | Duration | What It Shows |
|------|----------|---------------|
| **quick** | ~30 sec | Fastest path to "wow" — ownership detection |
| **ccve** | ~2 min | CCVE-2025-0027: The BIGBANK Grafana bug |
| **query** | ~1 min | Query language: `owner!=Native`, `namespace=prod*` |
| **healthy** | ~2 min | Enterprise healthy: IITS hub-and-spoke pattern |
| **unhealthy** | ~2 min | Enterprise problems: suspended resources, orphans |

### Narrative Scenarios

Story-driven walkthroughs.

| Scenario | Duration | Story |
|----------|----------|-------|
| **bigbank-incident** | ~3 min | Walk through the 4-hour BIGBANK outage |
| **orphan-hunt** | ~2 min | Find and fix mystery resources |
| **monday-morning** | ~1 min | Weekly health check ritual |

### Connected Mode

Requires ConfigHub authentication.

| Demo | Duration | Requirements |
|------|----------|--------------|
| **connected** | ~1 min | `cub` CLI authenticated + workers running |

---

## Demo Details

### quick — Ownership Detection

**Duration:** ~30 seconds
**Requirements:** Kubernetes cluster

```bash
# DEPRECATED: ./test/atk/demo quick
```

**What happens:**
1. Applies Flux, ArgoCD, and Native fixtures
2. Runs `cub-scout map`
3. Shows ownership detection in action
4. Cleans up

**What to look for:**
- Resources grouped by owner (Flux, ArgoCD, Native)
- Native resources highlighted as orphans
- Color-coded status indicators

---

### ccve — BIGBANK Incident

**Duration:** ~2 minutes
**Requirements:** Kubernetes cluster

```bash
# DEPRECATED: ./test/atk/demo ccve
```

**The story:**
BIGBANK had a 4-hour production outage caused by a trailing space in a Grafana sidecar annotation (CCVE-2025-0027).

**What happens:**
1. Deploys the bad configuration
2. Shows how it's invisible to normal tools
3. Runs CCVE scanner
4. Scanner catches the issue immediately

**What to look for:**
- CCVE-2025-0027 detected
- Whitespace issue highlighted
- "4 hours → 30 seconds" value prop

---

### healthy — Enterprise Pattern

**Duration:** ~2 minutes
**Requirements:** Kubernetes cluster

```bash
# DEPRECATED: ./test/atk/demo healthy
```

**What happens:**
1. Deploys IITS-style hub-and-spoke GitOps
2. Shows healthy Flux + ArgoCD + Helm deployments
3. Demonstrates mixed-tool visibility

**What to look for:**
- All deployers showing green
- No orphans
- Clean ownership chain

---

### unhealthy — Common Problems

**Duration:** ~2 minutes
**Requirements:** Kubernetes cluster

```bash
# DEPRECATED: ./test/atk/demo unhealthy
```

**What happens:**
1. Deploys resources with various issues
2. Shows suspended Kustomization
3. Shows broken HelmRelease
4. Shows orphan resources

**What to look for:**
- Issues view catches all problems
- Suspended resources highlighted
- Orphans identified

---

### orphan-hunt — Finding Shadow IT

**Duration:** ~2 minutes
**Requirements:** Kubernetes cluster

```bash
# DEPRECATED: ./test/atk/demo scenario orphan-hunt
```

**The story:**
Production has mystery resources. Who deployed them? When? Why?

**What happens:**
1. Creates orphan resources (simulating shadow IT)
2. Shows how to find them
3. Demonstrates orphan investigation
4. Options for remediation

---

### monday-morning — Weekly Health Check

**Duration:** ~1 minute
**Requirements:** Kubernetes cluster

```bash
# DEPRECATED: ./test/atk/demo scenario monday-morning
```

**The story:**
It's Monday morning. Before starting work, you do a quick health check.

**What happens:**
1. Shows status dashboard
2. Checks for issues
3. Reviews drift
4. Identifies action items

---

### connected — ConfigHub Integration

**Duration:** ~1 minute
**Requirements:** ConfigHub authentication, worker running

```bash
# First, ensure you're authenticated
cub context get

# Then run the demo
# DEPRECATED: ./test/atk/demo connected
```

**What happens:**
1. Shows ConfigHub hierarchy TUI
2. Navigates Org → Space → Unit
3. Demonstrates fleet view

---

## Running Demos

### Prerequisites

```bash
# Build cub-scout
go build ./cmd/cub-scout

# Ensure kubectl access
kubectl cluster-info

# For connected demos
cub auth login
cub worker run
```

### Running with Cleanup

Each demo supports `--cleanup`:

```bash
# DEPRECATED: ./test/atk/demo quick
# DEPRECATED: ./test/atk/demo quick --cleanup   # Remove demo resources
```

### Running All Demos

```bash
# Via test suite
./test/prove-it-works.sh --level=demos

# Manually
for demo in quick ccve healthy unhealthy; do
  # DEPRECATED: ./test/atk/demo $demo
  # DEPRECATED: ./test/atk/demo $demo --cleanup
done
```

---

## Demo Requirements

| Demo | Cluster | Flux | ArgoCD | ConfigHub |
|------|---------|------|--------|-----------|
| quick | ✓ | - | - | - |
| ccve | ✓ | - | - | - |
| query | ✓ | - | - | - |
| healthy | ✓ | ✓ | ✓ | - |
| unhealthy | ✓ | ✓ | ✓ | - |
| connected | ✓ | - | - | ✓ |

---

## Expected Outputs

Each demo has documented expected output in `test/expected-outputs/`:

| Demo | Expected Output |
|------|-----------------|
| quick | `test/expected-outputs/demos/quick.yaml` |
| ccve | `test/expected-outputs/demos/ccve.yaml` |
| healthy | `test/expected-outputs/demos/healthy.yaml` |
| unhealthy | `test/expected-outputs/demos/unhealthy.yaml` |
| connected | `test/expected-outputs/demos/connected.yaml` |
| scenarios | `test/expected-outputs/demos/scenarios/*.yaml` |

Validate all expected outputs:

```bash
./test/validate-expected-outputs.sh --category=demos
```

---

## Troubleshooting

### Demo fails to start

```bash
# Check kubectl access
kubectl cluster-info

# Check cub-scout built
./cub-scout version
```

### Cleanup didn't run

```bash
# Manual cleanup for demo namespace
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

## Creating New Demos

Demo scripts are in `test/atk/demo`. To add a new demo:

1. Create script in `test/atk/demo.d/your-demo.sh`
2. Add expected output in `test/expected-outputs/demos/your-demo.yaml`
3. Document in this README

---

## Working Examples

Beyond demos, we have working examples that prove zero-friction adoption.

### apptique (Google Online Boutique)

Reference app demonstrating all major GitOps patterns.

| Pattern | Directory | What It Shows |
|---------|-----------|---------------|
| **Flux Monorepo** | `examples/apptique-examples/source/` | Kustomize + HelmRelease |
| **ArgoCD ApplicationSet** | `examples/apptique-examples/argo-applicationset/` | Directory generator |
| **ArgoCD App of Apps** | `examples/apptique-examples/argo-app-of-apps/` | Parent→children pattern |

```bash
# Test apptique ownership detection (already deployed)
./cub-scout map list -n apptique-prod
./cub-scout trace deploy/frontend -n apptique-prod
```

### IITS / Jesper Examples

Real-world GitOps patterns from IITS consulting.

```bash
# Validate IITS examples
# DEPRECATED: ./test/atk/examples jesper

# Individual tests
# DEPRECATED: ./test/atk/examples jesper_argocd    # ArgoCD patterns
# DEPRECATED: ./test/atk/examples jesper_fluxcd    # Flux patterns
```

### Brian's KubeCon Demos

Production-ready demo spaces in ConfigHub.

| Space | Units | Description |
|-------|-------|-------------|
| `apptique-dev` | 11 | E-commerce (dev) |
| `apptique-prod` | 11 | E-commerce (prod) |
| `appchat-dev/prod` | 4 | Chat application |
| `appvote-dev/prod` | 6 | Voting application |
| `traderx` | 1 | TraderX demo |

```bash
# Requires ConfigHub connection
cub context set space apptique-prod
./cub-scout map --hub
```

### RM Pattern Demos (ArgoCD)

Rendered Manifest pattern scenarios.

| Scenario | Directory | Story |
|----------|-----------|-------|
| **monday-panic** | `examples/rm-demos-argocd/scenarios/monday-panic/` | Find problem in 30 seconds |
| **2am-kubectl** | `examples/rm-demos-argocd/scenarios/2am-kubectl/` | Catch drift |
| **security-patch** | `examples/rm-demos-argocd/scenarios/security-patch/` | Patch 847 services |

Repo patterns:
- `examples/rm-demos-argocd/repo-patterns/monorepo/`
- `examples/rm-demos-argocd/repo-patterns/multi-repo/`
- `examples/rm-demos-argocd/repo-patterns/applicationsets/`
- `examples/rm-demos-argocd/repo-patterns/helm-umbrella/`

### Integrations

Third-party tool integrations.

| Integration | Directory | Status |
|-------------|-----------|--------|
| **ArgoCD Extension** | `examples/integrations/argocd-extension/` | Scanner in ArgoCD UI |
| **Flux Operator** | `examples/integrations/flux-operator/` | Flux9s integration |

```bash
# Validate integrations
# DEPRECATED: ./test/atk/examples integrations
```

### Public Examples (confighub org)

Standard reference architectures.

```bash
# Validate public examples
# DEPRECATED: ./test/atk/examples public

# Individual
# DEPRECATED: ./test/atk/examples global_app      # Multi-cluster global app
# DEPRECATED: ./test/atk/examples helm_platform   # Helm platform components
# DEPRECATED: ./test/atk/examples vm_fleet        # VM fleet management
```

---

## Examples Validation

All examples have expected outputs and can be validated:

```bash
# Run all example tests
# DEPRECATED: ./test/atk/examples --all

# By category
# DEPRECATED: ./test/atk/examples jesper      # IITS examples (6 tests)
# DEPRECATED: ./test/atk/examples public      # Public examples (7 tests)
# DEPRECATED: ./test/atk/examples integrations # Integrations (3 tests)

# Validate expected outputs
./test/validate-expected-outputs.sh --category=examples
```

---

## See Also

- [Expected Outputs](../../test/expected-outputs/README.md) - All documented outputs
- [Testing Guide](../TESTING-GUIDE.md) - Full testing documentation
- [Map Documentation](../map/README.md) - Feature documentation
- [Examples Overview](../EXAMPLES-OVERVIEW.md) - Detailed examples index
