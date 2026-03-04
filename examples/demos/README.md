# Interactive Demos

**Status: Working** — All demos apply real Kubernetes resources and run on your cluster.

Demos that create resources, show problems, and let you explore.

## Running Demos

```bash
cub-scout demo list           # List all demos
cub-scout demo quick            # Run quick demo
cub-scout demo <name> --cleanup # Remove demo resources
```

## Available Demos

| Demo | Time | Description |
|------|------|-------------|
| `quick` | ~30 sec | Fastest path to see Map in action |
| `ccve` | ~2 min | RISK-2025-0027: The BIGBANK Grafana bug |
| `query` | ~1 min | Query language filtering by owner/namespace/cluster |

## Narrative Scenarios

| Scenario | Time | Story |
|----------|------|-------|
| `bigbank-incident` | ~3 min | Walk through the BIGBANK 4-hour outage |
| `break-glass` | ~2 min | Emergency kubectl change and follow-up decisions |

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
- ConfigMap with namespace whitespace bug (RISK-2025-0027)

Shows:
- risk issue scanner detecting the bug
- Remediation steps
- Before/after fix

Story: [BIGBANK - GitOps Lessons Learned](https://www.youtube.com/watch?v=VJiuu-GqfXk)

---

## Query Demo

Query language walkthrough with realistic mixed ownership fixtures.

```bash
cub-scout demo query
```

Shows:
- Filtering by owner and namespace
- OR/AND query patterns
- Fast orphan discovery (`owner=Native`)

---

## Options

| Option | Description |
|--------|-------------|
| `--no-pods` | Apply without running pods (faster) |
| `--cleanup` | Remove demo resources |

Example:
```bash
cub-scout demo quick --no-pods   # Fast structural demo
cub-scout demo quick --cleanup   # Clean up after
```

---

## Demo Fixtures

Demo YAML files are in `test/atk/fixtures/`, `examples/demos/`, and `examples/impressive-demo/bad-configs/`:

| File | Used By |
|------|---------|
| `test/atk/fixtures/flux-basic.yaml` | quick demo |
| `test/atk/fixtures/argo-basic.yaml` | quick demo |
| `examples/impressive-demo/bad-configs/monitoring-bad.yaml` | ccve demo |
| `examples/demos/multi-cluster.yaml` | query demo |
| `examples/demos/break-glass.yaml` | break-glass scenario |

---

## Cross-Owner Reference Demo (NEW)

Shows the new features in v0.3.3: Crossplane detection, cross-owner references, elapsed time.

```bash
# Visual demo (no cluster required)
./examples/demos/cross-owner-demo.sh

# Real cluster demo
kubectl apply -f examples/demos/cross-owner-demo.yaml
./cub-scout trace deploy/api-server -n ecommerce
./cub-scout map workloads -n ecommerce
```

Creates:
- **Crossplane resources** - RDS and ElastiCache proxies with claim labels
- **Terraform secrets** - DB credentials, Redis auth, payment API keys
- **Flux workloads** - API server, payment service, frontend
- **ArgoCD workload** - Analytics collector
- **Native workload** - Debug pod

Shows:
- Crossplane ownership detection (claim-name, composite, composition)
- Cross-owner reference warnings (Flux deployment → Terraform secret)
- Elapsed time since last reconciliation
- Warning highlights for stuck resources

Use case: Platform teams using Crossplane/Terraform for infrastructure while app teams use Flux/ArgoCD for workloads.

---

## Visual Demo Scripts

Standalone scripts that show feature output with sample data (no cluster required).

| Script | Description |
|--------|-------------|
| `cross-owner-demo.sh` | Crossplane, cross-owner refs, elapsed time |
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
- [docs/testing/README.md](../../docs/testing/README.md) - Testing guide
- [docs/howto/trace-ownership.md](../../docs/howto/trace-ownership.md) - Trace documentation
- [docs/howto/scan-for-risks.md](../../docs/howto/scan-for-risks.md) - Scan documentation
