// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestMapList_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "map", "list", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_MAP_ENTRIES_JSON": fixtureAbs,
		},
		"map", "list",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "list", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapList_OwnerNative(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "map", "list", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_MAP_ENTRIES_JSON": fixtureAbs,
		},
		"map", "list", "--owner", "Native",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "list", "owner_native.txt")
	golden.AssertGolden(t, out, goldenPath)
}
