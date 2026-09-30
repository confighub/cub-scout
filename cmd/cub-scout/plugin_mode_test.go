// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// The strings below are not hints, so they do not pass through
// hintsToStrings or ToStructured; each call site applies
// preferInvocationForm itself (#386). Every test checks both forms: the
// plugin form must read `cub scout ...` and the standalone output must not
// move.

func TestRenderRootLanding_FollowsInvocationForm(t *testing.T) {
	for _, tc := range []struct {
		name   string
		plugin string
		want   []string
		reject string
	}{
		{
			name:   "plugin",
			plugin: "1",
			want: []string{
				"cub scout quickstart --yes  Guided first-run walkthrough",
				"cub scout doctor           Cluster health summary",
				"cub scout explain deploy/x -n <namespace>  Explain one resource",
				"cub scout import --dry-run Preview ConfigHub import (connected)",
				"Run 'cub scout --help' for all commands",
			},
			reject: "  cub-scout ",
		},
		{
			name:   "standalone",
			plugin: "",
			want: []string{
				"cub-scout quickstart --yes  Guided first-run walkthrough",
				"cub-scout doctor           Cluster health summary",
				"cub-scout explain deploy/x -n <namespace>  Explain one resource",
				"cub-scout import --dry-run Preview ConfigHub import (connected)",
				"Run 'cub-scout --help' for all commands",
			},
			reject: "cub scout",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CUB_PLUGIN", tc.plugin)
			var b bytes.Buffer
			renderRootLanding(&b, true)
			out := b.String()
			for _, s := range tc.want {
				if !strings.Contains(out, s) {
					t.Errorf("expected %q in root landing:\n%s", s, out)
				}
			}
			if strings.Contains(out, tc.reject) {
				t.Errorf("root landing contains %q in %s mode:\n%s", tc.reject, tc.name, out)
			}
			// The product name in the title is not a command and stays put.
			if !strings.HasPrefix(out, "cub-scout - GitOps explorer for agents\n") {
				t.Errorf("root landing title changed:\n%s", out)
			}
		})
	}
}

func TestWithKubeRecoveryHint_FollowsInvocationForm(t *testing.T) {
	cause := fmt.Errorf("build kubernetes config: no configuration has been provided")
	for _, tc := range []struct {
		name, plugin, command string
		want                  []string
	}{
		{name: "plugin", plugin: "1", command: "cub-scout doctor",
			want: []string{"3) cub scout doctor --help", "4) cub scout quickstart"}},
		{name: "plugin_empty_command", plugin: "1", command: "",
			want: []string{"3) cub scout --help", "4) cub scout quickstart"}},
		{name: "standalone", plugin: "", command: "cub-scout doctor",
			want: []string{"3) cub-scout doctor --help", "4) cub-scout quickstart"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CUB_PLUGIN", tc.plugin)
			err := withKubeRecoveryHint(cause, tc.command)
			for _, s := range tc.want {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("expected %q in recovery error:\n%s", s, err.Error())
				}
			}
			if !strings.HasPrefix(err.Error(), cause.Error()) {
				t.Errorf("recovery error lost its cause:\n%s", err.Error())
			}
		})
	}
}

func TestDoctorThreeWayHint_FollowsInvocationForm(t *testing.T) {
	for _, tc := range []struct{ name, plugin, want string }{
		{name: "plugin", plugin: "1", want: "cub scout compare three-way --scope namespace/prod"},
		{name: "standalone", plugin: "", want: "cub-scout compare three-way --scope namespace/prod"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CUB_PLUGIN", tc.plugin)
			stubConnectedGate(t, nil)
			summary := buildDoctorSummary(nil, nil, "kind-dev", "prod", 3)
			if summary.ThreeWay == nil {
				t.Fatal("expected a three-way hint with the gate open")
			}
			if summary.ThreeWay.Hint != tc.want {
				t.Errorf("three-way hint = %q, want %q", summary.ThreeWay.Hint, tc.want)
			}
			ascii := renderDoctorASCII(summary, PresentationHuman, false, HintContext{})
			if !strings.Contains(ascii, tc.want) {
				t.Errorf("doctor ASCII does not show %q:\n%s", tc.want, ascii)
			}
		})
	}
}

func TestScanFooterAndNextSteps_FollowInvocationForm(t *testing.T) {
	oldExplain := scanExplain
	t.Cleanup(func() { scanExplain = oldExplain })
	scanExplain = true

	for _, tc := range []struct {
		name, plugin string
		want         []string
		reject       string
	}{
		{name: "plugin", plugin: "1", want: []string{
			"Track violations in ConfigHub: cub scout scan --confighub",
			"See all patterns:        cub scout scan --list",
			"Scan a YAML file:        cub scout scan --file manifest.yaml",
			"Trace failing resource:  cub scout trace <kind>/<name> -n <namespace>",
		}, reject: "cub-scout scan -"},
		{name: "standalone", plugin: "", want: []string{
			"Track violations in ConfigHub: cub-scout scan --confighub",
			"See all patterns:        cub-scout scan --list",
			"Scan a YAML file:        cub-scout scan --file manifest.yaml",
			"Trace failing resource:  cub-scout trace <kind>/<name> -n <namespace>",
		}, reject: "cub scout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CUB_PLUGIN", tc.plugin)
			out := captureStdout(t, func() {
				if err := outputCombinedHuman(nil, nil, nil, nil, nil); err != nil {
					t.Errorf("outputCombinedHuman: %v", err)
				}
			})
			for _, s := range tc.want {
				if !strings.Contains(out, s) {
					t.Errorf("expected %q in scan output:\n%s", s, out)
				}
			}
			if strings.Contains(out, tc.reject) {
				t.Errorf("scan output contains %q in %s mode:\n%s", tc.reject, tc.name, out)
			}
		})
	}
}
