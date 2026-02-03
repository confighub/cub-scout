# cub-scout CLI Guide

Complete reference for all commands, options, TUI keys, and expected outputs.

---

## Top-Level Commands (19)

| Command | Description | Standalone | Connected |
|---------|-------------|:----------:|:---------:|
| `map` | Interactive TUI explorer | Yes | Yes |
| `tree` | Hierarchical views (runtime, git, config) | Yes | Yes |
| `status` | Show connection status, cluster, and worker info | Yes | Yes |
| `gitops status` | Show GitOps pipeline health and failures | Yes | - |
| `debug` | Guided GitOps debugging wizard | Yes | - |
| `discover` | Find workloads (alias for map workloads) | Yes | - |
| `health` | Check for issues (alias for map issues) | Yes | - |
| `trace` | Show GitOps ownership chain | Yes | - |
| `scan` | Scan and score issues | Yes | - |
| `snapshot` | Dump cluster state as JSON | Yes | - |
| `import` | Import workloads into ConfigHub | - | Yes |
| `import-argocd` | Import ArgoCD Application | - | Yes |
| `app-space` | Manage App Spaces | - | Yes |
| `remedy` | Execute CCVE remediation | Yes | - |
| `combined` | Git repo + cluster alignment | Yes | Yes |
| `parse-repo` | Parse GitOps repo structure | Yes | - |
| `demo` | Run interactive demos | Yes | - |
| `version` | Print version | Yes | - |
| `completion` | Generate shell completions | Yes | - |
| `setup` | Set up shell config | Yes | - |

---

## `map` — Interactive TUI

**What it does:** Opens an interactive terminal UI showing all cluster resources grouped by owner.

```bash
./cub-scout map
```

**Without cub-scout:**
```bash
kubectl get all -A -o wide
kubectl get all -A -o json | jq '.items[] | select(.metadata.labels["kustomize.toolkit.fluxcd.io/name"])'
kubectl get all -A -o json | jq '.items[] | select(.metadata.labels["argocd.argoproj.io/instance"])'
# ... and manually correlate results
```

**Expected output:**
```
┌─ cub-scout map ──────────────────────────────────────────────────┐
│ CLUSTER: kind-kind                                               │
├──────────────────────────────────────────────────────────────────┤
│ FLUX (12)         ARGOCD (8)        HELM (3)        NATIVE (45)  │
├──────────────────────────────────────────────────────────────────┤
│ > flux-system/Deployment/source-controller          Flux         │
│   flux-system/Deployment/kustomize-controller       Flux         │
│   argocd/Deployment/argocd-server                   ArgoCD       │
│   monitoring/Deployment/prometheus                  Helm         │
│   default/Deployment/nginx                          Native       │
└──────────────────────────────────────────────────────────────────┘
Press ? for help, q to quit
```

**Options:**
| Option | Description |
|--------|-------------|
| `--hub` | Launch ConfigHub hierarchy TUI (requires `cub auth`) |
| `--json` | Output in JSON format |
| `--verbose` | Show additional details |

---

## `tree` — Hierarchical Views

**What it does:** Shows different hierarchical perspectives on your cluster, Git sources, and ConfigHub units.

```bash
./cub-scout tree              # Runtime: Deployment → ReplicaSet → Pod
./cub-scout tree ownership    # Resources grouped by GitOps owner
./cub-scout tree git          # Git source structure
./cub-scout tree patterns     # Detected GitOps patterns (D2, Arnie, Banko, Fluxy)
./cub-scout tree config       # ConfigHub Unit relationships (wraps cub unit tree)
./cub-scout tree suggest      # Suggested Hub/AppSpace organization
```

**Expected output (runtime):**
```
RUNTIME HIERARCHY (51 Deployments)
════════════════════════════════════════════════════════════════════

NAMESPACE: boutique
────────────────────────────────────────────────────────────────────
├── cart [Flux: apps/boutique] 2/2 ready
│   └── ReplicaSet cart-86f68db776 [2/2]
│       ├── Pod cart-86f68db776-hzqgf  ✓ Running  10.244.0.15  node-1
│       └── Pod cart-86f68db776-mp8kz  ✓ Running  10.244.0.16  node-2
│
├── checkout [Flux: apps/boutique] 1/1 ready
│   └── ReplicaSet checkout-5d8f9c7b4 [1/1]
│       └── Pod checkout-5d8f9c7b4-abc12  ✓ Running  10.244.0.17  node-1
│
└── frontend [Flux: apps/boutique] 3/3 ready
    └── ReplicaSet frontend-8e6f7a9c2 [3/3]
        ├── Pod frontend-8e6f7a9c2-def34  ✓ Running  10.244.0.18  node-1
        ├── Pod frontend-8e6f7a9c2-ghi56  ✓ Running  10.244.0.19  node-2
        └── Pod frontend-8e6f7a9c2-jkl78  ✓ Running  10.244.0.20  node-3

NAMESPACE: monitoring
────────────────────────────────────────────────────────────────────
└── prometheus [Helm: kube-prometheus] 1/1 ready
    └── ReplicaSet prometheus-7d4b8c [1/1]
        └── Pod prometheus-7d4b8c-xyz99  ✓ Running  10.244.0.25  node-1

════════════════════════════════════════════════════════════════════
Summary: 51 Deployments │ 189 Pods │ 186 Running │ 3 Pending
         Flux(28) ArgoCD(12) Helm(5) ConfigHub(4) Native(2)
```

