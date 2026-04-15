// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestPreferInvocationForm_StandaloneIsNoop verifies that in standalone
// mode (CUB_PLUGIN unset) the rewrite helper returns the input unchanged,
// preserving the legacy `cub-scout ...` form for hint output.
func TestPreferInvocationForm_StandaloneIsNoop(t *testing.T) {
	t.Setenv("CUB_PLUGIN", "")
	cases := []string{
		"cub-scout doctor",
		"cub-scout explain deploy/foo -n bar",
		"cub-scout",
		"",
		"kubectl get pods",
		"cub unit list",
	}
	for _, in := range cases {
		if got := preferInvocationForm(in); got != in {
			t.Errorf("preferInvocationForm(%q) = %q, want unchanged input in standalone mode", in, got)
		}
	}
}

// TestPreferInvocationForm_PluginRewritesPrefix verifies that the rewrite
// helper only touches the exact `cub-scout` token at the start of a
// command string. Substrings, URLs, and arbitrary content that happens to
// contain the word must be preserved.
func TestPreferInvocationForm_PluginRewritesPrefix(t *testing.T) {
	t.Setenv("CUB_PLUGIN", "1")

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "doctor", in: "cub-scout doctor", want: "cub scout doctor"},
		{name: "explain_with_args", in: "cub-scout explain deploy/foo -n bar", want: "cub scout explain deploy/foo -n bar"},
		{name: "bare_binary_name", in: "cub-scout", want: "cub scout"},
		{name: "empty", in: "", want: ""},
		{name: "unrelated_command", in: "kubectl get pods", want: "kubectl get pods"},
		{name: "cub_without_scout", in: "cub unit list", want: "cub unit list"},
		// Must not mangle a URL that happens to contain "cub-scout".
		{name: "url_with_cub_scout", in: "https://github.com/confighub/cub-scout/releases", want: "https://github.com/confighub/cub-scout/releases"},
		// Must not mangle mid-string occurrences.
		{name: "mid_string_mention", in: "see cub-scout docs for details", want: "see cub-scout docs for details"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := preferInvocationForm(tc.in); got != tc.want {
				t.Errorf("preferInvocationForm(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestHintToStructured_PluginModeRewritesCommand proves the JSON/MCP hint
// boundary applies the plugin-mode invocation rewrite. This is the field
// that appears in `nextSteps[].nextCommand` in every JSON output path.
func TestHintToStructured_PluginModeRewritesCommand(t *testing.T) {
	t.Setenv("CUB_PLUGIN", "1")
	h := Hint{
		Command:   "cub-scout doctor -n prod",
		Rationale: "first check",
	}
	got := h.ToStructured()
	if got.NextCommand != "cub scout doctor -n prod" {
		t.Errorf("NextCommand = %q, want %q", got.NextCommand, "cub scout doctor -n prod")
	}
}

// TestHintToStructured_StandaloneLeavesCommandAlone locks in that the
// standalone form keeps emitting the legacy `cub-scout ...` strings so
// existing tests and users are unaffected.
func TestHintToStructured_StandaloneLeavesCommandAlone(t *testing.T) {
	t.Setenv("CUB_PLUGIN", "")
	h := Hint{
		Command:   "cub-scout doctor -n prod",
		Rationale: "first check",
	}
	got := h.ToStructured()
	if got.NextCommand != "cub-scout doctor -n prod" {
		t.Errorf("NextCommand = %q, want %q", got.NextCommand, "cub-scout doctor -n prod")
	}
}

// TestPluginMode_UseStringFlip exercises the built cub-scout binary with and
// without CUB_PLUGIN=1 and confirms the cobra "Usage:" line reflects the
// preferred invocation in each mode.
//
// Plugin form:    `cub scout [flags]`
// Standalone:     `cub-scout [flags]`
//
// The test builds the binary into a temp directory so it does not depend on
// the repo-root `cub-scout` artifact.
func TestPluginMode_UseStringFlip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode: builds cub-scout binary")
	}

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "cub-scout")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Stderr = new(bytes.Buffer)
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("go build cub-scout failed: %v\n%s", err, buildCmd.Stderr.(*bytes.Buffer).String())
	}

	cases := []struct {
		name       string
		pluginMode bool
		wantUsage  string
	}{
		{
			name:       "standalone",
			pluginMode: false,
			wantUsage:  "cub-scout [flags]",
		},
		{
			name:       "plugin",
			pluginMode: true,
			wantUsage:  "cub scout [flags]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(binPath, "--help")
			// Start from a minimal env so the parent test harness's variables
			// do not leak into the child's plugin-mode detection.
			cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
			if tc.pluginMode {
				cmd.Env = append(cmd.Env, "CUB_PLUGIN=1")
			}

			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("cub-scout --help failed: %v\n%s", err, string(out))
			}

			if !bytes.Contains(out, []byte(tc.wantUsage)) {
				t.Errorf("help output does not contain expected usage line %q\nfull output:\n%s", tc.wantUsage, string(out))
			}
		})
	}
}
