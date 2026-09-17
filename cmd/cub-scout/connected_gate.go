// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

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
	if err := requireCubConnectedFn(); err != nil {
		return fmt.Errorf("%s needs ConfigHub: %w", feature, err)
	}
	return nil
}
