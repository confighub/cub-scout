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

func TestMapOrphans_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "map", "orphans", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_MAP_ENTRIES_JSON": fixtureAbs,
		},
		"map", "orphans",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "orphans", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapOrphans_IncludesAppSetOrphans(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "map", "orphans", "testdata", "appset_orphans.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_MAP_ENTRIES_JSON": fixtureAbs,
		},
		"map", "orphans",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "orphans", "appset_orphans.txt")
	golden.AssertGolden(t, out, goldenPath)
}