**Expected output (ownership):**
```
OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════

Flux (28 resources)
────────────────────────────────────────────────────────────────────
  Managed by: kustomize.toolkit.fluxcd.io labels
  ✓ boutique/cart          Deployment   2/2
  ✓ boutique/checkout      Deployment   1/1
  ✓ boutique/frontend      Deployment   3/3
  └── ... (25 more)

ArgoCD (12 resources)
────────────────────────────────────────────────────────────────────
  Managed by: argocd.argoproj.io/instance label
  ✓ cert-manager/cert-manager   Deployment   1/1
  └── ... (11 more)

Native (2 resources)  ⚠ ORPHANS
────────────────────────────────────────────────────────────────────
  ⚠ temp-test/debug-nginx      Deployment   3d old
  ⚠ default/test-pod           Pod          1d old

════════════════════════════════════════════════════════════════════
Ownership: Flux 56% │ ArgoCD 24% │ Helm 10% │ ConfigHub 6% │ Native 4%
```

**Expected output (suggest):**
```
HUB/APPSPACE SUGGESTION
════════════════════════════════════════════════════════════════════

Detected Pattern: "Control Plane" (D2-style)
  Named after the Flux CD community reference architecture.
  └── clusters/prod, clusters/staging structure found

SUGGESTED STRUCTURE
────────────────────────────────────────────────────────────────────

Hub: acme-platform
├── Space: boutique-prod
│   ├── Unit: cart         (Deployment boutique/cart)
│   ├── Unit: checkout     (Deployment boutique/checkout)
│   ├── Unit: frontend     (Deployment boutique/frontend)
│   └── Unit: payment-api  (Deployment boutique/payment-api)
│
└── Space: platform
    ├── Unit: nginx-ingress  (Deployment ingress/nginx)
    └── Unit: monitoring     (StatefulSet monitoring/prometheus)

════════════════════════════════════════════════════════════════════
Next steps:
  1. Import workloads: cub-scout import -n boutique --space boutique-prod
  2. View in ConfigHub: cub unit tree --space boutique-prod
```

**Views:**
| View | Command | Description |
|------|---------|-------------|
| runtime | `tree` or `tree runtime` | Deployment → ReplicaSet → Pod trees |
| ownership | `tree ownership` | Resources grouped by GitOps owner |
| git | `tree git` | Git repository structure |
| patterns | `tree patterns` | Detected GitOps patterns |
| config | `tree config --space X` | ConfigHub Unit relationships |
| suggest | `tree suggest` | Recommended Hub/AppSpace structure |

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Filter by namespace |
| `-A, --all` | Include system namespaces |
| `--space` | ConfigHub space for config view |
| `--edge` | Edge type for config view: clone or link |
| `--json` | JSON output |

**Relationship with `cub unit tree`:**
- `cub-scout tree`: What's deployed in THIS cluster (cluster perspective)
- `cub unit tree`: How Units relate ACROSS your fleet (ConfigHub perspective)

---

## `status` — Connection and Cluster Status

**What it does:** Shows cub-scout connection status, cluster info, and worker status. Useful for verifying your ConfigHub connection.

```bash
./cub-scout status
./cub-scout status --json
```

**Expected output (connected with worker):**
```
ConfigHub:  ● Connected (alexis@confighub.com)
Cluster:    prod-east
Context:    eks-prod-east
Worker:     ● bridge-prod (connected)
```

**Expected output (connected, no worker):**
```
ConfigHub:  ● Connected
Cluster:    default
Context:    kind-cub-scout-test
Worker:     (none for this cluster)
```

**Expected output (standalone):**
```
ConfigHub:  ○ Online (not authenticated)
            Run: cub auth login
Cluster:    default
Context:    docker-desktop
```

**JSON output:**
```bash
./cub-scout status --json
```
```json
{
  "mode": "connected",
  "email": "alexis@confighub.com",
  "cluster_name": "prod-east",
  "context": "eks-prod-east",
  "space": "platform-prod",
  "worker": {
    "name": "bridge-prod",
    "status": "connected",
    "cluster": "prod-east"
  }
}
```

**Options:**
| Option | Description |
|--------|-------------|
| `--json` | Output as JSON |

**TUI equivalent:** The Local Cluster TUI header shows the same information:
```
Connected │ Cluster: prod-east │ Context: eks-prod-east │ Worker: ● bridge-prod
```

---

## `gitops status` — GitOps Pipeline Health (v0.14.1)

**What it does:** Shows the health of your GitOps pipeline, including which backend is managing resources and where failures are occurring.

```bash
./cub-scout gitops status
./cub-scout gitops status -n flux-system
./cub-scout gitops status --json
```

**Without cub-scout:**
```bash
kubectl get kustomizations.kustomize.toolkit.fluxcd.io -A
kubectl get helmreleases.helm.toolkit.fluxcd.io -A
kubectl get applications.argoproj.io -A
kubectl get ocirepositories.source.toolkit.fluxcd.io -A
# ... and manually correlate Ready conditions and failure reasons
```

**Expected output (healthy):**
```
GITOPS STATUS
════════════════════════════════════════════════════════════════════

Backend:    flux
Transport:  oci

SOURCES (1)
────────────────────────────────────────────────────────────────────
  ✓ OCIRepository/manifests (flux-system)    healthy

DEPLOYERS (1)
────────────────────────────────────────────────────────────────────
  ✓ Kustomization/app (flux-system)          healthy

Summary: 1 healthy, 0 failing
```

