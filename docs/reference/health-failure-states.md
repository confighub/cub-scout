# Health & Failure States Reference

This document defines how cub-scout determines health and failure states
for Kubernetes resources and GitOps deployers.

**This is the authoritative reference for v0.5.**

---

## Core Status Values

cub-scout uses five status values:

| Status | Meaning | Display |
|--------|---------|---------|
| **Ready** | Resource is operational and serving | Green checkmark |
| **NotReady** | Resource exists but is not fully operational | Yellow warning |
| **Failed** | Resource has failed and needs attention | Red X |
| **Pending** | Resource is being created or reconciled | Gray pending |
| **Unknown** | Status cannot be determined | No indicator |

---

## Status Detection by Resource Type

### Generic Kubernetes Resources

Uses the standard `status.conditions` array:

| Condition | Status | Result |
|-----------|--------|--------|
| `type: Ready, status: "True"` | Ready | Resource is operational |
| `type: Ready, status: "False"` | NotReady | Resource is not operational |
| `type: Ready, status: "Unknown"` | Pending | Status indeterminate |
| No conditions | Unknown | No status information |

**Source:** `internal/mapsvc/status.go`

---

### Deployments

| K8s Signal | Description |
|------------|-------------|
| `spec.replicas` | Desired replica count (default: 1) |
| `status.replicas` | Total replicas |
| `status.readyReplicas` | Replicas ready to serve |
| `status.availableReplicas` | Replicas available to users |
| `status.updatedReplicas` | Replicas at current spec |

**Status Logic:**

| Condition | Status |
|-----------|--------|
| `readyReplicas >= desired AND availableReplicas >= desired` | Ready |
| `updatedReplicas < desired OR readyReplicas < desired` | NotReady |
| `replicas == 0 AND desired > 0` | Pending |
| Other | Unknown |

---

### StatefulSets

| K8s Signal | Description |
|------------|-------------|
| `spec.replicas` | Desired replica count (default: 1) |
| `status.replicas` | Current replicas |
| `status.readyReplicas` | Ready replicas |

**Status Logic:**

| Condition | Status |
|-----------|--------|
| `readyReplicas == desired` | Ready |
| `readyReplicas < desired` | NotReady |
| `replicas == 0 AND desired > 0` | Pending |

---

### DaemonSets

| K8s Signal | Description |
|------------|-------------|
| `status.desiredNumberScheduled` | Target node count |
| `status.numberReady` | Nodes with ready pods |

**Status Logic:**

| Condition | Status |
|-----------|--------|
| `numberReady == desiredNumberScheduled > 0` | Ready |
| `numberReady < desiredNumberScheduled` | NotReady |
| `numberReady == 0` | Pending |

---

### Pods

| K8s Signal | Description |
|------------|-------------|
| `status.phase` | Pod lifecycle phase |
| `status.containerStatuses[*].state.waiting.reason` | Container wait reason |

**Status Logic:**

| Condition | Status |
|-----------|--------|
| Phase is "Running" or "Succeeded" | Ready |
| Phase is "Failed" | Failed |
| Phase is "Pending" | Pending |
| Container in "CrashLoopBackOff" | Failed |
| Container in "ImagePullBackOff" | Failed |
| Other | Unknown |

---

### Jobs

| K8s Signal | Description |
|------------|-------------|
| `status.succeeded` | Successful completions |
| `status.failed` | Failed completions |

**Status Logic:**

| Condition | Status |
|-----------|--------|
| `succeeded > 0` | Ready |
| `failed > 0` | Failed |
| Neither | Pending |

---

## GitOps Deployer Status

### Flux Kustomization / HelmRelease

Flux resources use standard Kubernetes conditions with the `Ready` type.

| Signal | Source |
|--------|--------|
| `status.conditions[type=Ready]` | Primary status |
| `status.conditions[*].reason` | Failure reason |

**Status Indicators (from condition reason/message):**

Ready indicators:
- "applied", "succeeded", "ready", "up to date", "stored", "artifact is"

Not-ready indicators:
- "failed", "error", "not ready", "stalled", "suspended", "reconciling", "pending"

**Status Logic:**

