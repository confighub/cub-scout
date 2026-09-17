// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