**Expected output (with failures):**
```
GITOPS STATUS
════════════════════════════════════════════════════════════════════

Backend:    flux
Transport:  oci

SOURCES (1)
────────────────────────────────────────────────────────────────────
  ✗ OCIRepository/manifests (flux-system)    failing
    Reason:  AuthenticationFailed
    Message: failed to login to OCI registry: UNAUTHORIZED

DEPLOYERS (2)
────────────────────────────────────────────────────────────────────
  ✓ Kustomization/app-frontend (flux-system)  healthy
  ✗ Kustomization/app-backend (flux-system)   failing at source
    Reason:  ArtifactFailed
    Message: Source 'OCIRepository/flux-system/manifests' is not ready

Summary: 1 healthy, 1 failing

NEXT STEPS
────────────────────────────────────────────────────────────────────
• Source failure: Check OCI registry credentials and network access
• Use 'kubectl describe ocirepository manifests -n flux-system' for details
```

**JSON output:**
```bash
./cub-scout gitops status --json
```
```json
{
  "backend": "flux",
  "transport": "oci",
  "deployers": [
    {
      "kind": "Kustomization",
      "name": "app-frontend",
      "namespace": "flux-system",
      "ready": true,
      "stage": "healthy"
    }
  ],
  "sources": [
    {
      "kind": "OCIRepository",
      "name": "manifests",
      "namespace": "flux-system",
      "ready": true,
      "url": "oci://ghcr.io/acme/manifests"
    }
  ],
  "healthyCount": 1,
  "failedCount": 0
}
```

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to scan (default: all) |
| `--json` | Output as JSON |

**Failure Stages:**
| Stage | Description |
|-------|-------------|
| `source` | OCI/Git/Helm fetch failed (auth, network, not found) |
| `build` | Kustomize/Helm rendering failed |
| `apply` | Kubernetes apply failed (validation, RBAC) |
| `sync` | ArgoCD sync failed |
| `healthy` | All stages passed |

**Detected Backends:**
| Backend | Detection |
|---------|-----------|
| `flux` | Kustomization or HelmRelease CRDs present |
| `argocd` | Application CRD present |
| `worker` | ConfigHub worker labels on resources |
| `none` | No GitOps backend detected |

---

## `debug` — Guided GitOps Debugging Wizard (v0.14.2)

**What it does:** Walks you step-by-step through diagnosing why a workload isn't working correctly. Shows workload health, ownership chain, pipeline status, and root cause analysis with suggested fixes.

```bash
# Interactive wizard - pick from unhealthy workloads
./cub-scout debug

# Direct analysis of a specific workload
./cub-scout debug deployment/api-server -n production

# Output as JSON
./cub-scout debug deployment/api-server -n production --format json

# Output as Markdown
./cub-scout debug deployment/api-server -n production --format md
```

**Without cub-scout:**
```bash
kubectl describe deployment api-server -n production
kubectl get pods -l app=api-server -n production
kubectl logs deployment/api-server -n production --previous
kubectl describe kustomization apps -n flux-system
kubectl describe ocirepository manifests -n flux-system
# ... and manually correlate failures across resources
```

**Expected output:**
```
DEBUG: Deployment/api-server in production

Workload: Deployment/api-server (1/3 ready)
  - api-server-7d9b8c-x4k2p: CrashLoopBackOff

Owner: flux
  Managed by: flux/apps

Pipeline: ✓ Kustomization/apps

Source: ✓ OCIRepository/platform-config
  URL: oci://ghcr.io/acme/platform-config

─────────────────────────────────────────────────────────────
ROOT CAUSE ANALYSIS

Category: crash_loop
Stage: workload

Summary: Workload Deployment/api-server has pod issues: CrashLoopBackOff

Probable Causes:
  - Config file missing: /etc/config/app.yaml
  - Database connection refused: postgres:5432

Suggested Fixes:
  kubectl get configmap -n production
  kubectl logs api-server-7d9b8c-x4k2p -n production --previous
  kubectl describe pod api-server-7d9b8c-x4k2p -n production
```

**Interactive wizard flow:**
1. **Select Mode**: Broken workload, Failing pipeline, Sync issue, or Freeform
2. **Pick Resource**: Select from filtered list of unhealthy resources
3. **Workload Status**: View pod issues, restart counts, recent events
4. **Container Logs**: View logs with automatic pattern detection (press `l`)
5. **Event Timeline**: View Kubernetes events with explanations (press `e`)
6. **Ownership**: See K8s and GitOps ownership chains
7. **Pipeline Health**: Check Kustomization/HelmRelease/Application status
8. **Source Health**: Check GitRepository/OCIRepository status
9. **Root Cause**: Get diagnosis with probable causes and suggested fixes

**Container log pattern detection:**

When viewing container logs, cub-scout automatically detects common error patterns:

| Pattern | Example Match | Suggestion |
|---------|--------------|------------|
| connection_refused | `ECONNREFUSED`, `connect: connection refused` | Check target service is running |
| file_not_found | `ENOENT`, `FileNotFoundError` | Check ConfigMaps/Secrets mounting |
| permission_denied | `EACCES`, `forbidden` | Check security context and RBAC |
| out_of_memory | `OOM`, `heap out of memory` | Increase memory limits |
| database_error | `SQLSTATE`, `db error` | Check DB credentials and connectivity |
| timeout | `deadline exceeded`, `ETIMEDOUT` | Check network and increase timeouts |
| dns_error | `no such host`, `NXDOMAIN` | Check hostname and DNS |
| panic | `panic:`, `SIGSEGV` | Check stack trace for root cause |

