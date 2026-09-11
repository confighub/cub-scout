# Host and Plugin Compatibility Matrix

> **Status:** Updated for v2.9.0 preparation. Plugin packaging has shipped since v2.0.0. Supported combinations depend on both host commands and server capabilities; see the requirements below.
> **Related:** [`plugin-install.md`](../howto/plugin-install.md), [`v2.0.0-migration-guide.md`](../releases/v2.0.0-migration-guide.md), [`cub vs cub scout`](../concepts/cub-vs-cub-scout.md).

## Supported Invocation Forms

| Form | Command | Install | Auth | Notes |
|---|---|---|---|---|
| Plugin | `cub scout ...` | `cub plugin install confighub/cub-scout` | Inherits `CUB_TOKEN`/`CUB_SERVER` from parent `cub` | Preferred form during `v2.x`. |
| Standalone | `cub-scout ...` | Homebrew, krew, tar.gz, source | `cub auth get-token` fallback or own token store | Fully supported. No feature gap vs plugin form. |
| Local dev | `./cub-scout ...` | `go build ./cmd/cub-scout` | Same as standalone | Used inside the `cub-scout` repo for development. |

All three forms invoke the same binary with the same arguments. Flags, exit codes, JSON contracts, and MCP tool names are identical across forms.

## `cub-scout` Version × `cub` Version

| `cub-scout` | Plugin form support | Recommended `cub` host | Notes |
|---|---|---|---|
| `v1.12.x` and earlier | ❌ Not supported as plugin | any | Predates plugin packaging; use standalone `cub-scout` only. |
| `v1.13.x` | ❌ Not supported as plugin | any | Release-gate hardening for the trust-surface work. Plugin packaging had not started. |
| `v2.0.0`+ | ✅ First plugin-compatible release | `cub` with `plugin install` support | Archive layout contains a `main` entry point for plugin extraction. `CUB_PLUGIN=1` detected for help text, cobra command-path rendering, and auth inheritance. Release-gate parity test enforces standalone/plugin-form behavior equivalence. |
| `v2.x` (later) | ✅ | `cub` with `plugin install` support | Additive only during `v2.x` line. No breaking changes planned. |

Plugin support is exercised by `TestPluginParity_StandaloneMatchesPlugin` in
`cmd/cub-scout/plugin_parity_test.go`. Linux/macOS release archives contain both
`cub-scout` and the plugin entry point `main`; Windows archives are standalone.

## `cub` Host Requirements

Plugin form requires a `cub` version that provides:

- `cub plugin install <source>` — install from GitHub shorthand, GitHub URL, direct HTTPS URL, or tar.gz archive
- `cub plugin list` — discovery and path reporting
- `cub plugin uninstall <name>` — removal
- Plugin invocation path that execs the plugin binary with `CUB_PLUGIN=1`, `CUB_CONFIG`, `CUB_CONTEXT`, `CUB_SERVER`, `CUB_SPACE`, and `CUB_TOKEN` environment variables set

Verify with:

```bash
cub plugin --help
```

