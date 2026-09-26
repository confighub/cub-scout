// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hub

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Reasons RequireCubConnected refuses. Callers wrap them with the command that
// was refused; the returned error also carries the specific cause.
var (
	// ErrConfigHubReadsDisabled means the user turned network features off.
	ErrConfigHubReadsDisabled = errors.New("ConfigHub reads are turned off")
	// ErrCubNotInstalled means there is no `cub` binary to run the reads with.
	ErrCubNotInstalled = errors.New("the `cub` CLI is not on PATH; install it, then run `cub auth login`")
	// ErrCubNotAuthenticated means `cub auth status` did not report a usable
	// session: no token, an expired one, or a context cub cannot resolve.
	ErrCubNotAuthenticated = errors.New("`cub auth status` did not report an authenticated session")
	// ErrCubVersionSkew means `cub auth status` accepted the session but
	// refused because this cub is older than the server. Below 1.0 a change in
	// cub's second version number is not backward compatible, so cub treats an
	// older client as unusable. The fix is upgrading cub, not logging in again.
	ErrCubVersionSkew = errors.New("`cub` is older than the ConfigHub server; run `cub upgrade`")
)

// cubTooOldForServer matches the refusal `cub auth status` (cub v0.5.3 and
// later) prints for a client older than its server, for example
// "cub v0.5.7 is too old for server v0.6.5: pre-1.0, ...". cub offers no
// machine-readable form of this, so the match is anchored to the start of the
// message: an unrecognised message stays an authentication refusal rather
// than being guessed into a version problem.
var cubTooOldForServer = regexp.MustCompile(`^cub \S+ is too old for server \S+:`)

// cubAuthStatus runs `cub auth status`, which exits 0 only for a session cub
// itself considers usable. On failure it returns cub's own explanation.
//
// `cub auth get-token` is not that check: it prints a stored token and exits 0
// even after the token has expired.
//
// It is a variable so tests need no real `cub`.
var cubAuthStatus = func() (detail string, err error) {
	var stderr bytes.Buffer
	cmd := exec.Command("cub", "auth", "status")
	cmd.Stderr = &stderr
	err = cmd.Run()
	return firstLine(stderr.String()), err
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(strings.TrimPrefix(text, "Failed:"))
}

// RequireCubConnected reports whether ConfigHub reads that go through the
// `cub` CLI can run. It is the gate for commands that read ConfigHub only by
// running `cub`.
//
// Such a command needs exactly two things: a `cub` binary, and a session that
// `cub` itself accepts. So the check is `cub auth status`, run the same way in
// the standalone and plugin forms, which therefore always agree.
//
// It is deliberately none of the older checks:
//   - QuickMode is a display helper that never consults `cub`.
//   - CurrentMode probes hub.confighub.com, which says nothing about whether
//     `cub` can reach its own server and fails for a self-hosted or air-gapped
//     ConfigHub.
//   - cub-scout's own auth.json is not a credential `cub` uses, so it cannot
//     show that a `cub` read will succeed.
func RequireCubConnected() error {
	if os.Getenv("CUB_SCOUT_OFFLINE") == "true" {
		return fmt.Errorf("%w: CUB_SCOUT_OFFLINE=true is set; unset it to use connected commands", ErrConfigHubReadsDisabled)
	}
	if telemetryDisabled() {
		return fmt.Errorf("%w: telemetry is disabled (CUB_SCOUT_TELEMETRY=false, or %s exists), and that also turns off ConfigHub reads", ErrConfigHubReadsDisabled, telemetryConfigPath())
	}
	detail, err := cubAuthStatus()
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return ErrCubNotInstalled
	}
	if detail == "" {
		detail = err.Error()
	}
	if cubTooOldForServer.MatchString(detail) {
		return fmt.Errorf("%w: %s", ErrCubVersionSkew, detail)
	}
	return fmt.Errorf("%w: %s", ErrCubNotAuthenticated, detail)
}

// CubSessionValid reports whether `cub` has a session it considers usable.
// Unlike CubCLIAuthenticated it notices an expired token; it costs one `cub`
// process, so it is not for hot paths.
func CubSessionValid() bool {
	_, err := cubAuthStatus()
	return err == nil
}
