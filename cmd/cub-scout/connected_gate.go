// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"sync"

	"github.com/confighub/cub-scout/pkg/hub"
)

// requireCubConnectedFn is the gate's seam for tests.
var requireCubConnectedFn = hub.RequireCubConnected

// requireConfigHubFor refuses a command that reads ConfigHub by running `cub`
// when `cub` has no session it accepts. The error names the command, carries
// the specific cause, and never offers a remedy that cannot fix it.
//
// Commands call this rather than hub.QuickMode() or hub.IsAuthenticated():
// neither consults the `cub` CLI, so they refuse a logged-in user in standalone
// form while the plugin form passes.
func requireConfigHubFor(feature string) error {
	if err := configHubReads(); err != nil {
		return fmt.Errorf("%s needs ConfigHub: %w", feature, err)
	}
	return nil
}

var (
	configHubReadsOnce sync.Once
	configHubReadsErr  error
)

// configHubReads is the gate, answered once per process: nil when ConfigHub
// reads through `cub` can run, and otherwise the reason.
//
// The answer is memoized because callers ask it per resource, per receipt and
// per poll cycle, and each call runs a `cub` process. One command therefore
// spends one `cub auth status`, and every part of its output agrees about
// whether it was connected.
func configHubReads() error {
	configHubReadsOnce.Do(func() { configHubReadsErr = requireCubConnectedFn() })
	return configHubReadsErr
}

// configHubReadsAvailable is the same answer as a boolean, for the facts that
// record whether an observation was made in connected mode.
func configHubReadsAvailable() bool {
	return configHubReads() == nil
}
