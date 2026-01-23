# Interactive Demos

**Status: Working** — All demos apply real Kubernetes resources and run on your cluster.

> **Maintainer note:** When updating this file, also update [docs/EXAMPLES-OVERVIEW.md](../../docs/EXAMPLES-OVERVIEW.md).

Demos that create resources, show problems, and let you explore.

## Running Demos

```bash
cub-scout demo --list           # List all demos
cub-scout demo quick            # Run quick demo
cub-scout demo <name> --cleanup # Remove demo resources
```

## Available Demos

| Demo | Time | Description |
|------|------|-------------|
| `quick` | ~30 sec | Fastest path to see Map in action |
| `ccve` | ~2 min | CCVE-2025-0027: The BIGBANK Grafana bug |
| `healthy` | ~2 min | Enterprise healthy (IITS hub-and-spoke) |
| `unhealthy` | ~2 min | Common GitOps problems |
| `connected` | ~1 min | ConfigHub connected mode (requires cub auth) |

## Narrative Scenarios

| Scenario | Time | Story |
|----------|------|-------|
| `bigbank-incident` | ~3 min | Walk through the BIGBANK 4-hour outage |
| `orphan-hunt` | ~2 min | Find and fix orphan resources |
| `monday-morning` | ~1 min | Weekly health check ritual |

Run with: `cub-scout demo scenario <name>`

---

## Quick Demo

Fastest path to see the Map in action.

```bash
cub-scout demo quick
```

Creates:
- Flux Kustomization with podinfo
- ConfigHub-labeled deployment
- Native deployment

Shows:
- Ownership detection across all types
- Map dashboard output
- Pipeline visualization

---

## CCVE Demo

The BIGBANK Grafana bug that caused a 4-hour outage.

```bash
cub-scout demo ccve
```

Creates:
- Grafana deployment with sidecar config
- ConfigMap with namespace whitespace bug (CCVE-2025-0027)

Shows:
- CCVE scanner detecting the bug
- Remediation steps
- Before/after fix

Story: [BIGBANK - GitOps Lessons Learned](https://www.youtube.com/watch?v=VJiuu-GqfXk)

---

## Enterprise Healthy Demo

IITS-style hub-and-spoke GitOps pattern, all working correctly.

```bash
cub-scout demo healthy
```

Creates:
- Platform layer (cert-manager, prometheus) via Argo CD
- Team workloads via Flux HelmRelease and Argo Application
- Helm-managed services
- ConfigHub-pure resources (feature-flags)

Shows:
- Multiple deployers coexisting
- ConfigHub hierarchy (Space → Unit → Revision)
- All pods healthy

---

## Enterprise Unhealthy Demo

Common GitOps problems and CCVEs.

```bash
cub-scout demo unhealthy
```

Creates:
- Suspended Kustomization (forgotten maintenance)
- HelmRelease with invalid chart (SourceNotReady)
- Orphan resources (no GitOps owner)
- CCVE-2025-0027 bug

Shows:
- Problem detection
- CCVE scanner output
- Troubleshooting workflow

---

## Options

| Option | Description |
|--------|-------------|
| `--no-pods` | Apply without running pods (faster) |
| `--cleanup` | Remove demo resources |

Example:
```bash
cub-scout demo healthy --no-pods   # Fast structural demo
cub-scout demo healthy --cleanup   # Clean up after
```

---

## Demo Fixtures

Demo YAML files are in `test/atk/demos/`:

| File | Used By |
|------|---------|
| `demo-full.yaml` | ccve demo |
| `enterprise-healthy.yaml` | healthy demo |
| `enterprise-unhealthy.yaml` | unhealthy demo |

---

## Visual Demo Scripts

Standalone scripts that show feature output with sample data (no cluster required).

| Script | Description |
|--------|-------------|
| `tui-queries-demo.sh` | Saved queries feature |
| `fleet-queries-demo.sh` | Fleet query examples |
| `tui-trace-demo.sh` | GitOps trace feature |
| `tui-import-demo.sh` | Import with path inference |
| `kyverno-scan-demo.sh` | Kyverno policy scan |
| `meta-pattern-demo.sh` | 5 meta-patterns (what Kyverno misses) |

Run any script:
```bash
./examples/demos/tui-trace-demo.sh
./examples/demos/tui-import-demo.sh
./examples/demos/kyverno-scan-demo.sh
```

---

## Screenshot Demo

Create a cluster with diverse ownership for capturing impressive TUI screenshots.

```bash
./examples/demos/capture-workloads-screenshot.sh
```

This creates a kind cluster with workloads managed by:
- **Flux Kustomization** - boutique microservices (frontend, cart, checkout)
- **ArgoCD Application** - payment services
- **Helm** - platform tools (nginx-ingress, cert-manager)
- **Flux HelmRelease** - monitoring stack (prometheus, grafana)
- **ConfigHub OCI** - analytics and reporting
- **Native** - debug tools (no GitOps)

Perfect for:
- Creating marketing screenshots
- Demonstrating ownership detection
- Testing TUI with diverse data

**Cleanup:**
```bash
kind delete cluster --name cub-scout-demo
```

---

## See Also

- [examples/README.md](../README.md) - All examples
- [examples/impressive-demo/](../impressive-demo/) - Full conference demo
- [docs/TESTING-GUIDE.md](../../docs/TESTING-GUIDE.md) - Testing guide
- [docs/map/howto/trace-ownership.md](../../docs/map/howto/trace-ownership.md) - Trace documentation
- [docs/map/howto/scan-for-ccves.md](../../docs/map/howto/scan-for-ccves.md) - Scan documentation
