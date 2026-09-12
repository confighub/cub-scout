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
		"docs/reference/json-contracts.md":     {"Configuration Release Check (Unreleased)", "controllerContext", "registry", "NOT_ASSESSED", "not the outer", "runningImage"},
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
