---
name: observe-sveltos
description: 'Use when the user wants to observe Sveltos-managed resources specifically. Natural phrasing: "is this from Sveltos?", "which ClusterProfile deployed this?", "what ConfigMap or Secret supplied this Sveltos policy?", "show me Sveltos Profile ownership", "what does projectsveltos.io/owner-kind mean?". Do NOT load for: generic ownership (use scout-observe), Sveltos authoring/mutation, or Sveltos notifications unless the task is read-only observation.'
phase: cross-cutting
allowed-tools: Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout map *) Bash(cub-scout map *) Bash(cub scout map *) Bash(./cub-scout tree *) Bash(cub-scout tree *) Bash(cub scout tree *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get --show-managed-fields *)
---

# observe-sveltos

The controller-specific observer skill for **Sveltos**. cub-scout classifies
Sveltos-deployed resources from explicit `projectsveltos.io/*` provenance
annotations and classifies Sveltos CRDs from their API groups.

Verified against `pkg/agent/ownership.go` `detectSveltosOwnership`,
`pkg/agent/manager_strings.go` `ManagerSveltosApplyPatch`, and upstream
Sveltos sources:

- `projectsveltos/libsveltos` `api/v1beta1` and `lib/deployer`
- `projectsveltos/addon-controller` `api/v1beta1`

## When To Use

- "Which ClusterProfile/Profile deployed this resource?"
- "What does `projectsveltos.io/owner-kind` mean here?"
- "Which ConfigMap or Secret did Sveltos use as the policy source?"
- "Is this Sveltos control-plane resource a Profile, ClusterProfile, HealthCheck, or EventSource?"
- "Show Sveltos-managed workloads."

## Do Not Load For

- Generic ownership detection - use `scout-observe`
- Mutating Sveltos configuration
- Treating Sveltos notifications as deployment proof

## Detection Signals

| Signal | SubType | Confidence | Notes |
|---|---|---|---|
| `projectsveltos.io/owner-kind` + `projectsveltos.io/owner-name` annotations | lowercased owner kind | high | Primary deployed-resource provenance. Usually `ClusterProfile` or `Profile`. |
| `projectsveltos.io/deployed-by-sveltos` annotation | `deployed-resource` | medium | Safe fallback when owner kind/name are absent. |
| OwnerRef API group `config.projectsveltos.io`, `lib.projectsveltos.io`, or `extension.projectsveltos.io` | lowercased owner kind | high | Sveltos-generated child link. |
| Own API group `config.projectsveltos.io`, `lib.projectsveltos.io`, or `extension.projectsveltos.io` | lowercased resource kind | high | Sveltos CRDs themselves. |

The `projectsveltos.io/reference-kind`, `reference-name`, and
`reference-namespace` annotations identify the ConfigMap or Secret that supplied
the policy/template. They explain source context; they are not the owner.

## Main Constructs

- `ClusterProfile` / `Profile`: tell Sveltos what to deploy where.
- `ClusterHealthCheck` / `HealthCheck`: notification and health-evaluation inputs.
- `EventSource` / event-triggered flows: react to events and may drive follow-on deployment.

Notifications and event sources are Sveltos resources, but they do not prove a
workload was Sveltos-deployed. For workload ownership, look for the deployed
resource annotations or Sveltos owner references.

## Manager String

Sveltos applies deployed policy resources with:

| Manager | Match | Source |
|---|---|---|
| `application/apply-patch` | exact | `projectsveltos/libsveltos` `lib/deployer.UpdateResource` |

`controllerManagersForOwner(OwnerSveltos, ...)` returns only this verified
writer. Other Sveltos-looking manager strings fall through to `unknown` until
verified.

## Worked Example

```bash
kubectl get deploy api -n prod -o yaml
```

Relevant metadata:

```yaml
metadata:
  annotations:
    projectsveltos.io/owner-kind: ClusterProfile
    projectsveltos.io/owner-name: config-to-production
    projectsveltos.io/reference-kind: ConfigMap
    projectsveltos.io/reference-name: webster-production
    projectsveltos.io/reference-namespace: control-clusters-config
```

cub-scout reads this as `OwnerSveltos`, `SubType=clusterprofile`,
`Name=config-to-production`. The reference annotations explain that the deployed
resource came from the named ConfigMap.
