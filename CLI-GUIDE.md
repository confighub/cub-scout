# cub-scout CLI Guide

Complete reference for all commands, options, TUI keys, and expected outputs.

---

## Top-Level Commands

The table below reflects the current `cub-scout --help` output.

| Command | Description |
|---------|-------------|
| `app-space` | Manage App Spaces |
| `apply` | Apply a proposal from JSON (GUI) |
| `bundle` | Work with debug bundles |
| `catalog` | Manage bundle catalogs |
| `combined` | Show Git repo structure + cluster workloads aligned |
| `completion` | Generate shell completion script |
| `connect` | Quickly configure kube context from server URL or kubeconfig |
| `debug` | Guided GitOps debugging wizard |
| `demo` | Run interactive demos |
| `discover` | Discover resources (alias for `map workloads`) |
| `drift` | Detect drift between desired and live state |
| `gitops` | GitOps status and diagnostics |
| `graph` | Resource graph operations |
| `health` | Check issues (alias for `map issues`) |
| `import` | Import workloads into ConfigHub |
| `import-argocd` | Import an ArgoCD Application into ConfigHub |
| `import-cluster-aggregator` | Aggregate imports from multiple clusters (GUI) |
| `map` | Interactive map of resources and ownership |
| `parse-repo` | Parse a GitOps repository structure |
| `patterns` | Pattern detection engine |
| `remedy` | Execute remediation for risk issue findings |
| `scan` | Scan for risk issues and stuck states |
| `setup` | Set up shell completions and configuration |
| `snapshot` | Dump cluster state as GSF JSON |
| `status` | Show connection status and cluster info |
| `trace` | Trace any resource to its Git source |
| `tree` | Show hierarchical views of resources |
| `version` | Print version information |

---

## `connect` — Quick Cluster Connect

**What it does:** Creates or imports a kubeconfig context, sets it as current, optionally verifies API access, and can launch the TUI immediately.

```bash
# Direct server + bearer token
./cub-scout connect https://api.example.com:6443 --token "$K8S_BEARER_TOKEN" --context prod --map

# Import context from shared kubeconfig
./cub-scout connect --from-kubeconfig ./artem.yaml --from-context ske-vcl-pro --map
```

**Options (common):**
| Option | Description |
|--------|-------------|
| `--context` | Context name to create |
| `--namespace` | Default namespace for context |
| `--kubeconfig` | Destination kubeconfig path |
| `--skip-verify` | Skip API connectivity check |
| `--map` | Launch `cub-scout map` immediately |

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
| `--save-bundle <dir>` | Save debug bundle to directory (non-interactive only) |
| `--kustomize <dir>` | Kustomize overlay directory for attribution (requires `--save-bundle`) |

**Kustomize Attribution:**

When saving a debug bundle with `--save-bundle`, you can optionally provide a Kustomize overlay directory with `--kustomize`. This records the overlay as an explicit ownership edge in the attribution graph.

```bash
# Save debug bundle with Kustomize overlay attribution
./cub-scout debug deployment/api -n prod --non-interactive \
  --save-bundle ./bundles \
  --kustomize ./overlays/prod
```

This is useful when:
- You want to trace a resource back to its Kustomize overlay source
- You're building an attribution graph that includes build-time provenance
- You need to answer "which overlay produced this resource?"

The overlay attribution is opt-in (explicit flag required) and does not guess or infer overlay relationships.

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

> **Deployer scope:** Deployers are Kubernetes Deployments (StatefulSets,
> DaemonSets also included). Flux Kustomizations, HelmReleases, and Argo
> Applications are surfaced via `map` and `trace` but not listed as deployers.

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

**Aliases:** `map native`, `map unmanaged`

Lists resources not managed by any GitOps tool (Flux, ArgoCD, Helm, ConfigHub).

**Default behavior:** System namespaces are hidden to reduce noise:
- `kube-system`, `kube-public`, `kube-node-lease`
- `flux-system`, `argocd`, `cert-manager`, `ingress-nginx`, `local-path-storage`

**Flags:**
| Flag | Description |
|------|-------------|
| `--include-system` | Include system namespaces |
| `--namespace` | Filter to specific namespace |

**Examples:**
```bash
./cub-scout map orphans                    # User namespaces only (default)
./cub-scout map orphans --include-system   # Include system namespaces
./cub-scout map orphans --namespace prod   # Specific namespace
./cub-scout map orphans --json             # JSON output
```

