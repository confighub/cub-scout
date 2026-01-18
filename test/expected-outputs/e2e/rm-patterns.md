# Expected Outputs for Rendered Manifest (RM) Patterns

This document defines expected TUI outputs when running against clusters with RM-managed workloads. These patterns are used in E2E testing.

## Multi-Tool Cluster Setup

After running `./test/e2e/setup-multi-tool-cluster.sh`, the cluster should have:

### Namespace Overview
```
NAMESPACE        STATUS   TOOLS
flux-system      Active   Flux CD controllers
flux-demo        Active   Flux-managed workloads
argocd           Active   Argo CD controllers
argo-demo        Active   Argo CD-managed workloads
helm-demo        Active   Helm-managed workloads
native-demo      Active   Native (kubectl) workloads
confighub-demo   Active   ConfigHub-labeled workloads
```

### Owner Breakdown (`cub-scout map` dashboard)
```
OWNER       COUNT   EXAMPLES
Flux        2+      podinfo (flux-demo)
ArgoCD      1+      guestbook (argo-demo)
Helm        1+      nginx (helm-demo)
Native      1+      mystery-app (native-demo)
ConfigHub   1       payment-api (confighub-demo)
```

## Journey 1: "What's running?"

### Dashboard View (`s` key)
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ cub-scout - Local Cluster                                             │
│ Cluster: kind-tui-e2e                                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│ WORKLOADS: 6+                          PIPELINES: 4+                        │
│ ✓ Healthy: 5+                          ✓ Synced: 4+                         │
│ ⚠ Degraded: 0                          ○ Pending: 0                         │
│                                                                              │
│ OWNERS                                 NAMESPACES                            │
│ Flux     ████████░░ 33%               flux-demo      ██████████ 2           │
│ ArgoCD   ████░░░░░░ 17%               argo-demo      █████░░░░░ 1           │
│ Helm     ████░░░░░░ 17%               helm-demo      █████░░░░░ 1           │
│ Native   ████░░░░░░ 17%               native-demo    █████░░░░░ 1           │
│ ConfigHub████░░░░░░ 16%               confighub-demo █████░░░░░ 1           │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Workloads View (`w` key)
```
NAME          NAMESPACE       KIND         OWNER      STATUS
podinfo       flux-demo       Deployment   Flux       Ready
guestbook-ui  argo-demo       Deployment   ArgoCD     Synced
nginx         helm-demo       Deployment   Helm       Ready
mystery-app   native-demo     Deployment   Native     Ready
payment-api   confighub-demo  Deployment   ConfigHub  Ready
```

## Journey 2: "Find the problem"

### Issues View (`i` key)
When no issues exist:
```
No issues detected

All workloads are healthy and in sync.
```

When issues exist (e.g., after scaling down guestbook):
```
RESOURCE                    ISSUE              OWNER    NAMESPACE
deployment/guestbook-ui     0/1 replicas       ArgoCD   argo-demo
```

### Crashes View (`c` key)
```
No crashing pods detected
```

## Journey 3: "Audit GitOps"

### Orphans View (`o` key)
```
RESOURCE                    NAMESPACE      KIND
mystery-app                 native-demo    Deployment
mystery-app                 native-demo    Service
```

### Pipelines View (`p` key)
```
KIND           NAME          NAMESPACE      STATUS    SOURCE
GitRepository  podinfo       flux-demo      Ready     github.com/stefanprodan/podinfo
Kustomization  podinfo       flux-demo      Ready     ./kustomize
Application    guestbook     argocd         Synced    github.com/argoproj/argocd-example-apps
```

## Journey 4: "Import to ConfigHub"

### Import Wizard (`I` key)
When selecting a Native workload:
```
Import Wizard

Namespace: native-demo

Select workloads to import:
  [x] deployment/mystery-app
  [ ] service/mystery-app

Target Space: (select or create)
```

## Journey 5: "Check drift"

### Drift View (`d` key)
```
RESOURCE           NAMESPACE      OWNER    DRIFT STATUS
podinfo            flux-demo      Flux     In Sync
guestbook-ui       argo-demo      ArgoCD   In Sync
nginx              helm-demo      Helm     In Sync
```

After manual annotation change:
```
RESOURCE           NAMESPACE      OWNER    DRIFT STATUS
podinfo            flux-demo      Flux     Drifted
  - annotation added: manual-change=true
```

## Journey 6: "Hub navigation" (Connected Mode)

### Hub View (`H` to switch)
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ ConfigHub Hierarchy                                                         │
│ Mode: Connected │ Cluster: tui-e2e │ Showing: Current cluster only          │
├─────────────────────────────────────────────────────────────────────────────┤
│ ▼ platform-org                                                              │
│   ▼ payments-space                                                          │
│     ○ payment-api  ← matches current cluster                                │
│     ○ payment-worker                                                        │
│   ▶ orders-space                                                            │
└─────────────────────────────────────────────────────────────────────────────┘
```

After pressing `a` (show all):
```
Mode: Connected │ Cluster: tui-e2e │ Showing: All units
```

## RM-Specific Patterns

### Flux Helm+Kustomize Pattern

After deploying `flux-helm-kustomize` fixtures:
```
PIPELINES (Flux)

KIND             NAME                NAMESPACE        STATUS
GitRepository    flux-rm-source      rm-flux-dev      Ready
Kustomization    core-crds           rm-flux-dev      Ready
Kustomization    core-apps           rm-flux-dev      Ready
Kustomization    security-apps       rm-flux-dev      Ready
Kustomization    observability-apps  rm-flux-dev      Ready
```

### Argo Umbrella Charts Pattern

After deploying `argo-umbrella-charts` fixtures:
```
PIPELINES (ArgoCD)

KIND             NAME           NAMESPACE   STATUS   SOURCE
ApplicationSet   platform-apps  argocd      Synced   helm://charts.bitnami.com
Application      dev-core       argocd      Synced   umbrella-chart
Application      dev-security   argocd      Synced   umbrella-chart
```

## Filter Query Examples

### By Owner
```bash
cub-scout map list -q "owner=Flux"
# Returns: podinfo, flux-demo workloads

cub-scout map list -q "owner!=Native"
# Returns: All GitOps-managed workloads
```

### By Namespace
```bash
cub-scout map list -q "namespace=flux-*"
# Returns: flux-system, flux-demo workloads
```

### Combined
```bash
cub-scout map list -q "owner=Flux AND status=Ready"
# Returns: Healthy Flux workloads only
```

## Test Assertions

These outputs should be validated by journey tests:

1. **Owner Detection**: Each tool's workloads correctly attributed
2. **Status Accuracy**: Sync status matches actual cluster state
3. **Navigation**: All views reachable via documented keys
4. **Filter Accuracy**: Queries return expected subsets
5. **No Null Values**: No "null" or "unknown" in output (Issue #1)
