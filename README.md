# cub-scout 
# explores, maps and communicates facts about GitOps clusters

**Demystify GitOps. See what's really happening in your cluster.**

GitOps is powerful but can be a opaque at times. Where did this Deployment come from? Why isn't my change applying? Is this managed by Git or was it kubectl'd? cub-scout makes the invisible visible.

```bash
brew install confighub/tap/cub-scout
cub-scout map
```

Press `w` for workloads. Press `T` to trace. Press `4` for deep-dive.

> **🧪 Vibe Coded:** This whole project has been vibe coded. One motive: it is an experiment to learn how AI and ConfigHub interact with GitOps clusters. We want you to try this too, and tell us what you learn.

---

## The Problem

GitOps tools are powerful but can hide complexity behind layers of abstraction.

**What's obscure:**
- A Deployment exists, but where did it come from? (Kustomization? HelmRelease? kubectl?)
- A change isn't applying, but why? (Source not ready? Reconciliation stuck? Wrong path?)
- Resources exist with no owner — who created them and when?
- Dependencies between apps are invisible until something breaks

**What you end up doing:**
- `kubectl get kustomization -A` + `kubectl get helmrelease -A` + `kubectl get application -A`
- Manually checking labels to figure out ownership
- Tribal knowledge: "Oh, that's managed by the platform team's Flux setup"

cub-scout shows you the whole picture in seconds.

---

## The Solution

cub-scout shows you the whole picture in one view.

### Status Dashboard

```bash
cub-scout map status
```

```
  ✓ ALL HEALTHY   prod-east

  Deployers  5/5
  Workloads  47/47

  OWNERSHIP
  ────────────────────────────────────────────────
  Flux(28) ArgoCD(12) Helm(5) Native(2)
  ██████████████░░░░░░
```

When things go wrong:

```
  🔥 3 FAILURE(S)   prod-east

  Deployers  3/5
  Workloads  44/47

  PROBLEMS
  ────────────────────────────────────────────────
  ✗ HelmRelease/redis-cache      SourceNotReady
  ✗ Application/payment-api      OutOfSync
  ⏸ Kustomization/monitoring     suspended
```

---

### Trace Any Resource

```bash
cub-scout trace deploy/payment-api -n prod
```

See the full chain: Git repo → Kustomization → Deployment → Pod

```
┌─────────────────────────────────────────────────────────────────────┐
│  TRACE: Deployment/payment-api                                      │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  🟢 ✓ GitRepository/platform-config                                 │
│      │ URL: git@github.com:acme/platform-config.git                 │
│      │ Revision: main@sha1:abc123f                                  │
│      │ Status: Artifact is up to date                               │
│      │                                                              │
│      └─▶ 🟢 ✓ Kustomization/apps-payment                            │
│              │ Path: ./clusters/prod/apps/payment                   │
│              │ Status: Applied revision main@sha1:abc123f           │
│              │                                                      │
│              └─▶ 🟢 ✓ Deployment/payment-api                        │
│                      │ Namespace: prod                              │
│                      │ Status: 3/3 ready                            │
│                      │                                              │
│                      └─▶ ReplicaSet/payment-api-7d4b8c              │
│                          ├── Pod/payment-api-7d4b8c-abc12 ✓ Running │
│                          ├── Pod/payment-api-7d4b8c-def34 ✓ Running │
│                          └── Pod/payment-api-7d4b8c-xyz99 ✓ Running │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│ 🟢 ✓ All levels in sync. Managed by Flux.                           │
└─────────────────────────────────────────────────────────────────────┘
```

---

### Tree Command — Multiple Hierarchy Views

```bash
cub-scout tree
```

**Runtime Hierarchy** — Deployment → ReplicaSet → Pod:

```
RUNTIME HIERARCHY (47 Deployments)
════════════════════════════════════════════════════════════════════
├── boutique/cart [Flux] 2/2 ready
│   └── ReplicaSet cart-86f68db776 [2/2]
│       ├── Pod cart-86f68db776-hzqgf  ✓ Running  10.244.0.15
│       └── Pod cart-86f68db776-mp8kz  ✓ Running  10.244.0.16
├── boutique/checkout [Flux] 1/1 ready
│   └── ReplicaSet checkout-5d8f9c7b4 [1/1]
│       └── Pod checkout-5d8f9c7b4-abc12  ✓ Running  10.244.0.17
├── monitoring/prometheus [Helm] 1/1 ready
│   └── ReplicaSet prometheus-7d4b8c [1/1]
│       └── Pod prometheus-7d4b8c-xyz99  ✓ Running  10.244.0.18
└── temp-test/debug-nginx [Native] 1/1 ready
    └── ReplicaSet debug-nginx-6c5d7b [1/1]
        └── Pod debug-nginx-6c5d7b-def34  ⚠ Pending  (no node)

────────────────────────────────────────────────────────────────────
Summary: 47 Deployments │ 189 Pods │ 186 Running │ 3 Pending
```

```bash
cub-scout tree ownership
```

