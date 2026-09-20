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
			"v2.12.1",
			"no inventory LIST",
			"Watch/bot do not schedule this check yet",
			"active ConfigHub registry",
			"Can a script, CI job or agent check this without a TUI?",
			"Can I reproduce the image check against real controllers, not just fixtures?",
			"Can a stalled credential helper hang my unattended image check?",
			"Can a large API error bypass the response-size limit?",
		},
		"examples/oci-release-check/README.md": {
			"image-deployment.yaml", "--kube-context", "--controller-context",
			"11 Kubernetes requests", "2N + 4", "2N + 8", "2R + 3", "32 MiB", "16 MiB",
			"4 MiB", "not an atomic snapshot", "NOT_ASSESSED", "No source index is guessed",
			"v2.12.1", "check-running-image", "TestReleaseCheck(RunningImage|CLIAndMCP)$",
		},
		"docs/reference/json-contracts.md": {
			"## Configuration Release Check", "v2.12.0", "v2.12.1",
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
			"What does incomplete proof mean?", "v2.12.1",
		},
		"docs/howto/is-this-image-deployed.md": {
			"v2.12.0", "v2.12.1", "## Start With The Identity", "## Run The Check",
			"## Interpret The Result", "## Ways To Run It",
			"--bundle", "--controller", "--api-version", "--controller-namespace",
			"--kube-context", "--check-running-image", "--max-pods 50", "--format json",
			"--out release-check.json", "--fail-on any-non-pass", "--interactive",
			"cub scout release check", "check_running_image: true",
			"Release.ManifestDigest", "Release.Digest", "imageID",
			"Deployment", "StatefulSet", "DaemonSet", "Job",
			"PASS", "BLOCK", "INCONCLUSIVE", "Windows", "--controller-context",
			"../reference/json-contracts.md", "../releases/image-verification-readiness.md",
		},
		"CLI-GUIDE.md": {
			"Available since v2.11.0", "v2.12.0", "v2.12.1", "docs/howto/is-this-image-deployed.md",
		},
		"docs/getting-started/start-here.md": {
			"../howto/is-this-image-deployed.md", "v2.12.0", "v2.12.1",
		},
		"examples/oci-release-check/README.md": {
			"v2.12.0", "v2.12.1", "../../docs/howto/is-this-image-deployed.md",
			"image-deployment.yaml", "replicaSetName", "replicaSetUID", "11 Kubernetes requests",
			"TestReleaseCheck(RunningImage|CLIAndMCP)$", "state.running",
		},
		"docs/reference/commands.md": {
			"Available since v2.11.0", "../howto/is-this-image-deployed.md",
			"v2.12.1", "structured-selector Pod LIST", "workload-ownership-unsupported",
		},
		"docs/reference/cli-contract.md": {
			"v2.12.0", "../howto/is-this-image-deployed.md", "v2.12.1",
			"replicaSetName", "replicaSetUID", "observedAt", "digest-form-unresolved",
		},
		"docs/reference/json-contracts.md": {
			"../howto/is-this-image-deployed.md", "v2.12.0", "v2.12.1",
			"state.running", "replicaSetName", "replicaSetUID", "observedAt",
		},
		"docs/reference/cli-reference.md": {"v2.12.0", "Image deployment guide"},
		"docs/proposals/running-image-identity.md": {
			"v2.12.1", "../howto/is-this-image-deployed.md",
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
