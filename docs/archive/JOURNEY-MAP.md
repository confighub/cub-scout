# Journey: Using the Map TUI

**Time:** 10 minutes
**Goal:** Navigate your cluster, understand ownership, trace resource chains

**Prerequisites:** Have a Kubernetes cluster with some workloads running.

---

## What is the Map?

The Map shows you:
- **What's running** — All workloads in your cluster
- **Who owns it** — Flux, Argo CD, Helm, or Native (kubectl)
- **Is it healthy** — Pod status, sync state
- **Where it came from** — Git source, pipeline, ownership chain

---

## Step 1: Launch the Map

```bash
./test/atk/map
```

**Expected output:**

```
┌─ ⚡ MAP ─────────────────────────────────────────────────────────────────────┐
│                                                                              │
│  Context: kind-atk                                                           │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

┌─ CLUSTER HEALTH ─────────────────────────────────────────────────────────────┐
│                                                                              │
│  ████████████████████░░░░  85%  (17/20 ready)                               │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

┌─ RESOURCES ────────────────┬─ PIPELINES ─────────────────────────────────────┐
│                            │                                                 │
│  Flux        8  ████████   │  ✓ GitRepo → Kustomization → Deployment        │
│  ArgoCD      5  █████      │  ✓ GitRepo → Application → Deployment          │
│  Helm        4  ████       │  ⚠ HelmRelease pending                         │
│  Native      3  ███        │                                                 │
│                            │                                                 │
└────────────────────────────┴─────────────────────────────────────────────────┘

Keys: [↑↓] navigate  [Enter] details  [t] trace  [f] filter  [q] quit
```

---

## Step 2: Navigate Resources

Use arrow keys to move through resources:

```
┌─ RESOURCES ──────────────────────────────────────────────────────────────────┐
│                                                                              │
│  NAMESPACE        NAME                    KIND           OWNER    STATUS     │
│  ─────────────────────────────────────────────────────────────────────────   │
│  flux-system      source-controller       Deployment     Flux     ✓ Ready   │
│  flux-system      kustomize-controller    Deployment     Flux     ✓ Ready   │
│> argocd           argocd-server           Deployment     ArgoCD   ✓ Ready   │
│  argocd           argocd-repo-server      Deployment     ArgoCD   ✓ Ready   │
│  payments-prod    payment-api             Deployment     Flux     ✓ Ready   │
│  payments-prod    payment-worker          Deployment     Flux     ⚠ 1/2     │
│  default          mystery-app             Deployment     Native   ✓ Ready   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

- **Green ✓** — Healthy, all pods ready
- **Amber ⚠** — Partial (some pods not ready)
- **Red ✗** — Failed or missing

---

## Step 3: View Resource Details

Press **Enter** on a resource to see details:

```
┌─ DETAILS: payment-api ───────────────────────────────────────────────────────┐
│                                                                              │
│  Kind:        Deployment                                                     │
│  Namespace:   payments-prod                                                  │
│  Owner:       Flux                                                           │
│  Status:      Ready (3/3 replicas)                                           │
│                                                                              │
│  Labels:                                                                     │
│    app.kubernetes.io/name: payment-api                                       │
│    app.kubernetes.io/part-of: payments                                       │
│    kustomize.toolkit.fluxcd.io/name: payments-apps                           │
│                                                                              │
│  Ownership Chain:                                                            │
│    GitRepository/flux-system → Kustomization/payments-apps → Deployment      │
│                                                                              │
│  Source:                                                                     │
│    Path: ./apps/payments/overlays/prod                                       │
│    Revision: main@sha1:abc123                                                │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘

Press [Esc] to go back, [t] to trace
```

---

## Step 4: Trace Ownership

Press **t** to trace the full ownership chain:

```
┌─ TRACE: payment-api ─────────────────────────────────────────────────────────┐
│                                                                              │
│  ┌─────────────────────────┐                                                │
│  │ GitRepository           │                                                │
│  │ flux-system/platform    │                                                │
│  │ https://github.com/...  │                                                │
│  │ Revision: main@abc123   │                                                │
│  └───────────┬─────────────┘                                                │
│              │                                                               │
│              ▼                                                               │
│  ┌─────────────────────────┐                                                │
│  │ Kustomization           │                                                │
│  │ flux-system/payments    │                                                │
│  │ Path: ./apps/payments   │                                                │
│  │ Status: Applied         │                                                │
│  └───────────┬─────────────┘                                                │
│              │                                                               │
│              ▼                                                               │
│  ┌─────────────────────────┐                                                │
│  │ Deployment              │                                                │
│  │ payments-prod/          │                                                │
│  │   payment-api           │                                                │
│  │ Status: 3/3 Ready       │                                                │
│  └─────────────────────────┘                                                │
│                                                                              │
│  ✓ Full chain traced: Git → Flux → Kubernetes                               │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

