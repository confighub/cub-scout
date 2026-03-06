# Installation

## Quick Install (Recommended)

```bash
# macOS/Linux via Homebrew
brew install confighub/tap/cub-scout

# Verify installation
cub-scout version
kubectl cub-scout version
```

## Alternative Methods

### Download Binary

Download from [GitHub Releases](https://github.com/confighub/cub-scout/releases):

```bash
# Linux (amd64)
curl -LO https://github.com/confighub/cub-scout/releases/latest/download/cub-scout-linux-amd64.tar.gz
tar xzf cub-scout-linux-amd64.tar.gz
sudo mv cub-scout /usr/local/bin/

# macOS (arm64/Apple Silicon)
curl -LO https://github.com/confighub/cub-scout/releases/latest/download/cub-scout-darwin-arm64.tar.gz
tar xzf cub-scout-darwin-arm64.tar.gz
sudo mv cub-scout /usr/local/bin/

# Windows (PowerShell, amd64)
curl.exe -LO https://github.com/confighub/cub-scout/releases/latest/download/cub-scout-windows-amd64.zip
tar -xf cub-scout-windows-amd64.zip
.\cub-scout.exe version
```

### Go Install

```bash
go install github.com/confighub/cub-scout/cmd/cub-scout@latest
```

### Container

```bash
docker run ghcr.io/confighub/cub-scout:latest version
```

### Build from Source

```bash
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
go build -o cub-scout ./cmd/cub-scout
sudo mv cub-scout /usr/local/bin/
```

### kubectl krew

After `cub-scout` is published in krew-index:

```bash
kubectl krew install cub-scout
```

## Prerequisites

- **kubectl** configured with cluster access
- Kubernetes cluster running (local or remote)

```bash
# Verify kubectl works
kubectl get pods -A
```

## Optional: GitOps CLIs

For tracing GitOps ownership chains, install these if you use them:

| Tool | Install | Purpose |
|------|---------|---------|
| **flux** | `brew install fluxcd/tap/flux` | Trace Flux Kustomizations |
| **argocd** | `brew install argocd` | Trace ArgoCD Applications |
| **helm** | `brew install helm` | View Helm release info |

## Verify Installation

```bash
# Check cub-scout version
cub-scout version

# Check kubectl plugin mode
kubectl cub-scout version

# Guided first run (non-interactive)
cub-scout quickstart --yes

# One-command summary
cub-scout doctor

# Interactive TUI
cub-scout map
```

## kubectl Plugin Mode

kubectl discovers plugins by executable name. For `kubectl cub-scout`, it looks
for `kubectl-cub_scout` in `PATH`.

Homebrew installs both binaries automatically. For source builds:

```bash
make build-kubectl-plugin
sudo cp kubectl-cub_scout /usr/local/bin/
```

## Next Steps

- [Your First Map](first-map.md) - 5-minute quick start
- [Find Orphans](../howto/find-orphans.md) - Discover unmanaged resources
- [Trace Ownership](../howto/trace-ownership.md) - See the GitOps chain
