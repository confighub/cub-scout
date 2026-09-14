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
		"README.md":                            {"Did this exact OCI configuration release reach the selected cluster?", "Where is my release waiting, different, or unverifiable?", "Configuration-only changes", "release check --interactive", "release_check", "no inventory LIST", "Watch/bot do not schedule this check yet", "Is the image actually running the one I intended?", "check-running-image"},
		"examples/oci-release-check/README.md": {"--bundle", "--kube-context", "--controller-context", "2N + 4", "2N + 8", "32 MiB", "16 MiB", "4 MiB", "not an atomic snapshot", "NOT_ASSESSED", "No source index is guessed", "check-running-image"},
		"docs/reference/json-contracts.md":     {"## Configuration Release Check", "Available since v2.11.0", "controllerContext", "registry", "NOT_ASSESSED", "not the outer", "runningImage"},
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
		"README.md": {"[Is this image deployed?](docs/howto/is-this-image-deployed.md)", "I only have an image reference", "missing per-pod status"},
		"docs/howto/is-this-image-deployed.md": {
			"v2.11.0", "## Five Different Questions", "## Start With What You Have",
			"## Why OCI Is Involved", "## Run The Check", "## Read The Result",
			"## Ways To Run It", "## Limits Before Trusting A Match",
			"--bundle", "--controller", "--api-version", "--controller-namespace",
			"--kube-context", "--check-running-image", "--max-pods 50", "--format json",
			"--out release-check.json", "--fail-on any-non-pass", "--interactive",
			"cub scout release check", "check_running_image: true",
			"configuration-bundle digest is never compared to a container-image",
			"no dedicated image-reference search", "state.running", "ownerReference/UID",
			"missing status arrays", "matchExpressions", "initContainers", "ephemeral",
			"index/platform", "`mismatch`/`BLOCK`", "NOT_ASSESSED",
			"Do not schedule", "not signed or an immutable", "not problems fixed by this guide",
		},
		"CLI-GUIDE.md":                             {"Available since v2.11.0", "docs/howto/is-this-image-deployed.md"},
		"docs/getting-started/start-here.md":       {"../howto/is-this-image-deployed.md"},
		"examples/oci-release-check/README.md":     {"Available since v2.11.0", "../../docs/howto/is-this-image-deployed.md", "state.running"},
		"docs/reference/commands.md":               {"Available since v2.11.0", "../howto/is-this-image-deployed.md", "all-replicas-running"},
		"docs/reference/cli-contract.md":           {"Available since v2.11.0", "../howto/is-this-image-deployed.md", "state.running"},
		"docs/reference/json-contracts.md":         {"../howto/is-this-image-deployed.md", "state.running"},
		"docs/releases/v2.11.0.md":                 {"../howto/is-this-image-deployed.md", "index/platform", "per-pod status completeness"},
		"docs/proposals/running-image-identity.md": {"shipped in v2.11.0", "../howto/is-this-image-deployed.md", "state.running"},
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
			for _, stale := range []string{"never a false alarm", "Never a false alarm", "Unreleased, planned v2.11", "Configuration Release Check (Unreleased)"} {
				if strings.Contains(content, stale) {
					t.Errorf("stale or overconfident image-check guidance %q", stale)
				}
			}
		})
	}
}