**Trace shows:**
- Where the config came from (Git)
- How it was deployed (Flux/Argo)
- Current state in cluster

---

## Step 5: Filter Resources

Press **f** to filter:

```
┌─ FILTER ─────────────────────────────────────────────────────────────────────┐
│                                                                              │
│  Filter by:                                                                  │
│                                                                              │
│  [1] Owner:     Flux | ArgoCD | Helm | Native                               │
│  [2] Namespace: ____________________                                         │
│  [3] Status:    Ready | Pending | Failed                                    │
│  [4] Query:     ____________________                                         │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

Or use command-line queries:

```bash
# Only Flux-managed resources
./test/atk/map -q "owner=Flux"

# Only unhealthy resources
./test/atk/map -q "status!=Ready"

# Specific namespace
./test/atk/map -q "namespace=payments-prod"

# Resources with specific label
./test/atk/map -q "labels[app]=payment-api"
```

---

## Step 6: View Pipelines

The right panel shows GitOps pipelines:

```
┌─ PIPELINES ──────────────────────────────────────────────────────────────────┐
│                                                                              │
│  FLUX                                                                        │
│  ✓ flux-system/source-controller                                             │
│    GitRepository → Kustomization → Resources                                 │
│                                                                              │
│  ⚠ payments/helm-release                                                     │
│    GitRepository → HelmRepository → HelmRelease (pending)                    │
│                                                                              │
│  ARGOCD                                                                      │
│  ✓ argocd/guestbook                                                          │
│    GitRepository → Application → Resources                                   │
│                                                                              │
│  ✗ argocd/broken-app                                                         │
│    GitRepository → Application (OutOfSync)                                   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Step 7: Connected Mode

When connected to ConfigHub, the map shows additional context:

```bash
./test/atk/map confighub
```

```
┌─ CONFIGHUB ──────────────────────────────────────────────────────────────────┐
│                                                                              │
│  Org: your-org                                                               │
│  └─ Hub: platform-team                                                       │
│     └─ AppSpace: payments-team                                               │
│        ├─ payment-api [variant=prod]     ✓ Synced                           │
│        ├─ payment-api [variant=staging]  ✓ Synced                           │
│        └─ payment-worker [variant=prod]  ⚠ Drifted                          │
│                                                                              │
│  Worker: dev ──▶ kind-atk                                                   │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑`/`↓` | Navigate resources |
| `Enter` | View details |
| `t` | Trace ownership chain |
| `f` | Filter resources |
| `r` | Refresh |
| `n` | Next namespace |
| `p` | Previous namespace |
| `o` | Toggle old/new model (ConfigHub) |
| `q` | Quit |
| `?` | Help |

---

## Command-Line Options

```bash
./test/atk/map                          # Default view
./test/atk/map -q "owner=Flux"          # Filter by owner
./test/atk/map confighub                # ConfigHub connected view
./test/atk/map --mode=admin             # Admin mode (all namespaces)
./test/atk/map --mode=fleet             # Fleet mode (cross-cluster)
```

---

## What Map Tells You

| Question | How Map Answers |
|----------|-----------------|
| What's running? | Resource list with health status |
| Who manages it? | Owner column (Flux/ArgoCD/Helm/Native) |
| Where's it from? | Trace shows Git source |
| Is it in sync? | Pipeline status + drift detection |
| What's broken? | Red status, failing pipelines |

---

## Next Steps

| Journey | What You'll Learn |
|---------|-------------------|
| [**JOURNEY-SCAN.md**](JOURNEY-SCAN.md) | Find configuration issues |
| [**JOURNEY-QUERY.md**](JOURNEY-QUERY.md) | Query across fleet |
| [**TUI-TRACE.md**](TUI-TRACE.md) | Deep dive on tracing |

---

**Previous:** [JOURNEY-IMPORT.md](JOURNEY-IMPORT.md) — Import workloads | **Next:** [JOURNEY-SCAN.md](JOURNEY-SCAN.md) — Find configuration issues

---

## See Also

- [TUI-SAVED-QUERIES.md](TUI-SAVED-QUERIES.md) — Save and reuse queries
- [GLOSSARY-OF-CONCEPTS.md](GLOSSARY-OF-CONCEPTS.md) — Glossary of terms
- [CLI-EXPECTED-OUTPUT.md](CLI-EXPECTED-OUTPUT.md) — What healthy vs unhealthy looks like
