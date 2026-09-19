// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCheckUserQuestionsAndBoundaries(t *testing.T) {
	for file, snippets := range map[string][]string{
		"README.md": {
			"Did this exact OCI configuration release reach the selected cluster?",
			"Do I have the exact release identity?",
			"Are all replicas running the intended image?",
			"What does incomplete proof mean?",
			"UNRELEASED source branch",
			"no inventory LIST",
			"Watch/bot do not schedule this check yet",
			"active ConfigHub registry",
			"Can a script, CI job or agent check this without a TUI?",
		},
		"examples/oci-release-check/README.md": {
			"image-deployment.yaml", "--kube-context", "--controller-context",
			"11 Kubernetes requests", "2N + 4", "2N + 8", "2R + 3", "32 MiB", "16 MiB",
			"4 MiB", "not an atomic snapshot", "NOT_ASSESSED", "No source index is guessed",
			"UNRELEASED", "check-running-image", "TestReleaseCheck(RunningImage|CLIAndMCP)$",
		},
		"docs/reference/json-contracts.md": {
			"## Configuration Release Check", "v2.12.0", "UNRELEASED",
			"controllerContext", "registry", "NOT_ASSESSED", "not the outer",
			"runningImage", "replicaSetName", "replicaSetUID", "observedAt",
			"correlation.ociSourceVerified", "correlation.ociSourceRead", "fresh configured-registry lookup",
		},
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatal(err)
		}
		for _, snippet := range snippets {
			if !strings.Contains(string(data), snippet) {
				t.Errorf("%s missing release-check value/boundary %q", file, snippet)
			}
		}
	}
}

func TestImageDeploymentGuideAndLinks(t *testing.T) {
	for file, snippets := range map[string][]string{
		"README.md": {
			"[Is this image deployed?](docs/howto/is-this-image-deployed.md)",
			"Do I have the exact release identity?", "Are all replicas running the intended image?",
			"What does incomplete proof mean?", "UNRELEASED source branch",
		},
		"docs/howto/is-this-image-deployed.md": {
			"v2.12.0", "UNRELEASED", "## Three Questions To Ask", "## Five Different Questions",
			"## Start With What You Have", "## Why OCI Is Involved", "## Run The Check",
			"## Read The Result", "## Ways To Run It", "## Limits Before Trusting A Match",
			"--bundle", "--controller", "--api-version", "--controller-namespace",
			"--kube-context", "--check-running-image", "--max-pods 50", "--format json",
			"--out release-check.json", "--fail-on any-non-pass", "--interactive",
			"cub scout release check", "check_running_image: true",
			"configuration-bundle digest is never compared with a container-image",
			"no dedicated image-reference search", "state.running", "owner UID",
			"missing or ambiguous status", "digest-form-unresolved", "PodReady=True",
			"matchExpressions", "structured selector", "single-source Argo CD Application",
			"Flux v1 Kustomization", "General HelmRelease support",
			"not signed or an immutable receipt", "current builders emit no mismatch/BLOCK",
			"no supported workloads is",
			"ociSourceVerified", "ociSourceRead", "status.sync.comparedTo",
			"OCIRepository URL", "current Ready condition",
			"replicaSetName", "replicaSetUID", "deployment.complete", "observedAt",
			"workload-ownership-unsupported", "no registry image resolution",
			"Watch and in-cluster bot", "not an atomic snapshot",
			"No TUI is required", "### Get The Expected Identity First",
			"### Argo CLI", "### Flux CLI", "### Read Permissions",
			"### Scripts And CI", "### When It Does Not Pass", "### Keep Reads Bounded",
			"## Validation Status", "stdout write failures", "No TTY required",
			"11 Argo / 15 Flux", "7 / 11", "--controller-context",
			"enclosing CI job/process timeout", "50 and 200 Pods",
		},
		"CLI-GUIDE.md": {
			"Available since v2.11.0", "v2.12.0", "UNRELEASED", "docs/howto/is-this-image-deployed.md",
		},
		"docs/getting-started/start-here.md": {
			"../howto/is-this-image-deployed.md", "v2.12.0", "UNRELEASED",
		},
		"examples/oci-release-check/README.md": {
			"v2.12.0", "UNRELEASED", "../../docs/howto/is-this-image-deployed.md",
			"image-deployment.yaml", "replicaSetName", "replicaSetUID", "11 Kubernetes requests",
			"TestReleaseCheck(RunningImage|CLIAndMCP)$", "state.running",
		},
		"docs/reference/commands.md": {
			"Available since v2.11.0", "../howto/is-this-image-deployed.md",
			"UNRELEASED", "structured-selector Pod LIST", "workload-ownership-unsupported",
		},
		"docs/reference/cli-contract.md": {
			"v2.12.0", "../howto/is-this-image-deployed.md", "UNRELEASED",
			"replicaSetName", "replicaSetUID", "observedAt", "digest-form-unresolved",
		},
		"docs/reference/json-contracts.md": {
			"../howto/is-this-image-deployed.md", "v2.12.0", "UNRELEASED",
			"state.running", "replicaSetName", "replicaSetUID", "observedAt",
		},
		"docs/reference/cli-reference.md": {"v2.12.0", "Image deployment guide"},
		"docs/proposals/running-image-identity.md": {
			"UNRELEASED source-branch implementation", "../howto/is-this-image-deployed.md",
			"replicaSetName", "replicaSetUID", "state.running", "11 Kubernetes requests",
		},
	} {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", file))
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, snippet := range snippets {
				if !strings.Contains(content, snippet) {
					t.Errorf("missing image-check guidance %q", snippet)
				}
			}
			for _, stale := range []string{
				"never a false alarm", "Never a false alarm", "Unreleased, planned v2.11",
				"Configuration Release Check (Unreleased)",
			} {
				if strings.Contains(content, stale) {
					t.Errorf("stale or overconfident image-check guidance %q", stale)
				}
			}
		})
	}
}

func TestImageGuideSurfaceTableColumns(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "howto", "is-this-image-deployed.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, found := strings.Cut(string(data), "## Ways To Run It\n")
	if !found {
		t.Fatal("missing run surfaces")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "|") && strings.Count(line, "|") != 3 {
			t.Errorf("run-surface table must have two columns; avoid unescaped option pipes: %s", line)
		}
	}
}
