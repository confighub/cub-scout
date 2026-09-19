// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The shell suite uses fake observer/plugin executables and never contacts a
// cluster. Keep the operator acceptance gate in the normal regression suite.
func TestImageLiveAcceptanceHarness(t *testing.T) {
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required for the shell acceptance contract", tool)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", filepath.Join("..", "..", "examples", "oci-release-check", "validate-live_test.sh"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("live acceptance harness contract: %v\n%s", err, output)
	}
}
