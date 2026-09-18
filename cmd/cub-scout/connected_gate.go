// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"sync"

	"github.com/confighub/cub-scout/pkg/hub"
)

// requireCubConnectedFn is the gate's seam for tests.
var requireCubConnectedFn = hub.RequireCubConnected

// errConfigHubUnavailable marks every refusal the gate produces, whatever the
// cause. A caller that only needs to know "this failed because ConfigHub reads
// are unavailable" tests for this rather than listing the causes, which would
// silently stop matching when a new one is added.
var errConfigHubUnavailable = errors.New("ConfigHub reads are unavailable")

// configHubGateError is the gate's refusal: it names the command, carries the
// specific cause, and also answers to errConfigHubUnavailable.
type configHubGateError struct {
	feature string
	cause   error
}

func (e *configHubGateError) Error() string {
	return fmt.Sprintf("%s needs ConfigHub: %v", e.feature, e.cause)
}

// Unwrap reports both the cause and the marker, so errors.Is finds either.
func (e *configHubGateError) Unwrap() []error {
	return []error{e.cause, errConfigHubUnavailable}
}

// requireConfigHubFor refuses a command that reads ConfigHub by running `cub`
// when `cub` has no session it accepts. The error names the command, carries
// the specific cause, and never offers a remedy that cannot fix it.
//
// Commands call this rather than hub.QuickMode() or hub.IsAuthenticated():
// neither consults the `cub` CLI, so they refuse a logged-in user in standalone
// form while the plugin form passes.
func requireConfigHubFor(feature string) error {
	if err := configHubReads(); err != nil {
		return &configHubGateError{feature: feature, cause: err}
	}
	return nil
}

var (
	configHubReadsMu       sync.Mutex
	configHubReadsAnswered bool
	configHubReadsErr      error
)

// configHubReads is the gate: nil when ConfigHub reads through `cub` can run,
// and otherwise the reason.
//
// The answer is kept because callers ask it per resource, per receipt and per
// poll cycle, and each call runs a `cub` process. A command therefore spends
// one `cub auth status`, and every part of its output agrees about whether it
// was connected.
//
// A command that outlives one answer — `watch`, the TUI — asks again at a
// boundary its user can see, through refreshConfigHubReads. See that function
// for why the kept answer is not enough there.
func configHubReads() error {
	configHubReadsMu.Lock()
	defer configHubReadsMu.Unlock()
	if !configHubReadsAnswered {
		configHubReadsErr = requireCubConnectedFn()
		configHubReadsAnswered = true
	}
	return configHubReadsErr
}

// refreshConfigHubReads asks `cub` again and replaces the kept answer.
//
// A session can expire, or be restored by `cub auth login` in another terminal,
// while a long-running command is up. Without this, a watch would stamp every
// receipt for days with the verdict it read at start-up, and the TUI would
// repeat a refusal the user has already fixed. Each call runs a `cub` process,
// so callers ask once per poll cycle or per keypress, not per event.
func refreshConfigHubReads() error {
	configHubReadsMu.Lock()
	defer configHubReadsMu.Unlock()
	configHubReadsErr = requireCubConnectedFn()
	configHubReadsAnswered = true
	return configHubReadsErr
}

// configHubReadsAvailable is the same answer as a boolean, for the facts that
// record whether an observation was made in connected mode.
func configHubReadsAvailable() bool {
	return configHubReads() == nil
}

// refreshConfigHubReadsAvailable re-asks the gate and returns the new answer as
// a boolean, for a long-running loop that records the fact per cycle.
func refreshConfigHubReadsAvailable() bool {
	return refreshConfigHubReads() == nil
}
