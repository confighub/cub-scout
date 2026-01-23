# ConfigHub OCI Integration Example

This example demonstrates how cub-scout detects and traces resources deployed from ConfigHub acting as an OCI registry.

## Overview

ConfigHub can serve as an OCI registry for GitOps tools (Flux, ArgoCD), enabling the Rendered Manifest (RM) pattern:

```
Git (DRY) → ConfigHub (render) → OCI (WET) → Flux/Argo → Cluster (LIVE)
```

**ConfigHub OCI URL Format:**
```
oci://oci.{instance}/target/{space}/{target}
```

**Example:**
```
oci://oci.api.confighub.com/target/prod/us-west
```

## Supported GitOps Tools

### Flux OCIRepository

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: confighub-prod
  namespace: flux-system
spec:
  interval: 1m
  url: oci://oci.api.confighub.com/target/prod/us-west
  ref:
    tag: latest
  secretRef:
    name: confighub-worker-credentials
```

### ArgoCD Application

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: frontend-app
  namespace: argocd
spec:
  source:
    repoURL: oci://oci.api.confighub.com/target/prod/us-west
    targetRevision: latest
    path: .
  destination:
    server: https://kubernetes.default.svc
    namespace: prod
```

## Tracing ConfigHub OCI Sources

cub-scout automatically detects ConfigHub OCI sources and displays detailed information.

### Example: Flux OCIRepository

```bash
./cub-scout trace deploy/frontend -n prod
```

**Output:**
```
TRACE: Deployment/frontend in prod

  ✓ ConfigHub OCI/prod/us-west
    │ Space: prod
    │ Target: us-west
    │ Registry: oci.api.confighub.com
    │ Revision: latest@sha1:abc123
    │
    └─▶ ✓ Kustomization/apps
          │ Path: .
          │ Status: Applied revision latest@sha1:abc123
          │
          └─▶ ✓ Deployment/frontend
                Status: 3/3 ready

✓ All levels in sync. Managed by flux.
```

### Example: ArgoCD Application

```bash
./cub-scout trace --app frontend-app
```

**Output:**
```
TRACE: Application/frontend-app

  ✓ ConfigHub OCI/prod/us-west
    │ Space: prod
    │ Target: us-west
    │ Registry: oci.api.confighub.com
    │ Revision: latest@sha1:abc123
    │
    └─▶ ✓ Application/frontend-app
          │ Status: Synced / Healthy
          │
          └─▶ ✓ Deployment/frontend
                Status: Synced / Healthy

✓ All levels in sync. Managed by argocd.
```

## What cub-scout Detects

When tracing ConfigHub OCI sources, cub-scout extracts:

1. **Space** - The ConfigHub space (e.g., `prod`, `staging`)
2. **Target** - The ConfigHub target cluster (e.g., `us-west`, `eu-central`)
3. **Instance** - The ConfigHub instance host
4. **Registry** - The full OCI registry URL
5. **Revision** - The OCI artifact revision/tag

## Discovery

Find all resources deployed from ConfigHub OCI:

```bash
# List workloads with ownership
./cub-scout discover

# Output includes:
STATUS  NAMESPACE  NAME        OWNER         MANAGED-BY
✓       prod       frontend    Flux          OCIRepository/confighub-prod
✓       prod       backend     ArgoCD        Application/backend-app
```

## Map Command

View ConfigHub OCI sources in the TUI:

```bash
./cub-scout map
```

Press `T` to trace any selected resource and see the ConfigHub OCI source chain.

## JSON Output

For programmatic use:

```bash
./cub-scout trace deploy/frontend -n prod --json
```

```json
{
  "object": {
    "kind": "Deployment",
    "name": "frontend",
    "namespace": "prod"
  },
  "chain": [
    {
      "kind": "ConfigHub OCI",
      "name": "prod/us-west",
      "url": "oci://oci.api.confighub.com/target/prod/us-west",
      "revision": "latest@sha1:abc123",
      "ready": true,
      "status": "Artifact is up to date",
      "ociSource": {
        "raw": "oci://oci.api.confighub.com/target/prod/us-west",
        "isConfigHub": true,
        "instance": "api.confighub.com",
        "space": "prod",
        "target": "us-west",
        "registry": "oci.api.confighub.com",
        "repository": "target/prod/us-west"
      }
    },
    {
      "kind": "Kustomization",
      "name": "apps",
      "namespace": "flux-system",
      "path": ".",
      "ready": true,
      "status": "Applied"
    },
    {
      "kind": "Deployment",
      "name": "frontend",
      "namespace": "prod",
      "ready": true,
      "status": "3/3 ready"
    }
  ],
  "fullyManaged": true,
  "tool": "flux"
}
```

## Authentication

ConfigHub OCI requires authentication using worker credentials.

### Flux Secret

```bash
kubectl create secret generic confighub-worker-credentials \
  --namespace=flux-system \
  --from-literal=username=<worker-id> \
  --from-literal=password=<worker-secret>
```

### ArgoCD Secret

```bash
kubectl create secret generic repo-oci-prod-us-west \
  --namespace=argocd \
  --type=Opaque \
  --from-literal=type=oci \
  --from-literal=url=oci://oci.api.confighub.com/target/prod/us-west \
  --from-literal=username=<worker-id> \
  --from-literal=password=<worker-secret>

kubectl label secret repo-oci-prod-us-west \
  --namespace=argocd \
  argocd.argoproj.io/secret-type=repository
```

## Helm Charts with ConfigHub OCI

ConfigHub can package rendered manifests as Helm charts for deployment via Flux HelmRelease or ArgoCD. This is useful when:

- You need Helm release tracking and rollback capabilities
- Your manifests contain template-like syntax that should not be re-rendered (e.g., Grafana dashboards with `{{instance}}`)
- You want to deploy large charts with CRD/workload separation

See the complete guide: [rendered-helm-chart.md](./rendered-helm-chart.md)

## Benefits of ConfigHub OCI

1. **Clear Provenance** - See exactly which ConfigHub space/target deployed each resource
2. **Immutable Artifacts** - OCI tags provide immutable snapshots
3. **Unified Tracing** - Single command works for Flux and ArgoCD
4. **Fleet Visibility** - Track which clusters pull from which ConfigHub targets
5. **No Merge Conflicts** - Rendered manifests stay in OCI, not Git

## Related Examples

- [examples-internal/argocd](https://github.com/confighubai/examples-internal/tree/main/argocd) - Full ArgoCD + ConfigHub OCI setup
- [Flux OCI Sources](../flux-operator/) - Flux OCIRepository examples
- [ArgoCD Extension](../argocd-extension/) - ArgoCD integration

## Learn More

- [Rendered Helm Charts Guide](./rendered-helm-chart.md) - Deploy pre-rendered Helm charts from ConfigHub
- [ConfigHub Documentation](https://docs.confighub.com)
- [Rendered Manifest Pattern](../../../docs/outcomes/confighub-integration.md)
- [OCI Artifacts Specification](https://github.com/opencontainers/artifacts)
