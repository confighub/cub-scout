# Demo Walkthrough: Mixed Ownership with risk issues

**Status: Working** — Step-by-step walkthrough with expected output at each step.

This example creates a realistic multi-owner cluster with ConfigHub-managed resources, introduces problems and risk issues, shows how to diagnose them using the map tool, then fixes them.

---

## Step 1: Apply Demo Fixtures

```bash
kubectl apply -f test/atk/demos/demo-full.yaml
```

**Expected output:**
```
namespace/demo-payments created
namespace/demo-orders created
namespace/demo-monitoring created
namespace/grafana created
deployment.apps/payment-api created
service/payment-api created
configmap/payment-api-config created
deployment.apps/order-processor created
service/order-processor created
gitrepository.source.toolkit.fluxcd.io/infra-repo created
kustomization.kustomize.toolkit.fluxcd.io/monitoring-stack created
helmrepository.source.toolkit.fluxcd.io/bitnami created
helmrelease.helm.toolkit.fluxcd.io/redis-cache created
application.argoproj.io/frontend-app created
deployment.apps/frontend created
deployment.apps/postgresql created
deployment.apps/debug-tools created
configmap/legacy-config created
deployment.apps/grafana created
service/grafana created
configmap/important-dashboard created
```

---

## Step 2: View Map Dashboard (With Problems)

```bash
cub-scout map
```

**Expected output:**
```
  🔥 7 FAILURE(S)   atk

  Deployers  0/3
  Workloads  11/16

  PROBLEMS
  ────────────────────────────────────────────────
  ✗ HelmRelease/redis-cache  SourceNotReady
  ✗ Application/frontend-app  null
  ⏸ Kustomization/monitoring-stack  suspended
  ✗ demo-monitoring/grafana  0/1 pods
  ✗ demo-orders/order-processor  0/2 pods
  ✗ demo-orders/postgresql  0/1 pods
  ✗ demo-payments/frontend  0/2 pods
  ✗ demo-payments/payment-api  0/3 pods

  PIPELINES
  ────────────────────────────────────────────────
  ⏸ company/infrastructure@main  →  monitoring-stack  →  0 resources
  ✗ company/frontend/k8s@HEAD  →  frontend-app  →  demo-payments

  OWNERSHIP
  ────────────────────────────────────────────────
  Argo(1) ConfigHub(2) Helm(1) Native(12)
  █████░░░░░░░░░░░

  ConfigHub Hierarchy:
  Org → Space → Unit (with Resources, Targets, Workers)

  Cluster Resources with ConfigHub Labels:
  orders-prod / order-processor-prod @ rev 89  [demo-orders/order-processor]
  payments-prod / payment-api-prod @ rev 127  [demo-payments/payment-api]
```

> **Note:** Use `cub-scout map --mode=hub` for experimental Hub → App Space → Application → Variant hierarchy.

---

## Step 3: View Workloads by Owner

```bash
cub-scout map workloads
```

**Expected output:**
```
STATUS  NAMESPACE                NAME                      OWNER       MANAGED-BY           IMAGE
────────────────────────────────────────────────────────────────────────────────────────────────────
✗       demo-payments           frontend                  ArgoCD      frontend-app        frontend:3.1.0
✗       demo-orders             order-processor           ConfigHub   order-processor-prod  processor:1.8.0
✗       demo-payments           payment-api               ConfigHub   payment-api-prod    api:2.4.1
✓       demo-orders             postgresql                Helm        orders-db           postgres:15
✓       argocd                  argocd-applicationset-controller  Native      -                   argocd:v3.2.3
✓       argocd                  argocd-notifications-controller  Native      -                   argocd:v3.2.3
✓       argocd                  argocd-repo-server        Native      -                   argocd:v3.2.3
✓       argocd                  argocd-server             Native      -                   argocd:v3.2.3
✓       demo-payments           debug-tools               Native      -                   busybox:1.36
✓       argocd                  argocd-dex-server         Native      -                   dex:v2.43.0
✓       demo-monitoring         grafana                   Native      -                   grafana:10.2.0
✓       flux-system             helm-controller           Native      -                   helm-controller:v1.3.0
✓       flux-system             kustomize-controller      Native      -                   kustomize-controller:v1.6.1
✓       flux-system             notification-controller   Native      -                   notification-controller:v1.6.0
✓       argocd                  argocd-redis              Native      -                   redis:8.2.2-alpine
✓       flux-system             source-controller         Native      -                   source-controller:v1.6.2
```

