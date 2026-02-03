// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestDebug_CrashLoop(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestDebug_CrashLoop_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug", "--format", "json",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop.json.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestDebug_CrashLoop_Markdown(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug", "--format", "md",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop.md.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestDebug_CrashLoopWithLogs(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop_with_logs.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop_with_logs.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestDebug_CrashLoopWithLogs_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop_with_logs.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug", "--format", "json",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop_with_logs.json.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestDebug_CrashLoopWithLogs_Markdown(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop_with_logs.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug", "--format", "md",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop_with_logs.md.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestDebug_CrashLoopWithEvents(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop_with_events.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop_with_events.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestDebug_CrashLoopWithEvents_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop_with_events.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug", "--format", "json",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop_with_events.json.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestDebug_CrashLoopWithEvents_Markdown(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "debug", "testdata", "crash_loop_with_events.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{"CUB_SCOUT_TEST_DEBUG_JSON": fixtureAbs},
		"debug", "--format", "md",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "debug", "crash_loop_with_events.md.txt")
	golden.AssertGolden(t, out, goldenPath)
}
