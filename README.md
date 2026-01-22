# cub-scout

**Understand your Kubernetes cluster in seconds.**

Navigate 500 resources as easily as 5. Trace any deployment back to its Git source. See why pods are failing without digging through kubectl.

```bash
brew install confighub/tap/cub-scout
cub-scout map
```

Press `w` for workloads. Press `T` to trace. Press `4` for deep-dive.

---

## The Problem

You have 50+ Kustomizations. 200+ deployments. 10+ namespaces.

When something breaks:
- Which Kustomization manages this deployment?
- What Git repo is it from?
- What changed recently?

You piece it together with kubectl, Git, and tribal knowledge. It takes 15 minutes when you need answers in 15 seconds.

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
| `cub-scout map workloads` | All deployments grouped by owner |
| `cub-scout map deep-dive` | Deployment → ReplicaSet → Pod trees |
| `cub-scout trace deploy/x -n y` | Full ownership chain to Git source |
| `cub-scout map orphans` | Resources not managed by GitOps |
| `cub-scout scan` | Configuration risk patterns (46 patterns) |

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
