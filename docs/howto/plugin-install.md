# Install `cub scout` as a `cub` Plugin

> **Status:** Install guide for the `v2.0.0` plugin form. In-binary plugin support (auth inheritance, cobra help/command-path adjustments, release-gate parity test) has landed on `main`. The step that produces a plugin-compatible release archive is configured in `.goreleaser.yaml` and ships with the first release cut from `main` after `v1.13.x`.
> The `cub plugin install` mechanism requires `cub` with plugin support and a `cub-scout` release that ships a plugin-compatible archive.
> If you only want standalone `cub-scout`, see the top-level [README](../../README.md) instead.
>
> **Related:** [`v2.0.0-migration-guide.md`](../releases/v2.0.0-migration-guide.md), [`cub vs cub scout`](../concepts/cub-vs-cub-scout.md), [`host-plugin-compatibility.md`](../reference/host-plugin-compatibility.md).

## What You Get

After installation, you can run:

```bash
cub scout doctor
cub scout explain deploy/frontend -n default
cub scout compare three-way --scope deploy/frontend -n default
cub scout mcp serve
```

The plugin form is the same binary as standalone `cub-scout`. It inherits `cub`'s active auth context automatically, so you do not need a separate login for connected features.

## Prerequisites

- `cub` installed with plugin support. Verify with:
  ```bash
  cub plugin --help
  ```
  If the command is missing, upgrade `cub` first. See the [`confighub/sdk`](https://github.com/confighub/sdk) release notes.
- A `kubeconfig` pointing at the cluster you want to observe (`cub scout` remains read-only by default).
- For connected features: `cub auth login` completed, or `CUB_TOKEN` and `CUB_SERVER` exported.

## Quick Install

```bash
cub plugin install confighub/cub-scout
```

This will:

1. Fetch the latest `cub-scout` release from GitHub.
2. Match a release archive for your OS and architecture (`darwin`/`macos`, `linux`, `amd64`/`x86_64`, `arm64`/`aarch64`).
3. Download the archive to a temporary directory.
4. Extract it to `$CUB_CONFIG/plugins/scout/`.
5. Make the main executable runnable.

Verify:

```bash
cub plugin list
cub scout version
cub scout doctor --help
```

Expected `cub plugin list` output contains a row with `NAME=scout`.

## Pinned Version

To pin a specific release instead of tracking latest:

```bash
cub plugin install confighub/cub-scout@v2.0.0
```

Use this for CI pipelines and reproducible environments. See the compatibility matrix for which `cub` versions are supported alongside which `cub-scout` versions.

## Install From a Direct URL

If your environment cannot reach GitHub, you can install from any HTTPS tar.gz or raw binary URL:

```bash
cub plugin install https://example.com/path/cub-scout_v2.0.0_linux_amd64.tar.gz
cub plugin install https://example.com/path/cub-scout_v2.0.0_linux_amd64
```

`cub` will derive the plugin name from the URL unless you pass `--name`:

```bash
cub plugin install --name scout https://example.com/custom-build.tar.gz
```

## Overwrite an Existing Install

```bash
cub plugin install --force confighub/cub-scout
```

Use `--force` when upgrading or when switching between tagged versions.

## Uninstall

```bash
cub plugin uninstall scout
```

This removes `$CUB_CONFIG/plugins/scout/`. Your standalone `cub-scout` install (if any) is untouched.

## Where The Plugin Lives

The plugin directory is derived from `cub`'s config path:

```bash
cub plugin list
```

will show the path under the `PATH` column. On typical systems that is `$HOME/.confighub/plugins/scout/`.

The plugin binary is `$HOME/.confighub/plugins/scout/main` (or a single executable file named `scout` for single-file plugin layouts).

## How `cub` Invokes the Plugin

When you run `cub scout doctor`, `cub` does the following:

1. Parses `scout` as a non-built-in command.
2. Looks for `$CUB_CONFIG/plugins/scout` — either a single executable or a directory with an executable named `main`.
3. Sets environment variables for the plugin:
   - `CUB_PLUGIN=1`
   - `CUB_CONFIG=<cub config dir>`
   - `CUB_CONTEXT=<active cub context>`
   - `CUB_SERVER=<active context server URL>`
   - `CUB_SPACE=<default space, if set>`
   - `CUB_TOKEN=<access token, if available>`
4. Replaces the `cub` process with the plugin binary via `syscall.Exec`, passing `["doctor"]` as arguments.

The plugin binary receives the same stdin, stdout, stderr, and environment as the original `cub` invocation. Exit codes pass through unchanged.

## Auth Inheritance

Connected `cub scout` commands (`compare three-way`, MCP connected tools, `history`) need a ConfigHub access token.

In plugin form, the plugin reads `CUB_TOKEN` and `CUB_SERVER` directly from the environment that `cub` sets. No separate `cub-scout` auth step is needed.

In standalone form, `cub-scout` falls back to `cub auth get-token` or its own token store.

## Troubleshooting

### `unknown command "scout" for "cub"`

The plugin was not found on `$CUB_CONFIG/plugins/scout`. Check:

```bash
cub plugin list
ls -la "$(cub plugin list -o json | jq -r '.[] | select(.name=="scout") | .path')"
```

If `cub plugin list` does not show `scout`, reinstall with `cub plugin install confighub/cub-scout`.

### `directory plugin missing executable 'main'` or `'main' is not executable`

The archive was extracted but the main entry point is missing or not executable. This usually means you installed an archive that does not match the `v2.0.0+` plugin layout. Upgrade to a `v2.0.0` or newer release and reinstall with `--force`.

### `no release asset found for <os>/<arch>`

`cub` could not find an asset whose filename contains both your OS and architecture tokens. Check the release page on GitHub and confirm an archive like `cub-scout_<ver>_<os>_<arch>.tar.gz` exists. File a `cub-scout` release bug if not.

### `GitHub API rate limit exceeded`

Export a GitHub token:

```bash
export GITHUB_TOKEN=ghp_...
cub plugin install confighub/cub-scout
```

### Plugin prints `cub-scout` in help output instead of `cub scout`

Cosmetic. The plugin binary uses its own name in cobra `Use:` strings. `v2.0.0` detects `CUB_PLUGIN=1` and switches help text to `cub scout`. If you see the legacy form, you have an older `cub-scout` build; upgrade with `cub plugin install --force confighub/cub-scout`.

### Connected commands say "not logged in" in plugin form

The plugin is not seeing `CUB_TOKEN`. Verify:

```bash
cub auth status
cub scout doctor
```

If `cub auth status` is healthy but the plugin says otherwise, upgrade to a `cub-scout` release that reads `CUB_TOKEN` from the environment.

## Coexistence With Standalone `cub-scout`

You can keep standalone `cub-scout` on your `$PATH` alongside the plugin install. They do not conflict:

- `cub-scout doctor` → runs the standalone binary from `$PATH`
- `cub scout doctor` → runs the plugin binary from `$CUB_CONFIG/plugins/scout/`

The two may be different versions if you upgrade one without the other. For parity verification, use:

```bash
cub-scout version
cub scout version
```

## See Also

- [`v2.0.0-migration-guide.md`](../releases/v2.0.0-migration-guide.md) — full migration from standalone to plugin form
- [`cub vs cub scout`](../concepts/cub-vs-cub-scout.md) — when to use which tool
- [`host-plugin-compatibility.md`](../reference/host-plugin-compatibility.md) — which `cub` and `cub-scout` versions are supported together
- [`v2.0.0-plugin-plan.md`](../releases/v2.0.0-plugin-plan.md) — plugin switchover milestone plan
