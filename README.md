# cub-scout

**Find who owns every Kubernetes resource in 10 seconds.**

<!-- TODO: Add screenshot
![cub-scout TUI](docs/images/screenshot.png)
-->

- Detect Flux, ArgoCD, Helm, or orphaned resources
- Trace any resource back to its Git source
- Find misconfigurations before they cause outages

**No signup required. Works on any cluster.**

---

## Install

### Homebrew (macOS/Linux)

```bash
brew install confighub/tap/cub-scout
```

### Docker

```bash
docker run --rm -v ~/.kube:/root/.kube ghcr.io/confighub/cub-scout map list
```

### From Source

```bash
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
go build ./cmd/cub-scout
./cub-scout version
```

---

## Quick Start

```bash
# What's in my cluster? Who owns it?
cub-scout map

# Plain text output
cub-scout map list

# Who manages this deployment?
cub-scout trace deploy/nginx -n production

# Find unmanaged resources
cub-scout map orphans

# Scan for misconfigurations
cub-scout scan
```

---

## What It Detects

| Owner | How It's Detected |
|-------|-------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` labels |
| **ArgoCD** | `argocd.argoproj.io/instance` labels |
| **Helm** | `app.kubernetes.io/managed-by: Helm` |
| **ConfigHub** | `confighub.com/UnitSlug` labels |
| **Orphan** | None of the above |

---

## Pricing

| Feature | Free | Pro |
|---------|:----:|:---:|
| Single cluster | ✓ | ✓ |
| Ownership detection | ✓ | ✓ |
| Orphan detection | ✓ | ✓ |
| Misconfiguration scanning | ✓ | ✓ |
| Multi-cluster fleet | — | ✓ |
| Import to ConfigHub | — | ✓ |
| Team collaboration | — | ✓ |

**Free forever for single cluster.**

---

## Documentation

**[Read the complete guide →](GUIDE.md)**

| Topic | Link |
|-------|------|
| All commands | [GUIDE.md](GUIDE.md#commands) |
| Testing | [GUIDE.md](GUIDE.md#testing) |
| Troubleshooting | [GUIDE.md](GUIDE.md#common-errors) |

---

## Part of ConfigHub

cub-scout is the open-source cluster observer from [ConfigHub](https://confighub.com).

- **Website:** [confighub.com](https://confighub.com)
- **Discord:** [discord.gg/confighub](https://discord.gg/confighub)
- **Issues:** [GitHub Issues](https://github.com/confighub/cub-scout/issues)

---

## License

MIT License — see [LICENSE](LICENSE)
