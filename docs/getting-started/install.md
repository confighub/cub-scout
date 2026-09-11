# Installation

For v2.10.1, use Homebrew, the `cub` plugin, or the published archives below.
You do not need a ConfigHub account for standalone cluster observation.

## ConfigHub Plugin

With the separately installed `cub` CLI:

```sh
cub plugin install confighub/cub-scout@v2.10.1
cub scout version
cub scout doctor
```

For an existing installation: `cub plugin upgrade scout@v2.10.1`.
The plugin has the same observation commands as the standalone client. ConfigHub
authentication is needed only for connected operations. Use `--kube-context`
for bounded Kubernetes reads; the host's `--context` selects a ConfigHub context.

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

The [v2.10.1 release](https://github.com/confighub/cub-scout/releases/tag/v2.10.1)
contains six archives and [checksums.txt](https://github.com/confighub/cub-scout/releases/download/v2.10.1/checksums.txt).
Archive names include the version and use underscores, not hyphens:

| Platform | Archive |
|---|---|
| macOS Apple Silicon | [cub-scout_2.10.1_darwin_arm64.tar.gz](https://github.com/confighub/cub-scout/releases/download/v2.10.1/cub-scout_2.10.1_darwin_arm64.tar.gz) |
| macOS Intel | [cub-scout_2.10.1_darwin_amd64.tar.gz](https://github.com/confighub/cub-scout/releases/download/v2.10.1/cub-scout_2.10.1_darwin_amd64.tar.gz) |
| Linux arm64 | [cub-scout_2.10.1_linux_arm64.tar.gz](https://github.com/confighub/cub-scout/releases/download/v2.10.1/cub-scout_2.10.1_linux_arm64.tar.gz) |
| Linux amd64 | [cub-scout_2.10.1_linux_amd64.tar.gz](https://github.com/confighub/cub-scout/releases/download/v2.10.1/cub-scout_2.10.1_linux_amd64.tar.gz) |
| Windows arm64 | [cub-scout_2.10.1_windows_arm64.zip](https://github.com/confighub/cub-scout/releases/download/v2.10.1/cub-scout_2.10.1_windows_arm64.zip) |
| Windows amd64 | [cub-scout_2.10.1_windows_amd64.zip](https://github.com/confighub/cub-scout/releases/download/v2.10.1/cub-scout_2.10.1_windows_amd64.zip) |

For example, on Linux amd64:

```sh
curl -fLO https://github.com/confighub/cub-scout/releases/download/v2.10.1/cub-scout_2.10.1_linux_amd64.tar.gz
curl -fLO https://github.com/confighub/cub-scout/releases/download/v2.10.1/checksums.txt
sha256sum cub-scout_2.10.1_linux_amd64.tar.gz
# Compare with the matching entry in checksums.txt before extracting.
tar -xzf cub-scout_2.10.1_linux_amd64.tar.gz
./cub-scout version
```

On macOS use `shasum -a 256 <archive>`; on Windows use PowerShell
`Get-FileHash <archive> -Algorithm SHA256`. Extract the archive after checking it.
Unix archives include `cub-scout`, `kubectl-cub_scout`, and the plugin entry point
`main`. Windows archives include the two `.exe` clients, not the `cub` plugin.

### Go Module Version Caveat

Do not use `go install github.com/confighub/cub-scout/cmd/cub-scout@latest` to
install v2.10.1. The module path does not have a `/v2` suffix: the default resolver
returned v1.13.0 for `@latest` during release verification, and an explicit
`@v2.10.0` query failed Go's major-version check. The patch keeps the same module
path. Use an archive or build the tagged checkout below. Distribution follow-up:
[#520](https://github.com/confighub/cub-scout/issues/520).

### Container and Bot

The v2.10.1 workflow publishes a Linux amd64 image with numeric non-root UID
65532. **Anonymous registry pulls returned 403** during v2.10.0 verification;
the patch's pull check is tracked in [#530](https://github.com/confighub/cub-scout/issues/530).
Do not treat the command below as a verified public installation route:

```bash
docker run ghcr.io/confighub/cub-scout:v2.10.1 version
```

Linux arm64 archive binaries are available, but there is no published multiarch
container in this release. The [bot example](../../examples/bot/) describes
ServiceAccount/RBAC and sink configuration; [release proof](../releases/v2.10.1.md)
distinguishes local-image smoke from registry access. Do not weaken permissions
or change package visibility just to make a smoke test pass.

### Build from Source

```bash
git clone --branch v2.10.1 --depth 1 https://github.com/confighub/cub-scout.git
cd cub-scout
go build -ldflags '-X main.BuildTag=2.10.1' -o cub-scout ./cmd/cub-scout
./cub-scout version
```

### kubectl krew

`kubectl krew install cub-scout` is not a verified distribution path. Use the
`kubectl-cub_scout` executable shipped with Homebrew or the archives. Only use
the following after confirming an entry is available in your configured index:

```bash
kubectl krew install cub-scout
```

## Prerequisites

- A kubeconfig context and permissions for the resources being inspected
- A reachable Kubernetes API for live checks; no cluster needed for file/bundle analysis
- `kubectl` is useful for checking access; richer trace paths may need the CLIs below

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
