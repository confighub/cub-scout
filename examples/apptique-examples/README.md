# Apptique Examples — Real-World GitOps Patterns

Real-world examples demonstrating ConfigHub Agent's ownership detection across multiple GitOps patterns.

> **Source:** Based on [Google's Online Boutique](https://github.com/GoogleCloudPlatform/microservices-demo) (microservices-demo), adapted from [Brian's KubeCon 2025 demos](https://github.com/confighub-kubecon-2025).

---

## What's Here

| Pattern | Tool | Layout | Use Case |
|---------|------|--------|----------|
| [flux-monorepo/](flux-monorepo/) | **Flux** | Monorepo with overlays | Most common Flux pattern |
| [argo-applicationset/](argo-applicationset/) | **Argo CD** | ApplicationSet + directories | Auto-discover environments |
| [argo-app-of-apps/](argo-app-of-apps/) | **Argo CD** | Parent manages children | Enterprise hierarchy |
| [scenarios/](scenarios/) | — | RM goal demos | Monday Panic, Drift, Security Patch |
| [confighub/](confighub/) | — | Hub/Space definitions | Skeleton → hierarchy mapping |
| [source/](source/) | — | Original KubeCon repos | Reference only |

---

## Prerequisites

### For Flux Patterns

```bash
# Install Flux CLI
curl -s https://fluxcd.io/install.sh | sudo bash

# Bootstrap Flux on your cluster (or use existing)
flux check --pre
flux install
```

### For Argo CD Patterns

```bash
# Install Argo CD
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for Argo CD to be ready
kubectl wait --for=condition=available deployment/argocd-server -n argocd --timeout=300s

# Get admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
```

---

## Deployment Instructions

### Option 1: Flux Monorepo Pattern

This is the most common Flux pattern — one repo with Kustomize overlays for different environments.

```bash
# Step 1: Create the GitRepository source
kubectl apply -f - <<EOF
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: apptique-examples
  namespace: flux-system
spec:
  interval: 1m
  url: https://github.com/confighubai/confighub-agent
  ref:
    branch: main
EOF

# Step 2: Deploy dev environment
kubectl apply -f - <<EOF
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apptique-dev
  namespace: flux-system
spec:
  interval: 5m
  path: ./examples/apptique-examples/flux-monorepo/apps/apptique/overlays/dev
  prune: true
  sourceRef:
    kind: GitRepository
    name: apptique-examples
  targetNamespace: apptique-dev
EOF

# Step 3: (Optional) Deploy prod environment
kubectl apply -f - <<EOF
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apptique-prod
  namespace: flux-system
spec:
  interval: 5m
  path: ./examples/apptique-examples/flux-monorepo/apps/apptique/overlays/prod
  prune: true
  sourceRef:
    kind: GitRepository
    name: apptique-examples
  targetNamespace: apptique-prod
EOF

# Step 4: Watch Flux reconcile
flux get kustomizations --watch
```

### Option 2: Argo CD ApplicationSet Pattern

Auto-discovers environments from directory structure.

```bash
# Step 1: Apply the ApplicationSet
kubectl apply -f examples/apptique-examples/argo-applicationset/bootstrap/applicationset.yaml

# Step 2: Watch Applications get created
kubectl get applications -n argocd --watch

# Expected: apptique-dev and apptique-prod Applications created automatically
```

### Option 3: Argo CD App of Apps Pattern

Parent Application manages child Applications — enterprise pattern.

```bash
# Step 1: Apply the root Application
kubectl apply -f examples/apptique-examples/argo-app-of-apps/root/root-app.yaml

# Step 2: Watch the hierarchy unfold
kubectl get applications -n argocd --watch

# Expected:
# - apptique-apps (root) → Synced
# - apptique-dev (child) → Synced
# - apptique-prod (child) → Synced
```

---

## Testing with TUI (Terminal)

### 1. Verify Deployment

```bash
# Check pods are running
kubectl get pods -n apptique-dev
kubectl get pods -n apptique-prod

# Expected output:
# NAME                        READY   STATUS    RESTARTS   AGE
# frontend-xxxxx-xxxxx        1/1     Running   0          2m
```

### 2. Test Ownership Detection

```bash
# Run the map command to see ownership
./test/atk/map

# Expected: Health dashboard showing apptique workloads
```

```bash
# List workloads with owners
./test/atk/map workloads

# Expected output for Flux pattern:
# STATUS  NAMESPACE      NAME      OWNER   MANAGED-BY    IMAGE
# ✓       apptique-dev   frontend  Flux    apptique-dev  frontend:v0.10.3
# ✓       apptique-prod  frontend  Flux    apptique-prod frontend:v0.10.3

# Expected output for Argo patterns:
# STATUS  NAMESPACE      NAME      OWNER   MANAGED-BY     IMAGE
# ✓       apptique-dev   frontend  ArgoCD  apptique-dev   frontend:v0.10.3
# ✓       apptique-prod  frontend  ArgoCD  apptique-prod  frontend:v0.10.3
```

### 3. Test Trace Command

```bash
# Trace ownership chain for Flux-managed deployment
cub-scout trace deployment/frontend -n apptique-dev

# Expected output (Flux):
# TRACE: deployment/frontend
#   ┌─────────────────────────────────────────────┐
#   │ deployment/frontend                          │
#   │ namespace: apptique-dev                      │
#   │ owner: Flux                                  │
#   └─────────────────────────────────────────────┘
#            │
#            ▼
#   ┌─────────────────────────────────────────────┐
#   │ Kustomization/apptique-dev                   │
#   │ namespace: flux-system                       │
#   └─────────────────────────────────────────────┘
#            │
#            ▼
#   ┌─────────────────────────────────────────────┐
#   │ GitRepository/apptique-examples              │
#   │ url: https://github.com/confighubai/...      │
#   └─────────────────────────────────────────────┘
```

```bash
# Trace ownership chain for Argo-managed deployment
cub-scout trace deployment/frontend -n apptique-prod

# Expected output (Argo):
# TRACE: deployment/frontend
#   deployment/frontend → Application/apptique-prod → Git repo
```

### 4. Test CCVE Scanning

```bash
# Scan for configuration issues
./test/atk/scan

# Expected: No critical CCVEs in clean deployment
# (The apptique manifests follow best practices)
```

### 5. Test Query Language

```bash
# Find all Flux-managed workloads
cub-scout map list -q "owner=Flux"

# Find all Argo-managed workloads
cub-scout map list -q "owner=ArgoCD"

# Find workloads in dev namespaces
cub-scout map list -q "namespace=*-dev"

# Find apptique workloads specifically
cub-scout map list -q "labels[app]=apptique"
```

---

## Testing with GUI (ConfigHub Web UI)

### 1. Connect to ConfigHub

```bash
# Ensure you're authenticated
cub auth status

# If not authenticated:
cub auth login
```

### 2. Verify in ConfigHub Dashboard

1. **Open ConfigHub UI**: Navigate to https://hub.confighub.com

2. **Check Organizations**:
   - Go to Organizations view
   - Verify your organization appears

3. **Check Spaces**:
   - Navigate to your space (e.g., `platform-dev`)
   - Verify workers are connected (green status)

4. **Check Targets**:
   - Click into your space
   - Verify the cluster target shows connected

### 3. View Workloads in GUI

1. **Navigate to Workloads**:
   - Click on your target cluster
   - Go to "Workloads" tab

2. **Verify Apptique Deployments**:
   - Look for `apptique-dev` and `apptique-prod` namespaces
   - Verify `frontend` deployment appears
   - Check owner column shows "Flux" or "ArgoCD"

3. **Test Drill-Down**:
   - Click on a deployment
   - Verify you can see pod details
   - Check resource usage graphs

### 4. Test ConfigHub Map View

```bash
# Run map with ConfigHub hierarchy
./test/atk/map confighub

# Expected: Shows ConfigHub hierarchy with your units
```

### 5. Verify via Argo CD UI (for Argo patterns)

```bash
# Port-forward Argo CD UI
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Open https://localhost:8080
# Login with admin / <password from above>
```

1. **Check Applications**:
   - For ApplicationSet: See `apptique-dev` and `apptique-prod` applications
   - For App of Apps: See `apptique-apps` parent with children

2. **Verify Sync Status**:
   - All applications should show "Synced" (green)
   - Click into each to see resource tree

3. **Test Refresh**:
   - Click "Refresh" on an application
   - Verify it re-syncs from Git

---

## RM Scenarios (Rendered Manifest Goals)

These scenarios demonstrate the **Rendered Manifest pattern goals** using real Kubernetes resources.

> **Unlike the simulation demos in `rm-demos-argocd/`**, these are **working deployments** that you can apply to your cluster and test with the ConfigHub Agent.

| Scenario | Pain Point | Demo |
|----------|------------|------|
| **[Monday Panic](scenarios/monday-panic/)** | "47 clusters, where's the problem?" | Find broken deployment in 30 seconds |
| **[Drift Detection](scenarios/drift-detection/)** | "Someone edited prod directly" | Detect kubectl changes |
| **[Security Patch](scenarios/security-patch/)** | "CVE affects 847 services" | Find and fix CCVEs |

### Quick Start — Monday Panic

```bash
# 1. Deploy the scenario (creates 3 namespaces simulating clusters)
kubectl apply -k examples/apptique-examples/scenarios/monday-panic/

# 2. Find the problem
./test/atk/map workloads

# Expected output:
# STATUS  NAMESPACE      NAME      OWNER  IMAGE
# ✓       apptique-east  frontend  Flux   nginx:alpine
# ✓       apptique-west  frontend  Flux   nginx:alpine
# ✗       apptique-eu    frontend  Flux   frontend:v0.10.3-broken  ← PROBLEM!

# 3. Cleanup
kubectl delete -k examples/apptique-examples/scenarios/monday-panic/
```

### Quick Start — Drift Detection

```bash
# 1. Deploy base configuration
kubectl apply -f examples/apptique-examples/scenarios/drift-detection/base-deployment.yaml

# 2. Create drift (simulates someone editing prod directly)
./examples/apptique-examples/scenarios/drift-detection/create-drift.sh

# 3. Detect the drift
./cub-scout trace deployment/frontend -n apptique-drift

# 4. Cleanup
kubectl delete -f examples/apptique-examples/scenarios/drift-detection/base-deployment.yaml
```

### Quick Start — Security Patch

```bash
# 1. Deploy vulnerable deployments
kubectl apply -k examples/apptique-examples/scenarios/security-patch/

# 2. Scan for CCVEs
./test/atk/scan

# Expected output:
# CRITICAL (1)
# [CCVE-2025-0027] apptique-vulnerable/grafana-ccve
#
# HIGH (2)
# [CCVE-2025-0001] apptique-vulnerable/no-limits
# [CCVE-2025-0003] apptique-vulnerable/latest-tag

# 3. Cleanup
kubectl delete -k examples/apptique-examples/scenarios/security-patch/
```

See [scenarios/README.md](scenarios/README.md) for detailed walkthroughs of each scenario.

---

## ConfigHub Hierarchy Visualization

See how your repo skeleton maps to ConfigHub's **Hub → App Space → Unit** model:

```bash
# Run the interactive TUI demo
./examples/apptique-examples/confighub/demo-hierarchy.sh
```

### The Core Mapping

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        YOUR GITOPS REPO                                  │
│                                                                          │
│   apps/                     ──────────────────▶  App Spaces              │
│   ├── frontend/                                                          │
│   │   └── overlays/         ──────────────────▶  Units + Variants        │
│   │       ├── dev/                               (frontend: dev, prod)   │
│   │       └── prod/                                                      │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         CONFIGHUB HIERARCHY                              │
│                                                                          │
│   Hub: apptique-platform                                                │
│   └── App Space: apptique                                               │
│       └── Unit: frontend                                                │
│           ├── variant: dev   → target: apptique-dev                     │
│           └── variant: prod  → target: apptique-prod                    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### Pattern-Specific Mappings

| Pattern | Repo Structure | ConfigHub Mapping |
|---------|---------------|-------------------|
| **Flux Monorepo** | `overlays/{env}/` | Unit variants |
| **Argo ApplicationSet** | ApplicationSet CR | Generator Unit → Instance Units |
| **Argo App-of-Apps** | Child Applications | Child Apps → Units (root ignored) |

### Live Demo

```bash
# 1. Deploy any pattern
kubectl apply -k flux-monorepo/apps/apptique/overlays/dev/

# 2. Import to ConfigHub
./cub-scout import -n apptique-dev

# 3. View the hierarchy
./test/atk/map confighub

# Expected:
# Hub: apptique-platform
# └── App Space: apptique
#     └── Unit: frontend
#         └── dev ✓ synced @ rev 127
```

See [confighub/README.md](confighub/README.md) for detailed skeleton-to-hierarchy mapping documentation.

---

## Cleanup

### Remove Flux Pattern

```bash
# Delete Kustomizations (this also deletes managed resources)
kubectl delete kustomization apptique-dev apptique-prod -n flux-system

# Delete GitRepository
kubectl delete gitrepository apptique-examples -n flux-system

# Verify namespaces are gone
kubectl get ns | grep apptique
```

### Remove Argo CD Patterns

```bash
# For ApplicationSet pattern
kubectl delete applicationset apptique -n argocd

# For App of Apps pattern
kubectl delete application apptique-apps -n argocd

# Verify Applications are gone
kubectl get applications -n argocd

# Clean up namespaces if needed
kubectl delete ns apptique-dev apptique-prod
```

---

## Pattern Details

### A1: Flux Monorepo

The most common Flux pattern — one repo with environment overlays using Kustomize.

```
flux-monorepo/
├── clusters/
│   ├── dev/kustomization.yaml      # Flux Kustomization CR
│   └── prod/kustomization.yaml
├── apps/apptique/
│   ├── base/                       # Shared manifests
│   └── overlays/
│       ├── dev/                    # Dev-specific patches
│       └── prod/                   # Prod-specific patches
└── infrastructure/
    └── gitrepository.yaml          # Flux GitRepository CR
```

**Ownership labels added by Flux:**
```yaml
labels:
  kustomize.toolkit.fluxcd.io/name: apptique-dev
  kustomize.toolkit.fluxcd.io/namespace: flux-system
```

### B1: Argo CD ApplicationSet

Auto-discovers environments from directory structure using ApplicationSet generators.

```
argo-applicationset/
├── bootstrap/
│   └── applicationset.yaml         # Directory generator
└── apps/apptique/
    ├── dev/deployment.yaml         # Auto-discovered
    └── prod/deployment.yaml        # Auto-discovered
```

**How it works:**
1. ApplicationSet scans `apps/apptique/*` directories
2. Generates one Application per directory (dev, prod)
3. Each Application deploys to its own namespace

**Ownership labels:**
```yaml
labels:
  app.kubernetes.io/instance: apptique-dev
  argocd.argoproj.io/instance: apptique-dev
```

### B4: Argo CD App of Apps

Parent Application manages child Applications — common in enterprise setups.

```
argo-app-of-apps/
├── root/
│   └── root-app.yaml               # Parent Application
├── apps/
│   ├── apptique-dev.yaml           # Child Application CR
│   └── apptique-prod.yaml          # Child Application CR
└── manifests/apptique/
    ├── dev/deployment.yaml         # Actual K8s manifests
    └── prod/deployment.yaml
```

**How it works:**
1. Root app syncs `apps/` directory → creates child Applications
2. Child apps sync their respective manifests
3. Two-level hierarchy: Root → Child App → Workloads

**When to use:**
- Multi-team environments where teams own their Application CRs
- Need to version Application configurations separately from workloads
- Want to use Argo CD RBAC at the Application level

---

## Troubleshooting

### Flux Not Reconciling

```bash
# Check Flux status
flux get all

# Check for errors
flux logs --level=error

# Force reconciliation
flux reconcile kustomization apptique-dev
```

### Argo CD Application Not Syncing

```bash
# Check Application status
kubectl get applications -n argocd

# Describe for errors
kubectl describe application apptique-dev -n argocd

# Force sync via CLI
argocd app sync apptique-dev
```

### Ownership Not Detected

```bash
# Check labels on deployment
kubectl get deployment frontend -n apptique-dev -o yaml | grep -A10 labels

# For Flux, look for:
#   kustomize.toolkit.fluxcd.io/name: apptique-dev

# For Argo, look for:
#   app.kubernetes.io/instance: apptique-dev
#   argocd.argoproj.io/instance: apptique-dev
```

---

## Source Apps

The `source/` directory contains copies of Brian's KubeCon 2025 demo repos:

| Repo | Description |
|------|-------------|
| `setup/` | Cluster setup scripts |
| `apptique/` | Store app (Google Online Boutique fork) |

These are reference copies — the patterns above are adapted from this source.

---

## See Also

- [examples/README.md](../README.md) — All examples overview
- [docs/EXAMPLES-OVERVIEW.md](../../docs/EXAMPLES-OVERVIEW.md) — Central examples reference
- [rm-demos-argocd/](../rm-demos-argocd/) — Simulation demos (sales presentations)
- [docs/planning/RENDERED-MANIFEST-PATTERN.md](../../docs/planning/RENDERED-MANIFEST-PATTERN.md) — Full RM pattern documentation
- [PLAN-APPTIQUE-EXAMPLES.md](../../docs/planning/sessions/PLAN-APPTIQUE-EXAMPLES.md) — Implementation plan
