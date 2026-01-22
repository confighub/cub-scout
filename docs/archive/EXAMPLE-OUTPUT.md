Example of logic used in read from live.

100% deterministic (no AI)
Works in CLI
Works in larger packages


Ownership Detection (detectOwnership)
  Simple label/annotation lookups in priority order:
  // 1. ConfigHub label
  if labels["confighub.com/UnitSlug"] exists → "ConfigHub"
  // 2. Flux Kustomize label  
  if labels["kustomize.toolkit.fluxcd.io/name"] exists → "Flux"
  // 3. Flux Helm label
  if labels["helm.toolkit.fluxcd.io/name"] exists → "Flux"
  // 4. ArgoCD label
  if labels["argocd.argoproj.io/instance"] exists → "ArgoCD"
  // 5. ArgoCD tracking annotation
  if annotations["argocd.argoproj.io/tracking-id"] exists → "ArgoCD"
  // 6. Helm managed-by label
  if labels["app.kubernetes.io/managed-by"] == "Helm" → "Helm"
  // 7. Otherwise
  return "Native"
  Environment Inference (inferEnvironment)
  Simple string matching:
  combined := lowercase(namespace + path)
  if contains("prod")    → "production"
  if contains("staging") → "staging"
  if contains("dev")     → "development"
  if contains("test")    → "testing"
  else                   → "unknown"
  Unit/Workload Linking
  - Reads Kubernetes CRDs (Kustomizations, Applications, HelmReleases)
  - Follows ownerReferences to build Deployment → ReplicaSet → Pod trees
  - Matches workloads to deployers via labels


