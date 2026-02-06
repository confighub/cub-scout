# Demo Suite

Interactive demos to showcase cub-scout features.

## Quick Reference

```bash
# Build first
go build ./cmd/cub-scout

# List available demos
./cub-scout demo list

# Run demos
./cub-scout demo quick              # Quick ownership detection (~30 sec)
./cub-scout demo ccve               # CCVE-2025-0027 demo (~2 min)
./cub-scout demo query              # Query language demo
./cub-scout demo healthy            # Enterprise healthy pattern
./cub-scout demo unhealthy          # Common GitOps problems

# Narrative scenarios
./cub-scout demo scenario bigbank   # BIGBANK incident walkthrough
./cub-scout demo scenario orphan    # Find shadow IT

# Cleanup
./cub-scout demo quick --cleanup    # Remove demo resources
```

---

## Demo Inventory

### Quick Demos (Standalone)

These work without ConfigHub connection.

| Demo | Duration | What It Shows |
|------|----------|---------------|
| `quick` | ~30 sec | Ownership detection — Flux, ArgoCD, Helm, Native |
| `ccve` | ~2 min | CCVE-2025-0027: The BIGBANK Grafana bug |
| `query` | ~1 min | Query language: `owner!=Native`, `namespace=prod*` |
| `healthy` | ~2 min | Enterprise healthy: IITS hub-and-spoke pattern |
| `unhealthy` | ~2 min | Enterprise problems: suspended resources, orphans |

### Narrative Scenarios

Story-driven walkthroughs.

| Scenario | Duration | Story |
|----------|----------|-------|
| `bigbank` | ~3 min | Walk through the 4-hour BIGBANK outage |
| `orphan` | ~2 min | Find and fix mystery resources |
| `monday` | ~1 min | Weekly health check ritual |

### Connected Mode

Requires ConfigHub authentication.

| Demo | Duration | Requirements |
|------|----------|--------------|
| `connected` | ~1 min | `cub` CLI authenticated + workers running |

---

## Demo Details

### quick — Ownership Detection

**Duration:** ~30 seconds
**Requirements:** Kubernetes cluster

```bash
./cub-scout demo quick
```

**What happens:**
1. Applies Flux, ArgoCD, and Native fixtures
2. Runs `cub-scout map`
3. Shows ownership detection in action
4. Cleans up (with `--cleanup`)

**What to look for:**
- Resources grouped by owner (Flux, ArgoCD, Native)
- Native resources highlighted as orphans
- Color-coded status indicators

---

### ccve — BIGBANK Incident

**Duration:** ~2 minutes
**Requirements:** Kubernetes cluster

```bash
./cub-scout demo ccve
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
./cub-scout demo healthy
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
./cub-scout demo unhealthy
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

## Running Demos

### Prerequisites

```bash
# Build cub-scout
go build ./cmd/cub-scout

# Ensure kubectl access
kubectl cluster-info

# For connected demos
cub auth login
```

### Running with Cleanup

Each demo supports `--cleanup`:

```bash
./cub-scout demo quick
./cub-scout demo quick --cleanup   # Remove demo resources
```

### Running without Pods

For faster demos (YAML only, no running containers):

```bash
./cub-scout demo quick --no-pods
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

## See Also

- [Examples Overview](../EXAMPLES-OVERVIEW.md) - All examples and integrations
- [CLI-GUIDE.md](../../CLI-GUIDE.md) - Complete CLI reference
- [Testing Guide](../testing/README.md) - Testing documentation
