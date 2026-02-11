// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestTraceFlux_Artifacts(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	traceFixture := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "flux.json")
	artifactFixture := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "flux.artifacts.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_TRACE_JSON":           traceFixture,
		"CUB_SCOUT_TEST_TRACE_ARTIFACTS_JSON": artifactFixture,
	}, "trace", "deployment/payment-api", "-n", "ecommerce", "--artifacts")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "flux_artifacts.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestTraceFlux_ArtifactsJSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	traceFixture := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "flux.json")
	artifactFixture := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "flux.artifacts.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_TRACE_JSON":           traceFixture,
		"CUB_SCOUT_TEST_TRACE_ARTIFACTS_JSON": artifactFixture,
	}, "trace", "deployment/payment-api", "-n", "ecommerce", "--artifacts", "--format", "json")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "flux_artifacts.json.golden")
	golden.AssertGolden(t, out, goldenPath)
}

func TestTraceFlux_ArtifactsUnknown(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	traceFixture := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "flux.json")

	out := runner.RunWithEnv(t, repoRoot, map[string]string{
		"CUB_SCOUT_TEST_TRACE_JSON": traceFixture,
	}, "trace", "deployment/payment-api", "-n", "ecommerce", "--artifacts")

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "flux_artifacts_unknown.txt")
	golden.AssertGolden(t, out, goldenPath)
}
