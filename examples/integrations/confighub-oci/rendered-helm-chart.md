# Rendered Helm Charts with ConfigHub OCI

**Problem solved**: Deploying pre-rendered manifests via Flux HelmRelease without re-rendering.

## The Problem

When Flux HelmRelease deploys a Helm chart, it re-renders all templates with Helm's template engine. This causes issues when your manifests contain template-like syntax that should **not** be re-rendered:

```yaml
# Grafana ConfigMap with dashboard template
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboard
data:
  dashboard.json: |
    {
      "query": "rate(requests_total{instance=\"{{instance}}\"}[5m])"
    }
```

**What happens**: Flux sees `{{instance}}` and tries to render it as a Helm template variable, causing errors because `instance` is not defined in the chart's values.

**What you want**: Deploy the manifest exactly as-is, with `{{instance}}` preserved for Grafana's runtime interpolation.

## The Solution: Files Directory + `.Files.Get`

ConfigHub can render your manifests into a Helm chart that **prevents re-rendering** by using Helm's `.Files` feature:

### Chart Structure

```
kube-prometheus-stack/
├── Chart.yaml
├── files/
│   └── manifest.yaml          # Pre-rendered content (from ConfigHub)
└── templates/
    └── all.yaml                # Just passes through .Files.Get
```

### Chart.yaml

```yaml
apiVersion: v2
name: kube-prometheus-stack
version: 1.0.0
description: Pre-rendered Prometheus stack from ConfigHub
```

### templates/all.yaml

```yaml
{{ .Files.Get "files/manifest.yaml" }}
```

That's it! Helm's `.Files.Get` returns the file content **without template processing**.

### files/manifest.yaml

Contains your pre-rendered manifests with ConfigHub labels and any template-like syntax preserved:

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboard
  labels:
    confighub.com/UnitSlug: monitoring
