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
