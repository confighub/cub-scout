# Observe Modelplane

This guide shows how cub-scout observes Modelplane-managed resources without modifying the cluster.

Modelplane support is based on Modelplane API groups, explicit Modelplane labels, owner references, and status. Broad scheduling labels are not treated as workload ownership proof.

## What cub-scout Understands

Modelplane API groups:

| API group | Kinds |
|-----------|-------|
| `modelplane.ai/v1alpha1` | `InferenceGateway`, `InferenceClass`, `InferenceCluster`, `ModelDeployment`, `ModelService`, `ModelEndpoint`, `ModelCache`, `ModelReplica` |
| `infrastructure.modelplane.ai/v1alpha1` | `EKSCluster`, `GKECluster`, `ServingStack` |

Modelplane ownership labels:

| Label | Meaning |
|-------|---------|
| `modelplane.ai/deployment` | points to a `ModelDeployment` |
| `modelplane.ai/modelcache` | points to a `ModelCache` |
| `modelplane.ai/serving` | points to Modelplane serving/replica context |
| `modelplane.ai/cluster` | points to an `InferenceCluster` |

Labels such as `modelplane.ai/region`, `modelplane.ai/pool`, or GPU/placement labels are scheduling metadata. cub-scout does not treat them as ownership by themselves.

## Commands

List Modelplane-owned resources:

```bash
./cub-scout map list --owner Modelplane
```

Trace a workload composed for a model deployment:

```bash
./cub-scout trace deployment/qwen-engine -n models
```

Expected chain shape:

```text
ModelDeployment/qwen
  -> Deployment/qwen-engine
```

When related Modelplane resources are readable, the `ModelDeployment` link includes composed children such as `ModelReplica` and `ModelEndpoint`.

Show controller activity:

```bash
./cub-scout map activity --owner Modelplane
```

Open the interactive TUI:

```bash
./cub-scout map
```

The TUI includes Modelplane API resources in the pipeline/trace picker and Modelplane-owned workloads in ownership views.

## Doctor

`doctor` reports Modelplane as a first-class ownership bucket:

```bash
./cub-scout doctor --format json
```

Look for:

```json
{
  "ownership": {
    "modelplane": 4
  }
}
```

## Safe Degradation

If a workload has a Modelplane ownership label but the referenced `ModelDeployment`, `ModelCache`, or `InferenceCluster` is not readable, cub-scout still shows the label-proven owner link. It does not downgrade the workload to Native/orphan and does not use broad placement labels as ownership evidence.
