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

func TestEvalRecordedScalePacketGuards(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-m", "unittest", "discover", "-s", "evals/recorded-scale", "-p", "test_*.py")
	cmd.Dir = filepath.Join("..", "..")
	// Permit Python's owned-process cleanup before escalating a stuck test runner.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 3 * time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recorded scale guards: %v\n%s", err, out)
	}
}

func TestINV04CaptureGuards(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-m", "unittest", "discover", "-s", "evals/inv04-rbac", "-p", "test_*.py")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	// Permit Python's owned-process cleanup before escalating a stuck test runner.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 3 * time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("INV-04 capture guards: %v\n%s", err, out)
	}
}
