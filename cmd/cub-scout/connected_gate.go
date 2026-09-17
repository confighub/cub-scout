// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"

	"github.com/confighub/cub-scout/pkg/hub"
)

// requireCubConnectedFn is the gate's seam for tests.
var requireCubConnectedFn = hub.RequireCubConnected

// requireConfigHubFor refuses a connected-only command when there is no usable
// ConfigHub credential. The error carries the specific cause and a remedy that
// works, so it never sends the user to a step that cannot fix it.
//
// Connected-only commands call this rather than comparing hub.QuickMode() to
// hub.Connected: QuickMode never consults the `cub` CLI, so that comparison
// refuses a logged-in user in standalone form while the plugin form passes.
func requireConfigHubFor(feature string) error {
	if err := requireCubConnectedFn(); err != nil {
		return fmt.Errorf("%s needs ConfigHub: %w", feature, err)
	}
	return nil
}
