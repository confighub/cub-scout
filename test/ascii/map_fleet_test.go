// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestMapFleet_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "fleet", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_FLEET_JSON": fixture,
	}, "map", "fleet")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "fleet", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapFleet_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "fleet", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_FLEET_JSON": fixture,
	}, "map", "fleet", "--json")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "fleet", "basic.json.golden")
	golden.AssertGolden(t, out, goldenPath)
}
