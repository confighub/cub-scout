// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package runner provides helpers for running the cub-scout CLI in tests.
package runner

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Run executes the cub-scout binary with args and returns stdout.
// It uses `go run ./cmd/cub-scout` so no pre-built binary is needed.
// The workDir should be the repo root.
func Run(t *testing.T, workDir string, args ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", append([]string{"run", "./cmd/cub-scout"}, args...)...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Inherit environment but add test-specific vars
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		// Some commands exit non-zero intentionally
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			// Exit code 1: "not managed" (trace native resources) - per cli-contract.md
			// Exit code 4: pattern failures
			if code == 1 || code == 4 {
				return stdout.String()
			}
		}
		t.Fatalf("cub-scout %v failed: %v\nstdout:\n%s\nstderr:\n%s",
			args, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// RunWithEnv executes cub-scout with additional environment variables.
func RunWithEnv(t *testing.T, workDir string, env map[string]string, args ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", append([]string{"run", "./cmd/cub-scout"}, args...)...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Inherit environment and add custom vars
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if err := cmd.Run(); err != nil {
		// Some commands exit non-zero intentionally
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			// Exit code 1: "not managed" (trace native resources) - per cli-contract.md
			// Exit code 4: pattern failures
			if code == 1 || code == 4 {
				return stdout.String()
			}
		}
		t.Fatalf("cub-scout %v failed: %v\nstdout:\n%s\nstderr:\n%s",
			args, err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

// RepoRoot finds the repository root by walking up to find go.mod.
func RepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}
