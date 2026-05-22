# Reference: `cub scout` (plugin) vs `cub-scout` (standalone binary)

cub-scout has two invocation forms. They are **functionally equivalent** but contextually different.

| Form | Where it comes from | When it's the right form |
|---|---|---|
| `cub-scout` (or `./cub-scout`) | Standalone binary built from this repo's `cmd/cub-scout/`. Distributed via Homebrew cask, direct download, GoReleaser releases. | Local development on this repo (use `./cub-scout` from a fresh `go build ./cmd/cub-scout`); standalone-mode investigation without ConfigHub; CI / agent invocation against a built artifact. |
| `cub scout` | Subcommand of the `cub` CLI when the `scout` plugin is installed. | User has `cub` installed already and wants cub-scout as a subcommand of the ConfigHub workflow CLI. The preferred documented form for `cub`-first audiences. |

## Documentation wording

Per `AI-README-FIRST.md`:

> Preferred wording in AI/product prose: `cub scout`.
> When showing exact local repo commands in this repo, use `./cub-scout ...`.

The reason: most user-facing docs assume the operator has `cub` installed (it's the ConfigHub CLI, the entry point most teams adopt). In that context, `cub scout` reads naturally. In *this* repo's local commands, `./cub-scout` is exact — it's the binary you just built.

The skills throughout the repo follow this:
- `allowed-tools` lines enumerate **both** forms: `Bash(./cub-scout <verb>)`, `Bash(cub-scout <verb>)`, `Bash(cub scout <verb>)`. All three resolve.
- Worked examples use the form most relevant to the skill's context: `cub-scout` for local-development demos; `cub scout` for product-prose snippets.

## Functional parity

The plugin and the standalone binary are the **same code**. Building cub-scout produces a binary that's both:

- Invokable as `cub-scout` (standalone)
- Discoverable as `cub-<command>` by the `cub` CLI's plugin mechanism (so `cub scout <verb>` dispatches to the same binary)

The plugin path is a thin shim over the standalone path — same flags, same JSON contracts, same exit codes. There is no plugin-only or standalone-only verb.

## The v2.0.0 plugin switchover

`HANDOVER.md` records the next major milestone:

> The next major milestone is now the `cub scout` plugin switchover for `v2.0.0`.

Core direction:

- `cub scout` becomes the **preferred** invocation in product docs
- `cub scout mcp serve` becomes the preferred AI-explorer / investigation gateway
- `cub` remains the authority and governed-execution host

The standalone `cub-scout` binary is **not** going away — it stays for users without `cub` installed, for local development, and for CI. The switchover is about the **documented** invocation form, not the codebase.

Canonical planning doc: `docs/releases/v2.0.0-plugin-plan.md`.

## CLI migration table (`#375`)

The v2.0.0 plan also folded some legacy top-level commands under their canonical parents. The migration table from `HANDOVER.md`:

| Old top-level | New canonical path | Compatibility |
|---|---|---|
| `discover` | `map workloads` | Hidden deprecated alias kept for one release |
| `health` | `map issues` | Hidden deprecated alias kept for one release |
| `combined` | `compare` | Alias kept; `compare` is the primary name |
| `connect` | `setup connect` | Hidden deprecated alias kept for one release |
| `completion` | `setup completion` | Hidden deprecated alias kept for one release |
| `apply` | `import apply` | Hidden deprecated alias kept for one release |
| `parse-repo` | `import parse-repo` | Hidden deprecated alias kept for one release |
| `import-argocd` | `import argocd` | Hidden deprecated alias kept for one release |
| `import-cluster-aggregator` | `import cluster-aggregator` | Hidden deprecated alias kept for one release |
| `drift` | `compare drift` | Hidden deprecated alias kept for one release |
| `demo` | `quickstart demo` | Hidden deprecated alias kept for one release |

These aliases work for both `cub scout` and `cub-scout`. The renames apply equally; the plugin form is not a separate parser.

## Top-level command count cap

`cmd/cub-scout/command_layout_test.go` (`TestRootCommandLayout_VisibleTopLevelCount`) caps visible top-level commands at **32**. The cap exists to keep `--help` readable and to force structural decisions when adding new top-level verbs.

History of the cap:
- 30 (initial)
- 31 (bumped in `#391` to admit `views` as a real new top-level capability)
- 32 (bumped in `#446` batch 1 to admit `receipt` as the 8th verb group)

Adding a top-level verb requires bumping the cap with a comment cite — not silent. The same applies to `cub scout` because the plugin and standalone parsers are the same.

## `cub scout` vs `kubectl scout` (does not exist)

cub-scout was at one point considered as a `kubectl` plugin (`kubectl scout`), but that path was dropped in favor of `cub scout`. The reason: cub-scout's value is **GitOps-aware** observation, and `kubectl` plugins traditionally focus on the live cluster only. `cub scout` keeps cub-scout in the same neighborhood as ConfigHub workflows; `kubectl scout` would imply the cluster is the source of truth, which contradicts cub-scout's "complement GitOps" principle (CLAUDE.md § "Key Principles" #5).

If a user asks about a `kubectl scout` plugin: it doesn't exist. The `cub scout` plugin is the analog.

## Practical patterns

### Local development (this repo)

```bash
# Build from this repo
$ go build ./cmd/cub-scout

# Run against the cluster
$ ./cub-scout doctor -n prod
$ ./cub-scout receipt verify deploy/api -n prod --save
```

Use `./cub-scout` to be unambiguous — the built artifact in the current directory, not whatever's on `$PATH`.

### Product documentation / blog posts / AI prose

```bash
# Preferred form in product docs
$ cub scout doctor -n prod
$ cub scout receipt verify deploy/api -n prod
```

Reads naturally as "cub's `scout` subcommand."

### CI / agent invocation against a deployed binary

```bash
# Either form works; pick what matches your install path
$ cub-scout doctor -n prod --format json
# or
$ cub scout doctor -n prod --format json
```

The MCP server config typically uses `cub-scout` (the standalone binary) because that's what's on PATH:

```jsonc
{
  "mcpServers": {
    "cub-scout": {
      "command": "cub-scout",
      "args": ["mcp", "serve"]
    }
  }
}
```

### CLI guides and reference docs

`docs/reference/commands.md` and `docs/reference/cli-reference.md` use **`cub-scout`** (standalone form) in examples — exact, no plugin-install assumption. The product README (`README.md`) and skill descriptions favor `cub scout`.

## The `preferInvocationForm` lint

`cub-scout/#386` tracks extending an automated linter that catches legacy invocation-form leaks in hint strings. The high-severity finding from `#410` (architectural-triad audit) on hint-command wording was resolved as part of the `#428` work; lower-severity items remain open in `#386`.

## Skills that consume this reference

- The umbrella router [`cub-scout`](../cub-scout/SKILL.md) — names both forms in its description
- Every verb-group / observer / workflow skill — `allowed-tools` enumerates both `./cub-scout <verb>` and `cub-scout <verb>` and `cub scout <verb>`
- [`ai-agent-readonly-context`](../ai-agent-readonly-context/SKILL.md) — the MCP config snippet picks `cub-scout`

## References

- v2.0.0 plan: `docs/releases/v2.0.0-plugin-plan.md`
- AI cold-start preferred wording: [`AI-README-FIRST.md`](../../AI-README-FIRST.md)
- CLI migration table: [`HANDOVER.md`](../../HANDOVER.md) § "CLI migration table"
- Top-level command count cap: `cmd/cub-scout/command_layout_test.go` `TestRootCommandLayout_VisibleTopLevelCount`
- `#375` — top-level command sprawl reduction
- `#391` — views top-level addition (cap bumped 30 → 31)
- `#446` batch 1 (PR `#454`) — receipt top-level addition (cap bumped 31 → 32)
- `#386` — `preferInvocationForm` lint follow-ups
