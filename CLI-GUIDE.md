# cub-scout CLI Guide

**cub-scout observes and explains; it never decides.** It is the read-only
Kubernetes and GitOps observer — every command in this guide reads cluster
or ConfigHub state and explains it, but nothing here mutates the cluster.

Workflow-first guide for learning how to use cub-scout without turning this
file into a second command encyclopedia.

Need a specific command or flag?
- [Complete CLI Reference (A-Z)](docs/reference/cli-reference.md)
- [Command Reference](docs/reference/commands.md)
- [CLI Contract Reference](docs/reference/cli-contract.md)
- [JSON Contracts](docs/reference/json-contracts.md)

---

## Choose The Right Doc

| If you need... | Go here |
|----------------|---------|
| The full command catalog | [docs/reference/cli-reference.md](docs/reference/cli-reference.md) |
| Usage examples for a command | [docs/reference/commands.md](docs/reference/commands.md) |
| Stable flags, exit codes, and schema guarantees | [docs/reference/cli-contract.md](docs/reference/cli-contract.md) |
| JSON field-level contract details | [docs/reference/json-contracts.md](docs/reference/json-contracts.md) |
| A workflow-oriented tour | This guide |

---

## First 15 Minutes

Start with the smallest loop that proves value on a real cluster:

```bash
cub-scout quickstart
cub-scout doctor
cub-scout explain deploy/my-app -n prod
cub-scout trace deploy/my-app -n prod
cub-scout map
```

What each step gives you:
- `quickstart` shows the fastest meaningful path across doctor, explain, ownership, and scan.
- `doctor` gives the compact cluster health summary and next steps.
- `explain` tells you who owns one resource, what state it is in, and what to do next.
- `trace` shows the source chain behind the resource.
- `map` opens the TUI when you want to browse interactively.

If you prefer JSON first:

```bash
cub-scout doctor --format json
cub-scout explain deploy/my-app -n prod --format json
cub-scout trace deploy/my-app -n prod --format json
```

---

## Troubleshooting Flow

When something is broken and you need proof quickly, this is the default flow:

1. `cub-scout doctor`
2. `cub-scout explain <kind/name> -n <ns>`
3. `cub-scout trace <kind/name> -n <ns>`
4. `cub-scout compare three-way --scope <scope>` if you are in connected mode
5. `cub-scout history <kind/name> -n <ns>` if you need governed change history

Use this when:
- a workload is unhealthy
- you need to distinguish ownership from symptoms
- you need to compare governed state to observed state
- you want to stay out of the Argo CD GUI unless there is a real gap

Helpful follow-up paths:
- `scan` for risk patterns and stuck states
- `map issues` for a cluster-wide issue inventory
- `map hooks` for Helm and Argo lifecycle hooks
- `gitops status` for deployer/source health

See [docs/reference/commands.md](docs/reference/commands.md) for the detailed examples behind each command.

---

## Connected Workflows

Connected mode adds ConfigHub-backed comparison, history, import, and fleet workflows.

Typical connected flow:

```bash
cub auth login
cub-scout status
cub-scout compare three-way --scope namespace/prod
cub-scout history deploy/api -n prod
cub-scout impact payments-api
```

Import and migration flow:

```bash
cub-scout import --dry-run -n prod
cub-scout import --json
cub-scout import parse-repo --path ./repo
cub-scout import apply proposal.json --dry-run
```

Important boundary:
- `cub-scout` is the read-first explorer and import-preview tool.
- `cub` remains the intended-state authority and renderer/import lifecycle tool.

If you need more on import:
- [docs/howto/import-to-confighub.md](docs/howto/import-to-confighub.md)
- [docs/reference/commands.md#import](docs/reference/commands.md#import)

---

## AI And Automation

cub-scout is designed to expose read-only cluster facts to automation and AI
agents. The primary AI gateway is a Model Context Protocol (MCP) server that
exposes the same read-only commands you use from the CLI.

High-value entrypoints:

```bash
cub-scout mcp serve
cub-scout context-pack --format json --max-bytes 16384
cub-scout doctor --format json
cub-scout explain deploy/api -n prod --format json
cub-scout compare three-way --scope namespace/prod --format json
```

Use:
- `mcp serve` when you want a read-only MCP (Model Context Protocol) server over stdio
- `context-pack` when you want a bounded AI handoff bundle
- JSON outputs when you want stable machine-readable facts

For AI-specific operating guidance:
- [AI-README-FIRST.md](AI-README-FIRST.md)
- [skills/cub-scout/SKILL.md](skills/cub-scout/SKILL.md)
- [docs/howto/using-cub-scout-from-ai-tool.md](docs/howto/using-cub-scout-from-ai-tool.md)

---

## Choose Your Interface

| Interface | Best for | Start here |
|-----------|----------|------------|
| TUI | Interactive exploration and keyboard-driven debugging | `cub-scout map` |
| CLI | One-off commands, shell use, pipelines | `cub-scout doctor`, `cub-scout explain`, `cub-scout trace` |
| JSON | Automation, AI, Model Context Protocol (MCP) workflows | `--format json` or `--json` |

Good rule of thumb:
- start with `doctor` if you do not know where to begin
- use `explain` when you have one resource in hand
- use `trace` when you need source lineage
- use `map` when you need to browse across many resources

---

## Verify Behavior Locally

The local binary is always the runtime truth:

```bash
go build ./cmd/cub-scout
./cub-scout --help
./cub-scout <command> --help
```

Examples:

```bash
./cub-scout compare --help
./cub-scout import --help
./cub-scout mcp serve --help
./cub-scout trace --help
```

Use help output to confirm:
- current command placement after CLI reorganizations
- canonical subcommand names
- flag names and defaults

---

## Canonical Doc Layout

This is the intended split for the core CLI docs:

| Doc | Canonical role |
|-----|----------------|
| [README.md](README.md) | Project overview, Fast Path, and links |
| [CLI-GUIDE.md](CLI-GUIDE.md) | Workflow-first CLI tour |
| [docs/reference/cli-reference.md](docs/reference/cli-reference.md) | A-Z command catalog |
| [docs/reference/commands.md](docs/reference/commands.md) | Detailed command usage and examples |
| [docs/reference/cli-contract.md](docs/reference/cli-contract.md) | Stable flags, exit codes, and schemas |

If you find command-detail duplication outside those roles, treat it as doc drift and clean it up instead of copying it forward.

---

## Related Docs

- [docs/getting-started/start-here.md](docs/getting-started/start-here.md)
- [docs/reference/cli-reference.md](docs/reference/cli-reference.md)
- [docs/reference/commands.md](docs/reference/commands.md)
- [docs/reference/cli-contract.md](docs/reference/cli-contract.md)
- [docs/reference/json-contracts.md](docs/reference/json-contracts.md)
- [examples/README.md](examples/README.md)
