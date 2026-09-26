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
		{
			// Recorded 2026-09-26: cub v0.5.7 against a v0.6.5 server, with a
			// session the server accepted (stdout said "Status Authenticated").
			name:       "cub is older than the server",
			detail:     firstLine(recordedCubTooOldStderr),
			err:        exit1,
			want:       ErrCubVersionSkew,
			wantDetail: "cub v0.5.7 is too old for server v0.6.5",
		},
		{
			// The same refusal from a cub built from a working tree, which
			// cub words differently after the colon (cmd/cub/version_check.go).
			name:       "a development cub is older than the server",
			detail:     "cub v0.6.0-dev is too old for server v0.6.5: pre-1.0, a change in the second version number is not backward compatible. This cub was built from a working tree. Check out a tree at the server's version and rebuild it",
			err:        exit1,
			want:       ErrCubVersionSkew,
			wantDetail: "built from a working tree",
		},
		{
			// Only the start of the message counts: a refusal that mentions a
			// version later on is still about authentication.
			name:       "a version mentioned inside another refusal",
			detail:     "not authenticated: server https://hub.example.com rejected the access token (401 Unauthorized). cub v0.5.7 is too old for server v0.6.5: quoted",
			err:        exit1,
			want:       ErrCubNotAuthenticated,
			wantDetail: "401 Unauthorized",
		},
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

// recordedCubTooOldStderr is what `cub auth status` wrote to stderr, exit 1,
// for cub v0.5.7 against a v0.6.5 server on 2026-09-26.
const recordedCubTooOldStderr = "Failed: cub v0.5.7 is too old for server v0.6.5: pre-1.0, a change in the second version number is not backward compatible. Run 'cub upgrade' to update cub\n"

// A cub older than its server is not an authentication problem: callers that
// advise `cub auth login` on ErrCubNotAuthenticated must not see it, and the
// refusal must name the fix.
func TestRequireCubConnected_VersionSkewIsNotAnAuthRefusal(t *testing.T) {
	stubCubAuthStatus(t, firstLine(recordedCubTooOldStderr), errors.New("exit status 1"))
	got := RequireCubConnected()
	if errors.Is(got, ErrCubNotAuthenticated) {
		t.Fatalf("RequireCubConnected() = %v; a version refusal must not read as an authentication refusal", got)
	}
	for _, want := range []string{"cub upgrade", "older than the ConfigHub server"} {
		if !strings.Contains(got.Error(), want) {
			t.Fatalf("RequireCubConnected() = %q, want it to name the fix %q", got, want)
		}
	}
}

// Recorded 2026-09-26: cub v0.6.2 against a v0.5.1 server exits 0 and only
// warns on stderr, so reads go ahead and the warning is not a refusal.
func TestRequireCubConnected_NewerCubOnlyWarns(t *testing.T) {
	stubCubAuthStatus(t, "Warning: cub v0.6.2 is newer than server v0.5.1: pre-1.0, a change in the second version number is not backward compatible, so some commands may fail. Ask the server's operator to upgrade ConfigHub.", nil)
	if got := RequireCubConnected(); got != nil {
		t.Fatalf("RequireCubConnected() = %v, want nil: cub exits 0 for a client newer than its server", got)
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