**Expected output:**
```
ORPHAN RESOURCES
════════════════════════════════════════════════════════════════════
Resources not managed by GitOps (Flux, ArgoCD, Helm, ConfigHub).
These may be: legacy systems, manual hotfixes, debug pods, or shadow IT.

Note: System namespaces hidden by default. Use --include-system to show all.

NAMESPACE         KIND          NAME                    AGE
default           Deployment    debug-pod               3d
default           ConfigMap     test-config             5d

Total: 2 orphaned resources
```

---

### `map hooks` — Lifecycle Hooks

```bash
./cub-scout map hooks
```

Lists resources with lifecycle hook annotations (Helm and ArgoCD).

**Detected annotations:**
| Annotation | Tool | Purpose |
|------------|------|---------|
| `helm.sh/hook` | Helm | Hook phase (pre-install, post-upgrade, etc.) |
| `helm.sh/hook-weight` | Helm | Execution order within phase |
| `argocd.argoproj.io/hook` | ArgoCD | Hook phase (PreSync, Sync, PostSync) |
| `argocd.argoproj.io/sync-wave` | ArgoCD | Execution order within sync |

**Helm to ArgoCD Phase Mapping:**
| Helm Hook | ArgoCD Phase |
|-----------|--------------|
| `pre-install`, `pre-upgrade` | PreSync |
| `post-install`, `post-upgrade` | PostSync |
| `test`, `test-success` | PostSync |

**Flags:**
| Flag | Description |
|------|-------------|
| `--file` | YAML file to analyze (static analysis) |
| `--namespace` | Filter by namespace |
| `--format` | Output format: ascii, json, md |

**Examples:**
```bash
./cub-scout map hooks                         # All hooks from cluster
./cub-scout map hooks --namespace prod        # Hooks in prod namespace
./cub-scout map hooks --file chart.yaml       # Static analysis
./cub-scout map hooks --format json           # JSON output
```

**Expected output:**
```
LIFECYCLE HOOKS
════════════════════════════════════════════════════════════════════
Source: live cluster

KIND                 NAME                           NAMESPACE       HELM HOOK                 ARGO PHASE      WEIGHT
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
Job                  db-backup                      prod            post-install              PostSync
Job                  db-migrate                     prod            post-install,post-upgrade PostSync        -5

Total: 2 hook(s)
```

**Use case:** When migrating Helm charts to ArgoCD, use this to understand which hooks exist and how they'll map to ArgoCD sync phases. Combine with `scan --lifecycle-hazards` to detect potential issues.

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

**Aliases:** `map problems`

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

**Aliases:** `map cluster-data`, `map data`

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

**Aliases:** `map hierarchy`, `map infer`

Infers ConfigHub-style hierarchy from cluster analysis.

---

### `map patterns` — GitOps Repository Analysis

**What it does:** Analyzes your GitOps repositories to discover organizational patterns and suggest ConfigHub structure.

```bash
./cub-scout map patterns
./cub-scout map patterns --json
./cub-scout map patterns --verbose
```

**Aliases:** `map repos`, `map structure`

**Expected output:**
```
GITOPS REPOSITORY PATTERNS
════════════════════════════════════════════════════════════════════

REPOSITORY PATTERNS
────────────────────────────────────────────────────────────────────
  monorepo     1 repository with multiple apps
  polyrepo     0 repositories (one per app)

PATH CONVENTIONS
────────────────────────────────────────────────────────────────────
  flux2-kustomize-helm    apps/*/overlays/{env}
  d2-style                clusters/{cluster}/apps/*

ENVIRONMENT CHAINS
────────────────────────────────────────────────────────────────────
  payment-api:  dev → staging → prod
  frontend:     dev → prod

SUGGESTED CONFIGHUB STRUCTURE
────────────────────────────────────────────────────────────────────
  Hub: platform
  ├── AppSpace: payments
  │   └── Units: payment-api-dev, payment-api-staging, payment-api-prod
  └── AppSpace: frontend
      └── Units: frontend-dev, frontend-prod
```

**Shows:**
- Repository patterns (monorepo, polyrepo, platform, external)
- Path conventions (D2-style, flux2-kustomize-helm, etc.)
- Environment chains (same app across dev/staging/prod)
- Team groupings (from namespace patterns)
- Suggested ConfigHub organization (Hubs, AppSpaces)

**Options:**
| Option | Description |
|--------|-------------|
| `--json` | Output as JSON for tooling |
| `--verbose` | Include detailed path information |

**When to use it:** Before importing into ConfigHub, to understand your GitOps structure and plan the organization.

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
risk issue SCAN: kind-kind
═══════════════════════════════════════════════════════════════════

