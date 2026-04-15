// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestPluginParity_StandaloneMatchesPlugin is the v2.0 release-gate parity
// test: it builds the cub-scout binary once, stages it as both the
// standalone binary (cub-scout) and the plugin entry point (main, as it
// will appear under $CUB_CONFIG/plugins/scout/main), runs a representative
// set of read-only commands against both, and confirms the outputs match
// modulo a small, documented set of benign differences.
//
// This is the proof that `cub scout <cmd>` and `cub-scout <cmd>` remain
// two labels for the same behavior — the single most important invariant
// of the v2.0 plugin switchover.
//
// Commands covered here must:
//   - be offline and deterministic (no cluster, no network, no clock values)
//   - exercise the plumbing that most needs to behave identically across forms
//
// Any command-output difference is a real parity bug. Benign differences
// are filtered out by normalizeForParity below; everything else must match.
func TestPluginParity_StandaloneMatchesPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in -short mode: builds cub-scout binary and runs it twice")
	}

	tmpDir := t.TempDir()
	standaloneBin := filepath.Join(tmpDir, "cub-scout")
	pluginBin := filepath.Join(tmpDir, "scout", "main")
	if runtime.GOOS == "windows" {
		standaloneBin += ".exe"
		pluginBin += ".exe"
	}

	// Build once and stage the same binary into two locations. This proves
	// that bit-for-bit identical code produces bit-for-bit identical output
	// regardless of invocation form.
	buildCmd := exec.Command("go", "build", "-o", standaloneBin, ".")
	buildCmd.Stderr = new(bytes.Buffer)
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("go build cub-scout failed: %v\n%s", err, buildCmd.Stderr.(*bytes.Buffer).String())
	}
	if err := copyExecutable(standaloneBin, pluginBin); err != nil {
		t.Fatalf("stage plugin binary: %v", err)
	}

	repoRoot, err := filepath.Abs(filepath.Join(".", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	scanFixture := filepath.Join(repoRoot, "test", "fixtures", "edge-cases", "edge-03-hpa-statefulset.yaml")

	cases := []struct {
		name string
		args []string
		// extraEnv is appended to the minimal env used by runForParity.
		// Use this to force code paths that depend on environment state
		// (e.g. CUB_SCOUT_OFFLINE for the offline-mode error path).
		extraEnv []string
		// expectNonZeroExit is true when the case deliberately exercises
		// an error path. The parity test then compares the combined
		// stdout+stderr output instead of asserting a zero exit code.
		expectNonZeroExit bool
	}{
		{name: "version", args: []string{"version"}},
		{name: "top_help", args: []string{"--help"}},
		{name: "doctor_help", args: []string{"doctor", "--help"}},
		{name: "explain_help", args: []string{"explain", "--help"}},
		{name: "trace_help", args: []string{"trace", "--help"}},
		{name: "map_help", args: []string{"map", "--help"}},
		{name: "scan_help", args: []string{"scan", "--help"}},
		{name: "compare_three_way_help", args: []string{"compare", "three-way", "--help"}},
		{name: "mcp_serve_help", args: []string{"mcp", "serve", "--help"}},
		// Connected-command help: these commands require ConfigHub auth
		// at runtime but their --help output is pure cobra rendering and
		// must be identical across invocation forms. Adding them here
		// catches any plugin-mode template or command-path bug that
		// specifically affects connected subtrees.
		{name: "history_help", args: []string{"history", "--help"}},
		{name: "fleet_help", args: []string{"fleet", "--help"}},
		{name: "import_help", args: []string{"import", "--help"}},
		{name: "gitops_help", args: []string{"gitops", "--help"}},
		// Offline deterministic JSON output. scan --file does not touch a
		// cluster and emits a scannedAt timestamp that is normalized below.
		{name: "scan_file_json", args: []string{"scan", "--file", scanFixture, "--json"}},
		// Offline-mode error path for a connected command: forcing
		// CUB_SCOUT_OFFLINE=true should produce a deterministic
		// "requires ConfigHub connection" error with a non-zero exit code
		// in BOTH invocation forms. This is the first end-to-end parity
		// check that exercises a connected-subtree code path without
		// requiring a real cub binary or a real ConfigHub backend.
		{
			name:              "history_offline_error",
			args:              []string{"history", "deploy/parity-test-resource", "-n", "parity-test", "--format", "json"},
			extraEnv:          []string{"CUB_SCOUT_OFFLINE=true"},
			expectNonZeroExit: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			standalone, standaloneErr := runForParityWithEnv(standaloneBin, tc.args, false /* pluginMode */, tc.extraEnv)
			plugin, pluginErr := runForParityWithEnv(pluginBin, tc.args, true /* pluginMode */, tc.extraEnv)

			if !tc.expectNonZeroExit {
				if standaloneErr != nil {
					t.Fatalf("standalone run failed: %v\n%s", standaloneErr, standalone)
				}
				if pluginErr != nil {
					t.Fatalf("plugin run failed: %v\n%s", pluginErr, plugin)
				}
			} else {
				// Both forms must fail. If one succeeds or they fail with
				// different exit statuses that is itself a parity bug.
				if (standaloneErr == nil) != (pluginErr == nil) {
					t.Fatalf("exit status parity drift: standaloneErr=%v pluginErr=%v\n--- standalone ---\n%s\n--- plugin ---\n%s",
						standaloneErr, pluginErr, standalone, plugin)
				}
				if standaloneErr == nil {
					t.Fatalf("case %q was marked expectNonZeroExit but both forms succeeded", tc.name)
				}
			}

			wantStandalone := normalizeForParity(standalone, false)
			wantPlugin := normalizeForParity(plugin, true)

			if wantStandalone != wantPlugin {
				t.Errorf("parity drift for %q:\n--- standalone (normalized) ---\n%s\n--- plugin (normalized) ---\n%s\n--- raw standalone ---\n%s\n--- raw plugin ---\n%s",
					strings.Join(tc.args, " "),
					wantStandalone, wantPlugin,
					standalone, plugin,
				)
			}
		})
	}
}

