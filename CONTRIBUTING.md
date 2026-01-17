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

### Read-Only by Default

cub-scout is a read-only observer. Commands should never modify cluster state unless:
- Explicitly documented as write operations
- Require explicit flags (e.g., `--apply`, `--force`)

### Deterministic Behavior

All logic must be deterministic:
- Same input = same output
- No AI/ML in core logic
- Auditable and explainable

### Graceful Degradation

Features should work in degraded environments:
- No cluster? Use `--file` for static analysis
- No ConfigHub? Standalone mode works
- No internet? Offline mode works

## Questions?

- Open an issue for bugs or feature requests
- Join our Discord for discussion (link in README)

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