CRITICAL (1)
───────────────────────────────────────────────────────────────────
[RISK-2025-0001] GitRepository not ready
  Resource: flux-system/GitRepository/apps
  Message:  authentication required
  Fix:      kubectl create secret generic git-credentials ...

WARNING (2)
───────────────────────────────────────────────────────────────────
[RISK-2025-0005] Application out of sync
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
| `--lifecycle-hazards` | GitOps lifecycle hazards (Helm hooks under ArgoCD) |
| `--include-unresolved` | Include Trivy/Kyverno findings |
| `--file` | YAML file to scan (static analysis, no cluster) |
| `--list` | List all KPOL policies in database |
| `--threshold` | Duration threshold for stuck (default: 5m) |
| `--json` | Output as JSON |
| `--verbose` | Detailed output |
| `--explain` | Show explanatory content to help learn GitOps risk concepts |

### Lifecycle Hazards (v0.19.5)

Detect GitOps lifecycle hazards — Helm hook semantics that behave differently under ArgoCD.

```bash
# Scan a YAML file for lifecycle hazards
./cub-scout scan --lifecycle-hazards --file manifest.yaml
```

**Detected hazards:**
| Rule | Detection |
|------|-----------|
| Helm hook ambiguity | Comma-separated `helm.sh/hook` values (e.g., `post-install,post-upgrade`) |
| PostSync idempotency | Job with `hook-delete-policy: before-hook-creation` reruns on every sync |

**Example output:**
```
GitOps Lifecycle Hazards
──────────────────────────────────────────────────

Helm Hook Ambiguity (1)
────────────────────────────────────────

⚠ Job/db-migrate (ns: prod)
  Hooks: post-install, post-upgrade
  Phase: PostSync
  Risk: ArgoCD maps comma-separated Helm hooks to a single phase
  Fix:  Split into separate resources or convert to ArgoCD Sync hook
```

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
./cub-scout remedy RISK-2025-0687 -n production --dry-run
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
| `--list` | List auto-fixable risk issues |
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
./cub-scout demo list
./cub-scout demo quick
./cub-scout demo risk
./cub-scout demo query
./cub-scout demo scenario bigbank
./cub-scout demo quick --cleanup
```

---

## `bundle` — Debug Bundles (v0.14)

Work with debug bundles for offline inspection and sharing.

**What it does:** Debug bundles are portable snapshots of debugging context that can be inspected offline or shared across time and people.

**When to use it:** When you need to capture, share, or compare debugging state across time or with others.

**A bundle contains:**
- `metadata.json` — Bundle version, target, creation time, tool version
- `session.json` — Debug session data (if captured)
- `drift.json` — Drift findings (if captured)
- `events.json` — Timeline events (if captured)
- `logs.json` — Container logs (if captured)
- `README.md` — Human-readable summary

```bash
# Inspect a bundle
./cub-scout bundle inspect ./debug-bundle-2024-01-15

# Replay bundle as ASCII
./cub-scout bundle replay ./debug-bundle-2024-01-15

# Replay bundle as JSON
./cub-scout bundle replay ./debug-bundle-2024-01-15 --format json

# Compare two bundles
./cub-scout bundle diff ./bundle-before ./bundle-after

# Show timeline from a catalog
./cub-scout bundle timeline ./my-catalog --order created_at
```

**Subcommands:**
| Command | Description |
|---------|-------------|
| `inspect` | Show bundle metadata and contents summary |
| `replay` | Re-render bundle contents with existing renderers |
| `diff` | Compare two bundles and show what changed |
| `timeline` | Show time-series view of objects across a catalog |
| `summarize` | Generate human-readable summary for tickets, PRs, or Slack |

### `bundle summarize` (v0.19.5)

Generate human-readable summaries from debug bundles for external systems.

```bash
# Generate ticket summary (stdout)
./cub-scout bundle summarize ./bundle

# Generate ticket summary (write to file)
./cub-scout bundle summarize ./bundle --format ticket --out jira.md

# Generate PR summary
./cub-scout bundle summarize ./bundle --format pr

# Generate Slack notification (Block Kit JSON)
./cub-scout bundle summarize ./bundle --format slack --out notification.json

