# See cub-scout at Scale

This guide shows cub-scout exploring a realistic Flux deployment with 50+ resources across multiple environments.

## Why Scale Matters

cub-scout's value shows at scale:
- **Orphan detection** finds the resources you forgot about
- **deep-dive** shows complex Deployment→ReplicaSet→Pod relationships
- **app-hierarchy** infers logical groupings from labels

With 5 resources, you don't need a tool. With 500, you do.

## Prerequisites

- kind or any Kubernetes cluster
- flux CLI (`brew install fluxcd/tap/flux`)
- cub-scout (`brew install confighub/tap/cub-scout`)

---

## Option 1: Quick Scale Test (10 minutes)

Deploy the official Flux reference architecture to see cub-scout at realistic scale.

### Step 1: Create Cluster

```bash
kind create cluster --name flux-scale-demo
```

### Step 2: Install Flux

```bash
flux install
```

### Step 3: Deploy Reference Architecture

```bash
# Clone the official Flux example
git clone https://github.com/fluxcd/flux2-kustomize-helm-example.git
cd flux2-kustomize-helm-example

# Apply the staging cluster configuration
kubectl apply -k clusters/staging
```

This deploys:
- Infrastructure components (ingress-nginx, cert-manager)
- Monitoring stack (Prometheus, Grafana)
- Sample applications across dev/staging namespaces
- HelmReleases managed by Flux

### Step 4: Wait for Resources

```bash
# Watch Flux reconcile everything
flux get all -A --watch

# Or wait for specific resources
kubectl wait --for=condition=available deployment --all --all-namespaces --timeout=300s
```

### Step 5: Explore with cub-scout

```bash
cub-scout map
```

Press:
- `s` - Status dashboard (see ownership breakdown)
- `w` - Workloads by owner
- `4` - Deep-dive (Deployment→Pod trees)
- `?` - Help

---

## Option 2: Add Orphan Resources

To see orphan detection in action, add unmanaged resources that simulate real-world drift.

```bash
kubectl apply -f https://raw.githubusercontent.com/confighub/cub-scout/main/examples/orphans/realistic-orphans.yaml
```

This creates:
- `legacy-apps` namespace with old monitoring
- `temp-testing` namespace with debug resources
- ConfigMaps and Secrets from manual operations
- CronJobs added outside GitOps

### Find the Orphans

```bash
cub-scout map orphans
```

Or in the TUI, press `o`.

You'll see ~15 resources with "Native" ownership - these are your orphans.

---

## What You'll See

### Status Dashboard (Press `s`)

```
cub-scout map

CLUSTER: flux-scale-demo
────────────────────────────────────────
Resources:  127 across 8 namespaces
Workloads:   47 (Deployments, StatefulSets, DaemonSets)

OWNERSHIP
  Flux      ████████████████████  68 (54%)
  Helm      ██████████░░░░░░░░░░  31 (24%)
  Native    ████████░░░░░░░░░░░░  28 (22%)

HEALTH
  Ready     ███████████████████░  119 (94%)
  Pending   ██░░░░░░░░░░░░░░░░░░    5 (4%)
  Failed    █░░░░░░░░░░░░░░░░░░░    3 (2%)
```

### Workloads by Owner (Press `w`)

```
WORKLOADS BY OWNER
────────────────────────────────────────
Flux (28)
  ├── podinfo           apps        Deployment  ✓
  ├── nginx-ingress     ingress     Deployment  ✓
  ├── cert-manager      cert-mgr    Deployment  ✓
  └── ...

Helm (12)
  ├── prometheus        monitoring  StatefulSet ✓
  ├── grafana           monitoring  Deployment  ✓
  └── ...

Native (7)
  ├── legacy-prometheus legacy-apps Deployment  ⚠ (orphan)
  ├── debug-nginx       temp-test   Deployment  ⚠ (orphan)
  └── ...
```

### Deep-Dive (Press `4`)

```
RESOURCE TREE
────────────────────────────────────────
Deployments (47)
├── podinfo [Flux: apps/podinfo]
│   └── ReplicaSet podinfo-7d4b8c9f
│       ├── Pod podinfo-7d4b8c9f-abc12  ✓ Running  10.0.0.15
│       └── Pod podinfo-7d4b8c9f-def34  ✓ Running  10.0.0.16
├── nginx-ingress [Helm: ingress-nginx]
│   └── ReplicaSet nginx-ingress-controller-6c5d7b
│       ├── Pod nginx-ingress-controller-6c5d7b-gh78  ✓ Running
│       └── Pod nginx-ingress-controller-6c5d7b-ij90  ✓ Running
├── legacy-prometheus [Native - ORPHAN]
│   └── ReplicaSet legacy-prometheus-8e6f9a
│       └── Pod legacy-prometheus-8e6f9a-xyz99  ✓ Running
```

### Orphans (Press `o`)

```
ORPHAN RESOURCES (28)
────────────────────────────────────────
These resources have no GitOps owner.

NAMESPACE       KIND         NAME                  AGE
legacy-apps     Deployment   legacy-prometheus     3d
legacy-apps     Service      legacy-prometheus     3d
temp-testing    Deployment   debug-nginx           1d
default         ConfigMap    old-feature-flags     7d
default         ConfigMap    manual-override       2d
default         Secret       manual-api-key        5d
default         Deployment   hotfix-worker         12h
default         CronJob      manual-cleanup        4d
...

Total: 28 orphan resources across 4 namespaces
```

### Trace (Press `T` on a resource)

```
cub-scout trace deploy/podinfo -n apps

OWNERSHIP CHAIN
────────────────────────────────────────
GitRepository/flux-system/flux-system
    ↓ source
Kustomization/flux-system/apps
    ↓ manages
Deployment/apps/podinfo
    ↓ creates
ReplicaSet/apps/podinfo-7d4b8c9f
    ↓ creates
Pod/apps/podinfo-7d4b8c9f-abc12  ✓ Running
```

---

## Cleanup

```bash
kind delete cluster --name flux-scale-demo
```

---

## Next Steps

- [CLI-GUIDE.md](../CLI-GUIDE.md) - Complete command reference
- [examples/](../examples/) - More demo scenarios
- [confighub.com](https://confighub.com) - Connect for multi-cluster visibility

---

## Troubleshooting

### Flux resources stuck in "Not Ready"

```bash
flux get all -A
flux logs --all-namespaces
```

### cub-scout shows fewer resources than expected

Make sure all namespaces are accessible:
```bash
kubectl get ns
cub-scout map list -q "namespace=*"
```

### Deep-dive shows empty trees

Pods may not have started yet. Wait for deployments:
```bash
kubectl get pods -A
```
