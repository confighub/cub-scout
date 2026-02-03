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

func TestTrace_Flux(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "flux.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_TRACE_JSON": fixtureAbs,
		},
		"trace", "deployment/payment-api", "-n", "ecommerce",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "flux.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestTrace_Argo(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "argo.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_TRACE_JSON": fixtureAbs,
		},
		"trace", "deployment/cert-manager", "-n", "platform",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "argo.txt")
	golden.AssertGolden(t, out, goldenPath)
}

func TestTrace_Native(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "native.json")

	// Native trace exits with code 1, which runner.RunWithEnv handles
	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_TRACE_JSON": fixtureAbs,
		},
		"trace", "deployment/debug-nginx", "-n", "temp-test",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "native.txt")
	golden.AssertGolden(t, out, goldenPath)
}
