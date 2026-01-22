# Third-Party Integrations

Plugins and extensions for popular Kubernetes tools.

> **Maintainer note:** When updating this file, also update [docs/EXAMPLES-OVERVIEW.md](../../docs/EXAMPLES-OVERVIEW.md).

## Status Legend

| Status | Meaning |
|--------|---------|
| **Working** | Tested, production-ready code |
| **Mockup** | UI design/mockups for discussion |
| **Proposal** | Architecture proposal, not yet built |

---

## Available Integrations

| Integration | Status | Description |
|-------------|--------|-------------|
| [argocd-extension/](argocd-extension/) | **Working** | Argo CD UI extension with CCVE tab |
| [flux-operator/](flux-operator/) | **Working** | Prometheus metrics exporter for Flux Operator |
| [flux9s/](flux9s/) | **Proposal** | flux9s TUI enhancement with ownership columns |

---

## Argo CD Extension

**Status: Working**

Adds a "CCVEs" tab and status badge to Argo CD application views.

```bash
# Install the extension
kubectl apply -f argocd-extension/

# Extension adds:
# - CCVE badge in application header
# - CCVEs tab with findings
# - Per-resource ownership info
```

See [argocd-extension/README.md](argocd-extension/README.md) for setup.

---

## Flux Operator Integration

**Status: Working**

Prometheus metrics exporter that exposes CCVE findings as metrics.

```bash
# Deploy the exporter
kubectl apply -f flux-operator/ccve-exporter.yaml

# Metrics exposed at :9877/metrics
# confighub_ccve_findings{severity="critical",cluster="prod"} 1
# confighub_ccve_findings{severity="warning",cluster="prod"} 3
```

See [flux-operator/README.md](flux-operator/README.md) for setup.

---

## flux9s TUI Enhancement

**Status: Proposal (Mockups Only)**

Proposed enhancements to the flux9s terminal UI.

See [MOCKUPS.md](MOCKUPS.md) for detailed UI mockups and [flux9s/README.md](flux9s/README.md) for the full proposal.

---

## Building an Integration

All integrations use the Agent's JSON API:

```bash
# Get cluster map
cub-scout snapshot -o - | jq '.entries[]'

# Get CCVE findings
cub-scout scan --json | jq '.findings[]'
```

### API Endpoints (Proposed)

> **Not Yet Implemented:** These endpoints are proposed for a future HTTP API mode.

| Endpoint | Description |
|----------|-------------|
| `GET /api/map` | Full resource map with ownership |
| `GET /api/map?namespace=X` | Filter by namespace |
| `GET /api/ccve/findings` | CCVE scan results |
| `GET /api/summary` | Fleet-wide aggregation |
| `WS /ws/watch` | Real-time updates |

### JSON Output Example

```json
{
  "cluster": "prod-east",
  "workloads": [
    {
      "name": "backend",
      "namespace": "prod",
      "owner": "ConfigHub",
      "confighub": {
        "unit": "backend",
        "space": "payments-prod",
        "revision": "42"
      }
    }
  ],
  "findings": [
    {
      "id": "CCVE-2025-0027",
      "severity": "critical",
      "resource": "monitoring/grafana"
    }
  ]
}
```

---

## See Also

- [examples/README.md](../README.md) - All examples
- [docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md) - GSF protocol
- [docs/EXTENDING.md](../../docs/EXTENDING.md) - Custom integrations
