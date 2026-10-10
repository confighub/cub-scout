// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/confighub/cub-scout/v2/internal/hubread"
)

// errSDKRouteInATest is returned when a test reaches the SDK route without
// saying which server to read.
var errSDKRouteInATest = errors.New("a test reached the SDK route without a test server; use sdkReadsFrom, or set " + configHubReaderEnv + "=cub")

// TestMain keeps every test in this package away from real ConfigHub
// credentials.
//
// The SDK route is the default, and it resolves the credential cub would use:
// on a developer's machine, a real login, possibly to production. Many tests
// here put a fake cub on PATH and expect it to be run, and some run commands
// that read ConfigHub on the way. So the test binary reads through cub unless
// a test asks for the SDK route, and the SDK route's credential resolver is
// replaced with one that refuses. A test that wants the SDK route supplies its
// own server with sdkReadsFrom, or restores resolveSDKReader after isolating
// HOME and CUB_CONFIG.
func TestMain(m *testing.M) {
	if os.Getenv(configHubReaderEnv) == "" {
		os.Setenv(configHubReaderEnv, "cub")
	}
	sdkReader = func(context.Context) (*hubread.Reader, error) { return nil, errSDKRouteInATest }
	os.Exit(m.Run())
}