═══════════════════════════════════════════════════════════════════════════════            
                         RICH APPLICATION HIERARCHY (STANDALONE)
     ═══════════════════════════════════════════════════════════════════════════════
     Full tree view of cluster resources mapped to ConfigHub model.
     Legend: ✓ Ready  ✗ Not Ready  ⚡ Flux  🅰 Argo  ⎈ Helm  📦 ConfigHub  ☸ Native
     ───────────────────────────────────────────────────────────────────────────────
     UNITS TREE (GitOps deployers + workloads + inferred dependencies)
     ───────────────────────────────────────────────────────────────────────────────
     🅰 ✗ ArgoCD/guestbook
     │
     ├─ Source: https://github.com/argoproj/argocd-example-apps.git
     │          path: guestbook
     ├─ Status: Unknown/Healthy
     ├─ Target: argo-demo
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/guestbook-ui (1/1)
     │     ├─ image: gcr.io/google-samples/gb-frontend:v5
     │     └─ ReplicaSet/guestbook-ui-84774bdc6f (1/1)
     │        └─ ✓ Pod/guestbook-ui-84774bdc6f-8wbqc (Running, 10.244.0.24)
     │
     └─ (no dependencies detected)
     📦 ✓ ConfigHub/payment-api
     │
     ├─ Status: imported
     ├─ Target: confighub-demo
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/payment-api (2/2)
     │     ├─ image: nginx:alpine
     │     └─ ReplicaSet/payment-api-794b8d7c46 (2/2)
     │        ├─ ✓ Pod/payment-api-794b8d7c46-2lgmr (Running, 10.244.0.23)
     │        └─ ✓ Pod/payment-api-794b8d7c46-h9mbf (Running, 10.244.0.22)
     │
     └─ (no dependencies detected)
     ⚡ ✗ Flux/cart
     │
     ├─ Source: https://github.com/stefanprodan/podinfo
     │          path: ./kustomize
     ├─ Status: Reconciliation in progress
     ├─ Target: boutique
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/cart (2/2)
     │     ├─ image: ghcr.io/stefanprodan/podinfo:6.9.4
     │     └─ ReplicaSet/cart-86f68db776 (2/2)
     │        ├─ ✓ Pod/cart-86f68db776-94mkb (Running, 10.244.0.32)
     │        └─ ✓ Pod/cart-86f68db776-zrbcp (Running, 10.244.0.26)
     │
     └─ (no dependencies detected)
     ⚡ ✗ Flux/checkout
     │
     ├─ Source: https://github.com/stefanprodan/podinfo
     │          path: ./kustomize
     ├─ Status: Reconciliation in progress
     ├─ Target: boutique
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/checkout (2/2)
     │     ├─ image: ghcr.io/stefanprodan/podinfo:6.9.4
     │     └─ ReplicaSet/checkout-86f68db776 (2/2)
     │        ├─ ✓ Pod/checkout-86f68db776-mvjzt (Running, 10.244.0.31)
     │        └─ ✓ Pod/checkout-86f68db776-tkfcc (Running, 10.244.0.29)
     │
     └─ (no dependencies detected)
     ⚡ ✗ Flux/frontend
     │
     ├─ Source: https://github.com/stefanprodan/podinfo
     │          path: ./kustomize
     ├─ Status: failed to download archive: GET http://source-controller.flux-system.svc.clus...
     ├─ Target: boutique
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/frontend (2/2)
     │     ├─ image: ghcr.io/stefanprodan/podinfo:6.9.4
     │     └─ ReplicaSet/frontend-86f68db776 (2/2)
     │        ├─ ✓ Pod/frontend-86f68db776-grf5r (Running, 10.244.0.34)
     │        └─ ✓ Pod/frontend-86f68db776-j4bk8 (Running, 10.244.0.28)
     │
     └─ (no dependencies detected)
     ⚡ ✗ Flux/payment
     │
     ├─ Source: https://github.com/stefanprodan/podinfo
     │          path: ./kustomize
     ├─ Status: failed to download archive: GET http://source-controller.flux-system.svc.clus...
     ├─ Target: boutique
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/payment (2/2)
     │     ├─ image: ghcr.io/stefanprodan/podinfo:6.9.4
     │     └─ ReplicaSet/payment-86f68db776 (2/2)
     │        ├─ ✓ Pod/payment-86f68db776-4wnk2 (Running, 10.244.0.33)
     │        └─ ✓ Pod/payment-86f68db776-54jnv (Running, 10.244.0.30)
     │
     └─ (no dependencies detected)
     ⚡ ✗ Flux/podinfo
     │
     ├─ Source: https://github.com/stefanprodan/podinfo
     │          path: ./kustomize
     ├─ Status: failed to download archive: GET http://source-controller.flux-system.svc.clus...
     ├─ Target: flux-demo
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/podinfo (2/2)
     │     ├─ image: ghcr.io/stefanprodan/podinfo:6.5.0
     │     ├─ ReplicaSet/podinfo-69c97645d7 (2/2)
     │     │  ├─ ✓ Pod/podinfo-69c97645d7-qph5t (Running, 10.244.0.18)
     │     │  └─ ✓ Pod/podinfo-69c97645d7-sjb2l (Running, 10.244.0.19)
     │     └─ calls: podinfo
     │
     └─ (no dependencies detected)
     ⚡ ✗ Flux/shipping
     │
     ├─ Source: https://github.com/stefanprodan/podinfo
     │          path: ./kustomize
     ├─ Status: failed to download archive: GET http://source-controller.flux-system.svc.clus...
     ├─ Target: boutique
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/shipping (2/2)
     │     ├─ image: ghcr.io/stefanprodan/podinfo:6.9.4
     │     └─ ReplicaSet/shipping-86f68db776 (2/2)
     │        ├─ ✓ Pod/shipping-86f68db776-g7lkw (Running, 10.244.0.35)
     │        └─ ✓ Pod/shipping-86f68db776-xllh6 (Running, 10.244.0.27)
     │
     └─ (no dependencies detected)
     ⎈ ✗ Helm/nginx
     │
     ├─ Status: failed
     ├─ Target: helm-demo
     │
     ├─ Workloads (1):
     │  └─ ✓ Deployment/nginx-helm (1/1)
     │     ├─ image: nginx:alpine
     │     └─ ReplicaSet/nginx-helm-69965d6dc8 (1/1)
     │        └─ ✓ Pod/nginx-helm-69965d6dc8-nkdls (Running, 10.244.0.25)
     │
     └─ (no dependencies detected)
     ☸ Native/Unmanaged Workloads (1) - not tracked by GitOps
     │
     └─ ✓ native-demo/mystery-app (1/1)
        ├─ image: nginx:alpine
        └─ ReplicaSet/mystery-app-6957c4c947 (1/1)
           └─ ✓ Pod/mystery-app-6957c4c947-j67cn (Running, 10.244.0.21)
     ───────────────────────────────────────────────────────────────────────────────
     NAMESPACE ANALYSIS → INFERRED APPSPACES
     ───────────────────────────────────────────────────────────────────────────────
     Namespaces map to ConfigHub Spaces (environments/teams).
     [UNKNOWN] 6 namespace(s)
       helm-demo
         Workloads: 1 total
           - Helm: 1
       native-demo
         Workloads: 1 total
           - Native: 1
       argo-demo
         Workloads: 1 total
           - ArgoCD: 1
       boutique
         Workloads: 5 total
           - Flux: 5
       confighub-demo
         Workloads: 1 total
           - ConfigHub: 1
       flux-demo
         Workloads: 1 total
           - Flux: 1
     ───────────────────────────────────────────────────────────────────────────────
     OWNERSHIP GRAPH → WHO MANAGES WHAT
     ───────────────────────────────────────────────────────────────────────────────
     Kustomization/payment
       └── boutique/payment
     Kustomization/shipping
       └── boutique/shipping
     Kustomization/podinfo
