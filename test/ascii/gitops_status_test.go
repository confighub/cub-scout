// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package ascii_test

import (
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

func TestGitOpsStatus_ArgoHealthyButPodsFailing(t *testing.T) {
	repoRoot := runner.RepoRoot(t)
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "gitops-status", "testdata", "argo_healthy_pods_failing.json")

	out := runner.RunWithEnv(t, repoRoot,
		map[string]string{
			"CUB_SCOUT_TEST_GITOPS_JSON": fixtureAbs,
		},
		"gitops", "status",
	)

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "gitops-status", "argo_healthy_pods_failing.txt")
	golden.AssertGolden(t, out, goldenPath)
}