// runForParity executes a staged cub-scout binary with a minimal environment
// so neither the parent test process nor user state can leak into the child.
// In plugin mode CUB_PLUGIN=1 is set to match how the cub host execs plugins.
func runForParity(binPath string, args []string, pluginMode bool) (string, error) {
	return runForParityWithEnv(binPath, args, pluginMode, nil)
}

// runForParityWithEnv is runForParity plus an extraEnv slice that gets
// appended to the minimal environment before execution. Used by cases that
// need to force specific runtime behavior (e.g. CUB_SCOUT_OFFLINE=true to
// exercise the offline-mode error path of a connected command).
func runForParityWithEnv(binPath string, args []string, pluginMode bool, extraEnv []string) (string, error) {
	cmd := exec.Command(binPath, args...)
	env := []string{
		"HOME=" + filepath.Dir(binPath), // throwaway HOME
		"PATH=/usr/bin:/bin",
		"NO_COLOR=1", // strip ANSI; not relevant to parity
		"TERM=dumb",
	}
	if pluginMode {
		env = append(env, "CUB_PLUGIN=1")
	}
	env = append(env, extraEnv...)
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	return string(out), err
}

// normalizeForParity strips out the documented benign differences between
// the two invocation forms so diff failures reflect real parity bugs.
//
// Benign differences today:
//
//  1. The cobra "Usage:" and similar Use-string mentions:
//     standalone form shows `cub-scout <cmd>`
//     plugin form     shows `cub scout <cmd>`
//     This is deliberate. The TestPluginMode_UseStringFlip test covers
//     this separately, so we normalize it out here.
//  2. Timestamps: scan --file --json emits a "scannedAt" RFC3339 value that
//     changes every run. Normalize to a stable token.
//  3. Trailing whitespace differences.
func normalizeForParity(output string, pluginMode bool) string {
	if pluginMode {
		// Rewrite every `cub scout` back to `cub-scout` so standalone and
		// plugin renderings land on the same shape.
		output = strings.ReplaceAll(output, "cub scout", "cub-scout")
		// Cobra's auto-generated `-h, --help` description uses the root
		// command's Name(), which is "scout" in plugin mode because we
		// set root.Use = "scout" (see applyPluginModeHelp). Standalone
		// form renders "help for cub-scout". Rewrite to match.
		output = strings.ReplaceAll(output, "help for scout", "help for cub-scout")
	}

	// Timestamps in scan --file --json output.
	output = scanScannedAtRE.ReplaceAllString(output, `"scannedAt": "<NORMALIZED>"`)

	// Collapse trailing whitespace per line.
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

var scanScannedAtRE = regexp.MustCompile(`"scannedAt":\s*"[^"]*"`)

// copyExecutable reads src, writes it to dst (creating parent dirs), and
// chmods dst to 0755. Used to stage the same binary at two locations so the
// parity test can invoke it under two names.
func copyExecutable(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}
