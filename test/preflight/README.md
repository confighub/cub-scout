# Pre-flight Validation

Run these checks **before any other tests** to validate your environment.

## mini-tck

Technology Compatibility Kit - validates environment is ready for testing.

```bash
# Basic check (Kubernetes only)
./test/preflight/mini-tck

# Full check (includes ConfigHub connected mode)
./test/preflight/mini-tck --connected
```

### What It Checks

**Basic mode:**
- Go installed
- kubectl works
- Cluster accessible
- Flux CRDs installed (optional)
- Argo CD CRDs installed (optional)
- cub-scout binary exists or can build

**Connected mode (--connected):**
- cub CLI installed
- cub authenticated
- Active space set
- Worker connected (not null, not unknown)
- Target exists (not null)
- Units exist in space

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All checks passed (or passed with warnings) |
| 1 | Critical failures - fix before running tests |

### Common Failures

| Failure | Fix |
|---------|-----|
| "cub not authenticated" | Run `cub auth login` |
| "No active space" | Run `cub context set space <slug>` |
| "No workers in space" | Install a worker in your space |
| "Worker slug is null" | Issue #1 pattern - check cub CLI version |
| "No targets in space" | Create a target in your space |

### When to Run

1. Before starting a test session
2. After changing Kubernetes context
3. After changing ConfigHub space
4. When tests fail unexpectedly
