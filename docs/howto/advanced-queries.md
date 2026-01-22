# Enhanced Query Features

This document covers the advanced query capabilities in cub-scout for finding relationships, detecting drift, and identifying dangling references.

## Reverse Trace

Trace any resource backwards to its Git source, automatically detecting the GitOps tool (Flux, Argo, or Helm).

### Usage

```bash
# Trace a pod to its Git source
cub-scout trace pod/nginx-7d9b8c-x4k2p -n prod

# Trace a deployment
cub-scout trace deployment/api -n prod

# JSON output for scripting
cub-scout trace deployment/api -n prod --json
```

### Example Output

```
Target: Deployment/nginx (ns: prod)

Owner Chain:
  ✓ Deployment/nginx [3/3 ready]

GitOps Chain:
  ✓ Kustomization/apps
  ✓ GitRepository/infra
    URL: https://github.com/acme/infra.git
    Revision: main@abc123f

Ownership: flux (apps)

✓ Fully managed (traced to Git source)
```

### Key Features

- **Auto-detection**: Automatically detects if resource is managed by Flux, Argo CD, or Helm
- **Full chain**: Walks ownerReferences (Pod → ReplicaSet → Deployment) before tracing to GitOps
- **Broken link detection**: Identifies missing sources or broken references
- **Cross-tool support**: Works in mixed Flux + Argo environments

---

## Relationship Queries

Find all resources that depend on or reference a given resource.

### Usage

```bash
# Find what uses a ConfigMap
cub-scout query refs configmap/app-config -n prod

# Find what uses a Secret
cub-scout query refs secret/db-creds -n prod

# Find what uses a Service
cub-scout query refs service/api -n prod

# Find what references a PVC
cub-scout query refs pvc/data-volume -n prod
```

### Example Output

```
$ cub-scout query refs configmap/app-config -n prod

References to ConfigMap/app-config:

  Deployment/frontend (ns: prod)
    Type: volume
    Path: spec.template.spec.volumes[].configMap

  Deployment/backend (ns: prod)
    Type: envFrom
    Path: spec.template.spec.containers[].envFrom[].configMapRef

  Deployment/worker (ns: prod)
    Type: env
    Path: spec.template.spec.containers[].env[].valueFrom.configMapKeyRef

Impact: 3 resources depend on ConfigMap/app-config
```

### Supported Reference Types

| Source | Target | Reference Type |
|--------|--------|----------------|
| Deployment/StatefulSet | ConfigMap | volume, envFrom, env |
| Deployment/StatefulSet | Secret | volume, envFrom, env |
| Ingress | Service | backend |
| Ingress | Secret | tls |
| HPA | Deployment/StatefulSet | scaleTarget |
| PDB | Pods | selector |
| ServiceAccount | Secret | imagePullSecret |

---

## Dangling Reference Detection

Find resources that reference targets that no longer exist.

### Usage

```bash
# Find all dangling references in a namespace
cub-scout query dangling -n prod

# Find all dangling references cluster-wide
cub-scout query dangling

# JSON output
cub-scout query dangling -n prod --json
```

### Example Output

```
$ cub-scout query dangling -n prod

Dangling References Found: 4

  Service/old-api (ns: prod)
    → Pod (selector: app=old-api)
    Reason: no matching pods
    Suggestion: Check if the deployment exists and has matching labels

  HorizontalPodAutoscaler/worker-hpa (ns: prod)
    → Deployment/worker-v1
    Reason: target not found
    Suggestion: Create Deployment/worker-v1 or delete this HPA

  Ingress/legacy (ns: prod)
    → Service/legacy-svc
    Reason: service not found
    Suggestion: Create Service/legacy-svc or update Ingress backend

  PersistentVolumeClaim/old-data (ns: prod)
    → Pod (none)
    Reason: not mounted by any pod
    Suggestion: Delete if no longer needed (may contain data!)
```

### What's Detected

| Resource Type | Check |
|---------------|-------|
| Service | No pods match selector |
| HPA | Scale target doesn't exist |
| Ingress | Backend service doesn't exist |
| PDB | No pods match selector |
| PVC | Not mounted by any pod |

---

## Drift Detection

Detect resources that have drifted from their declared state (comparing live state vs. `kubectl.kubernetes.io/last-applied-configuration` annotation).

### Usage

```bash
# Find drifted resources in a namespace
cub-scout query drifted -n prod

# Find drifted resources cluster-wide
cub-scout query drifted

# JSON output
cub-scout query drifted -n prod --json
```

