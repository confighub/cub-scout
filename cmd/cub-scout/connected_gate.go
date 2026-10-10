// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/confighub/cub-scout/v2/internal/hubread"
	"github.com/confighub/cub-scout/v2/pkg/hub"
)

// requireCubConnectedFn is the gate's seam for tests.
var requireCubConnectedFn = requireConfigHubSession

// sessionCheckTimeout bounds the gate's one request on the SDK route.
const sessionCheckTimeout = 30 * time.Second

// sessionRefusal is a refusal of the gate on the SDK route. It answers to the
// same sentinel a `cub auth status` refusal of that kind does, so callers
// that ask "is this about the session?" or "is this a version problem?" get
// the same answer by either route; its text does not mention a `cub` check
// that was not run.
type sessionRefusal struct {
	kind error
	text string
}

func (e *sessionRefusal) Error() string        { return e.text }
func (e *sessionRefusal) Is(target error) bool { return target == e.kind }

// requireConfigHubSession is the gate's check: can ConfigHub be read with the
// session this machine has?
//
// By the `cub` route that is `cub auth status`. By the SDK route (#758) it is
// the same check made without `cub`: the credential `cub` would use, a token
// that has not expired, one request the server answers, and a client that is
// not older than the server. A machine with no `cub` binary passes it; a read
// that still needs `cub` then says so itself.
func requireConfigHubSession() error {
	route, err := configHubReaderRoute()
	if err != nil {
		return err
	}
	if route != "sdk" {
		return hub.RequireCubConnected()
	}
	if err := configHubReadsDisabledFn(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), sessionCheckTimeout)
	defer cancel()
	reader, err := sdkReader(ctx)
	if err == nil {
		var session hubread.Session
		if session, err = reader.CheckSession(ctx); err == nil {
			if session.ClientAhead {
				warnClientAhead(session)
			}
			return nil
		}
	}
	switch hubread.KindOf(err) {
	case hubread.KindNotConfigured, hubread.KindUnauthorized:
		return &sessionRefusal{kind: hub.ErrCubNotAuthenticated, text: "no ConfigHub session this machine can use: " + err.Error()}
	case hubread.KindVersionSkew:
		return &sessionRefusal{kind: hub.ErrCubVersionSkew, text: err.Error() + ", or set " + configHubReaderEnv + "=cub to read through an up-to-date `cub`"}
	default:
		// The server could not be asked, or did not answer as itself. That
		// says nothing about the session either way.
		return fmt.Errorf("the ConfigHub session could not be checked: %w", err)
	}
}

var clientAheadWarned sync.Once

// warnClientAhead says once, on stderr, that this binary's SDK is newer than
// the server, as cub warns about a cub newer than its server.
func warnClientAhead(session hubread.Session) {
	clientAheadWarned.Do(func() {
		fmt.Fprintf(os.Stderr, "Warning: this cub-scout reads ConfigHub with SDK %s, which is newer than server %s; below 1.0 a change in the second version number is not backward compatible, so some reads may fail.\n",
			session.SDKVersion, session.ServerVersion)
	})
}

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