Detected patterns are highlighted in the log view and used to enhance root cause analysis.

**Event timeline explanations:**

When viewing the event timeline, cub-scout provides explanations for common Kubernetes events:

| Event Reason | Explanation | Suggestion |
|-------------|-------------|------------|
| FailedScheduling | Kubernetes could not find a suitable node | Check node resources, taints/tolerations |
| ImagePullBackOff | Repeated failures pulling container image | Verify image exists, check imagePullSecrets |
| CrashLoopBackOff | Container keeps crashing and restarting | Check logs, verify config/secrets |
| FailedMount | Failed to mount a volume to the pod | Check volume exists and permissions |
| BackOff | Container is in restart backoff | Check container logs with --previous |
| Unhealthy | Container failed health probe | Check probe config and app health |
| Evicted | Pod was evicted from the node | Check for node resource pressure |
| OOMKilled | Container ran out of memory | Increase memory limits |

Press `a` to toggle between showing all events or only warnings/errors.

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace of the resource |
| `--format` | Output format: ascii, json, md |
| `--non-interactive` | Run without prompts (requires resource arg) |

**Root Cause Categories:**
| Category | Description |
|----------|-------------|
| `source_auth` | Source authentication failed |
| `source_fetch` | Source fetch failed (network, not found) |
| `build_error` | Kustomize/Helm rendering failed |
| `apply_error` | Kubernetes apply failed |
| `crash_loop` | Container is crash-looping |
| `image_pull` | Cannot pull container image |
| `oom_killed` | Container ran out of memory |

---

## `discover` — Find Workloads (Scout Alias)

**What it does:** Discovers all workloads in your cluster and who owns them. This is a scout-style alias for `map workloads`.

```bash
./cub-scout discover
```

**Expected output:**

```
WORKLOADS BY OWNER
════════════════════════════════════════════════════════════════════

STATUS  NAMESPACE       NAME              OWNER      MANAGED-BY
✓       boutique        cart              Flux       Kustomization/apps
✓       boutique        checkout          Flux       Kustomization/apps
✓       boutique        frontend          Flux       Kustomization/apps
✓       monitoring      prometheus        Helm       Release/kube-prometheus
✓       monitoring      grafana           Helm       Release/kube-prometheus
✓       cert-manager    cert-manager      ArgoCD     Application/cert-manager
✓       payments        payment-gateway   ConfigHub  Unit/payment-gateway
⚠       temp-test       debug-nginx       Native     — (orphan)

────────────────────────────────────────────────────────────────────
Summary: 47 workloads
  Flux(28) ArgoCD(12) Helm(5) ConfigHub(2) Native(2)

Ownership Distribution:
  Flux       ████████████████████████░░░░░░░░░  56%
  ArgoCD     ████████████░░░░░░░░░░░░░░░░░░░░░  24%
  Helm       █████░░░░░░░░░░░░░░░░░░░░░░░░░░░░  10%
  ConfigHub  ████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   6%
  Native     █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   4%
```

---

## `health` — Check Cluster Health (Scout Alias)

**What it does:** Checks your cluster for stuck states, issues, and problems. This is a scout-style alias for `map issues`.

```bash
./cub-scout health
```

**Expected output (healthy):**

```
CLUSTER HEALTH CHECK: prod-east
════════════════════════════════════════════════════════════════════

✓ ALL HEALTHY

  Deployers:  5/5 ready
  Workloads:  47/47 ready

No issues detected.
```

**Expected output (with issues):**

```
CLUSTER HEALTH CHECK: prod-east
════════════════════════════════════════════════════════════════════

🔥 3 ISSUES DETECTED

DEPLOYER ISSUES
────────────────────────────────────────────────────────────────────
  ✗ HelmRelease/redis-cache      SourceNotReady
    │ Message: failed to fetch Helm chart: connection refused
    │ Last attempt: 5 minutes ago
    └─▶ Fix: Check Helm repository connectivity

  ⏸ Kustomization/monitoring     suspended
    │ Suspended since: 2026-01-20T10:30:00Z
    │ Reason: Manual pause for maintenance
    └─▶ Resume: flux resume kustomization monitoring -n flux-system

WORKLOAD ISSUES
────────────────────────────────────────────────────────────────────
  ✗ temp-test/debug-nginx        0/1 pods ready
    │ Reason: ImagePullBackOff
    │ Image: nginx:nonexistent
    └─▶ Fix: Use valid image tag or check registry access

════════════════════════════════════════════════════════════════════
Summary: 2 deployer issues │ 1 workload issue │ 1 suspended
         Deployers: 3/5 │ Workloads: 46/47
```

---

## `map` Subcommands (17)

### `map list` — Plain Text Output

```bash
./cub-scout map list
./cub-scout map list -q "owner=Flux"
./cub-scout map list -q "owner=Native"    # Shadow IT
./cub-scout map list -q "namespace=prod*"
```

