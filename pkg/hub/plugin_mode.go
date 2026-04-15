// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hub

import (
	"os"
	"strings"
)

// Environment variables set by the `cub` host when it execs a plugin.
// See sdk/cmd/cub/plugin_exec.go for the authoritative source list.
const (
	envCubPlugin  = "CUB_PLUGIN"
	envCubToken   = "CUB_TOKEN"
	envCubServer  = "CUB_SERVER"
	envCubContext = "CUB_CONTEXT"
	envCubSpace   = "CUB_SPACE"
	envCubConfig  = "CUB_CONFIG"
)

// Shell-out audit (as of the v2.0 plugin switchover):
//
// cub-scout invokes the `cub` CLI ~73 times across cmd/ and pkg/ to delegate
// ConfigHub read queries (unit list/get, space list, context get, target
// list, worker list, gitops discover/import, link list, auth status,
// auth get-token). Every call shells out to whatever `cub` is on PATH and
// expects JSON back.
//
// Recursion risk analysis:
//
//   * None of the 73 call sites invoke a cub PLUGIN subcommand (`cub scout
//     ...` or any other installed plugin). They invoke cub built-ins only
//     (`unit`, `context`, `space`, `worker`, `target`, `auth`, `gitops`,
//     `link`, `--version`). The child `cub` process handles these in-proc
//     and does not re-exec back into cub-scout.
//
//   * The child cub process inherits CUB_PLUGIN=1 in its environment when
//     the parent cub-scout is itself running as a plugin. This is benign:
//     `cub` itself never reads CUB_PLUGIN, it only SETS it when dispatching
//     to a plugin. So even if a child cub does eventually exec a plugin,
//     that plugin will see CUB_PLUGIN=1 which is already the correct value.
//
//   * The two call sites that WOULD have been hot-path recursion problems
//     are pkg/hub/auth.go::CubCLIAuthenticated and
//     cmd/cub-scout/status.go::validateAuthToken. Both shell out to
//     `cub auth get-token` to check whether the host is authenticated.
//     In plugin mode these would fire on every CurrentMode() call, fork a
//     fresh cub process each time, and redundantly resolve a token we
//     already have via CUB_TOKEN. Both are short-circuited to read
//     PluginToken() when IsPluginMode() is true — see auth.go and status.go.
//
// Conclusion: no further shell-out call sites need plugin-mode awareness
// for correctness. The remaining sites are correct as-is and behave
// identically in standalone and plugin forms, which is what the v2.0
// release-gate parity test enforces.

// IsPluginMode reports whether this process was exec'd by `cub` as a plugin.
// `cub` sets CUB_PLUGIN=1 before calling syscall.Exec on the plugin binary.
//
// When this is true:
//   - auth should prefer CUB_TOKEN from the environment rather than shelling
//     out to `cub auth get-token` (which would recurse into the parent host)
//   - CLI help text should render `cub scout` instead of the binary's own name
//   - the plugin inherits CUB_CONTEXT, CUB_SERVER, CUB_SPACE from the host
func IsPluginMode() bool {
	return strings.TrimSpace(os.Getenv(envCubPlugin)) != ""
}

// PluginToken returns the ConfigHub access token provided by the host `cub`
// process via the CUB_TOKEN environment variable.
//
// Returns "" if CUB_TOKEN is not set. Callers should fall back to their own
// token store in that case.
func PluginToken() string {
	return strings.TrimSpace(os.Getenv(envCubToken))
}

// PluginServer returns the ConfigHub server URL provided by the host `cub`
// process via the CUB_SERVER environment variable.
func PluginServer() string {
	return strings.TrimSpace(os.Getenv(envCubServer))
}

// PluginContext returns the active `cub` context name provided by the host
// via the CUB_CONTEXT environment variable.
func PluginContext() string {
	return strings.TrimSpace(os.Getenv(envCubContext))
}

// PluginSpace returns the default ConfigHub space provided by the host
// via the CUB_SPACE environment variable.
func PluginSpace() string {
	return strings.TrimSpace(os.Getenv(envCubSpace))
}