| Condition | Status |
|-----------|--------|
| `Ready=True` | Ready |
| `Ready=False` with failed/error reason | Failed |
| `Ready=False` other | NotReady |
| No Ready condition | Unknown |

---

### Argo CD Application

| K8s Signal | Values | Description |
|------------|--------|-------------|
| `status.sync.status` | Synced, OutOfSync | Sync state |
| `status.health.status` | Healthy, Progressing, Degraded, Missing, Unknown | Health state |
| `status.operationState.phase` | Running, Error, Failed, Succeeded, Terminating | Operation state |

**Status Logic:**

| Condition | Status |
|-----------|--------|
| `sync=Synced AND health=Healthy` | Ready |
| `health=Degraded OR health=Missing` | Failed |
| `sync=OutOfSync OR health=Progressing` | NotReady |
| Other | Unknown |

---

## Health Assessment Levels

For workload aggregation, cub-scout uses four health levels:

| Level | Meaning | Condition |
|-------|---------|-----------|
| **healthy** | All replicas ready | `readyReplicas == desired AND availableReplicas == desired` |
| **degraded** | Some replicas not ready | `readyReplicas < desired OR availableReplicas < desired` |
| **critical** | No replicas ready | `readyReplicas == 0 AND availableReplicas == 0` (when desired > 0) |
| **unknown** | Cannot determine | No replica information |

**Source:** `pkg/agent/context_snapshot.go`

---

## Silent Failure Detection

cub-scout's scanner detects these silent failure conditions:

### CCVE-2025-0665: Disabled Reconciliation
- **Signal:** `spec.interval: 0` (or unset)
- **Risk:** Resource will never reconcile changes from Git

### CCVE-2025-0662: Optional ValuesFrom Missing
- **Signal:** `spec.valuesFrom[*].optional: true` with missing source
- **Risk:** Helm values silently defaulting, potential misconfiguration

### CCVE-2025-0666: Suspended Source
- **Signal:** Source dependency is suspended
- **Risk:** Resource appears healthy but cannot receive updates

### CCVE-2025-0169: Argo Operation Stuck
- **Signal:** `operationState.phase: Running` > threshold
- **Risk:** Sync operation hung, cluster may be drifting

**Source:** `pkg/agent/state_scanner.go`

---

## TreeNode Status (UI Display)

| Value | Icon | Color | Meaning |
|-------|------|-------|---------|
| `"ok"` | Check | Green | Operational |
| `"warn"` | Warning | Yellow | Degraded/OutOfSync |
| `"error"` | X | Red | Failed |
| `"pending"` | Dots | Gray | In progress |
| `""` | None | Gray | Unknown |

---

## Explicit Unknown States

cub-scout returns "Unknown" rather than guessing when:

| Scenario | Result |
|----------|--------|
| No `status.conditions` present | Unknown |
| No replica information | Unknown (health) |
| Unrecognized phase value | Unknown |
| Missing resource | Error (not Unknown) |

**cub-scout's stance:** When status cannot be determined from available
signals, it reports Unknown. It does not infer health from resource names,
namespace names, or other heuristics.

---

## Status Summary Table

| Resource Type | Primary Signal | Ready | Failed | NotReady | Pending |
|---------------|----------------|-------|--------|----------|---------|
| Generic | conditions[Ready] | True | False+error | False | Unknown |
| Deployment | readyReplicas | All ready | - | Some not ready | 0 replicas |
| StatefulSet | readyReplicas | All ready | - | Some not ready | 0 replicas |
| DaemonSet | numberReady | All ready | - | Some not ready | 0 ready |
| Pod | phase | Running/Succeeded | Failed/CrashLoop | - | Pending |
| Job | succeeded/failed | succeeded>0 | failed>0 | - | Neither |
| Flux | conditions[Ready] | True | False+error | False | - |
| Argo App | sync+health | Synced+Healthy | Degraded/Missing | OutOfSync/Progressing | - |

---

## Related Documentation

- [Reference: Commands](commands.md) - CLI usage
- [How To: Scan for Risks](../howto/scan-for-risks.md) - Using the scanner
- Source: `internal/mapsvc/status.go`, `pkg/agent/state_scanner.go`