If `cub plugin` is not a known subcommand, upgrade `cub` before attempting plugin install. See the [`confighub/sdk`](https://github.com/confighub/sdk) release notes for the minimum version.

### v2.9 Resource Read Capabilities

Connected MCP Resource tools require the corresponding host commands and a
server that exposes the Resource API:

```bash
cub k8s get --help
cub k8s types --help
cub resource list --help
```

Local preparation checked these command surfaces on `cub v0.4.4`. This is a
tested CLI capability baseline, not a claim that v0.4.4 is the minimum version
or that every server supports it. The authenticated live read was blocked by
an expired local token; context propagation and errors are covered by fixtures.
An unavailable command/API returns a tool error, never invented live status.
The gateway does not silently fall back to per-Unit or Kubernetes reads.

## Feature Parity Between Forms

Every feature must have parity across standalone and plugin forms. This is a non-negotiable release gate item. Parity is enforced by `TestPluginParity_StandaloneMatchesPlugin` in `cmd/cub-scout/plugin_parity_test.go`, which builds the binary once, stages it under both invocation names, runs a representative command set against both, and diffs the output modulo a small documented set of benign differences.

| Area | Parity guaranteed | Notes |
|---|---|---|
| Command surface | ✅ | Every command that works as `cub-scout <cmd>` works as `cub scout <cmd>`. |
| Flags and arguments | ✅ | Unchanged. |
| JSON contracts | ✅ | `--json` output is byte-identical for the same input and cluster state, modulo timestamps which the parity test normalizes. |
| Exit codes | ✅ | Unchanged. Includes `--fail-on` thresholds for `compare three-way`. |
| MCP tool set | ✅ | Standalone: `doctor`, `explain`, `gitops_status`, `map`, `scan`, `trace`. Connected adds `compare_three_way`, `compare_source_truth`, `confighub_changesets`, `confighub_k8s_resources`, `confighub_k8s_types`, `confighub_live_status`, `confighub_releases`, `confighub_resources`, `confighub_unit_events`, `confighub_units`, `confighub_unit_get`. |
| ASCII output | ✅ | Tables, colors (with `NO_COLOR` opt-out), presentation modes. |
| Help text token form | ⚠️ Cosmetic difference | Plugin form renders `cub scout <cmd>`; standalone form renders `cub-scout <cmd>`. Controlled by `CUB_PLUGIN=1` detection and a plugin-mode usage template. |
| Help flag description | ⚠️ Cosmetic difference | Plugin form shows `-h, --help   help for scout`; standalone form shows `help for cub-scout`. Cobra derives the wording from the root command's `Name()`, which the plugin-mode template leaves as `scout` to keep subcommand `CommandPath` walks correct. |
| Auth source | ⚠️ Intentional difference | Plugin form prefers `CUB_TOKEN`/`CUB_SERVER` from the parent `cub` process. Standalone form falls back to `cub auth get-token` or its own token store. |

Cosmetic and auth-source differences are by design. Substantive feature parity is enforced by the release-gate parity test; any new command that accidentally diverges will fail CI on the first run.

## Platform Support

Standalone builds support Linux, macOS, and Windows on amd64 and arm64. Plugin
builds support Linux and macOS on amd64 and arm64. Archives are named with OS and
architecture tokens to match `cub`'s asset matcher:

| OS | Arch | Archive suffix example |
|---|---|---|
| darwin | arm64 | `cub-scout_2.9.0_darwin_arm64.tar.gz` |
| darwin | amd64 | `cub-scout_2.9.0_darwin_amd64.tar.gz` |
| linux | amd64 | `cub-scout_2.9.0_linux_amd64.tar.gz` |
| linux | arm64 | `cub-scout_2.9.0_linux_arm64.tar.gz` |
| windows | amd64 / arm64 | `cub-scout_2.9.0_windows_amd64.zip` / `cub-scout_2.9.0_windows_arm64.zip` |

`cub plugin install` accepts `darwin`/`macos` and `amd64`/`x86_64` and `arm64`/`aarch64` as OS and architecture aliases in filenames.

Current Windows archives do not contain the plugin `main` binary. Use the
standalone or kubectl entry point on Windows.

## Distribution Channels

| Channel | Standalone | Plugin | Notes |
|---|---|---|---|
| `cub plugin install confighub/cub-scout` | ❌ | ✅ | Preferred path for plugin form. |
| Homebrew (`brew install confighub/tap/cub-scout`) | ✅ | ❌ | Standalone binary, not a plugin. |
| krew (`kubectl krew install cub-scout`) | ✅ (as `kubectl cub-scout`) | ❌ | kubectl plugin form; not a `cub` plugin. |
| GitHub release tar.gz | ✅ | ✅ | Linux/macOS archives include both standalone and plugin entry points. |
| Source build (`go build ./cmd/cub-scout`) | ✅ | ⚠️ | Local build works as standalone. Using a local build as a plugin requires manually copying the binary to `$CUB_CONFIG/plugins/scout/main`. |

## Known Limitations

- Plugin form does not currently override `cub scout version` display with the host `cub` version. `cub scout version` reports the plugin's own version. This is intentional — it is the authoritative version for the explorer surface.
- `cub plugin list` shows plugins by filesystem name only. The reported plugin name is `scout` because `cub`'s installer strips the `cub-` prefix from `cub-scout`. This is not a cub-scout-side bug.
- If standalone `cub-scout` and plugin `cub scout` are different versions on the same machine, drift is possible between the two invocation paths. Use `cub-scout version` and `cub scout version` to audit.

## Support Policy

During the `v2.x` line:

- Standalone `cub-scout` remains fully supported for bug fixes and features at parity with plugin form.
- Plugin form is the preferred form in docs, prompts, and AI guidance.
- Security patches apply to both forms in the same release.
- Breaking changes to JSON contracts, exit codes, or MCP tool names require a major version bump that lands in both forms simultaneously.

A decision to sunset standalone `cub-scout` has not been made. Any such decision will ship with a deprecation window of at least one minor release.

## See Also

- [`plugin-install.md`](../howto/plugin-install.md) — how to install the `cub scout` plugin
- [`v2.0.0-migration-guide.md`](../releases/v2.0.0-migration-guide.md) — step-by-step migration
- [`cub vs cub scout`](../concepts/cub-vs-cub-scout.md) — product boundary
- [`v2.0.0-plugin-plan.md`](../releases/v2.0.0-plugin-plan.md) — milestone plan
- [`cli-contract.md`](cli-contract.md) — stable flags, exit codes, and schemas
