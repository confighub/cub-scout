# ConfigHub Agent Architecture

The Agent is a protocol, not just a tool. This document describes that protocol: a stable contract for representing GitOps state that other tools can build on.

---

## The Problem

Every GitOps tool has its own state model:
- Flux knows about Kustomizations and HelmReleases
- Argo CD knows about Applications
- Helm knows about Releases
- kubectl knows about raw resources

**There's no universal representation of GitOps state.**

The Agent solves this by defining:

1. **GSF (GitOps State Format)** — A universal schema for representing resources, ownership, drift, relations, and findings
2. **Detection contracts** — How ownership is determined (with priority and confidence)
3. **Output contracts** — How state is transmitted (snapshot, streaming, API)
4. **Extension contracts** — How third parties add detectors, CCVEs, and outputs

### What You Can Build With GSF

GSF is valuable because it enables things that weren't possible when every tool had its own state model:

| Use Case | What It Enables |
|----------|-----------------|
| **Tool migration** | Export Argo CD state, generate Flux resources. GSF captures source refs, so you know where everything came from. |
| **Unified dashboards** | One Grafana dashboard across Flux, Argo, Helm clusters. Query GSF, not N different APIs. |
| **AI infrastructure agents** | LLMs need structured context. GSF gives them ownership, relationships, and drift — not just raw YAML. |
| **Compliance reporting** | "Show me everything without resource limits" works across all deployers, all clusters. |
| **Incident correlation** | Connect "config changed at 2:15" to "alert fired at 2:17" via GSF timestamps and provenance. |
| **Custom alerting** | Webhook on `finding.created` → Slack/PagerDuty. No polling, no scraping. |

**Potential examples we'd like to see built:**
- `gsf-to-flux`: Generate Flux Kustomizations from GSF export (tool migration)
- `gsf-grafana`: Grafana data source that queries GSF snapshots
- `gsf-diff`: Compare two GSF snapshots, show what changed
- `gsf-llm-context`: Format GSF for LLM consumption (structured prompts)

## The Contract

```
┌─────────────────────────────────────────────────────────────────────┐
│                      THE AGENT CONTRACT                              │
│                                                                      │
│  Input:    Kubernetes API (informers)                               │
│  Process:  Ownership detection + CCVE scanning                      │
│  Output:   GSF (GitOps State Format)                                │
│                                                                      │
│  Guarantees:                                                         │
│  • Schema stability (GSF 1.x is backwards compatible)               │
│  • Detection priority is deterministic                               │
│  • Extensions don't affect core behavior                            │
│  • Output is eventually consistent with cluster state               │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Kubernetes Cluster                               │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ ConfigHub Agent (read-only)                                   │  │
│  │                                                               │  │
│  │  ┌──────────┐   ┌───────────┐   ┌──────────┐   ┌──────────┐  │  │
│  │  │ Watcher  │ → │ Ownership │ → │  CCVE    │ → │  Output  │  │  │
│  │  │(informer)│   │ Detector  │   │ Scanner  │   │ (GSF)    │  │  │
│  │  └──────────┘   └───────────┘   └──────────┘   └──────────┘  │  │
│  │                                                       │       │  │
│  │                                                       ▼       │  │
│  │                                      ┌────────────────────┐   │  │
│  │                                      │ stdout / ConfigHub │   │  │
│  │                                      └────────────────────┘   │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                              ↓ watches                               │
│  Deployments, Services, ConfigMaps, Pods, Flux CRDs, Argo CRDs...   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Core Principles

1. **Read-only by default** — Core scanning uses only `get`, `list`, `watch`. Exception: `import-argocd` can modify ArgoCD Applications when explicitly requested.
2. **Protocol-first** — GSF is the contract. Tools build on it.
3. **Standalone-capable** — Works without ConfigHub connection.
4. **Deletable** — No state stored. Remove anytime.

---

## GitOps State Format (GSF)

GSF is the universal output format for GitOps state. It's what the Agent produces and what tools consume.

### Schema (TypeScript)

```typescript
interface GSFSnapshot {
  version: "1.0";
  cluster: string;
  timestamp: string;  // ISO 8601
  entries: GSFEntry[];
  relations: GSFRelation[];
  findings: GSFFinding[];
}

