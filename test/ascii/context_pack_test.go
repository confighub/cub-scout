// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestContextPack_FormatJSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "context-pack", "testdata", "basic_input.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_CONTEXT_PACK_INPUT_JSON": fixtureAbs,
			"CUB_SCOUT_TEST_TIME":                    "2026-03-06T20:30:00Z",
		},
		"context-pack", "--format", "json", "--top-risks", "3", "--trace-seeds", "3", "--max-bytes", "8192",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "context-pack", "basic.json.golden")
	golden.AssertGolden(t, out, goldenPath)
}
