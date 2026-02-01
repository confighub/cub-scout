# Contributing to cub-scout

Thank you for your interest in contributing to **cub-scout**!

cub-scout is a **read-only GitOps explorer and debugger**.
All contributions must respect this core identity.

---

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

---

## Development Guidelines

### Code Style

* Follow standard Go conventions
* Run `go fmt` and `go vet` before committing
* Keep functions focused and testable

### Testing Requirements

All PRs must pass the test suite:

```bash
go test ./... -v
```

Add tests for new functionality. We aim for high coverage on:

* Ownership detection logic
* Query parsing
* CLI command behavior

Fixture-based and snapshot-based tests are preferred where possible.

---

## Commit Messages

Use clear, descriptive commit messages:

* `feat: Add support for Terraform ownership detection`
* `fix: Correct namespace filtering in list command`
* `docs: Update trace command examples`
* `test: Add unit tests for Helm label detection`

---

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes with tests
4. Ensure `go test ./... -v` passes
5. Submit a pull request

PR descriptions should briefly answer:

* What user problem does this solve?
* Does this improve **exploration**, **debugging**, or both?

---

## Project Principles

These principles apply to **all** contributions:

| Principle             | What It Means                                                                       |
| --------------------- | ----------------------------------------------------------------------------------- |
| Explorer and debugger | cub-scout explores live cluster state and debugs GitOps behavior                    |
| Single cluster        | Standalone mode inspects one kubectl context; multi-cluster only via connected mode |
| Read-only by default  | Never modify cluster state; use Get, List, Watch only                               |
| Deterministic         | Same input = same output; no AI/ML in core logic                                    |
| Parse, don't guess    | Ownership from actual labels, annotations, and controller metadata                  |
| Complement GitOps     | Works alongside Flux, Argo, Helm — doesn't compete                                  |
| Graceful degradation  | Works without cluster (`--file`), ConfigHub, or internet                            |
| Test everything       | `go test ./...` must pass                                                           |

**Rule of thumb:**

> If a feature requires knowing *what should exist*, it belongs in **connected mode**.
> If it explains *what exists and why*, it belongs in **cub-scout core**.

---

## TUI & CLI Design Principles (Important)

cub-scout treats the **TUI as a guided debug shell**, not a replacement for the CLI.

### Key expectations:

* The `:` key **must remain supported**
* `:` allows shelling out to `cub` CLI commands while staying in TUI context
* Commands executed via `:` inherit the current context:

  * cluster
  * namespace
  * selected resource
* The TUI should be aware of all available `cub` CLI commands and help users:

  * discover commands
  * understand correct usage
  * choose the appropriate command for the situation
* In connected mode, the TUI may provide richer suggestions and completions using external context

The TUI must never hide, replace, or restrict CLI functionality.

Contributions that improve CLI discoverability, context-aware help, or command completion are strongly encouraged.

---

## Implementation Notes

### Read-only exceptions

Commands may modify state **only if**:

* Explicitly documented
* Require explicit flags (e.g., `--apply`, `--force`)
* Are clearly separated from default workflows

Such exceptions are rare and subject to stricter review.

### Universal interface

When building features spanning multiple GitOps tools:

* User-facing flags and output must be consistent
* Tool-specific behavior belongs behind a shared interface

---

## Code of Conduct

* Be respectful and constructive
* Focus on the code, not the person
* Help others learn and grow

---

## Questions?

* Discord: [https://discord.gg/confighub](https://discord.gg/confighub) — Ask questions, get help
* Issues: GitHub Issues — Bugs and feature requests

---

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