data:
  dashboard.json: |
    {
      "query": "rate(requests_total{instance=\"{{instance}}\"}[5m])"
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  labels:
    confighub.com/UnitSlug: monitoring
spec:
  # ... deployment spec
```

## Two-Chart Pattern for Large Deployments

For large charts like kube-prometheus-stack, split into two charts to avoid CRD installation race conditions:

### Chart 1: CRDs Only

```
kube-prometheus-stack-crds/
├── Chart.yaml
├── files/
│   └── crds.yaml              # All CustomResourceDefinitions
└── templates/
    └── all.yaml                # {{ .Files.Get "files/crds.yaml" }}
```

### Chart 2: Workloads

```
kube-prometheus-stack/
├── Chart.yaml
├── files/
│   └── manifest.yaml          # All workload resources
└── templates/
    └── all.yaml                # {{ .Files.Get "files/manifest.yaml" }}
```

### Flux HelmRelease Configuration

Deploy CRDs first, then workloads with dependency. Note: HelmRelease requires `HelmRepository` (not `OCIRepository`) with `type: oci` for OCI registries:

```yaml
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: confighub-prod-monitoring
  namespace: monitoring
spec:
  type: oci
  url: oci://oci.api.confighub.com/target/prod/monitoring
  interval: 10m
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: kube-prometheus-stack-crds
  namespace: monitoring
spec:
  interval: 10m
  chart:
    spec:
      chart: kube-prometheus-stack-crds
      version: "1.0.0"
      sourceRef:
        kind: HelmRepository
        name: confighub-prod-monitoring
      interval: 10m
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: kube-prometheus-stack
  namespace: monitoring
spec:
  interval: 10m
  dependsOn:
    - name: kube-prometheus-stack-crds
  chart:
    spec:
      chart: kube-prometheus-stack
      version: "1.0.0"
      sourceRef:
        kind: HelmRepository
        name: confighub-prod-monitoring
      interval: 10m
```

## ConfigHub Workflow

```
┌─────────────────┐
│ Git Repository  │  DRY source with templates
│ (your repo)     │  {{ .Values.namespace }}, {{ .Env.CLUSTER_NAME }}
└────────┬────────┘
         │
         ▼ ConfigHub renders with real values
┌─────────────────┐
│ ConfigHub       │  Renders all templates to WET manifests
│ (confighub.com) │  Adds confighub.com/* labels
└────────┬────────┘
         │
         ▼ Pushes to OCI registry
┌─────────────────┐
│ OCI Registry    │  oci://oci.api.confighub.com/target/prod/monitoring
│ (Helm chart)    │  files/manifest.yaml = rendered content
└────────┬────────┘
         │
         ▼ Flux HelmRelease pulls and deploys
┌─────────────────┐
│ Kubernetes      │  Manifests deployed exactly as rendered
│ (your cluster)  │  {{instance}} preserved, labels present
└─────────────────┘
```

## When to Use This Pattern

### ✅ Use Rendered Helm Charts When:

- Your manifests contain template-like syntax that should not be re-rendered (e.g., Grafana dashboards, Prometheus rules)
- You want ConfigHub to handle all rendering and value substitution
- You need deterministic deployments with pre-rendered, auditable manifests
- You want ConfigHub labels applied to all resources
- You're deploying from ConfigHub OCI registry

### ❌ Don't Use This Pattern When:

- You need Flux to override Helm values at deploy time
- Your chart requires dynamic value substitution in the cluster
- You're using standard Helm charts from public repositories (use regular HelmRelease)

## Working Example: kube-prometheus-stack

See the implementation in issue #3504 for a complete working example:
https://github.com/confighubai/confighub/issues/3504

The chart successfully deploys:
- 40+ CustomResourceDefinitions
- Prometheus Operator
- Prometheus StatefulSet
- Grafana with dashboards (containing `{{instance}}` and other template syntax)
- AlertManager
- Node exporters
- Service monitors

All Grafana dashboards work correctly because `{{instance}}` is preserved and interpolated by Grafana at runtime, not by Helm during deployment.

## Tracing with cub-scout

After deployment, use cub-scout to verify the ownership chain:

```bash
$ ./cub-scout trace deploy/prometheus -n monitoring

Resource: Deployment/prometheus (monitoring)
Status: Running (1/1 pods ready)

Ownership Chain:
  └─ HelmRelease/kube-prometheus-stack (monitoring)
     └─ ConfigHub OCI
        Space: prod
        Target: monitoring
        Registry: oci.api.confighub.com

Labels:
  confighub.com/UnitSlug: monitoring
  helm.toolkit.fluxcd.io/name: kube-prometheus-stack
  helm.toolkit.fluxcd.io/namespace: monitoring
```

## Troubleshooting

### Error: `undefined variable: instance`

**Cause**: You're using a regular Helm chart structure with templates that Flux is re-rendering.

**Fix**: Move rendered content to `files/manifest.yaml` and use `.Files.Get` in templates.

### Pods not starting: `CRD not found`

**Cause**: CRDs and workloads deployed simultaneously, workloads tried to start before CRDs registered.

**Fix**: Split into two charts (CRDs first, workloads with `dependsOn`).

### ConfigHub labels missing

**Cause**: Labels not present in source manifests.

**Fix**: Ensure ConfigHub adds labels during rendering phase. Check your ConfigHub configuration.

## References

- [Issue #3504: Rendered Helm Charts](https://github.com/confighubai/confighub/issues/3504) - Working implementation
- [Helm Files Documentation](https://helm.sh/docs/chart_template_guide/accessing_files/) - Official Helm docs on `.Files`
- [ConfigHub OCI Integration](./README.md) - Main OCI integration guide
- [Flux HelmRelease Guide](https://fluxcd.io/flux/components/helm/helmreleases/) - Flux documentation

## Alternative Approaches

### Option 1: Direct Kustomization (No Helm)

If you don't need Helm at all, use Flux Kustomization directly:

```yaml
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: monitoring
spec:
  sourceRef:
    kind: OCIRepository
    name: confighub-prod-monitoring
  path: ./
  prune: true
```

**Pros**: Simpler, no Helm layer
**Cons**: No Helm release tracking, no easy rollback

### Option 2: ConfigMap with Raw Manifests

Store rendered content in a ConfigMap and apply with a Job:

**Pros**: Maximum control
**Cons**: Complex, requires custom tooling

**Recommendation**: Use the Files pattern—it's the sweet spot between simplicity and functionality.