# Structured JSON output
./cub-scout bundle summarize ./bundle --format json
```

**Format options:**
| Format | Output | Use Case |
|--------|--------|----------|
| `ascii` | Human-readable plain text (default) | Terminal |
| `ticket` | Markdown for Jira/ServiceNow | Incident documentation |
| `pr` | Markdown for PR description/comment | Code review |
| `slack` | Slack Block Kit JSON | Channel notifications |
| `json` | Structured data | Tooling integration |

**Ticket format includes:**
- Context: cluster, namespace, target, git commit
- What changed: drift counts, affected resources
- Risk signals: critical/warning findings
- Evidence: bundle path for audit trail

**Related commands:** `debug`, `catalog`

---

## `catalog` — Bundle Catalogs (v0.15)

Manage catalogs of debug bundles for multi-bundle operations.

**What it does:** A catalog is a file-backed manifest that indexes multiple bundles, enabling multi-bundle comparisons, timeline construction, and explicit ordering.

**When to use it:** When you need to track multiple debug bundles over time (e.g., before/after a fix, or daily snapshots).

**Catalog layout:**
```
catalog/
  catalog.json      # Manifest with bundle entries
  bundles/          # Copied bundle directories
    bundle-1/
    bundle-2/
```

```bash
# Create a new catalog
./cub-scout catalog init ./my-catalog

# Add a bundle with custom ID
./cub-scout catalog add ./my-catalog ./debug-bundle --id before-fix

# Add with labels and scope
./cub-scout catalog add ./my-catalog ./debug-bundle \
  --id after-fix \
  --label ticket=INC-123 \
  --scope prod/api/Deployment/api

# List bundles
./cub-scout catalog list ./my-catalog

# Validate integrity
./cub-scout catalog validate ./my-catalog
```

**Subcommands:**
| Command | Description |
|---------|-------------|
| `init` | Create a new empty catalog |
| `add` | Add a bundle to a catalog |
| `list` | List bundles in a catalog |
| `validate` | Validate catalog integrity |

**Related commands:** `bundle`, `bundle timeline`

---

## `graph` — Resource Graph (v0.6)

Resource graph operations for exploring cluster relationships.

**What it does:** Exports and explains the resource relationship graph showing how resources connect to each other.

**When to use it:** When you need to understand resource dependencies or export the graph for external tools.

```bash
# Export resource graph as JSON
./cub-scout graph export

# Export with relations (owns, selects, mounts, references)
./cub-scout graph export --relations

# Explain a specific resource's relationships
./cub-scout graph explain deploy/nginx -n default
```

**Subcommands:**
| Command | Description |
|---------|-------------|
| `export` | Export resource graph as JSON |
| `explain` | Explain a resource's graph relationships |

**Related commands:** `snapshot`, `tree`

---

## `patterns` — Pattern Detection (v0.7)

Pattern detection engine for analyzing resource graphs.

**What it does:** Runs deterministic checks against the resource graph and reports findings. Each pattern has a unique ID, description, and detection logic.

**When to use it:** When you want to detect specific anti-patterns or misconfigurations beyond risk issue scanning.

```bash
# List all registered patterns
./cub-scout patterns list

# Run pattern detection
./cub-scout patterns detect

# Run detection with JSON output
./cub-scout patterns detect --json

# Explain a specific pattern with results
./cub-scout patterns explain orphan-configmap
```

**Subcommands:**
| Command | Description |
|---------|-------------|
| `list` | List all registered patterns |
| `detect` | Run pattern detection against the cluster |
| `explain` | Explain a specific pattern with results |

**Related commands:** `scan`, `graph`

---

## `drift` — Drift Detection (v0.14.3)

Detect differences between desired state (from file/git) and live state (from cluster).

**What it does:** Compares what should exist (desired) against what actually exists (live) and reports any differences as drift findings.

**When to use it:** In CI/CD pipelines or before deployments to verify cluster state matches expectations.

```bash
# Compare a YAML file against the cluster
./cub-scout drift --file manifests/deployment.yaml

# Compare with namespace filter
./cub-scout drift --file manifests/ -n production

# Output as JSON (for CI/automation)
./cub-scout drift --file manifests/deployment.yaml --format json

# Fail CI if any warning or critical drift found
./cub-scout drift --file manifests/ --fail-on warning

