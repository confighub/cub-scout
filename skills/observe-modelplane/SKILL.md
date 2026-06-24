---
name: observe-modelplane
description: 'Use when the user wants to observe Modelplane-managed resources specifically. Natural phrasing: "is this from Modelplane?", "which ModelDeployment produced this replica?", "show me Modelplane endpoints", "what does modelplane.ai/deployment mean?", "is this Modelplane or generic Crossplane?". Do NOT load for: generic ownership (use scout-observe), Crossplane-only resources (use observe-crossplane), or authoring Modelplane APIs.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout tree *) Bash(cub-scout tree *) Bash(cub scout tree *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *)
---

# observe-modelplane

The controller-specific observer skill for **Modelplane**. Modelplane is built
on Crossplane, but cub-scout classifies Modelplane as the higher-level platform
owner when exact Modelplane API groups or identity labels are present.

Verified against `pkg/agent/ownership.go` `detectModelplaneOwnership`,
`pkg/agent/manager_strings.go` `OwnerModelplane`, and
`modelplaneai/modelplane` API definitions and composition functions.

## When To Use

- "Is this resource Modelplane-owned or generic Crossplane?"
- "Which ModelDeployment produced this ModelReplica or endpoint?"
- "What does `modelplane.ai/deployment` mean?"
- "Show Modelplane ModelDeployment / ModelService / ModelEndpoint resources."
- "Which InferenceCluster or ModelCache is this resource related to?"

## Do Not Load For

- Generic ownership detection - use `scout-observe`
- Crossplane-only resources with no Modelplane API group or labels - use `observe-crossplane`
- Authoring or mutating Modelplane resources

## Detection Signals

| Signal | SubType | Confidence | Notes |
|---|---|---|---|
| Own API group `modelplane.ai` | lowercased resource kind | high | Platform and model APIs such as `InferenceCluster`, `ModelDeployment`, `ModelService`, `ModelEndpoint`, `ModelCache`. |
| Own API group `infrastructure.modelplane.ai` | lowercased resource kind | high | Composed infrastructure APIs such as `EKSCluster`, `GKECluster`, `ServingStack`. |
| `modelplane.ai/deployment` label | `modeldeployment` | high | Deployment identity for replicas/endpoints. |
| `modelplane.ai/modelcache` label | `modelcache` | high | Cache hydration resources. |
| `modelplane.ai/serving` label | `modelreplica` | high | Serving identity stamped on remote workload pods/services. |
| `modelplane.ai/workload` label | `workload` | medium | Per-engine workload selector. |
| `modelplane.ai/cluster` label | `inferencecluster` | medium | Cluster relationship for replicas/endpoints. |
| `modelplane.ai/release` label | `release` | medium | Modelplane-composed Helm Release metadata, such as gateway installs. |
| `modelplane.ai/resource` label | `resource` | medium | Generic composed-resource identity for serving substrate pieces. |
| `modelplane.ai/usage-consumer` label | `usage-consumer` | medium | ProviderConfig protection/usage wiring. |
| OwnerRef API group `modelplane.ai` or `infrastructure.modelplane.ai` | lowercased owner kind | high | Modelplane owner-reference fallback. |

Broad labels such as `modelplane.ai/region`, `modelplane.ai/gpu`, and
`modelplane.ai/pool` are placement or selection metadata. cub-scout does not
classify ownership from those labels alone.

## Main Constructs

- Platform-authored: `InferenceGateway`, `InferenceClass`, `InferenceCluster`
- Model-team authored: `ModelDeployment`, `ModelService`, `ModelEndpoint`, `ModelCache`
- Composed/generated: `ModelReplica`, `EKSCluster`, `GKECluster`, `ServingStack`

## Manager Strings

Modelplane composition is implemented through Crossplane, so Modelplane-owned
composed resources may carry the verified Crossplane manager strings:

- `apiextensions.crossplane.io/composite`
- `apiextensions.crossplane.io/composed-<hash>`
- `apiextensions.crossplane.io/claim`
- `apiextensions.crossplane.io/managed`
- `managed.crossplane.io/api-simple-reference-resolver`

`controllerManagersForOwner(OwnerModelplane, ...)` accepts those Crossplane
manager strings only when ownership detection has already found Modelplane.

## Worked Example

```yaml
apiVersion: modelplane.ai/v1alpha1
kind: ModelEndpoint
metadata:
  name: qwen-demo-0
  namespace: ml-team
  labels:
    modelplane.ai/deployment: qwen-demo
    modelplane.ai/cluster: prod-us-east
```

cub-scout reads this as `OwnerModelplane`, `SubType=modeldeployment`,
`Name=qwen-demo`. The `modelplane.ai/cluster` label remains useful context, but
the deployment label is the higher-confidence ownership signal.
