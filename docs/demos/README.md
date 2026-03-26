# Demo Suite

Interactive demos to showcase cub-scout features.

## Quick Reference

```bash
# Build first
go build ./cmd/cub-scout

# List available demos
./cub-scout demo list

# Run demos
./cub-scout demo quick                     # Quick ownership detection (~30 sec)
./cub-scout demo ccve                      # RISK-2025-0027 demo (~2 min)
./cub-scout demo query                     # Query language demo (~1 min)

# Narrative scenarios
./cub-scout demo scenario bigbank-incident # BIGBANK incident walkthrough
./cub-scout demo scenario break-glass      # Emergency kubectl workflow

# Cleanup (for demos that create resources)
./cub-scout demo quick --cleanup
```

---

## Demo Inventory

### Quick Demos

| Demo | Duration | What It Shows |
|------|----------|---------------|
| `quick` | ~30 sec | Ownership detection across Flux, ArgoCD, Helm, Native |
| `ccve` | ~2 min | RISK-2025-0027 detection story |
| `query` | ~1 min | Query language filtering and practical slices |

### Narrative Scenarios

| Scenario | Duration | Story |
|----------|----------|-------|
| `bigbank-incident` | ~3 min | Walk through the BIGBANK 4-hour outage |
| `break-glass` | ~2 min | Emergency kubectl change and workflow aftermath |

---

## Demo Details

### quick — Ownership Detection

**Duration:** ~30 seconds  
**Requirements:** Kubernetes cluster

```bash
./cub-scout demo quick
```

### ccve — BIGBANK Incident

**Duration:** ~2 minutes  
**Requirements:** Kubernetes cluster

```bash
./cub-scout demo ccve
```

### query — Query Language

**Duration:** ~1 minute  
**Requirements:** Kubernetes cluster

```bash
./cub-scout demo query
```

### scenario bigbank-incident

**Duration:** ~3 minutes  
**Requirements:** Kubernetes cluster

```bash
./cub-scout demo scenario bigbank-incident
```

### scenario break-glass

**Duration:** ~2 minutes  
**Requirements:** Kubernetes cluster

```bash
./cub-scout demo scenario break-glass
```

---

## Running Demos

### Prerequisites

```bash
# Build cub-scout
go build ./cmd/cub-scout

# Ensure kubectl access
kubectl cluster-info
```

### Running with Cleanup

```bash
./cub-scout demo quick
./cub-scout demo quick --cleanup
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

---

## See Also

- [Examples Overview](../../reference/examples-overview.md) - All examples and integrations
- [CLI-GUIDE.md](../../CLI-GUIDE.md) - Complete CLI reference
- [Testing Guide](../testing/README.md) - Testing documentation
