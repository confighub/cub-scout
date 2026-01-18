# cub-scout CLI Guide

Every command, what it does, and how you'd do it without cub-scout.

---

## Core Commands

### `map` — Interactive TUI

**What it does:** Opens an interactive terminal UI showing all cluster resources grouped by owner.

```bash
./cub-scout map
```

**Without cub-scout:**
```bash
# You'd have to run multiple commands and correlate manually
kubectl get all -A -o wide
kubectl get all -A -o json | jq '.items[] | select(.metadata.labels["kustomize.toolkit.fluxcd.io/name"])'
kubectl get all -A -o json | jq '.items[] | select(.metadata.labels["argocd.argoproj.io/instance"])'
# ... and somehow merge the results
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

**TUI Keys:**
| Key | Action |
|-----|--------|
| `?` | Help |
| `q` | Quit |
| `j/k` | Navigate up/down |
| `Enter` | Expand/collapse |
| `/` | Search |
| `1-5` | Switch tabs |

---

### `map list` — Plain Text Output

**What it does:** Lists all resources with their owners in plain text (for scripting).

```bash
./cub-scout map list
```

**Without cub-scout:**
```bash
kubectl get all -A -o custom-columns='NAMESPACE:.metadata.namespace,KIND:.kind,NAME:.metadata.name,MANAGED_BY:.metadata.labels.app\.kubernetes\.io/managed-by'
# Missing: Flux labels, ArgoCD labels, proper owner resolution
```

**Expected output:**
```
NAMESPACE         KIND          NAME                          OWNER
flux-system       Deployment    source-controller             Flux
flux-system       Deployment    kustomize-controller          Flux
argocd            Deployment    argocd-server                 ArgoCD
argocd            Application   guestbook                     Native
monitoring        Deployment    prometheus                    Helm
default           Deployment    nginx                         Native
default           Service       nginx                         Native
```

**With query filter:**
```bash
./cub-scout map list -q "owner=Flux"
./cub-scout map list -q "namespace=prod*"
./cub-scout map list -q "owner=Native"  # Find shadow IT
```

---

### `trace` — Ownership Chain

**What it does:** Shows the full delivery chain from Git source to deployed resource.

```bash
./cub-scout trace deploy/nginx -n production
```

**Without cub-scout:**
```bash
# For Flux resources:
flux trace deployment nginx -n production

# For ArgoCD resources:
argocd app get <app-name> --show-resources
kubectl get application -A -o json | jq '...'  # Complex correlation

# For Helm resources:
helm list -A | grep ...
kubectl get secret -A -l owner=helm | ...

# You need to know which tool manages the resource first
```

**Expected output:**
```
TRACE: Deployment/nginx in production

  ✓ GitRepository/flux-system
    │ URL: https://github.com/myorg/infra
    │ Revision: main@sha1:abc123
    │
    └─▶ ✓ Kustomization/apps
          │ Path: ./apps/production
          │ Revision: main@sha1:abc123
          │
          └─▶ ✓ Deployment/nginx
                Status: Managed by Flux
```

**For ArgoCD apps:**
```bash
./cub-scout trace --app guestbook
```

**Reverse trace (from Pod up):**
```bash
./cub-scout trace pod/nginx-7d9b8c-x4k2p -n prod --reverse
```

---

### `map orphans` — Unmanaged Resources

**What it does:** Lists resources not managed by any GitOps tool (shadow IT).

```bash
./cub-scout map orphans
```

**Without cub-scout:**
```bash
# Check for resources WITHOUT any GitOps labels
kubectl get all -A -o json | jq '
  .items[] |
  select(
    (.metadata.labels["kustomize.toolkit.fluxcd.io/name"] == null) and
    (.metadata.labels["argocd.argoproj.io/instance"] == null) and
    (.metadata.labels["app.kubernetes.io/managed-by"] != "Helm")
  ) |
  "\(.metadata.namespace)/\(.kind)/\(.metadata.name)"
'
# Complex, error-prone, and doesn't cover all cases
```

**Expected output:**
```
ORPHAN RESOURCES (not managed by GitOps)
═══════════════════════════════════════════════════════════════════