**Expected output:**
```
NAMESPACE         KIND          NAME                          OWNER
flux-system       Deployment    source-controller             Flux
flux-system       Deployment    kustomize-controller          Flux
argocd            Deployment    argocd-server                 ArgoCD
monitoring        Deployment    prometheus                    Helm
default           Deployment    nginx                         Native
```

**Options:**
| Option | Description |
|--------|-------------|
| `-q, --query` | Query expression |
| `--namespace` | Filter by namespace |
| `--kind` | Filter by resource kind |
| `--owner` | Filter by owner (Flux, ArgoCD, Helm, Crossplane, ConfigHub, Native) |
| `--since` | Resources changed since duration (1h, 24h, 7d) |
| `--count` | Output count only |
| `--names-only` | Output names only (for scripting) |
| `--json` | JSON output |

---

### `map status` — One-Line Health

```bash
./cub-scout map status
```

**Expected output (healthy):**
```
✓ healthy: <N>/<N> deployers, <N>/<N> workloads
```

**Expected output (problems detected):**
```
✗ <N> problem(s): <N>/<N> deployers, <N>/<N> workloads
```

**Exit codes:**
- `0`: All healthy
- `1`: Problems detected or error

---

### `map workloads` — Workloads by Owner

```bash
./cub-scout map workloads
```

Shows Deployments, StatefulSets, DaemonSets grouped by owner.

**Expected output:**

```
WORKLOADS BY OWNER
════════════════════════════════════════════════════════════════════

Flux (28 workloads)
────────────────────────────────────────────────────────────────────
  STATUS  NAMESPACE       NAME              KIND         REPLICAS
  ✓       boutique        cart              Deployment   2/2
  ✓       boutique        checkout          Deployment   1/1
  ✓       boutique        frontend          Deployment   3/3
  ✓       boutique        payment-api       Deployment   2/2
  ✓       ingress         nginx-ingress     Deployment   2/2
  └── ... (23 more)

ArgoCD (12 workloads)
────────────────────────────────────────────────────────────────────
  STATUS  NAMESPACE       NAME              KIND         REPLICAS
  ✓       cert-manager    cert-manager      Deployment   1/1
  ✓       cert-manager    cainjector        Deployment   1/1
  ✓       argocd          argocd-server     Deployment   1/1
  └── ... (9 more)

Helm (5 workloads)
────────────────────────────────────────────────────────────────────
  STATUS  NAMESPACE       NAME              KIND         REPLICAS
  ✓       monitoring      prometheus        StatefulSet  1/1
  ✓       monitoring      grafana           Deployment   1/1
  ✓       monitoring      alertmanager      StatefulSet  1/1

Native (2 workloads)  ⚠ ORPHANS
────────────────────────────────────────────────────────────────────
  STATUS  NAMESPACE       NAME              KIND         AGE
  ⚠       temp-test       debug-nginx       Deployment   3d
  ⚠       default         test-pod          Deployment   1d

════════════════════════════════════════════════════════════════════
Total: 47 workloads │ 45 healthy │ 2 orphans
```

---

### `map deployers` — Deployers

```bash
./cub-scout map deployers
./cub-scout map deployers --json
```

> **v0.5 contract:** Deployers are Kubernetes Deployments. Flux Kustomizations,
> HelmReleases, and Argo Applications are out of scope for v0.5 and may be
> added in a future release.

**JSON output (`--json`):**

```json
[
  {
    "kind": "Deployment",
    "name": "app-alpha",
    "namespace": "default",
    "status": "Ready",
    "ready": true,
    "revision": "-"
  }
]
```

**Stable JSON fields:**
| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Always `Deployment` in v0.5 |
| `name` | string | Deployment name |
| `namespace` | string | Deployment namespace |
| `status` | string | Status string (e.g., `Ready`) |
| `ready` | bool | Whether deployment is ready |
| `revision` | string | Revision string (or `-` if unavailable) |

---

### `map orphans` — Unmanaged Resources

```bash
./cub-scout map orphans
```

**Expected output:**
```
ORPHAN RESOURCES (not managed by GitOps)
═══════════════════════════════════════════════════════════════════

NAMESPACE         KIND          NAME                    AGE
default           Deployment    debug-pod               3d
default           ConfigMap     test-config             5d

Total: 2 orphaned resources
```

---

### `map crashes` — Failing Pods

```bash
./cub-scout map crashes
```

Lists pods in CrashLoopBackOff, Error, ImagePullBackOff.

---

### `map issues` — Resources with Problems

```bash
./cub-scout map issues
```

Shows resources with conditions != Ready.

---

### `map drift` — Desired vs Actual

```bash
./cub-scout map drift
```

Shows resources where live state differs from last-applied configuration.

---

### `map bypass` — Factory Bypass Detection

```bash
./cub-scout map bypass
```

Detects changes made outside GitOps (kubectl edits to managed resources).

---

### `map sprawl` — Configuration Sprawl

```bash
./cub-scout map sprawl
```

Analyzes configuration sprawl across namespaces.

---

### `map dashboard` — Unified Dashboard

```bash
./cub-scout map dashboard
```

Combined health + ownership view.

---

### `map deep-dive` — All Cluster Data

```bash
./cub-scout map deep-dive
```

Maximum detail for all GitOps resources with LiveTree views:
- Flux: GitRepositories, Kustomizations, HelmReleases
- ArgoCD: Applications, AppProjects, ApplicationSets
- Helm: Releases decoded from secrets
- Deployment → ReplicaSet → Pod trees

---

