// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package ascii_test provides ASCII golden tests for cub-scout CLI output.
//
// NOTE: These goldens lock user-facing ASCII output.
// Changes here should be reviewed as UX changes.
//
// To update goldens: go test ./test/ascii/... -update
package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestScan_StateFindings(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "scan", "testdata", "state.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_SCAN_JSON": fixtureAbs,
		},
		"scan", "--state",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "scan", "state.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestScan_Clean(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "scan", "testdata", "clean.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_SCAN_JSON": fixtureAbs,
		},
		"scan", "--state",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "scan", "clean.txt")
	golden.AssertGolden(t, out, goldenPath)
}
