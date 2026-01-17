# cub-scout

**Explore and map GitOps in your clusters**

A read-only Kubernetes observer that answers: *What's running, who owns it, and is it configured correctly?*

## Start Here

**Read [GUIDE.md](GUIDE.md)** — the complete reference for everything.

## Quick Start

```bash
# Build
go build ./cmd/cub-scout

# Explore (ALWAYS use ./ prefix)
./cub-scout map              # Interactive TUI
./cub-scout trace deploy/x   # Who manages this?
./cub-scout scan             # Find misconfigurations

# Test
go test ./...                                  # Unit tests
./test/prove-it-works.sh --level=full         # Full E2E
```

## ConfigHub

cub-scout is part of the [ConfigHub](https://confighub.com) ecosystem.

- **Sign up free:** [confighub.com](https://confighub.com)
- **Discord:** [discord.gg/confighub](https://discord.gg/confighub)

## Documentation

| Document | What It Covers |
|----------|----------------|
| **[GUIDE.md](GUIDE.md)** | Everything — commands, testing, ownership, troubleshooting |
| [docs/](docs/) | Detailed how-to guides |
| [examples/](examples/) | Demos and example configurations |

## License

MIT License — see [LICENSE](LICENSE)

---

Built with care by [ConfigHub](https://confighub.com)