NAMESPACE         KIND          NAME                    AGE
default           Deployment    debug-pod               3d
default           ConfigMap     test-config             5d
kube-system       ConfigMap     manual-override         12d

Total: 3 orphaned resources

These resources were created via kubectl apply or direct API calls.
Consider importing them into your GitOps workflow.
```

---

### `scan` — Configuration Issues

**What it does:** Scans for CCVEs (Configuration Common Vulnerabilities and Errors).

```bash
./cub-scout scan
```

**Without cub-scout:**
```bash
# Check for stuck Flux resources
kubectl get kustomizations -A -o json | jq '.items[] | select(.status.conditions[] | select(.type=="Ready" and .status!="True"))'

# Check for stuck ArgoCD apps
kubectl get applications -A -o json | jq '.items[] | select(.status.sync.status != "Synced")'

# Check for failed Helm releases
helm list -A --failed

# Check Kyverno violations
kubectl get policyreports -A -o json | jq '...'

# Repeat for 40+ other patterns...
```

**Expected output:**
```
CCVE SCAN: kind-kind
═══════════════════════════════════════════════════════════════════

CRITICAL (1)
───────────────────────────────────────────────────────────────────
[CCVE-2025-0001] GitRepository not ready
  Resource: flux-system/GitRepository/apps
  Message:  authentication required but no credentials found
  Fix:      kubectl create secret generic git-credentials ...

WARNING (2)
───────────────────────────────────────────────────────────────────
[CCVE-2025-0005] Application out of sync
  Resource: argocd/Application/guestbook
  Message:  OutOfSync - live differs from Git
  Fix:      argocd app sync guestbook

[CCVE-2025-0056] Kustomization reconciliation stalled
  Resource: flux-system/Kustomization/apps
  Message:  Last reconciled 2 hours ago
  Fix:      flux reconcile kustomization apps

