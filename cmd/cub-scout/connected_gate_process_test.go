// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeCub stands in for the cub CLI. `cub auth status` behaves as FAKE_CUB_AUTH
// says, using the messages the real cub v0.5.1 prints; every other command
// fails, which is enough to show a gated command got past the gate.
const fakeCub = `#!/bin/sh
if [ "$1 $2" = "auth status" ]; then
  case "$FAKE_CUB_AUTH" in
    ok) echo "Status               Authenticated"; exit 0 ;;
    expired) echo "Failed: not authenticated: access token expired at 2026-07-30T09:50:02+01:00. Ask the user to run 'cub auth login' to re-authenticate." >&2; exit 1 ;;
    badcontext) echo 'Failed: CUB_CONTEXT environment variable: context "kind-demo" not found' >&2; exit 1 ;;
  esac
fi
# What the real cub does here: a stored token is printed even when expired.
if [ "$1 $2" = "auth get-token" ]; then echo "stored-but-expired-token"; exit 0; fi
echo "Failed: fake cub does not implement: $*" >&2
exit 1
`

// TestConnectedGate_RealProcessBothForms runs the built binary as a separate
// process, in the standalone form and in the plugin form, against a fake cub.
// It is the gate by the cub route (CUB_SCOUT_CONFIGHUB_READER=cub), which is
// `cub auth status`; the gate by the SDK route has the test below.
// It is the recorded-input proof for the connected-only gate: for every
// credential state both forms must print the same thing, each refusal must
// name its true cause, and an expired session must be refused even though
// `cub auth get-token` still hands out its token.
func TestConnectedGate_RealProcessBothForms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode: builds cub-scout binary")
	}
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "cub-scout")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stderr = new(bytes.Buffer)
	if err := build.Run(); err != nil {
		t.Fatalf("go build: %v\n%s", err, build.Stderr)
	}
	cubDir := filepath.Join(dir, "bin")
	if err := os.Mkdir(cubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cubDir, "cub"), []byte(fakeCub), 0o755); err != nil {
		t.Fatal(err)
	}

	const viewID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	commands := map[string][]string{
		"compare source-truth": {"compare", "source-truth", "deployment/api", "--strategy", "git-argo"},
		"views resolve":        {"views", "resolve", viewID},
	}
	states := []struct {
		name     string
		withCub  bool
		auth     string
		extraEnv string
		want     []string
		wantNot  []string
	}{
		{name: "cub not installed", want: []string{"needs ConfigHub", "not on PATH"}},
		{name: "session expired", withCub: true, auth: "expired", want: []string{"needs ConfigHub", "access token expired"}},
		{
			// Not a login problem: the advice to log in must not replace the cause.
			name: "cub cannot resolve its context", withCub: true, auth: "badcontext",
			want: []string{"needs ConfigHub", `context "kind-demo" not found`},
		},
		{name: "reads turned off", withCub: true, auth: "ok", extraEnv: "CUB_SCOUT_OFFLINE=true", want: []string{"needs ConfigHub", "CUB_SCOUT_OFFLINE=true"}},
		{name: "authenticated", withCub: true, auth: "ok", wantNot: []string{"needs ConfigHub"}},
	}

	for label, args := range commands {
		for _, state := range states {
			t.Run(label+"/"+state.name, func(t *testing.T) {
				var outputs []string
				for _, plugin := range []bool{false, true} {
					path := "/usr/bin:/bin"
					if state.withCub {
						path = cubDir + ":" + path
					}
					home := t.TempDir()
					cmd := exec.Command(binary, args...)
					cmd.Env = []string{
						"HOME=" + home, "PATH=" + path, "NO_COLOR=1", "TERM=dumb",
						"KUBECONFIG=" + filepath.Join(home, "absent-kubeconfig"),
						"FAKE_CUB_AUTH=" + state.auth,
						configHubReaderEnv + "=cub",
					}
					if state.extraEnv != "" {
						cmd.Env = append(cmd.Env, state.extraEnv)
					}
					if plugin {
						cmd.Env = append(cmd.Env, "CUB_PLUGIN=1", "CUB_TOKEN=host-token")
					}
					out, _ := cmd.CombinedOutput()
					text := string(out)
					for _, want := range state.want {
						if !strings.Contains(text, want) {
							t.Errorf("plugin=%v output = %q, want it to contain %q", plugin, text, want)
						}
					}
					for _, wantNot := range state.wantNot {
						if strings.Contains(text, wantNot) {
							t.Errorf("plugin=%v output = %q, want the gate to have passed", plugin, text)
						}
					}
					// Only the refusal is compared across forms; past the gate the
					// two forms may word later errors differently.
					if len(state.want) > 0 {
						outputs = append(outputs, firstLineOf(text))
					}
				}
				if len(outputs) == 2 && outputs[0] != outputs[1] {
					t.Errorf("standalone and plugin forms disagree:\n  standalone: %s\n  plugin:     %s", outputs[0], outputs[1])
				}
			})
		}
	}
}

