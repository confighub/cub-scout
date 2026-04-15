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