### `map app-hierarchy` — Inferred Structure

```bash
./cub-scout map app-hierarchy
```

Infers ConfigHub-style hierarchy from cluster analysis.

---

### `map queries` — Saved Queries

```bash
./cub-scout map queries
```

List and manage saved queries.

---

### `map fleet` — Multi-Cluster View

```bash
./cub-scout map fleet
```

Fleet view grouped by app and variant. Requires ConfigHub labels.

---

### `map hub` — ConfigHub Hierarchy

```bash
./cub-scout map --hub
./cub-scout map hub
```

Interactive TUI for ConfigHub hierarchy. Requires `cub auth login`.

---

## `trace` — Ownership Chain

Works with **Flux, ArgoCD, or standalone Helm** — auto-detects the owner.

```bash
# Flux-managed resource
./cub-scout trace deploy/nginx -n production

# ArgoCD application
./cub-scout trace --app guestbook

# Standalone Helm release (not Flux-managed)
./cub-scout trace deploy/prometheus -n monitoring

# ConfigHub OCI source (Flux or ArgoCD)
./cub-scout trace deploy/frontend -n prod

# Reverse trace (walk up from Pod)
./cub-scout trace pod/nginx-abc123 -n prod --reverse
```

**Flux trace (GitRepository source):**
```
TRACE: Deployment/nginx in production

  ✓ GitRepository/flux-system
    │ URL: https://github.com/myorg/infra
    │ Revision: main@sha1:abc123
    │
    └─▶ ✓ Kustomization/apps
          │ Path: ./apps/production
          │
          └─▶ ✓ Deployment/nginx
                Status: Managed by Flux
```

**Helm standalone trace:**
```
TRACE: Deployment/prometheus in monitoring

  ✓ HelmChart/prometheus
    │ v15.3.2 (app: 2.45.0)
    │
    └─▶ ✓ Release/prometheus
          │ Status: deployed
          │ Revision: v3
          │
          └─▶ ✓ Deployment/prometheus
                Status: Managed by Helm
```

**ConfigHub OCI trace (Flux OCIRepository):**
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
          │
          └─▶ ✓ Deployment/frontend
                Status: Applied
```

**ConfigHub OCI trace (ArgoCD Application):**
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
```

**Reverse trace with orphan metadata:**
```
REVERSE TRACE: Deployment/debug-nginx in default

K8s Ownership Chain:
✓ Deployment/debug-nginx (1/1 ready)

Detected Owner: NATIVE

⚠ This resource is NOT managed by GitOps

Orphan Metadata:
  Created: 2026-01-15 10:30:00 UTC
  Labels: app=debug

✓ last-applied-configuration found
  💡 To see full manifest:
  kubectl get deployment debug-nginx -n default -o jsonpath='{...}' | jq .
```

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace of the resource |
| `--app` | Trace ArgoCD app by name |
| `-r, --reverse` | Reverse trace — walks ownerRefs up, shows orphan metadata |
| `-d, --diff` | Show diff between live and desired state |
| `--history` | Show deployment history (who deployed what, when) |
| `--limit` | Limit number of history entries (default: 10) |
| `--explain` | Show learning content explaining the trace |
| `--json` | Output as JSON |

**History mode (`--history`):**
```bash
./cub-scout trace deploy/nginx -n prod --history

# Output:
# TRACE: Deployment/nginx in prod
# ...
# History:
#   2026-01-28 10:00  v1.2.3@abc123         deployed    manual sync by alice@co.com
#   2026-01-27 14:00  v1.2.2@def456         deployed    auto-sync
#   2026-01-25 09:00  v1.2.1@789ghi         deployed    manual sync by bob@co.com
```

History data sources per tool:
- **ArgoCD**: `status.history` on Application resource
- **Flux**: `status.history` on Kustomization/HelmRelease
- **Helm**: Release secrets (`sh.helm.release.v1.<name>.v<N>`)

**Supported sources:** GitRepository, OCIRepository, HelmRepository, Bucket (Flux), plus standalone Helm releases.

---

## `scan` — Configuration Issues

```bash
./cub-scout scan
./cub-scout scan -n production
./cub-scout scan --file manifest.yaml
```

**Expected output:**
```
CCVE SCAN: kind-kind
═══════════════════════════════════════════════════════════════════

CRITICAL (1)
───────────────────────────────────────────────────────────────────
[CCVE-2025-0001] GitRepository not ready
  Resource: flux-system/GitRepository/apps
  Message:  authentication required
  Fix:      kubectl create secret generic git-credentials ...

WARNING (2)
───────────────────────────────────────────────────────────────────
[CCVE-2025-0005] Application out of sync
  Resource: argocd/Application/guestbook

═══════════════════════════════════════════════════════════════════
Summary: 1 critical, 2 warning, 0 info
```

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to scan |
| `--state` | State scan only (stuck reconciliations) |
| `--kyverno` | Kyverno scan only (PolicyReports) |
| `--timing-bombs` | Expiring certs, quota limits |
| `--dangling` | Orphan HPAs, Services, Ingress, NetworkPolicy |
| `--include-unresolved` | Include Trivy/Kyverno findings |
| `--file` | YAML file to scan (static analysis, no cluster) |
| `--list` | List all KPOL policies in database |
| `--threshold` | Duration threshold for stuck (default: 5m) |
| `--json` | Output as JSON |
| `--verbose` | Detailed output |

---

