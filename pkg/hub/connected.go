// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hub

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Reasons RequireCubConnected refuses. Each names a cause the user can act on;
// callers wrap them with the command that was refused.
var (
	// ErrConfigHubReadsDisabled means the user turned network features off.
	ErrConfigHubReadsDisabled = errors.New("ConfigHub reads are turned off (CUB_SCOUT_OFFLINE=true, or telemetry is disabled)")
	// ErrCubNotInstalled means there is no `cub` binary to run the reads with.
	ErrCubNotInstalled = errors.New("the `cub` CLI is not on PATH; install it, then run `cub auth login`")
	// ErrCubNotLoggedIn means `cub` has no usable token (none, or expired).
	ErrCubNotLoggedIn = errors.New("the `cub` CLI is not logged in, or its session has expired; run `cub auth login`")
)

// Seams for tests: no real `cub` is needed to exercise the gate.
var (
	cubLookPath = func() error {
		_, err := exec.LookPath("cub")
		return err
	}
	cubAuthToken = func() (string, error) {
		out, err := exec.Command("cub", "auth", "get-token").Output()
		return strings.TrimSpace(string(out)), err
	}
)

// RequireCubConnected reports whether ConfigHub reads that go through the
// `cub` CLI can run. It is the gate for connected-only commands.
//
// Those commands only need a usable ConfigHub credential: the token the `cub`
// plugin host passed in, cub-scout's own auth.json, or a logged-in `cub`.
// QuickMode is not that check: it is a display helper that never consults
// `cub`, so it refuses a logged-in user in standalone form. CurrentMode is not
// it either: it probes hub.confighub.com, which says nothing about whether
// `cub` can reach its own server and fails for self-hosted or air-gapped ones.
//
// The standalone and plugin forms reach the same decision for the same
// credential state.
func RequireCubConnected() error {
	if os.Getenv("CUB_SCOUT_OFFLINE") == "true" || telemetryDisabled() {
		return ErrConfigHubReadsDisabled
	}
	if IsAuthenticated() {
		return nil
	}
	if IsPluginMode() {
		// The host owns the credential and passed none. Re-executing `cub`
		// from inside its own plugin would recurse through the host.
		return ErrCubNotLoggedIn
	}
	if err := cubLookPath(); err != nil {
		return ErrCubNotInstalled
	}
	if token, err := cubAuthToken(); err != nil || token == "" {
		return ErrCubNotLoggedIn
	}
	return nil
}
