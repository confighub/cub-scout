# Flux Operator cub-scout Integration

[Flux Operator](https://fluxcd.control-plane.io/operator/) by Stefan Prodan provides a web UI with GitOps graph visualization. This example shows how to integrate cub-scout capabilities.

## What the Integration Adds

| Feature | Description |
|---------|-------------|
| **Graph Overlay** | Show ConfigHub ownership on graph nodes |
| **Ownership Panel** | Breakdown by owner (ConfigHub/Flux/Argo/Helm/Native) |
| **ConfigHub Context** | Link nodes to Space/Unit/Revision |
| **Drift Indicators** | Visual markers for drifted resources |
| **risk issue Badges** | Warning indicators on affected nodes |
| **Prometheus Metrics** | Export ownership and status data |

## Mockup: Enhanced GitOps Graph

```
┌─ GitOps Pipeline: payments ─────────────────────────────────────────────────┐
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                                                                     │   │
│  │   ┌────────────┐      ┌────────────────┐      ┌────────────────┐   │   │
│  │   │GitRepository│─────▶│ Kustomization │─────▶│  Deployment    │   │   │
│  │   │            │      │                │      │                │   │   │
│  │   │ payments   │      │ payments       │      │ backend        │   │   │
│  │   │ ✓ Ready    │      │ ✓ Applied      │      │ ✓ Ready 3/3   │   │   │
│  │   └────────────┘      │                │      │                │   │   │
│  │        Flux           │ ┌────────────┐ │      │ ┌────────────┐ │   │   │
│  │                       │ │ ConfigHub  │ │      │ │ ConfigHub  │ │   │   │
│  │                       │ │ payments/  │ │      │ │ payments/  │ │   │   │
│  │                       │ │ infra @42  │ │      │ │ backend@42 │ │   │   │
│  │                       │ └────────────┘ │      │ └────────────┘ │   │   │
│  │                       └────────────────┘      └────────────────┘   │   │
│  │                              │                       │             │   │
│  │                              │                       ▼             │   │
│  │                              │              ┌────────────────┐     │   │
│  │                              │              │   Service      │     │   │
│  │                              │              │   backend      │     │   │
│  │                              │              │   ✓ Ready      │     │   │
│  │                              │              │ ┌────────────┐ │     │   │
│  │                              │              │ │ ConfigHub  │ │     │   │
│  │                              ▼              │ │ backend@42 │ │     │   │
│  │                       ┌────────────────┐    │ └────────────┘ │     │   │
│  │                       │  HelmRelease   │    └────────────────┘     │   │
│  │                       │  redis         │                           │   │
│  │                       │  ✓ Deployed    │                           │   │
│  │                       │ ┌────────────┐ │                           │   │
│  │                       │ │   Helm     │ │                           │   │
│  │                       │ └────────────┘ │                           │   │
│  │                       └────────────────┘                           │   │
│  │                                                                     │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─ Legend ─────────────────────────────────────────────────────────────┐  │
│  │ ConfigHub ████  Flux ████  Helm ████  Native ████   ⚠ Drift  🔴 risk issue │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Mockup: Ownership Panel

```
┌─ Fleet Overview ────────────────────────────────────────────────────────────┐
│                                                                             │
│  ┌─ Ownership ─────────────┐  ┌─ Health ───────────────┐                   │
│  │                         │  │                        │                   │
│  │  ConfigHub  ████████ 45 │  │  Ready      ████████ 52│                   │
│  │  Flux       ████     12 │  │  Progressing ██      3 │                   │
│  │  ArgoCD     ███       8 │  │  Degraded   ██       2 │                   │
│  │  Helm       █         3 │  │  Failed     █        1 │                   │
│  │  Native     ██        5 │  │  Unknown    █        2 │                   │
│  │                         │  │                        │                   │
│  └─────────────────────────┘  └────────────────────────┘                   │
│                                                                             │
│  ┌─ ConfigHub Spaces ───────────────────────────────────────────────────┐  │
│  │ Space              Units  Healthy  Drifted  risk issues                    │  │
│  │ payments-prod        12      12        0       0                     │  │
│  │ payments-staging      8       7        1       1                     │  │
│  │ monitoring            5       4        0       2                     │  │
│  │ platform-infra       20      19        1       0                     │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌─ Issues ─────────────────────────────────────────────────────────────┐  │
│  │ 🔴 RISK-2025-0004  Kustomization/monitoring  BuildFailed             │  │
│  │ ⚠️  RISK-2025-0023  Deployment/api           Missing limits          │  │
│  │ ⚠️  DRIFT          ConfigMap/app-config     LOG_LEVEL changed        │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Flux Operator Web UI                                  │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                                                                       │  │
│  │   GitOps Graph        Ownership Panel        Issues Panel             │  │
│  │   (D3.js)             (React)                (React)                  │  │
│  │                                                                       │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                    │                                        │
│                                    ▼                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                    UI Data Layer                                      │  │
│  │  - Flux resources from K8s API                                        │  │
│  │  - Agent data from cub-scout API                                │  │
│  │  - Merged view                                                        │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────┬──────────────────────────────────────┘
                                       │
              ┌────────────────────────┼────────────────────────┐
              ▼                        ▼                        ▼
┌──────────────────────┐  ┌──────────────────────┐  ┌──────────────────────┐
│   Kubernetes API     │  │  cub-scout     │  │    Prometheus        │
│                      │  │                      │  │                      │
│  - FluxInstance      │  │  GET /api/map        │  │  agent_resource_*    │
│  - Kustomization     │  │  GET /api/graph      │  │  agent_risk_*        │
│  - HelmRelease       │  │  GET /api/summary    │  │  agent_drift_*       │
│  - GitRepository     │  │  WS /ws/watch        │  │                      │
└──────────────────────┘  └──────────────────────┘  └──────────────────────┘
```

## Integration Options

### Option 1: Prometheus Metrics

Export Agent data as Prometheus metrics for Grafana dashboards.

```yaml
# agent-deployment.yaml includes Prometheus exporter
# See risk-exporter.yaml for full manifest
```

**Metrics exported:**

```prometheus
# Resource ownership
agent_resource_total{owner="ConfigHub"} 45
agent_resource_total{owner="Flux"} 12
agent_resource_total{owner="ArgoCD"} 8

# ConfigHub-specific
agent_confighub_unit_info{space="payments-prod",unit="backend",revision="42"} 1
agent_confighub_drift_total{space="payments-prod"} 1

# Health by owner
agent_resource_health{owner="ConfigHub",status="ready"} 43
agent_resource_health{owner="ConfigHub",status="degraded"} 2

# risk issue findings
agent_risk_total{severity="critical"} 0
agent_risk_total{severity="warning"} 3
agent_risk_finding{id="RISK-2025-0023",resource="default/Deployment/api"} 1
```

### Option 2: API Integration (Proposed)

> **Not Yet Implemented:** This API integration is proposed for a future HTTP API mode. Currently, use CLI commands like `cub-scout snapshot -o -` and `cub-scout scan --json`.

Query Agent API directly from the Flux Operator UI.

```typescript
// services/agent.ts
export class AgentService {
  private baseUrl: string;

  constructor(baseUrl = '/api/agent') {
    this.baseUrl = baseUrl;
  }

  async getResourceMap(): Promise<ResourceMap> {
    const response = await fetch(`${this.baseUrl}/map`);
    return response.json();
  }

  async getGraph(root?: string): Promise<Graph> {
    const url = root
      ? `${this.baseUrl}/map/graph?root=${root}`
      : `${this.baseUrl}/map/graph`;
    const response = await fetch(url);
    return response.json();
  }

  async getSummary(): Promise<Summary> {
    const response = await fetch(`${this.baseUrl}/summary`);
    return response.json();
  }

  watchUpdates(callback: (update: Update) => void): WebSocket {
    const ws = new WebSocket(`ws://${window.location.host}${this.baseUrl}/ws/watch`);
    ws.onmessage = (event) => callback(JSON.parse(event.data));
    return ws;
  }
}
```

### Option 3: Graph Node Enhancement

Add ownership badges to existing graph nodes.

```typescript
// components/GraphNode.tsx
interface GraphNodeProps {
  node: FluxResource;
  agentInfo?: AgentResourceInfo;
}

export function GraphNode({ node, agentInfo }: GraphNodeProps) {
  return (
    <g className="graph-node">
      {/* Existing node rendering */}
      <circle r={20} fill={statusColor(node.status)} />
      <text>{node.name}</text>

      {/* ConfigHub ownership badge */}
      {agentInfo?.owner === 'ConfigHub' && (
        <g transform="translate(15, -15)">
          <rect width={60} height={16} fill="#6366f1" rx={3} />
          <text x={5} y={12} fill="white" fontSize={10}>
            {agentInfo.ownerDetails.unit}
          </text>
        </g>
      )}

      {/* Drift indicator */}
      {agentInfo?.drift && (
        <circle cx={18} cy={-18} r={6} fill="#f59e0b" />
      )}

      {/* risk issue badge */}
      {agentInfo?.riskIssues?.length > 0 && (
        <g transform="translate(-20, -20)">
          <circle r={8} fill={riskSeverityColor(agentInfo.riskIssues)} />
          <text fill="white" fontSize={10}>{agentInfo.riskIssues.length}</text>
        </g>
      )}
    </g>
  );
}
```

## Deployment

### Agent as Sidecar to Flux Operator

```yaml
# flux-operator-patch.yaml
spec:
  template:
    spec:
      containers:
        - name: flux-operator
          # ... existing container
        - name: cub-scout
          image: ghcr.io/confighub/agent:latest
          args:
            - serve
            - --port=8080
            - --metrics-port=9090
          ports:
            - containerPort: 8080
              name: api
            - containerPort: 9090
              name: metrics
```

### Standalone Agent

```yaml
# See agent-deployment.yaml
kubectl apply -f agent-deployment.yaml
```

## Files

- `README.md` - This file
- `risk-exporter.yaml` - Prometheus exporter (from previous example)
- `agent-deployment.yaml` - Standalone agent deployment

## References

- [Flux Operator](https://fluxcd.control-plane.io/operator/)
- [Flux Operator Web UI](https://fluxoperator.dev/web-ui/)
- [Stefan Prodan's Blog](https://stefanprodan.com/blog/2024/flux-operator/)
- [cub-scout GSF Schema](../../docs/GSF-SCHEMA.md)
