// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// suggest-remedy is granted to agents as a read-only command. Its --namespace
// value was interpolated into a command line run by `sh -c`, so a crafted
// value ran as a shell command. The value must be rejected as an invalid
// namespace and nothing must run.
func TestSuggestRemedyDoesNotRunItsArgumentsInAShell(t *testing.T) {
	binary := sharedTestBinary(t)
	work := t.TempDir()
	// suggest-remedy looks for its finding database in ./cve/ccve.
	require.NoError(t, os.MkdirAll(filepath.Join(work, "cve", "ccve"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(work, "cve", "ccve", "CCVE-TEST-0001.yaml"), []byte(
		"id: CCVE-TEST-0001\ncategory: test\nname: fixture\nseverity: low\ndetection:\n  resources: [ConfigMap]\nremedy:\n  type: delete_resource\n"), 0o600))
	marker := filepath.Join(work, "marker")

	for name, namespace := range map[string]string{
		"command separator":    "x; echo reached > marker #",
		"command substitution": "$(echo reached > marker)",
		"backticks":            "`echo reached > marker`",
		"pipe":                 "x | tee marker",
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "suggest-remedy", "CCVE-TEST-0001", "--namespace", namespace)
			cmd.Dir = work
			cmd.Env = []string{"HOME=" + work, "PATH=/usr/bin:/bin"}
			out, err := cmd.CombinedOutput()
			require.Error(t, err, "a namespace that is not a valid name was accepted:\n%s", out)
			require.Contains(t, string(out), "invalid namespace")
			_, statErr := os.Stat(marker)
			require.True(t, os.IsNotExist(statErr), "the namespace value was executed by a shell")
		})
	}
}

// No product code may start a shell. Arguments go to a subprocess as a vector.
func TestNoShellInvocationInProductCode(t *testing.T) {
	shell := regexp.MustCompile(`exec\.Command(Context)?\([^)]*"(sh|bash|/bin/sh|/bin/bash|zsh)"\s*,\s*"-c"`)
	for _, root := range []string{".", "../../pkg", "../../internal"} {
		require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			source, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if location := shell.FindIndex(source); location != nil {
				line := 1 + strings.Count(string(source[:location[0]]), "\n")
				t.Errorf("%s:%d starts a shell; pass arguments as a vector instead", path, line)
			}
			return nil
		}))
	}
}