interface GSFEntry {
  // Identity
  id: string;                    // "cluster/namespace/Kind/name"
  cluster: string;
  namespace: string;
  kind: string;
  name: string;
  apiVersion: string;

  // Ownership
  owner: GSFOwner;

  // State
  status: "healthy" | "degraded" | "progressing" | "unknown";
  conditions: GSFCondition[];

  // Drift
  drift?: GSFDrift;

  // Metadata
  labels: Record<string, string>;
  annotations: Record<string, string>;
  createdAt: string;
  modifiedAt: string;
}

interface GSFOwner {
  type: "flux" | "argocd" | "helm" | "terraform" | "confighub" | "native" | "unknown";
  ref?: string;                  // e.g., "kustomization/apps"
  labels?: Record<string, string>;
}

interface GSFRelation {
  from: string;                  // Entry ID
  to: string;                    // Entry ID
  type: "owned-by" | "selects" | "mounts" | "references" | "depends-on";
}

interface GSFDrift {
  detected: boolean;
  fields: GSFDriftField[];
}

interface GSFDriftField {
  path: string;                  // JSON path, e.g., "spec.replicas"
  desired: any;
  live: any;
}

interface GSFFinding {
  id: string;                    // CCVE ID, e.g., "CCVE-2025-0027"
  severity: "critical" | "warning" | "info";
  category: "SOURCE" | "RENDER" | "APPLY" | "DRIFT" | "DEPEND" | "STATE" | "ORPHAN" | "CONFIG";
  resource: string;              // Entry ID
  message: string;
  remediation: string;
}

interface GSFCondition {
  type: string;
  status: "True" | "False" | "Unknown";
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}
```

### JSON Schema

See [gsf-schema.json](./gsf-schema.json) for the full JSON Schema.

### Example Output

```json
{
  "version": "1.0",
  "cluster": "prod-east",
  "timestamp": "2025-01-02T18:30:00Z",
  "entries": [
    {
      "id": "prod-east/default/Deployment/nginx",
      "cluster": "prod-east",
      "namespace": "default",
      "kind": "Deployment",
      "name": "nginx",
      "apiVersion": "apps/v1",
      "owner": {
        "type": "flux",
        "ref": "kustomization/apps"
      },
      "status": "healthy",
      "conditions": [
        {
          "type": "Available",
          "status": "True"
        }
      ],
      "drift": {
        "detected": true,
        "fields": [
          {
            "path": "spec.replicas",
            "desired": 2,
            "live": 3
          }
        ]
      },
      "labels": {
        "app": "nginx"
      },
      "annotations": {},
      "createdAt": "2024-12-01T10:00:00Z",
      "modifiedAt": "2025-01-02T18:00:00Z"
    }
  ],
  "relations": [
    {
      "from": "prod-east/default/Deployment/nginx",
      "to": "prod-east/default/Service/nginx",
      "type": "selects"
    }
  ],
  "findings": [
    {
      "id": "CCVE-2025-0011",
      "severity": "warning",
      "category": "DRIFT",
      "resource": "prod-east/default/Deployment/nginx",
      "message": "Manual kubectl edit detected - replicas changed from 2 to 3",
      "remediation": "Accept drift with 'cub drift accept' or revert with 'cub drift revert'"
    }
  ]
}
```

---

## Output Modes

The Agent supports multiple output modes:

| Mode | Command | Description |
|------|---------|-------------|
| **Snapshot** | `cub-scout snapshot -o -` | One-time GSF dump to stdout |
| **Map** | `cub-scout map list --json` | Resource ownership list |
| **Fleet** | `cub-scout map fleet` | Fleet view via cub CLI |
| **Deep Dive** | `cub-scout map deep-dive` | All cluster data sources with LiveTree |
| **App Hierarchy** | `cub-scout map app-hierarchy` | Inferred ConfigHub Units with workloads |

### Deep Dive and App Hierarchy

These commands provide maximum detail for cluster analysis:

- **deep-dive**: Shows ALL data sources (Flux CRDs, ArgoCD Applications, Helm releases, workloads) with full details including LiveTree (Deployment → ReplicaSet → Pod)
- **app-hierarchy**: Shows the inferred ConfigHub model with Units mapped to deployers, namespace analysis, and ownership graph

Both use **deterministic rule-based logic** (no AI) for inference:
- Ownership via label lookups (`kustomize.toolkit.fluxcd.io/name`, etc.)
- Environment inference via string matching (`prod`, `staging`, `dev`)
- Workload-to-deployer linking via labels and ownerReferences

---

## Workload Import

The `cub-scout import` command bridges standalone and connected modes by adopting existing workloads into ConfigHub:

```bash
# Preview what would be imported
cub-scout import --namespace my-app --dry-run

