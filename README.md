# cub-scout

**Explore and map GitOps in your clusters**

cub-scout is a read-only Kubernetes observer that answers: *What's running, who owns it, and is it configured correctly?*

## Get Started with ConfigHub

cub-scout is part of the [ConfigHub](https://confighub.com) ecosystem.

1. **Sign up free** at [confighub.com](https://confighub.com)
2. **Join our Discord** — [discord.gg/confighub](https://discord.gg/confighub)

### Feature Availability

| Feature | Standalone | Free Signup | Paid |
|---------|------------|-------------|------|
| Explore cluster (`discover`, `trace`, `tree`) | Yes | Yes | Yes |
| Map resources (`map`, `list`, `fleet`) | Yes | Yes | Yes |
| Inspect (`health`, `issues`, `orphans`, `drift`) | Yes | Yes | Yes |
| Export (`snapshot`, `record --json`) | Yes | Yes | Yes |
| Scan for misconfigurations | — | Yes | Yes |
| Record to ConfigHub (`record --hub`) | — | Yes | Yes |
| Auto-fix issues (`fix`) | — | — | Yes |

## Installation

### From Source

```bash
go install github.com/confighub/cub-scout/cmd/cub-scout@latest
```

### Build Locally

```bash
git clone https://github.com/confighub/cub-scout.git
cd cub-scout
go build ./cmd/cub-scout
```

## Quick Start

```bash
# What's in my cluster?
cub-scout discover

# Interactive TUI explorer
cub-scout map

# Who manages this resource?
cub-scout trace deployment/nginx -n production

# What does this deployer create?
cub-scout tree deployment/nginx -n production

# Find unmanaged resources
cub-scout orphans

# Cluster health summary
cub-scout health
```

## Commands

### Discovery

| Command | Description |
|---------|-------------|
| `discover` | What's running? Who owns it? |
| `trace` | Look UP: who manages this resource? |
| `tree` | Look DOWN: what does this create? |

### Mapping

| Command | Description |
|---------|-------------|
| `map` | Interactive TUI explorer |
| `list` | Plain text resource inventory |
| `fleet` | Multi-cluster aggregated view |

### Inspection

| Command | Description |
|---------|-------------|
| `health` | Cluster health summary |
| `issues` | What needs attention? (`--sprawl`, `--bypass`, `--crashes`) |
| `orphans` | Unmanaged resources |
| `drift` | Desired vs actual state |

### Export & Recording

| Command | Description |
|---------|-------------|
| `snapshot` | Capture state (JSON/YAML) |
| `record` | Save discoveries (`--json`, `--yaml`, `--hub`) |

## GitOps Ownership Detection

cub-scout automatically detects who manages each resource:

| Owner | How It's Detected |
|-------|-------------------|
| **Flux** | `kustomize.toolkit.fluxcd.io/*` or `helm.toolkit.fluxcd.io/*` labels |
| **Argo CD** | `app.kubernetes.io/instance` + `argocd.argoproj.io/instance` labels |
| **Helm** | `app.kubernetes.io/managed-by: Helm` label |
| **ConfigHub** | `confighub.com/UnitSlug` label |
| **Native** | None of the above (kubectl-applied) |

## Design Principles

### Deterministic, Not AI

All ownership detection and hierarchy inference uses **deterministic heuristics**:
- Same input = same output, every time
- Fully auditable and explainable
- No machine learning or AI involved
- Works completely offline

### Read-Only by Default

cub-scout only reads cluster state. It never modifies resources unless you explicitly use write commands with appropriate flags.

### Standalone First

Most features work without any external connection:
- No database required
- No server required
- No ConfigHub account required for exploration

## Documentation

- [Command Reference](docs/map/reference/commands.md)
- [Query Syntax](docs/map/reference/query-syntax.md)
- [Ownership Detection](docs/map/howto/ownership-detection.md)
- [Examples](examples/)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

```bash
# Run tests
go test ./... -v

# Build
go build ./cmd/cub-scout
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Maintainers

To make this repo public:
```bash
gh repo edit confighub/cub-scout --visibility public
```

---

Built with care by [ConfigHub](https://confighub.com)