**Ownership Hierarchy** — Resources grouped by owner:

```
OWNERSHIP HIERARCHY
════════════════════════════════════════════════════════════════════
Flux (28 resources)
├── boutique/cart             Deployment  ✓ 2/2 ready
├── boutique/checkout         Deployment  ✓ 1/1 ready
├── boutique/frontend         Deployment  ✓ 3/3 ready
├── ingress/nginx-ingress     Deployment  ✓ 2/2 ready
└── ... (24 more)

ArgoCD (12 resources)
├── cert-manager/cert-manager   Deployment  ✓ 1/1 ready
├── argocd/argocd-server        Deployment  ✓ 1/1 ready
└── ... (10 more)

Helm (5 resources)
├── monitoring/prometheus       StatefulSet ✓ 1/1 ready
├── monitoring/grafana          Deployment  ✓ 1/1 ready
└── ... (3 more)

Native (2 resources)  ⚠ ORPHANS
├── temp-test/debug-nginx       Deployment  ✓ 1/1 ready
└── kube-system/coredns         Deployment  ✓ 2/2 ready

────────────────────────────────────────────────────────────────────
Ownership: Flux 60% │ ArgoCD 26% │ Helm 10% │ Native 4%
```

```bash
cub-scout tree suggest
```

**Suggested Organization** — Hub/AppSpace recommendation:

```
HUB/APPSPACE SUGGESTION
════════════════════════════════════════════════════════════════════

Detected pattern: D2 (Control Plane style)
  └── clusters/prod, clusters/staging structure

Suggested Hub/AppSpace organization:

  Hub: acme-platform
  ├── Space: boutique-prod
  │   ├── Unit: cart          (Deployment boutique/cart)
  │   ├── Unit: checkout      (Deployment boutique/checkout)
  │   ├── Unit: frontend      (Deployment boutique/frontend)
  │   └── Unit: payment-api   (Deployment boutique/payment-api)
  │
  ├── Space: boutique-staging
  │   └── (clone from boutique-prod with staging values)
  │
  └── Space: platform
      ├── Unit: nginx-ingress   (Deployment ingress/nginx)
      ├── Unit: cert-manager    (Deployment cert-manager/cert-manager)
      └── Unit: monitoring      (StatefulSet monitoring/prometheus)

────────────────────────────────────────────────────────────────────
Next steps:
  1. Review the suggested structure above
  2. Import workloads: cub-scout import -n boutique
  3. View in ConfigHub: cub unit tree --space boutique-prod
```

---

### Discover and Health (Scout-Style Commands)

```bash
cub-scout discover
```

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
⚠       temp-test       debug-nginx       Native     — (orphan)

────────────────────────────────────────────────────────────────────
Found: 47 workloads │ Flux(28) ArgoCD(12) Helm(5) Native(2)
```

```bash
cub-scout health
```

```
CLUSTER HEALTH CHECK
════════════════════════════════════════════════════════════════════

DEPLOYER ISSUES
────────────────────────────────────────────────────────────────────
  ✗ HelmRelease/redis-cache      SourceNotReady
    Message: failed to fetch Helm chart: connection refused
    Last attempt: 5 minutes ago

  ⏸ Kustomization/monitoring     suspended
    Suspended since: 2026-01-20T10:30:00Z
    Reason: Manual pause for maintenance

WORKLOAD ISSUES
────────────────────────────────────────────────────────────────────
  ✗ temp-test/debug-nginx        0/1 pods ready
    Reason: ImagePullBackOff
    Image: nginx:nonexistent

────────────────────────────────────────────────────────────────────
Summary: 2 deployer issues │ 1 workload issue │ 1 suspended
```

---

### Scan for Configuration Issues

```bash
cub-scout scan
```

```
CONFIG RISK SCAN: prod-east
════════════════════════════════════════════════════════════════════

CRITICAL (1)
────────────────────────────────────────────────────────────────────
  [CCVE-2025-0027] Grafana sidecar namespace whitespace error
    Resource: monitoring/ConfigMap/grafana-sidecar
    Impact:   Dashboard injection fails silently
    Fix:      Remove spaces: NAMESPACE="monitoring,grafana"
    Ref:      FluxCon 2025 — BIGBANK 3-day outage

WARNING (2)
────────────────────────────────────────────────────────────────────
  [CCVE-2025-0043] Thanos sidecar not uploading to object storage
    Resource: monitoring/StatefulSet/prometheus
    Fix:      Check objstore.yml bucket configuration

  [CCVE-2025-0066] SSL redirect blocking ACME HTTP-01 challenge
    Resource: ingress/Ingress/api-gateway
    Fix:      Add: kubernetes.io/ingress.allow-http: "true"

INFO (1)
────────────────────────────────────────────────────────────────────
  [CCVE-2025-0084] PodDisruptionBudget allows zero available
    Resource: cache/PodDisruptionBudget/redis-pdb
    Fix:      Set minAvailable to at least 1

