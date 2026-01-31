# Contributing to cub-scout

Thank you for your interest in contributing to cub-scout!

## Getting Started

```bash
# Clone the repo
git clone https://github.com/confighub/cub-scout.git
cd cub-scout

# Build
go build ./cmd/cub-scout

# Run tests
go test ./... -v
```

## Development Guidelines

### Code Style

- Follow standard Go conventions
- Run `go fmt` and `go vet` before committing
- Keep functions focused and testable

### Testing Requirements

All PRs must pass the test suite:

```bash
go test ./... -v
```

Add tests for new functionality. We aim for high coverage on:
- Ownership detection logic
- Query parsing
- CLI command behavior

### Commit Messages

Use clear, descriptive commit messages:

```
feat: Add support for Terraform ownership detection
fix: Correct namespace filtering in list command
docs: Update trace command examples
test: Add unit tests for Helm label detection
```

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes with tests
4. Ensure `go test ./... -v` passes
5. Submit a pull request

## Code of Conduct

- Be respectful and constructive
- Focus on the code, not the person
- Help others learn and grow

## Project Principles

These principles apply to all contributions:

| Principle | What It Means |
|-----------|---------------|
| **Single cluster** | Standalone mode inspects one kubectl context; multi-cluster only via connected mode |
| **Read-only by default** | Never modify cluster state; use `Get`, `List`, `Watch` only |
| **Deterministic** | Same input = same output; no AI/ML in core logic |
| **Parse, don't guess** | Ownership from actual labels, not heuristics |
| **Complement GitOps** | Works alongside Flux, Argo, Helm — doesn't compete |
| **Graceful degradation** | Works without cluster (`--file`), ConfigHub, or internet |
| **Test everything** | `go test ./...` must pass |

### Implementation Notes

**Read-only exceptions:** Commands may modify state only if explicitly documented and requiring explicit flags (e.g., `--apply`, `--force`).

**Universal interface:** When building features spanning multiple GitOps tools, the interface must be universal (same flags, same output) while implementation can be tool-specific.

## Questions?

- **Discord:** [discord.gg/confighub](https://discord.gg/confighub) — Ask questions, get help
- **Issues:** [GitHub Issues](https://github.com/confighub/cub-scout/issues) — Bugs and feature requests

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