### Example Output

```
$ cub-scout query drifted -n prod

Found 3 drifted resources:

Deployment/nginx (ns: prod)
  spec.replicas:
    declared: 3
    live:     5

ConfigMap/feature-flags (ns: prod)
  data.new_feature:
    declared: <not set>
    live:     "enabled"

Service/api (ns: prod)
  spec.ports[0].port:
    declared: 8080
    live:     9090
```

### How It Works

1. Reads the `kubectl.kubernetes.io/last-applied-configuration` annotation
2. Compares against live resource state
3. Ignores expected differences (resourceVersion, uid, managedFields, status)
4. Reports meaningful changes

### Limitations

- Only works for resources with `last-applied-configuration` annotation
- Server-side apply may not always set this annotation
- Some controller-managed fields are expected to change

---

## Context Snapshot (AI Mode)

Generate a cluster state snapshot optimized for AI/LLM consumption.

### Usage

```bash
# Generate snapshot for a namespace
cub-scout context snapshot -n prod

# Generate snapshot for entire cluster
cub-scout context snapshot

# JSON output (default for AI consumption)
cub-scout context snapshot -n prod --format json
```

### Example Output

```json
{
  "snapshot_time": "2026-01-14T15:30:00Z",
  "cluster": "prod-east",
  "namespace": "prod",
  "summary": {
    "total_resources": 47,
    "healthy": 42,
    "degraded": 3,
    "critical": 2,
    "unmanaged": 8
  },
  "critical_issues": [
    {
      "resource": "Deployment/payment-api",
      "namespace": "prod",
      "issue": "0/3 replicas ready",
      "since": "15m ago",
      "owner": "Flux/Kustomization/apps",
      "explanation": "ImagePullBackOff - image tag v2.3.1 not found"
    }
  ],
  "recent_changes": [
    {
      "time": "12m ago",
      "resource": "Deployment/payment-api",
      "change": "ScalingReplicaSet: Scaled up to 3",
      "source": "Kubernetes"
    }
  ],
  "ownership_breakdown": {
    "flux": 25,
    "argo": 12,
    "helm": 2,
    "Native": 8
  },
  "dependency_graph": {
    "Deployment/payment-api/prod": {
      "depends_on": ["ConfigMap/payment-config", "Secret/db-creds"],
      "depended_by": ["Service/payment-api", "HPA/payment-api"]
    }
  }
}
```

### Use Cases

- **AI Agents**: Feed cluster state to LLM-based assistants
- **Monitoring**: Quick health summary
- **Troubleshooting**: Identify issues and their dependencies
- **Compliance**: Audit ownership breakdown

---

## Combining Queries

Queries can be combined with the existing filter syntax:

```bash
# Find unmanaged resources with issues
cub-scout map list -q "owner=Native" | cub-scout query dangling

# Find drifted Flux-managed resources
cub-scout query drifted -n prod | grep -E "flux|Flux"

# Trace all critical issues
cub-scout context snapshot -n prod | jq -r '.critical_issues[].resource' | xargs -I {} cub-scout trace {} -n prod
```

---

## Implementation Details

### Package Structure

```
pkg/agent/
├── reverse_trace.go      # Reverse trace implementation
├── context_snapshot.go   # AI-friendly cluster snapshot

pkg/query/
├── query.go              # Core query parsing (existing)
├── relationships.go      # Reference finder
├── dangling.go           # Dangling reference detection
├── drift.go              # Drift detection via last-applied
```

### API Reference

```go
// Reverse Trace
tracer := agent.NewReverseTracer(dynamicClient)
result, err := tracer.Trace(ctx, "Deployment", "nginx", "prod")
fmt.Println(result.FormatChain())

// Relationship Query
finder := query.NewRelationshipFinder(dynamicClient)
refs, err := finder.FindReferences(ctx, "ConfigMap", "app-config", "prod")

// Dangling Detection
finder := query.NewDanglingFinder(dynamicClient)
dangling, err := finder.FindAll(ctx, "prod")

// Drift Detection
detector := query.NewDriftDetector(dynamicClient)
drifted, err := detector.FindDriftedResources(ctx, "prod")

// Context Snapshot
builder := agent.NewContextSnapshotBuilder(dynamicClient, "prod-east")
snapshot, err := builder.Build(ctx, "prod")
json, _ := snapshot.ToJSON()
```