## `snapshot` — Export State as JSON (GSF)

Exports cluster state in GitOps State Format (GSF) — a JSON format for third-party tool integration.

```bash
./cub-scout snapshot -o state.json
./cub-scout snapshot -o - | jq '.entries[] | select(.owner.type == "Native")'

# Include resource relations (dependency graph)
./cub-scout snapshot --relations

# Query relations
./cub-scout snapshot --relations | jq '.relations[] | select(.type == "owns")'
./cub-scout snapshot --relations | jq '.relations[] | select(.from | contains("Service/"))'
```

**Options:**
| Option | Description |
|--------|-------------|
| `-o, --output` | Output file (default: stdout) |
| `-n, --namespace` | Filter by namespace |
| `-k, --kind` | Filter by kind |
| `--relations` | Include resource relations (owns, selects, mounts, references) |

**Relation Types:**
| Type | Description | Example |
|------|-------------|---------|
| `owns` | K8s OwnerReference | ReplicaSet → Pod |
| `selects` | Label selector match | Service → Pod |
| `mounts` | Volume reference | Pod → ConfigMap |
| `references` | envFrom reference | Pod → Secret |

See [docs/reference/gsf-schema.md](docs/reference/gsf-schema.md) for full schema documentation.

---

## `remedy` — Execute Remediation

```bash
./cub-scout remedy CCVE-2025-0687 -n production --dry-run
./cub-scout remedy --all --dry-run -n production
./cub-scout remedy --list
```

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to operate in |
| `--all` | Fix all auto-fixable issues |
| `--dry-run` | Show what would be changed (default: true) |
| `--force` | Skip confirmation for high-risk actions |
| `--file` | YAML file to scan and fix |
| `--list` | List auto-fixable CCVEs |
| `--json` | Output as JSON |
| `--audit` | Log actions to audit file (default: true) |
| `--audit-file` | Audit log file path |
| `--timeout` | Timeout for each action (default: 30s) |

---

## `import` — Import Workloads

```bash
./cub-scout import -n production
./cub-scout import -n production --dry-run
./cub-scout import --wizard
```

**Options:**
| Option | Description |
|--------|-------------|
| `-n, --namespace` | Namespace to import |
| `-w, --wizard` | Launch interactive TUI wizard |
| `--dry-run` | Preview without making changes |
| `--json` | Output as JSON |
| `-y, --yes` | Skip confirmation |
| `--no-log` | Disable logging to file |

---

## `import-argocd` — Import ArgoCD App

```bash
./cub-scout import-argocd --list
./cub-scout import-argocd guestbook --dry-run
./cub-scout import-argocd guestbook --show-yaml
```

**Options:**
| Option | Description |
|--------|-------------|
| `--list` | List available ArgoCD Applications |
| `--dry-run` | Preview without making changes |
| `--show-yaml` | Show YAML content |
| `--disable-sync` | Disable auto-sync after import |
| `--delete-app` | Delete ArgoCD Application after import |
| `--space` | ConfigHub space to import into |
| `--argocd-namespace` | Namespace where ArgoCD is installed |
| `--raw` | Keep raw YAML with runtime fields |
| `--test-rollout` | Test by triggering rollout restart |
| `--test-update` | Test by adding annotation |
| `-y, --yes` | Skip confirmation |

---

## `combined` — Git + Cluster Alignment

```bash
./cub-scout combined --git-url https://github.com/org/repo --namespace demo
./cub-scout combined --git-url https://github.com/org/repo --suggest --apply
```

**Options:**
| Option | Description |
|--------|-------------|
| `--git-url` | Git repository URL |
| `--git-path` | Local path to Git repo |
| `-n, --namespace` | Namespace to scan |
| `--suggest` | Generate Hub/App Space proposal |
| `--apply` | Create App Space and Units |
| `--dry-run` | Show without making changes |
| `--json` | Output as JSON |

---

## `parse-repo` — Parse GitOps Repo

```bash
./cub-scout parse-repo --url https://github.com/fluxcd/flux2-kustomize-helm-example
./cub-scout parse-repo --path ./my-gitops-repo
```

**Options:**
| Option | Description |
|--------|-------------|
| `--url` | Git repository URL |
| `--path` | Local path to parse |
| `--json` | Output as JSON |

---

## `app-space` — Manage App Spaces

```bash
./cub-scout app-space list
./cub-scout app-space create
```

---

## `demo` — Interactive Demos

```bash
./cub-scout demo --list
./cub-scout demo quick
./cub-scout demo ccve
./cub-scout demo query
./cub-scout demo scenario bigbank
./cub-scout demo quick --cleanup
```

---

## `version` / `completion` / `setup`

```bash
./cub-scout version
./cub-scout completion bash > /etc/bash_completion.d/cub-scout
./cub-scout completion zsh > "${fpath[1]}/_cub-scout"
./cub-scout setup
```

---

## TUI Keyboard Shortcuts

Press `?` in the TUI to see help.

### Local Cluster Mode

The TUI header shows your connection status at all times:

```
Connected │ Cluster: prod-east │ Context: eks-prod-east │ Worker: ● bridge-prod
```

- **Connected** (green): Authenticated with ConfigHub
- **Standalone** (gray): Not authenticated, local-only mode
- **Worker ●** (green): Worker connected and syncing
- **Worker ○** (red): Worker disconnected