# Fail CI only on critical drift
./cub-scout drift --file manifests/ --fail-on critical
```

**Options:**
| Option | Description |
|--------|-------------|
| `--file` | YAML file or directory containing desired state (required) |
| `-n, --namespace` | Namespace to compare (default: all) |
| `--format` | Output format: ascii, json (default: ascii) |
| `--fail-on` | Exit non-zero if max severity >= level (info, warning, critical) |

**Exit codes:**
| Code | Meaning |
|------|---------|
| 0 | No failure triggered (or no --fail-on specified) |
| 1 | Operational error |
| 2 | Findings met the --fail-on severity threshold |

**Related commands:** `scan`, `bundle diff`

---

## `apply` — Apply Proposal (GUI Integration)

Apply a Hub/App Space proposal to create resources in ConfigHub.

**What it does:** This is the GUI companion to `cub-scout import`. It reads a proposal JSON and creates the specified resources in ConfigHub.

**When to use it:** When integrating with a GUI that generates proposals, or for scripted fleet operations.

```bash
# Single cluster: generate, edit, apply
./cub-scout import --json > proposal.json
# (GUI displays, user edits)
./cub-scout apply proposal.json

# Fleet: multiple clusters → unified proposal → apply
./cub-scout import-cluster-aggregator cluster*.json --suggest --json | ./cub-scout apply -

# Dry-run to preview
./cub-scout apply proposal.json --dry-run
```

**Options:**
| Option | Description |
|--------|-------------|
| `--dry-run` | Preview what would be created without making changes |
| `--no-log` | Disable logging to file |

**Related commands:** `import`, `import-cluster-aggregator`

---

## `import-cluster-aggregator` — Fleet Import (GUI Integration)

Aggregate import data from multiple clusters into a fleet view.

**What it does:** This is the GUI/multi-cluster companion to `cub-scout import`. It combines import data from multiple clusters and can generate a unified proposal.

**When to use it:** When onboarding multiple clusters to ConfigHub simultaneously.

```bash
# Full workflow: scan clusters, generate unified proposal, apply
for ctx in cluster-a cluster-b; do
  kubectl config use-context $ctx
  ./cub-scout import --json > ${ctx}.json
done
./cub-scout import-cluster-aggregator cluster-*.json --suggest --json | ./cub-scout apply -

# Generate unified proposal
./cub-scout import-cluster-aggregator cluster1.json cluster2.json --suggest

# Just aggregate (no proposal)
./cub-scout import-cluster-aggregator cluster1.json cluster2.json cluster3.json
```

**Options:**
| Option | Description |
|--------|-------------|
| `--json` | Output as JSON |
| `--suggest` | Generate unified Hub/App Space proposal |

**Related commands:** `import`, `apply`

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
| `S` | Scan | Scan for risk issues |
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
| **ArgoCD** | `argocd.argoproj.io/instance` label (primary), `app.kubernetes.io/instance` fallback, or `argocd.argoproj.io/tracking-id` annotation |
| **Helm** | `app.kubernetes.io/managed-by: Helm` label |
| **Crossplane** | `crossplane.io/claim-name` label or `*.crossplane.io` owner refs *(experimental)* |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | None of the above (kubectl-applied) |

**Priority:** Flux > ArgoCD > Helm > Crossplane > ConfigHub > Native

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig file |
| `CLUSTER_NAME` | `default` | Name for this cluster in output |
| `CUB_SCOUT_ASSUME_YES` | `0` | Skip confirmation prompts in example scripts (`1` = yes) |
| `CUB_SCOUT_TELEMETRY` | `1` | Enable/disable anonymous usage telemetry (`0` = disabled) |
| `CUB_SCOUT_GITHUB_API_BASE` | GitHub API | Override GitHub API base URL (for testing/enterprise) |

---

## Exit Codes

### General

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (check stderr) |
| 2 | No cluster connection |

### `bundle replay` and `drift` with `--fail-on`

When using `--fail-on <level>`, the exit code reflects the maximum severity found:

| Code | Meaning |
|------|---------|
| 0 | No findings at or above threshold |
| 1 | Error during execution |
| 3 | Info-level findings (when `--fail-on info`) |
| 4 | Warning-level findings (when `--fail-on warning`) |
| 5 | Critical-level findings (when `--fail-on critical`) |

**Example:**
```bash
# Exit non-zero if any critical drift found
./cub-scout drift --file desired.yaml --fail-on critical
echo $?  # 5 if critical drift, 0 otherwise

# CI pipeline usage
./cub-scout bundle replay ./bundle --fail-on warning || exit 1
```

---

## See Also

- [README.md](README.md) — Project overview
- [docs/FAQ.md](docs/FAQ.md) — Frequently asked questions
- [docs/semantic-contract.md](docs/semantic-contract.md) — Output format guarantees
- [docs/testing/README.md](docs/testing/README.md) — Testing documentation
- [CONTRIBUTING.md](CONTRIBUTING.md) — How to contribute