# Import workloads into current ConfigHub space
cub-scout import --namespace my-app --yes

# Import into a specific space
cub-scout import --namespace my-app --space production
```

### Import Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                         IMPORT FLOW                                  │
│                                                                      │
│  1. Discover    Scan namespace for Deployments, StatefulSets,       │
│                 DaemonSets                                           │
│                                                                      │
│  2. Detect      Check for existing confighub.com/UnitSlug           │
│                 (labels OR annotations)                              │
│                                                                      │
│  3. Create      For new workloads: cub unit create <name>           │
│                                                                      │
│  4. Label       Apply confighub.com/UnitSlug=<slug> label           │
│                                                                      │
│  5. Verify      Re-run map to confirm connected mode                │
└─────────────────────────────────────────────────────────────────────┘
```

### Already Connected

If workloads are already connected (have `confighub.com/UnitSlug` label or annotation), the import command offers to check for drift instead of re-importing.

---

## Ownership Detection

The Agent detects ownership by examining labels and annotations:

| Owner | Detection Method | Priority |
|-------|-----------------|----------|
| **ConfigHub** | `confighub.com/UnitSlug` label | 1 (highest) |
| **Flux Kustomize** | `kustomize.toolkit.fluxcd.io/name` label | 2 |
| **Flux Helm** | `helm.toolkit.fluxcd.io/name` label | 2 |
| **Argo CD** | `argocd.argoproj.io/instance` label | 2 |
| **Helm** | `app.kubernetes.io/managed-by: Helm` | 3 |
| **Terraform** | `app.terraform.io/workspace-name` annotation | 3 |
| **Native** | Has OwnerReferences | 4 |
| **Unknown** | No ownership markers | 5 (lowest) |

### Adding Custom Ownership Detectors

See [EXTENDING.md](EXTENDING.md) for how to add custom ownership detection.

---

## Resource Types Watched

### Core API
- Pods, Services, ConfigMaps, Secrets, ServiceAccounts, Namespaces

### Apps
- Deployments, StatefulSets, DaemonSets, ReplicaSets

### Batch
- Jobs, CronJobs

### Networking
- Ingresses, NetworkPolicies

### Flux CD
- GitRepositories, OCIRepositories, Buckets
- Kustomizations, HelmReleases
- HelmRepositories, HelmCharts

### Argo CD
- Applications, ApplicationSets

### Adding Custom Resources

See [EXTENDING.md](EXTENDING.md) for watching custom CRDs.

---

## RBAC Requirements

Minimal read-only permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: confighub-agent
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["get", "list", "watch"]
```

For a more restrictive policy, see [manifests/agent-minimal-rbac.yaml](../manifests/agent-minimal-rbac.yaml).

---

## Performance

| Metric | Value |
|--------|-------|
| Memory (idle) | ~50MB |
| Memory (10k resources) | ~200MB |
| CPU (idle) | <0.1 core |
| CPU (initial sync) | ~0.5 core |
| Startup time | <5 seconds |

---

## See Also

- [EXTENDING.md](EXTENDING.md) — Extension points and customization
- [CLI-REFERENCE.md](CLI-REFERENCE.md) — CLI reference and configuration
- [CCVE-GUIDE.md](CCVE-GUIDE.md) — CCVE detection and remediation