⏺ Bash(./cub-agent map app-hierarchy 2>&1 | tail -80) timeout: 1m 0s
  ⎿    argo-demo                                                                            
         Workloads: 1 total
           - ArgoCD: 1
       boutique
         Workloads: 5 total
           - Flux: 5
       confighub-demo
         Workloads: 1 total
           - ConfigHub: 1
       flux-demo
         Workloads: 1 total
           - Flux: 1
       helm-demo
         Workloads: 1 total
           - Helm: 1
     ───────────────────────────────────────────────────────────────────────────────
     OWNERSHIP GRAPH → WHO MANAGES WHAT
     ───────────────────────────────────────────────────────────────────────────────
     Kustomization/checkout
       └── boutique/checkout
     Kustomization/frontend
       └── boutique/frontend
     Kustomization/payment
       └── boutique/payment
     Kustomization/shipping
       └── boutique/shipping
     Kustomization/podinfo
       └── flux-demo/podinfo
     Kustomization/cart
       └── boutique/cart
     ───────────────────────────────────────────────────────────────────────────────
     LABEL ANALYSIS → POTENTIAL CONFIGHUB LABELS
     ───────────────────────────────────────────────────────────────────────────────
     Standard Kubernetes labels found:
       app.kubernetes.io/name:
         - nginx(1)
       app.kubernetes.io/managed-by:
         - Helm(1)
     ═══════════════════════════════════════════════════════════════════════════════
     SUGGESTED CONFIGHUB MAPPING
     ═══════════════════════════════════════════════════════════════════════════════
     Based on cluster analysis, here's how to map to ConfigHub model:
     RECOMMENDED IMPORT STRATEGY:
       Pure ArgoCD - import each Application as a Unit
     POTENTIAL SPACES:
       unknown-space: native-demo, argo-demo, boutique, confighub-demo, flux-demo, helm-demo
     COMMANDS TO IMPORT:
       cub-agent map              # Launch TUI, press 'i' for import wizard
       cub-agent import --help    # See import options
     ═══════════════════════════════════════════════════════════════════════════════
     WHAT CONFIGHUB PROVIDES (beyond inference)
     ═══════════════════════════════════════════════════════════════════════════════
       • Explicit Unit definitions with metadata
       • Space hierarchy (Organization → Space → Unit)
       • Cross-cluster Unit linking and dependencies
       • Revision history for every change
       • Diff between revisions
       • Approval workflows
       • Team-based access control
       • Audit log of all operations




Here's what you get from the enhanced CLI commands:
  cub-agent map deep-dive (Cluster Data / "What We Know")
  Shows everything readable from the cluster:
  FLUX GITREPOSITORIES (Source of Truth)
    ✓ boutique/boutique
      URL: https://github.com/stefanprodan/podinfo
      Branch: master, Revision: sha1:b6b680fe...
      Size: 301806 bytes, Conditions: Ready=True
  FLUX KUSTOMIZATIONS (Deployers)
    ✗ boutique/cart
      Source: boutique/GitRepository/boutique
      Path: ./kustomize, TargetNS: boutique
      Inventory: 3 resources
      LiveTree: cart-86f68db776 (2/2 ready)
                └─ Pod/cart-86f68db776-94mkb (Running, 10.244.0.32)
                └─ Pod/cart-86f68db776-zrbcp (Running, 10.244.0.26)
  HELM RELEASES
    nginx-22.4.3 with chart details, dependencies, NOTES.txt, custom values
  WORKLOADS by owner (Flux: 6, ArgoCD: 1, Helm: 1, ConfigHub: 1, Native: 1)
  cub-agent map app-hierarchy (Rich Hierarchy)
  Shows the inferred ConfigHub model:
  UNITS TREE (GitOps deployers + workloads)
  ───────────────────────────────────────
  🅰 ✗ ArgoCD/guestbook
  │  Source: https://github.com/argoproj/argocd-example-apps.git
  │  └─ Workloads (1):
  │     └─ ✓ Deployment/guestbook-ui (1/1)
  │        └─ ReplicaSet → Pod (Running, 10.244.0.24)
  ⚡ ✗ Flux/cart
  │  Source: https://github.com/stefanprodan/podinfo
  │  └─ Workloads (1):
  │     └─ ✓ Deployment/cart (2/2)
  │        └─ ReplicaSet → 2 Pods (Running)
  ☸ Native/Unmanaged Workloads (1) - not tracked by GitOps
     └─ mystery-app (1/1)
  NAMESPACE ANALYSIS → Inferred AppSpaces
  OWNERSHIP GRAPH → Who Manages What
  LABEL ANALYSIS → Potential ConfigHub Labels
  SUGGESTED CONFIGHUB MAPPING
  Both commands end with a "What ConfigHub adds/provides" box showing the value of connecting.