═══════════════════════════════════════════════════════════════════
Summary: 1 critical, 2 warning, 0 info
```

**Scan options:**
```bash
./cub-scout scan --state            # Stuck reconciliations only
./cub-scout scan --kyverno          # Kyverno violations only
./cub-scout scan --timing-bombs     # Expiring certs, quota limits
./cub-scout scan --dangling         # Orphan HPAs, Services, etc.
./cub-scout scan -n production      # Specific namespace
./cub-scout scan --json             # JSON output for tooling
./cub-scout scan --file manifest.yaml  # Static analysis (no cluster)
./cub-scout scan --list             # List all CCVE patterns
```

---

### `snapshot` — Export State as JSON

**What it does:** Dumps cluster state as GSF (GitOps State Format) JSON.

```bash
./cub-scout snapshot -o state.json
```

**Without cub-scout:**
```bash
kubectl get all -A -o json > raw-state.json
# Missing: owner resolution, drift detection, relationship mapping
```

**Expected output (abbreviated):**
```json
{
  "cluster": "kind-kind",
  "timestamp": "2026-01-18T10:30:00Z",
  "entries": [
    {
      "id": "production/Deployment/nginx",
      "kind": "Deployment",
      "namespace": "production",
      "name": "nginx",
      "owner": {
        "type": "flux",
        "ref": "flux-system/Kustomization/apps"
      }
    }
  ]
}
```

**Pipe to other tools:**
```bash
./cub-scout snapshot -o - | jq '.entries[] | select(.owner.type == "Native")'
./cub-scout snapshot -o - | your-custom-analyzer
```

---

## Map Subcommands

### `map deep-dive` — All Cluster Data

**What it does:** Shows maximum detail for all GitOps resources with live tree views.

```bash
./cub-scout map deep-dive
```

Shows: Flux GitRepositories, Kustomizations, HelmReleases → ArgoCD Applications, AppProjects → Helm releases decoded from secrets → Deployment → ReplicaSet → Pod trees.

---

### `map app-hierarchy` — Inferred Structure

**What it does:** Infers ConfigHub-style application hierarchy from cluster analysis.

```bash
./cub-scout map app-hierarchy
```

Groups workloads by inferred units based on labels, namespaces, and ownership.

---

### `map deployers` — GitOps Deployers

**What it does:** Lists all Flux Kustomizations, HelmReleases, and ArgoCD Applications.

```bash
./cub-scout map deployers
```

**Without cub-scout:**
```bash
kubectl get kustomizations -A
kubectl get helmreleases -A
kubectl get applications -A
# Three separate commands, no unified view
```

---

### `map workloads` — Workloads by Owner

**What it does:** Lists workloads (Deployments, StatefulSets, DaemonSets) grouped by owner.

```bash
./cub-scout map workloads
```

---

### `map issues` — Resources with Problems

**What it does:** Shows resources that are failing, stuck, or have conditions != Ready.

```bash
./cub-scout map issues
```

**Without cub-scout:**
```bash
kubectl get all -A -o json | jq '.items[] | select(.status.conditions[] | select(.status != "True"))'
# Messy, incomplete, doesn't cover all resource types
```

---

### `map crashes` — Failing Pods

**What it does:** Lists pods in CrashLoopBackOff, Error, or unhealthy deployments.

```bash
./cub-scout map crashes
```

**Without cub-scout:**
```bash
kubectl get pods -A --field-selector=status.phase!=Running,status.phase!=Succeeded
kubectl get pods -A | grep -E 'CrashLoop|Error|ImagePull'
```

---

### `map drift` — Desired vs Actual

**What it does:** Shows resources where live state differs from last-applied configuration.

```bash
./cub-scout map drift
```

**Without cub-scout:**
```bash
# Compare last-applied-configuration annotation to live spec
# Extremely complex, no native kubectl support
```

---

### `map status` — One-Line Health

**What it does:** Quick health check of the cluster.

```bash
./cub-scout map status
```

**Expected output:**
```
kind-kind: 45 resources | Flux: 12 ok | ArgoCD: 8 ok | Helm: 3 ok | Native: 22 | Issues: 0
```

---

### `map fleet` — Multi-Cluster View

**What it does:** Fleet view showing resources grouped by app and variant (Hub/AppSpace model).

```bash
./cub-scout map fleet
```

Requires ConfigHub labels on resources.

---

### `map hub` — ConfigHub Hierarchy

**What it does:** Interactive TUI showing ConfigHub hierarchy (requires `cub auth login`).

```bash
./cub-scout map --hub
# or
./cub-scout map hub
```

---

## Import Commands (Connected Mode)

These commands require ConfigHub authentication (`cub auth login`).

### `import` — Import Namespace

```bash
./cub-scout import -n production
./cub-scout import -n production --dry-run
```

### `import-argocd` — Import ArgoCD App

```bash
./cub-scout import-argocd guestbook
```

---

## Utility Commands

### `version`

```bash
./cub-scout version
```

### `completion` — Shell Completions

```bash
./cub-scout completion bash > /etc/bash_completion.d/cub-scout
./cub-scout completion zsh > "${fpath[1]}/_cub-scout"
```

### `demo` — Interactive Demos

```bash
./cub-scout demo
```

---

## Query Syntax

Filter resources with `-q`:

```bash
./cub-scout map list -q "owner=Flux"
./cub-scout map list -q "owner=Native"           # Shadow IT
./cub-scout map list -q "namespace=prod*"        # Wildcard
./cub-scout map list -q "kind=Deployment"
./cub-scout map list -q "owner=Flux AND namespace=production"
./cub-scout map list -q "owner=Flux OR owner=ArgoCD"
```

**Query fields:**
| Field | Values |
|-------|--------|
| `owner` | Flux, ArgoCD, Helm, ConfigHub, Native |
| `namespace` | Any namespace (supports wildcards) |
| `kind` | Deployment, Service, ConfigMap, etc. |
| `name` | Resource name (supports wildcards) |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBECONFIG` | `~/.kube/config` | Path to kubeconfig |
| `CLUSTER_NAME` | `default` | Name for this cluster in output |

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
- [CONTRIBUTING.md](CONTRIBUTING.md) — How to contribute
- [docs/SCAN-GUIDE.md](docs/SCAN-GUIDE.md) — Deep dive on CCVE scanning
- [docs/ALTERNATIVES.md](docs/ALTERNATIVES.md) — Comparison with other tools
