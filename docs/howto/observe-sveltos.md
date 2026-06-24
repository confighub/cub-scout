# Observe Sveltos

This guide shows how cub-scout observes Sveltos-managed resources without modifying the cluster.

Sveltos support is based on Kubernetes API objects and metadata only. cub-scout does not infer ownership from names or guesses.

If you want more than is shown on this page, such as parity with other supported GitOps tools, contact us.

## What cub-scout Understands

Sveltos has three main surfaces that matter for observation:

| Construct | Meaning in cub-scout |
|-----------|----------------------|
| `ClusterProfile` | Cluster-scoped Sveltos deployment intent |
| `Profile` | Namespaced Sveltos deployment intent |
| `EventSource` / `EventTrigger` | Event-driven Sveltos behavior |
| `ClusterHealthCheck` | Health/notification configuration |
| deployed resources | Workloads or config objects annotated by Sveltos |

Sveltos notifications are treated as observability/eventing configuration. They are not treated as proof that Sveltos deployed a workload.

## Ownership Detection

The strongest Sveltos signal is on deployed resources:

```yaml
metadata:
  annotations:
    projectsveltos.io/owner-kind: ClusterProfile
    projectsveltos.io/owner-name: config-to-production
    projectsveltos.io/reference-kind: ConfigMap
    projectsveltos.io/reference-name: webster-production
    projectsveltos.io/reference-namespace: control-clusters-config
```

cub-scout reads this as:

- owner: `Sveltos`
- deployer: `ClusterProfile/config-to-production`
- reference source: `ConfigMap/control-clusters-config/webster-production`

Fallback signals:

- `projectsveltos.io/deployed-by-sveltos`
- Sveltos API groups: `config.projectsveltos.io`, `lib.projectsveltos.io`, `extension.projectsveltos.io`
- owner references to those API groups

## Commands

List Sveltos-owned resources:

```bash
./cub-scout map list --owner Sveltos
```

Trace a deployed resource:

```bash
./cub-scout trace deployment/web -n prod
```

Expected chain shape:

```text
SveltosReference/ConfigMap/control-clusters-config/webster-production
  -> ClusterProfile/config-to-production
    -> Deployment/web
```

Show controller activity:

```bash
./cub-scout map activity --owner Sveltos
```

Open the interactive TUI:

```bash
./cub-scout map
```

The TUI includes Sveltos controller objects in the pipeline/trace picker and Sveltos workloads in ownership views.

## Doctor

`doctor` reports Sveltos as a first-class ownership bucket:

```bash
./cub-scout doctor --format json
```

Look for:

```json
{
  "ownership": {
    "sveltos": 3
  }
}
```

## Safe Degradation

If the deployed resource has Sveltos owner annotations but the `ClusterProfile` or `Profile` object is not readable, cub-scout still shows the annotation-proven owner link and marks the missing object context explicitly. It does not mark the resource as Native/orphan.