func firstLineOf(text string) string {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}

// TestConnectedGate_RealProcessOnTheSDKRoute runs the built binary as a
// separate process with no cub anywhere on PATH, which is the default route's
// gate (#758): the credential cub would use, a token that has not expired, and
// one request the server answers. Both forms must pass a session the server
// accepts and refuse the rest, each for its true reason.
func TestConnectedGate_RealProcessOnTheSDKRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode: builds cub-scout binary")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "cub-scout")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stderr = new(bytes.Buffer)
	if err := build.Run(); err != nil {
		t.Fatalf("go build: %v\n%s", err, build.Stderr)
	}

	status := http.StatusOK
	var asked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/me" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"Message":"not found"}`))
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"Message":"simulated"}`))
			return
		}
		_, _ = w.Write([]byte(`{"OrganizationID":"11111111-2222-4333-8444-555555555555","UserID":"99999999-8888-4777-8666-555555555555"}`))
	}))
	defer server.Close()

	// A cub configuration as cub writes one, with the token beside it.
	cubConfig := func(t *testing.T, token string) string {
		t.Helper()
		config := t.TempDir()
		if err := os.Mkdir(filepath.Join(config, "tokens"), 0o700); err != nil {
			t.Fatal(err)
		}
		yaml := "apiVersion: v1\nkind: Config\ncurrentContext: test\ncontexts:\n  - name: test\n    coordinate:\n      serverURL: " + server.URL +
			"\n      organizationID: 11111111-2222-4333-8444-555555555555\n      user: someone@example.test\n    settings: {}\n    metadata:\n      tokenFile: test.json\n"
		if err := os.WriteFile(filepath.Join(config, "config.yaml"), []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(config, "tokens", "test.json"), []byte(`{"accessToken":"`+token+`"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		return config
	}
	expired := "e30." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(-time.Hour).Unix()))) + ".c2ln"

	const viewID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	for _, state := range []struct {
		name     string
		token    string
		status   int
		extraEnv string
		want     []string
		wantNot  []string
		asks     bool
	}{
		{name: "a session the server accepts", token: "opaque-token", status: http.StatusOK, wantNot: []string{"needs ConfigHub", "not on PATH"}, asks: true},
		{name: "a token the server rejects", token: "opaque-token", status: http.StatusUnauthorized,
			want: []string{"needs ConfigHub", "did not accept the credential", "cub auth login"}, wantNot: []string{"cub auth status", "opaque-token"}, asks: true},
		{name: "a token that says it has expired", token: expired, status: http.StatusOK,
			want: []string{"needs ConfigHub", "the access token expired at"}, wantNot: []string{expired}},
		{name: "no token", token: "", status: http.StatusOK, want: []string{"needs ConfigHub", "cub auth login"}},
		{name: "a server that cannot answer", token: "opaque-token", status: http.StatusInternalServerError,
			want: []string{"needs ConfigHub", "could not be checked"}, wantNot: []string{"cub auth login"}, asks: true},
		{name: "reads turned off", token: "opaque-token", status: http.StatusOK, extraEnv: "CUB_SCOUT_OFFLINE=true",
			want: []string{"needs ConfigHub", "CUB_SCOUT_OFFLINE=true"}},
	} {
		t.Run(state.name, func(t *testing.T) {
			for _, plugin := range []bool{false, true} {
				status, asked = state.status, nil
				home := t.TempDir()
				cmd := exec.Command(binary, "views", "resolve", viewID)
				cmd.Env = []string{
					"HOME=" + home, "PATH=" + t.TempDir(), "NO_COLOR=1", "TERM=dumb",
					"KUBECONFIG=" + filepath.Join(home, "absent-kubeconfig"),
				}
				if state.extraEnv != "" {
					cmd.Env = append(cmd.Env, state.extraEnv)
				}
				if plugin {
					// As cub starts a plugin: the server and token in the
					// environment, and no configuration consulted.
					cmd.Env = append(cmd.Env, "CUB_PLUGIN=1", "CUB_SERVER="+server.URL, "CUB_TOKEN="+state.token, "CUB_CONFIG="+t.TempDir())
				} else {
					cmd.Env = append(cmd.Env, "CUB_CONFIG="+cubConfig(t, state.token))
				}
				out, _ := cmd.CombinedOutput()
				text := string(out)
				for _, want := range state.want {
					if !strings.Contains(text, want) {
						t.Errorf("plugin=%v output = %q, want it to contain %q", plugin, text, want)
					}
				}
				for _, wantNot := range state.wantNot {
					if strings.Contains(text, wantNot) {
						t.Errorf("plugin=%v output = %q, want it not to contain %q", plugin, text, wantNot)
					}
				}
				if state.asks != (len(asked) > 0) {
					t.Errorf("plugin=%v: the server was asked %v, want asked=%v", plugin, asked, state.asks)
				}
				if state.asks && asked[0] != "GET /api/me" {
					t.Errorf("plugin=%v: the first request was %q, want the session check", plugin, asked[0])
				}
			}
		})
	}
}
