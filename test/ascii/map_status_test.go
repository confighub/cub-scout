// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestMapStatus_Healthy(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "map", "status", "testdata", "healthy.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_MAP_STATUS_JSON": fixtureAbs,
		},
		"map", "status",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "status", "healthy.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapStatus_Problems(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "map", "status", "testdata", "problems.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_MAP_STATUS_JSON": fixtureAbs,
		},
		"map", "status",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "status", "problems.txt")
	golden.AssertGolden(t, out, goldenPath)
}
