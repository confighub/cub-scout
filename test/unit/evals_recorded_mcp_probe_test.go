// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestEvalRecordedMCPPreparationGuards(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-m", "unittest", "discover", "-s", "evals/recorded-mcp-probe", "-p", "test_*.py")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("preparation guards: %v\n%s", err, out)
	}
	if node, err := exec.LookPath("node"); err == nil {
		cmd := exec.CommandContext(ctx, python, "evals/recorded-mcp-probe/template/plugin/grader_contract.py")
		cmd.Dir = filepath.Join("..", "..")
		cmd.Env = append(os.Environ(), "NODE_BIN="+node)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("answer grader contract: %v\n%s", err, out)
		}
	} else {
		t.Log("node unavailable: paired Python/JavaScript regex check not run")
	}
}
