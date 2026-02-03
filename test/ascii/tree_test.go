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

func TestTreeRuntime_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "tree", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_TREE_JSON": fixtureAbs,
		},
		"tree", "runtime",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tree", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}
