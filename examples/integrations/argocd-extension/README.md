# Argo CD cub-scout Extension

An Argo CD UI extension that integrates with the cub-scout to show:

- **Resource ownership** - ConfigHub, Flux, Helm, or native
- **ConfigHub context** - Space, Unit, revision for ConfigHub-managed resources
- **Relationship graph** - Full delivery pipeline visualization
- **Drift detection** - Live vs desired state
- **risk issue findings** - Known misconfigurations

## Screenshots (Mockup)

### Application View with Ownership

```
┌─ guestbook ─────────────────────────────────────────────────────────┐
│ [Synced] [Healthy]                                     ⚙️ Settings  │
├─────────────────────────────────────────────────────────────────────┤
│ Summary │ Diff │ Logs │ Events │ Agent │  ← New Tab                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌─ Ownership Summary ─────────────────────────────────────────┐   │
│  │ ConfigHub: 3 resources │ Native: 2 resources               │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌─ Resource Tree ─────────────────────────────────────────────┐   │
│  │                                                             │   │
│  │  📦 Deployment/guestbook-ui                                 │   │
│  │     Owner: ConfigHub                                        │   │
│  │     Unit: guestbook-ui │ Space: demo │ Rev: 15              │   │
│  │     Status: ✓ Ready (3/3 replicas)                          │   │
│  │                                                             │   │
│  │  🔧 Service/guestbook-ui                                    │   │
│  │     Owner: ConfigHub                                        │   │
│  │     Unit: guestbook-ui │ Space: demo │ Rev: 15              │   │
│  │                                                             │   │
│  │  📄 ConfigMap/guestbook-config                              │   │
│  │     Owner: Native (no GitOps labels)                        │   │
│  │                                                             │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌─ Issues ────────────────────────────────────────────────────┐   │
│  │ ⚠️ RISK-2025-0023: Missing resource limits (info)           │   │
│  │    Resource: Deployment/guestbook-ui                        │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Resource Detail with Agent Info

```
┌─ Deployment/guestbook-ui ───────────────────────────────────────────┐
│ Summary │ YAML │ Events │ Logs │ Agent │                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  Ownership                                                          │
│  ──────────────────────────────────────────────────────────────     │
│  Owner:      ConfigHub                                              │
│  Unit:       guestbook-ui                                           │
│  Space:      demo (App Space)                                       │
│  Revision:   15 (live: 15 ✓)                                        │
│  Deployer:   Argo CD (this application)                             │
│                                                                     │
│  Delivery Pipeline                                                  │
│  ──────────────────────────────────────────────────────────────     │
│  ConfigHub Unit ─▶ Argo Application ─▶ Deployment ─▶ ReplicaSet    │
│  guestbook-ui      guestbook           guestbook-ui   ┌─ Pod x3    │
│                                                                     │
│  Drift Status                                                       │
│  ──────────────────────────────────────────────────────────────     │
│  ✓ No drift detected                                                │
│                                                                     │
│  Config CVEs                                                        │
│  ──────────────────────────────────────────────────────────────     │
│  ℹ️  RISK-2025-0023: Missing resource limits                        │
│      Remediation: Add resources.limits to container spec            │
│      [View Details]                                                 │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Argo CD Server                                │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                    UI (React)                                 │  │
│  │  ┌─────────────────────────────────────────────────────────┐  │  │
│  │  │              cub-scout Extension                  │  │  │
│  │  │  - Status Panel (ownership badge)                       │  │  │
│  │  │  - Application Tab (full agent view)                    │  │  │
│  │  │  - Resource Tab (per-resource details)                  │  │  │
│  │  └─────────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                              │                                       │
│                              ▼                                       │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │            Extension Backend Proxy                            │  │
│  │            /extensions/cub-scout/*                      │  │
│  └───────────────────────────────────────────────────────────────┘  │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    cub-scout Service                           │
│                    (in-cluster or sidecar)                           │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  GET /api/map           - Resource ownership map              │  │
│  │  GET /api/map/graph     - Relationship graph                  │  │
│  │  GET /api/risk/findings - risk issue scan results                   │  │
│  │  GET /api/drift         - Drift detection results             │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## Installation

### 1. Deploy the cub-scout

```bash
kubectl apply -f agent-deployment.yaml
```

### 2. Configure Extension Backend

Add to `argocd-cm` ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: argocd-cm
  namespace: argocd
data:
  extension.config: |
    extensions:
      - name: cub-scout
        backend:
          services:
            - url: http://cub-scout.confighub-system:8080
              headers:
                - name: X-Argo-App
                  value: '$app.metadata.name'
```

### 3. Install the Extension

```bash
# Copy extension to argocd-server
kubectl -n argocd cp extension.js argocd-server-xxx:/tmp/extensions/extension-cub-scout.js
```

Or use init container (see `extension-patch.yaml`).

## Files

- `extension.js` - UI extension code
- `agent-deployment.yaml` - cub-scout deployment
- `extension-patch.yaml` - Patch to add extension to argocd-server

## Extension API (Proposed)

> **Not Yet Implemented:** These API endpoints are proposed for a future HTTP API mode. Currently, integrations should use CLI commands like `cub-scout snapshot` and `cub-scout scan --json`.

### GET /api/map

Returns resource ownership map for a namespace or application:

```json
{
  "resources": [
    {
      "namespace": "default",
      "kind": "Deployment",
      "name": "guestbook-ui",
      "owner": "ConfigHub",
      "ownerDetails": {
        "unit": "guestbook-ui",
        "space": "demo",
        "revision": 15
      },
      "deployer": "ArgoCD",
      "deployerDetails": {
        "application": "guestbook"
      }
    }
  ]
}
```

### GET /api/map/graph

Returns relationship graph:

```json
{
  "nodes": [
    {"id": "Deployment/guestbook-ui", "kind": "Deployment", "owner": "ConfigHub"},
    {"id": "ReplicaSet/guestbook-ui-abc", "kind": "ReplicaSet", "owner": "ConfigHub"},
    {"id": "Pod/guestbook-ui-abc-xyz", "kind": "Pod", "owner": "ConfigHub"}
  ],
  "edges": [
    {"from": "Deployment/guestbook-ui", "to": "ReplicaSet/guestbook-ui-abc", "type": "owns"},
    {"from": "ReplicaSet/guestbook-ui-abc", "to": "Pod/guestbook-ui-abc-xyz", "type": "owns"}
  ]
}
```

### GET /api/risk/findings

Returns risk issue findings:

```json
{
  "findings": [
    {
      "id": "RISK-2025-0023",
      "severity": "info",
      "resource": "default/Deployment/guestbook-ui",
      "message": "Container lacks resource limits"
    }
  ]
}
```

## References

- [Argo CD UI Extensions](https://argo-cd.readthedocs.io/en/stable/developer-guide/extensions/ui-extensions/)
- [cub-scout GSF Schema](../../docs/GSF-SCHEMA.md)