════════════════════════════════════════════════════════════════════
Summary: 1 CRITICAL │ 2 WARNING │ 1 INFO
Scanned: 47 resources │ Patterns: 46 active (4,500+ reference)
```

---

## Quick Commands

| Command | What You Get |
|---------|--------------|
| `cub-scout map` | Interactive TUI - press `?` for help |
| `cub-scout discover` | Find workloads by owner (scout-style alias) |
| `cub-scout tree` | Hierarchical views (runtime, git, config) |
| `cub-scout tree suggest` | Suggested Hub/AppSpace organization |
| `cub-scout trace deploy/x -n y` | Full ownership chain to Git source |
| `cub-scout health` | Check for issues (scout-style alias) |
| `cub-scout scan` | Configuration risk patterns (46 patterns) |

### Tree Views

| View | Shows |
|------|-------|
| `cub-scout tree runtime` | Deployment → ReplicaSet → Pod hierarchies |
| `cub-scout tree ownership` | Resources grouped by GitOps owner |
| `cub-scout tree git` | Git source structure (repos, paths) |
| `cub-scout tree patterns` | Detected GitOps patterns (D2, Arnie, etc.) |
| `cub-scout tree config --space X` | ConfigHub Unit relationships (wraps `cub unit tree`) |
| `cub-scout tree suggest` | Recommended Hub/AppSpace structure |

---

## Keyboard Shortcuts

| Key | View |
|-----|------|
| `s` | Status dashboard |
| `w` | Workloads by owner |
| `o` | Orphans (unmanaged resources) |
| `4` | Deep-dive (resource trees) |
| `5` | App hierarchy (inferred Units) |
| `T` | Trace selected resource |
| `/` | Search |
| `?` | Help |
| `q` | Quit |

---

## Ownership Detection

| Owner | How Detected |
|-------|--------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` labels |
| **ArgoCD** | `argocd.argoproj.io/instance` label |
| **Helm** | `app.kubernetes.io/managed-by: Helm` |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | None of the above (kubectl-applied) |

---

## See It at Scale

For a realistic demo with 50+ resources, see [docs/SCALE-DEMO.md](docs/SCALE-DEMO.md).

```bash
# Deploy the official Flux reference architecture
flux bootstrap github --owner=you --repository=fleet-infra --path=clusters/staging

# Explore with cub-scout
cub-scout map
```

---

## Install

### Homebrew (macOS/Linux)

```bash
brew install confighub/tap/cub-scout
```

### From Source

```bash
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
go build ./cmd/cub-scout
./cub-scout version
```

### Docker

```bash
docker run --rm --network=host \
  -v ~/.kube:/home/nonroot/.kube \
  ghcr.io/confighub/cub-scout map list
```

---

## How It Works

cub-scout uses **deterministic label detection** — no AI, no magic:

1. Connect to your cluster via kubectl context
2. List resources across all namespaces
3. Examine labels and annotations on each resource
4. Match against known ownership patterns (Flux, Argo, Helm, etc.)
5. Display results

**Read-only by default.** Never modifies your cluster unless you explicitly use import commands.

---

## Design Principles

**Wrap, don't reinvent.** cub-scout builds on existing tools rather than replacing them:

| Principle | What It Means |
|-----------|---------------|
| **Use kubectl** | All cluster access goes through your existing kubeconfig |
| **Use cub CLI** | Fleet queries use ConfigHub's `cub` CLI, not a parallel API |
| **Parse, don't guess** | Ownership comes from actual labels, not heuristics |
| **Complement GitOps** | Works alongside Flux, Argo, Helm — doesn't compete |

**Why this matters:** Your existing tools, RBAC, and audit trails all still work. cub-scout is a lens, not a replacement.

---

## Part of ConfigHub

cub-scout is the open-source cluster explorer from [ConfigHub](https://confighub.com).

**Standalone mode:** Works forever, no signup required. See your cluster, trace ownership, scan for issues.

**Connected mode:** Link to ConfigHub for:
- Multi-cluster fleet visibility
- One-click import of discovered workloads
- Revision history and compare DRY↔WET↔LIVE
- Team collaboration and change tracking
- Git and other sources

---

## Documentation

| Doc | Content |
|-----|---------|
| [CLI-GUIDE.md](CLI-GUIDE.md) | Complete command reference |
| [docs/SCALE-DEMO.md](docs/SCALE-DEMO.md) | See cub-scout at scale |
| [docs/SCAN-GUIDE.md](docs/SCAN-GUIDE.md) | Risk scanning (46 patterns) |
| [examples/](examples/) | Demo scenarios |

---

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

- **Found a bug?** [Open an issue](https://github.com/confighub/cub-scout/issues)
- **Have an idea?** Start a discussion
- **Want to contribute?** PRs welcome

---

## Community

- **Discord:** [discord.gg/confighub](https://discord.gg/confighub)
- **Issues:** [GitHub Issues](https://github.com/confighub/cub-scout/issues)
- **Website:** [confighub.com](https://confighub.com)

---

## License

MIT License — see [LICENSE](LICENSE)