---

## Step 4: View Problems Only

```bash
cub-scout map problems
```

**Expected output:**
```
✗ HelmRelease/redis-cache in flux-system: SourceNotReady
✗ Application/frontend-app in argocd: null
⏸ Kustomization/monitoring-stack in flux-system: suspended
✗ Deployment/order-processor in demo-orders: 0/2 ready
✗ Deployment/frontend in demo-payments: 0/2 ready
✗ Deployment/payment-api in demo-payments: 0/3 ready
```

---

## Step 5: View Deployers Status

```bash
cub-scout map deployers
```

**Expected output:**
```
STATUS  KIND            NAME                      NAMESPACE            REVISION   RESOURCES
─────────────────────────────────────────────────────────────────────────────────────────────
⏸       Kustomization   monitoring-stack          flux-system                    0
✗       HelmRelease     redis-cache               flux-system                    -
✗       Application     frontend-app              argocd              HEAD       0
```

---

## Step 6: View Suspended Resources

```bash
cub-scout map suspended
```

**Expected output:**
```
⏸ Kustomization/monitoring-stack in flux-system
```

---

## Step 7: Scan for risk issues

```bash
cub-scout scan
```

**Expected output:**
```
CONFIG CVE SCAN: kind-atk
════════════════════════════════════════════════════════════════════

INFO (1)
────────────────────────────────────────────────────────────────────
[RISK-FLUX-005] flux-system/monitoring-stack

════════════════════════════════════════════════════════════════════
Summary: 0 critical, 0 warning, 1 info

⚠ Run './scan <RISK-ID>' for remediation steps
```

---

## Step 8: Scan with JSON Output

```bash
cub-scout scan --json
```

**Expected output:**
```json
{
  "cluster": "kind-atk",
  "scannedAt": "2025-12-31T09:27:36Z",
  "summary": {
    "critical": 0,
    "warning": 0,
    "info": 1
  },
  "findings": [
    {"id":"RISK-FLUX-005","resource":"flux-system/monitoring-stack","severity":"Info"}
  ]
}
```

---

## Step 9: Fix All Problems

```bash
# Fix 1: Resume the suspended Kustomization
kubectl patch kustomization monitoring-stack -n flux-system --type=merge -p '{"spec":{"suspend":false}}'

# Fix 2: Delete the broken HelmRelease (wrong chart version)
kubectl delete helmrelease redis-cache -n flux-system

# Fix 3: Delete the broken Argo Application (repo doesn't exist)
kubectl delete application frontend-app -n argocd

# Fix 4: Fix workloads by using real images
kubectl set image deployment/payment-api -n demo-payments api=nginx:alpine
kubectl set image deployment/order-processor -n demo-orders processor=nginx:alpine
kubectl set image deployment/frontend -n demo-payments frontend=nginx:alpine

# Remove the broken Flux resources (fake git repo)
kubectl delete kustomization monitoring-stack -n flux-system
kubectl delete gitrepository infra-repo -n flux-system
kubectl delete helmrepository bitnami -n flux-system
```

---

## Step 10: View Healthy Map

```bash
cub-scout map
```

**Expected output:**
```
  ✓ ALL HEALTHY   atk

  Deployers  0/0 ✓
  Workloads  16/16 ✓

  PIPELINES
  ────────────────────────────────────────────────

  OWNERSHIP
  ────────────────────────────────────────────────
  Argo(1) ConfigHub(2) Helm(1) Native(12)
  ████░░░░░░░░░░░░

  ConfigHub Hierarchy:
  Org → Space → Unit (with Resources, Targets, Workers)

  Cluster Resources with ConfigHub Labels:
  orders-prod / order-processor-prod @ rev 89  [demo-orders/order-processor]
  payments-prod / payment-api-prod @ rev 127  [demo-payments/payment-api]
```

