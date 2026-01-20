# cub-scout = the GitOps explorer

**Find who owns every Kubernetes resource in 10 seconds.**

```bash
./cub-scout map
```

---

## Why This Exists

Cub-scout is an explorer and mapping tool for Kubernetes clusters that run GitOps.  The tool can be used standalone (read-only) and has a 'ConfigHub connected' mode.

Every GitOps user faces the same problem: verifying the ownership and status of workloads and resources.  Several tools and scripting incanctations exist, but it is not always easy to recall what is most appropriate or how to use it.    

When something breaks at 2am, you need answers fast:
- Who manages this deployment?
- Is it Flux? ArgoCD? Someone's kubectl?
- What's the Git source?

**cub-scout gives you that visibility instantly.**

It reads your cluster (read-only), detects ownership by examining labels and annotations, and shows you exactly what's going on.  No agents to install, no databases, no signup.  As much as possible we are leveraging existing tools, bringing them into a single framework and making them easy to use.

---

## What It Does

| Command | What You Get |
|---------|--------------|
| `./cub-scout map` | Interactive TUI showing all resources by owner |
| `./cub-scout trace deploy/nginx -n prod` | Full ownership chain: Git → Flux/Argo → Deployment |
| `./cub-scout map orphans` | Resources not managed by GitOps (shadow IT) |
| `./cub-scout scan` | Configuration risk patterns |

### Ownership Detection

| Owner | How It's Detected |
|-------|-------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` labels |
| **ArgoCD** | `argocd.argoproj.io/instance` label |
| **Helm** | `app.kubernetes.io/managed-by: Helm` |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | None of the above (kubectl-applied) |

---

## Install

### From Source (Recommended)

```bash
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
go build ./cmd/cub-scout
./cub-scout version
```

### Homebrew (macOS/Linux)

```bash
brew install confighub/tap/cub-scout
```

### Docker

```bash
docker run --rm --network=host \
  -v ~/.kube:/home/nonroot/.kube \
  ghcr.io/confighub/cub-scout map list
```

---

## Quick Start

```bash
# Build
go build ./cmd/cub-scout

# What's in my cluster? Who owns it?
./cub-scout map

# Plain text output (for scripting)
./cub-scout map list

# Who manages this deployment?
./cub-scout trace deploy/nginx -n production

# Find unmanaged resources
./cub-scout map orphans

# Scan for configuration issues
./cub-scout scan
```

**Press `?` in the TUI for keyboard shortcuts.**

---

## CLI Guide

See **[CLI-GUIDE.md](CLI-GUIDE.md)** for the complete command reference with:
- Every command explained
- What you'd do without cub-scout (kubectl, bash, etc.)
- Expected output examples

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

Use it standalone forever, or connect to ConfigHub for:
- Multi-cluster fleet visibility
- One-click import of discovered workloads
- Team collaboration and change tracking

---

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

This is an early-stage project. If it proves useful, we'll expand the community:
- Found a bug? [Open an issue](https://github.com/confighub/cub-scout/issues)
- Have an idea? Start a discussion
- Want to contribute? PRs welcome

---

## Community

- **Discord:** [discord.gg/confighub](https://discord.gg/confighub) — Ask questions, share feedback
- **Issues:** [GitHub Issues](https://github.com/confighub/cub-scout/issues)
- **Website:** [confighub.com](https://confighub.com)

---

## License

MIT License — see [LICENSE](LICENSE)
