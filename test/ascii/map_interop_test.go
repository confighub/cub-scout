// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestMapCronJobs_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "cronjobs", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_CRONJOBS_JSON": fixture,
	}, "map", "cronjobs")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "cronjobs", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapCronJobs_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "cronjobs", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_CRONJOBS_JSON": fixture,
	}, "map", "cronjobs", "--format", "json")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "cronjobs", "basic.json.golden")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapJobs_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "jobs", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_JOBS_JSON": fixture,
	}, "map", "jobs")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "jobs", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapJobs_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "jobs", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_JOBS_JSON": fixture,
	}, "map", "jobs", "--format", "json")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "jobs", "basic.json.golden")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapActions_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "actions", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_ACTIONS_JSON": fixture,
	}, "map", "actions", "deployment/payment-api", "-n", "ecommerce")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "actions", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapActions_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "actions", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_ACTIONS_JSON": fixture,
	}, "map", "actions", "deployment/payment-api", "-n", "ecommerce", "--format", "json")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "actions", "basic.json.golden")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapActivity_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "activity", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_ACTIVITY_JSON": fixture,
	}, "map", "activity")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "activity", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapActivity_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "activity", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_ACTIVITY_JSON": fixture,
	}, "map", "activity", "--format", "json")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "activity", "basic.json.golden")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapPreviews_Basic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "previews", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_PREVIEWS_JSON": fixture,
	}, "map", "previews")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "previews", "basic.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestMapPreviews_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixture := filepath.Join(repoRoot, "test", "ascii", "map", "previews", "testdata", "basic.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_MAP_PREVIEWS_JSON": fixture,
	}, "map", "previews", "--format", "json")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "map", "previews", "basic.json.golden")
	golden.AssertGolden(t, out, goldenPath)
}