---

## Step 11: View Healthy Workloads

```bash
cub-scout map workloads
```

**Expected output:**
```
STATUS  NAMESPACE                NAME                      OWNER       MANAGED-BY           IMAGE
────────────────────────────────────────────────────────────────────────────────────────────────────
✓       demo-payments           frontend                  ArgoCD      frontend-app        nginx:alpine
✓       demo-orders             order-processor           ConfigHub   order-processor-prod  nginx:alpine
✓       demo-payments           payment-api               ConfigHub   payment-api-prod    nginx:alpine
✓       demo-orders             postgresql                Helm        orders-db           postgres:15
✓       argocd                  argocd-applicationset-controller  Native      -                   argocd:v3.2.3
✓       argocd                  argocd-notifications-controller  Native      -                   argocd:v3.2.3
✓       argocd                  argocd-repo-server        Native      -                   argocd:v3.2.3
✓       argocd                  argocd-server             Native      -                   argocd:v3.2.3
✓       demo-payments           debug-tools               Native      -                   busybox:1.36
✓       argocd                  argocd-dex-server         Native      -                   dex:v2.43.0
✓       demo-monitoring         grafana                   Native      -                   grafana:10.2.0
✓       flux-system             helm-controller           Native      -                   helm-controller:v1.3.0
✓       flux-system             kustomize-controller      Native      -                   kustomize-controller:v1.6.1
✓       flux-system             notification-controller   Native      -                   notification-controller:v1.6.0
✓       argocd                  argocd-redis              Native      -                   redis:8.2.2-alpine
✓       flux-system             source-controller         Native      -                   source-controller:v1.6.2
```

---

## Step 12: Verify No risk issues

```bash
cub-scout scan
```

**Expected output:**
```
CONFIG CVE SCAN: kind-atk
════════════════════════════════════════════════════════════════════

✓ No Config CVEs detected
```

---

## Step 13: Cleanup

```bash
kubectl delete -f test/atk/demos/demo-full.yaml
```

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `cub-scout map` | Full dashboard |
| `cub-scout map status` | One-line health check |
| `cub-scout map workloads` | List workloads by owner |
| `cub-scout map problems` | Show only problems |
| `cub-scout map deployers` | List GitOps deployers |
| `cub-scout map suspended` | List suspended resources |
| `cub-scout map confighub` | ConfigHub hierarchy (requires cub auth) |
| `cub-scout map --json` | JSON output |
| `cub-scout map --mode=hub` | Experimental hub hierarchy mode |
| `cub-scout scan` | Scan for risk issues |
| `cub-scout scan --list` | List all risk issues |
| `cub-scout scan --json` | JSON output |

### Hierarchy Display Modes

| Mode | Flag | Hierarchy |
|------|------|-----------|
| **Standard** (default) | `--mode=standard` | Org → Space → Unit |
| **Hub** (experimental) | `--mode=hub` | Hub → App Space → Application → Variant |

---

## Demo Runner

Use the built-in demo runner:

```bash
cub-scout demo list                      # List available demos
cub-scout demo quick                     # Ownership + map overview
cub-scout demo ccve                      # BIGBANK risk issue story
cub-scout demo query                     # Query language walkthrough
cub-scout demo scenario bigbank-incident # Narrative outage scenario
cub-scout demo scenario break-glass      # Emergency kubectl scenario
cub-scout demo quick --no-pods           # Fast structural run
cub-scout demo quick --cleanup           # Remove demo resources
```

### Demo Focus Areas

- `quick` — fast ownership detection and status views.
- `ccve` — reproduces and detects the RISK-2025-0027 Grafana namespace bug.
- `query` — demonstrates query filtering for owners, namespaces, and orphans.
- `scenario bigbank-incident` — guided outage story based on real incident patterns.
- `scenario break-glass` — break-glass deployment plus follow-up decision workflow.

---

## See Also

- [README.md](README.md) — Demo overview
- [../README.md](../README.md) — All examples
