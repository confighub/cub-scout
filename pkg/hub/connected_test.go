// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hub

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stubCubAuthStatus replaces `cub auth status` for one test and isolates the
// environment RequireCubConnected reads.
func stubCubAuthStatus(t *testing.T, detail string, err error) *int {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	t.Setenv("CUB_PLUGIN", "")
	t.Setenv("CUB_TOKEN", "")

	calls := 0
	old := cubAuthStatus
	t.Cleanup(func() { cubAuthStatus = old })
	cubAuthStatus = func() (string, error) {
		calls++
		return detail, err
	}
	return &calls
}

func TestRequireCubConnected(t *testing.T) {
	notFound := &exec.Error{Name: "cub", Err: exec.ErrNotFound}
	exit1 := errors.New("exit status 1")

	tests := []struct {
		name       string
		detail     string
		err        error
		want       error
		wantDetail string
	}{
		{name: "cub reports an authenticated session"},
		{name: "cub is not installed", err: notFound, want: ErrCubNotInstalled},
		{
			name:       "cub has no token",
			detail:     `not authenticated: no access token found for context "demo". Ask the user to run 'cub auth login' to re-authenticate.`,
			err:        exit1,
			want:       ErrCubNotAuthenticated,
			wantDetail: "cub auth login",
		},
		{
			// `cub auth get-token` exits 0 here; `cub auth status` does not.
			name:       "cub's token has expired",
			detail:     "not authenticated: access token expired at 2026-07-30T09:50:02+01:00. Ask the user to run 'cub auth login' to re-authenticate.",
			err:        exit1,
			want:       ErrCubNotAuthenticated,
			wantDetail: "access token expired",
		},
		{
			// Not a login problem at all: the cause must survive, not be
			// replaced by advice to log in.
			name:       "cub cannot resolve its context",
			detail:     `CUB_CONTEXT environment variable: context "kind-demo" not found`,
			err:        exit1,
			want:       ErrCubNotAuthenticated,
			wantDetail: `context "kind-demo" not found`,
		},
		{name: "cub fails without saying why", err: exit1, want: ErrCubNotAuthenticated, wantDetail: "exit status 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubCubAuthStatus(t, tt.detail, tt.err)
			got := RequireCubConnected()
			if !errors.Is(got, tt.want) {
				t.Fatalf("RequireCubConnected() = %v, want %v", got, tt.want)
			}
			if tt.wantDetail != "" && !strings.Contains(got.Error(), tt.wantDetail) {
				t.Fatalf("RequireCubConnected() = %q, want it to carry cub's own reason %q", got, tt.wantDetail)
			}
		})
	}
}

// Turning reads off wins over a valid session, names which switch did it, and
// does not run cub at all.
func TestRequireCubConnected_ReadsTurnedOff(t *testing.T) {
	t.Run("CUB_SCOUT_OFFLINE", func(t *testing.T) {
		calls := stubCubAuthStatus(t, "", nil)
		t.Setenv("CUB_SCOUT_OFFLINE", "true")
		err := RequireCubConnected()
		if !errors.Is(err, ErrConfigHubReadsDisabled) || !strings.Contains(err.Error(), "CUB_SCOUT_OFFLINE=true") {
			t.Fatalf("error = %v, want reads disabled naming CUB_SCOUT_OFFLINE", err)
		}
		if *calls != 0 {
			t.Fatalf("cub ran %d times, want 0", *calls)
		}
	})
	t.Run("telemetry env", func(t *testing.T) {
		calls := stubCubAuthStatus(t, "", nil)
		t.Setenv("CUB_SCOUT_TELEMETRY", "false")
		err := RequireCubConnected()
		if !errors.Is(err, ErrConfigHubReadsDisabled) || !strings.Contains(err.Error(), "CUB_SCOUT_TELEMETRY=false") {
			t.Fatalf("error = %v, want reads disabled naming CUB_SCOUT_TELEMETRY", err)
		}
		if *calls != 0 {
			t.Fatalf("cub ran %d times, want 0", *calls)
		}
	})
	t.Run("telemetry file", func(t *testing.T) {
		stubCubAuthStatus(t, "", nil)
		path := telemetryConfigPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		err := RequireCubConnected()
		if !errors.Is(err, ErrConfigHubReadsDisabled) || !strings.Contains(err.Error(), path) {
			t.Fatalf("error = %v, want reads disabled naming %s", err, path)
		}
	})
}

// cub-scout's own auth.json is not a credential cub uses. Every gated command
// reads ConfigHub by running cub, so the file must not admit a user whose cub
// is missing or logged out.
func TestRequireCubConnected_AuthJSONIsNotACubCredential(t *testing.T) {
	stubCubAuthStatus(t, "", &exec.Error{Name: "cub", Err: exec.ErrNotFound})
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".cub-scout")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"token":"leftover"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !IsAuthenticated() {
		t.Fatal("precondition: auth.json should make IsAuthenticated true")
	}
	if got := RequireCubConnected(); !errors.Is(got, ErrCubNotInstalled) {
		t.Fatalf("RequireCubConnected() = %v, want %v despite auth.json", got, ErrCubNotInstalled)
	}
}

// The standalone form (`cub-scout ...`) and the plugin form (`cub scout ...`)
// run the same check, so they cannot disagree, with or without a host token.
func TestRequireCubConnected_StandaloneAndPluginFormsAgree(t *testing.T) {
	for _, state := range []struct {
		name string
		err  error
	}{
		{name: "authenticated"},
		{name: "not authenticated", err: errors.New("exit status 1")},
		{name: "cub missing", err: &exec.Error{Name: "cub", Err: exec.ErrNotFound}},
	} {
		t.Run(state.name, func(t *testing.T) {
			var got []string
			for _, form := range []struct{ plugin, token string }{{"", ""}, {"1", "host-token"}, {"1", ""}} {
				stubCubAuthStatus(t, "", state.err)
				t.Setenv("CUB_PLUGIN", form.plugin)
				t.Setenv("CUB_TOKEN", form.token)
				got = append(got, fmt.Sprint(RequireCubConnected()))
			}
			if got[0] != got[1] || got[1] != got[2] {
				t.Fatalf("standalone = %q, plugin with token = %q, plugin without = %q; want all equal", got[0], got[1], got[2])
			}
		})
	}
}

func TestCubSessionValid(t *testing.T) {
	stubCubAuthStatus(t, "", nil)
	if !CubSessionValid() {
		t.Fatal("CubSessionValid() = false for an authenticated session")
	}
	stubCubAuthStatus(t, "not authenticated: access token expired", errors.New("exit status 1"))
	if CubSessionValid() {
		t.Fatal("CubSessionValid() = true for an expired session")
	}
}

func TestFirstLine(t *testing.T) {
	for in, want := range map[string]string{
		"Failed: not authenticated: expired\nmore detail\n": "not authenticated: expired",
		"  plain message  ": "plain message",
		"":                  "",
	} {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}
