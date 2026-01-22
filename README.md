# cub-scout

**Demystify GitOps. See what's really happening in your cluster.**

GitOps is powerful but opaque. Where did this Deployment come from? Why isn't my change applying? Is this managed by Git or was it kubectl'd? cub-scout makes the invisible visible.

```bash
brew install confighub/tap/cub-scout
cub-scout map
```

Press `w` for workloads. Press `T` to trace. Press `4` for deep-dive.

> **🧪 Vibe Coded:** This whole project has been vibe coded. One motive: it is an experiment to learn how AI and ConfigHub interact with GitOps clusters. We want you to try this too, and tell us what you learn.

---

## The Problem

GitOps tools are powerful but hide complexity behind layers of abstraction.

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

### Trace Any Resource

```bash
cub-scout trace deploy/payment-api -n prod
```

See the full chain: Git repo → Kustomization → Deployment → Pod

```
Pod payment-api-7d4b8c-xyz [Running]
  ↑ owned by
ReplicaSet payment-api-7d4b8c
  ↑ owned by
Deployment payment-api [3/3 ready]
  ↑ managed by
Kustomization apps/payment [Ready]
  ↑ sources from
GitRepository flux-system/main [rev abc123]
```

### Deep-Dive Trees

Press `4` in the TUI to see every Deployment with its ReplicaSets and Pods:

```
Deployments (47)
├── nginx-ingress [Helm]
│   └── ReplicaSet nginx-ingress-7d4b8c
│       ├── Pod nginx-ingress-7d4b8c-abc12  ✓ Running
│       └── Pod nginx-ingress-7d4b8c-def34  ✓ Running
├── payment-api [Flux: payments/payment-api]
│   └── ReplicaSet payment-api-6c5d7b
│       └── Pod payment-api-6c5d7b-xyz99  ✓ Running
```

### Structural Understanding

Press `w` to see workloads grouped by owner:

```
WORKLOADS BY OWNER
────────────────────────────────────────
Flux (28)
  ├── podinfo           apps        Deployment  ✓
  ├── nginx-ingress     ingress     Deployment  ✓
  └── ...

Helm (12)
  ├── prometheus        monitoring  StatefulSet ✓
  └── ...

Native (7)
  └── debug-nginx       temp-test   Deployment  ⚠ (orphan)
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

cub-scout is the open-source cluster observer from [ConfigHub](https://confighub.com).

**Standalone mode:** Works forever, no signup required. See your cluster, trace ownership, scan for issues.

**Connected mode:** Link to ConfigHub for:
- Multi-cluster fleet visibility
- One-click import of discovered workloads
- Revision history and compare WET↔LIVE
- Team collaboration and change tracking

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
