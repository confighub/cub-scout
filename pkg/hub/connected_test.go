// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hub

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// credentialState describes everything RequireCubConnected looks at.
type credentialState struct {
	offline      bool
	telemetryOff bool
	plugin       bool
	pluginToken  string
	authJSON     bool
	cubInstalled bool
	cubToken     string
	cubTokenErr  error
}

func applyCredentialState(t *testing.T, s credentialState) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	t.Setenv("CUB_PLUGIN", "")
	t.Setenv("CUB_TOKEN", "")
	if s.offline {
		t.Setenv("CUB_SCOUT_OFFLINE", "true")
	}
	if s.telemetryOff {
		t.Setenv("CUB_SCOUT_TELEMETRY", "false")
	}
	if s.plugin {
		t.Setenv("CUB_PLUGIN", "1")
		t.Setenv("CUB_TOKEN", s.pluginToken)
	}
	if s.authJSON {
		dir := filepath.Join(home, ".cub-scout")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"token":"local-token"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	oldLook, oldToken := cubLookPath, cubAuthToken
	t.Cleanup(func() { cubLookPath, cubAuthToken = oldLook, oldToken })
	cubLookPath = func() error {
		if s.cubInstalled {
			return nil
		}
		return errors.New("executable file not found in $PATH")
	}
	cubAuthToken = func() (string, error) {
		if !s.cubInstalled {
			t.Fatal("`cub auth get-token` must not run when cub is not installed")
		}
		return s.cubToken, s.cubTokenErr
	}
}

func TestRequireCubConnected(t *testing.T) {
	tests := []struct {
		name  string
		state credentialState
		want  error
	}{
		{
			// The case the old QuickMode gate got wrong: a logged-in cub, no
			// auth.json, standalone form.
			name:  "standalone, cub logged in",
			state: credentialState{cubInstalled: true, cubToken: "jwt"},
		},
		{name: "standalone, cub-scout auth.json", state: credentialState{authJSON: true}},
		{name: "plugin, host passed a token", state: credentialState{plugin: true, pluginToken: "jwt"}},
		{
			name:  "standalone, cub not installed",
			state: credentialState{},
			want:  ErrCubNotInstalled,
		},
		{
			name:  "standalone, cub logged out",
			state: credentialState{cubInstalled: true, cubTokenErr: errors.New("exit status 1")},
			want:  ErrCubNotLoggedIn,
		},
		{
			name:  "standalone, cub returns no token",
			state: credentialState{cubInstalled: true},
			want:  ErrCubNotLoggedIn,
		},
		{
			name:  "plugin, host passed no token",
			state: credentialState{plugin: true},
			want:  ErrCubNotLoggedIn,
		},
		{
			name:  "offline requested wins over a valid login",
			state: credentialState{offline: true, cubInstalled: true, cubToken: "jwt"},
			want:  ErrConfigHubReadsDisabled,
		},
		{
			name:  "telemetry off wins over a valid login",
			state: credentialState{telemetryOff: true, authJSON: true},
			want:  ErrConfigHubReadsDisabled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyCredentialState(t, tt.state)
			if got := RequireCubConnected(); !errors.Is(got, tt.want) {
				t.Fatalf("RequireCubConnected() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A plugin host that passed no token must not make the plugin shell back out
// to `cub`: that re-enters the host that launched it.
func TestRequireCubConnected_PluginNeverShellsOutToCub(t *testing.T) {
	applyCredentialState(t, credentialState{plugin: true})
	cubLookPath = func() error {
		t.Fatal("plugin form looked for cub on PATH")
		return nil
	}
	cubAuthToken = func() (string, error) {
		t.Fatal("plugin form ran `cub auth get-token`")
		return "", nil
	}
	if got := RequireCubConnected(); !errors.Is(got, ErrCubNotLoggedIn) {
		t.Fatalf("RequireCubConnected() = %v, want %v", got, ErrCubNotLoggedIn)
	}
}

// The standalone form (`cub-scout ...`) and the plugin form (`cub scout ...`)
// reach the same decision for the same credential state.
func TestRequireCubConnected_StandaloneAndPluginFormsAgree(t *testing.T) {
	pairs := []struct {
		name       string
		standalone credentialState
		plugin     credentialState
		want       error
	}{
		{
			name:       "logged in",
			standalone: credentialState{cubInstalled: true, cubToken: "jwt"},
			plugin:     credentialState{plugin: true, pluginToken: "jwt"},
		},
		{
			name:       "logged out",
			standalone: credentialState{cubInstalled: true, cubTokenErr: errors.New("exit status 1")},
			plugin:     credentialState{plugin: true},
			want:       ErrCubNotLoggedIn,
		},
	}
	for _, tt := range pairs {
		t.Run(tt.name, func(t *testing.T) {
			var got [2]error
			for i, state := range []credentialState{tt.standalone, tt.plugin} {
				t.Run(map[int]string{0: "standalone", 1: "plugin"}[i], func(t *testing.T) {
					applyCredentialState(t, state)
					got[i] = RequireCubConnected()
				})
			}
			if !errors.Is(got[0], tt.want) || !errors.Is(got[1], tt.want) {
				t.Fatalf("standalone = %v, plugin = %v, want both %v", got[0], got[1], tt.want)
			}
		})
	}
}

// QuickMode stays a display helper: it still ignores the cub login, which is
// exactly why it must not be used as a gate.
func TestQuickMode_IgnoresCubLogin(t *testing.T) {
	applyCredentialState(t, credentialState{cubInstalled: true, cubToken: "jwt"})
	if got := QuickMode(); got != Online {
		t.Fatalf("QuickMode() = %v, want Online: it never consults cub", got)
	}
	if err := RequireCubConnected(); err != nil {
		t.Fatalf("RequireCubConnected() = %v, want nil for the same state", err)
	}
}