#### Navigation
| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `←`/`h` | Collapse / go to parent |
| `→`/`l` | Expand |
| `Enter` | Cross-references (panel view) |
| `Tab` | Cycle views |
| `[` | Previous namespace |
| `]` | Next namespace |
| `/` | Search |
| `r` | Refresh data |

#### Views (17)
| Key | View | Description |
|-----|------|-------------|
| `s` | Status | Dashboard overview |
| `w` | Workloads | Workloads by owner |
| `a` | Apps | Grouped by app label + variant |
| `p` | Pipelines | GitOps deployers (Flux, ArgoCD) |
| `d` | Drift | Resources diverged from desired |
| `o` | Orphans | Native (unmanaged) resources |
| `c` | Crashes | Failing pods |
| `i` | Issues | Unhealthy resources |
| `u` | sUspended | Paused/forgotten resources |
| `b` | Bypass | Factory bypass detection |
| `x` | Sprawl | Config sprawl analysis |
| `D` | Dependencies | Upstream/downstream relationships |
| `G` | Git sources | Forward trace from Git |
| `4` | Cluster Data | All data sources TUI reads |
| `5`/`A` | App Hierarchy | Inferred ConfigHub model |
| `M` | Maps | Three Maps view |

#### Actions
| Key | Action | Description |
|-----|--------|-------------|
| `Q` | Saved Queries | Filter with saved queries |
| `T` | Trace | Trace ownership chain |
| `S` | Scan | Scan for CCVEs |
| `I` | Import | Import wizard |

#### Command Palette (`:`)
Press `:` to run shell commands:
```
:kubectl get pods
:cub-scout scan
:flux get kustomizations
```
- `↑`/`↓` — Navigate history (last 20)
- `Enter` — Execute
- `Esc` — Cancel

#### Help and Mode Switching
| Key | Action |
|-----|--------|
| `?` | Show help overlay |
| `H` | Switch to ConfigHub TUI |
| `q` | Quit |

### ConfigHub Hub Mode

#### Navigation
| Key | Action |
|-----|--------|
| `↑`/`k` | Move up |
| `↓`/`j` | Move down |
| `←`/`h` | Collapse |
| `→`/`l` | Expand |
| `Enter` | Load details |
| `Tab` | Focus details pane |

#### Search & Filter
| Key | Action |
|-----|--------|
| `/` | Start search |
| `n`/`N` | Next/previous match |
| `f` | Toggle filter |

#### Actions
| Key | Action |
|-----|--------|
| `a` | Activity view |
| `B` | Toggle Hub/AppSpace |
| `M` | Three Maps view |
| `P` | Panel view (WET↔LIVE) |
| `c` | Create resource |
| `d`/`x` | Delete resource |
| `i` | Import workloads |
| `o` | Open in browser |
| `O` | Switch organization |
| `r` | Refresh |
| `?` | Help |
| `L` | Switch to local TUI |
| `q` | Quit |

---

## Query Syntax

```bash
./cub-scout map list -q "owner=Flux"
./cub-scout map list -q "owner=Native"           # Shadow IT
./cub-scout map list -q "namespace=prod*"        # Wildcard
./cub-scout map list -q "kind=Deployment"
./cub-scout map list -q "owner=Flux AND namespace=production"
./cub-scout map list -q "owner=Flux OR owner=ArgoCD"
./cub-scout map list -q "labels[app]=nginx"
```

**Operators:**
| Operator | Example | Description |
|----------|---------|-------------|
| `=` | `owner=Flux` | Exact match |
| `!=` | `owner!=Native` | Not equal |
| `~=` | `name~=nginx.*` | Regex match |
| `=a,b` | `owner=Flux,ArgoCD` | IN list |
| `=prefix*` | `namespace=prod*` | Wildcard |
| `AND` | `kind=Deployment AND owner=Flux` | Both match |
| `OR` | `owner=Flux OR owner=ArgoCD` | Either matches |

**Fields:**
| Field | Values |
|-------|--------|
| `owner` | Flux, ArgoCD, Helm, Crossplane, ConfigHub, Native |
| `namespace` | Any namespace |
| `kind` | Deployment, Service, ConfigMap, etc. |
| `name` | Resource name |
| `status` | Ready, NotReady, Failed, Pending, Unknown |
| `cluster` | Cluster name |
| `labels[key]` | Label value |

---

## Ownership Detection

| Owner | Detection Method |
|-------|------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| **ArgoCD** | `app.kubernetes.io/instance` + `argocd.argoproj.io/instance` labels |
| **Helm** | `app.kubernetes.io/managed-by: Helm` label |
| **Crossplane** | `crossplane.io/claim-name` label or `*.crossplane.io` owner refs *(experimental)* |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | None of the above (kubectl-applied) |

**Priority:** Flux > ArgoCD > Helm > Crossplane > ConfigHub > Native

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig |
| `CLUSTER_NAME` | `default` | Name for this cluster |

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (check stderr) |
| 2 | No cluster connection |

---

## See Also

- [README.md](README.md) — Project overview
- [docs/COMMAND-MATRIX.md](docs/COMMAND-MATRIX.md) — Complete reference table
- [docs/SCAN-GUIDE.md](docs/SCAN-GUIDE.md) — CCVE scanning deep dive
- [docs/ALTERNATIVES.md](docs/ALTERNATIVES.md) — Comparison with other tools
- [CONTRIBUTING.md](CONTRIBUTING.md) — How to contribute
