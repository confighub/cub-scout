// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// TestSmoke_CLIHelp verifies that basic CLI commands work.
// These tests run without a Kubernetes cluster.
func TestSmoke_CLIHelp(t *testing.T) {
	// Build the binary first if needed
	if err := exec.Command("go", "build", "-o", "cub-scout-test", ".").Run(); err != nil {
		t.Fatalf("Failed to build binary: %v", err)
	}

	tests := []struct {
		name           string
		args           []string
		wantExitCode   int
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:         "root help",
			args:         []string{"--help"},
			wantExitCode: 0,
			wantContains: []string{"Usage:", "cub-scout", "Available Commands"},
		},
		{
			name:         "version",
			args:         []string{"version"},
			wantExitCode: 0,
			wantContains: []string{"cub-scout version"},
		},
		{
			name:         "map help",
			args:         []string{"map", "--help"},
			wantExitCode: 0,
			wantContains: []string{"map", "list", "Usage:"},
		},
		{
			name:         "map list help",
			args:         []string{"map", "list", "--help"},
			wantExitCode: 0,
			wantContains: []string{"list", "Usage:"},
		},
		{
			name:         "scan help",
			args:         []string{"scan", "--help"},
			wantExitCode: 0,
			wantContains: []string{"scan", "Usage:"},
		},
		{
			name:         "trace help",
			args:         []string{"trace", "--help"},
			wantExitCode: 0,
			wantContains: []string{"trace", "Usage:"},
		},
		{
			name:         "trace help has history flag",
			args:         []string{"trace", "--help"},
			wantExitCode: 0,
			wantContains: []string{"--history", "deployment history"},
		},
		{
			name:         "status help",
			args:         []string{"status", "--help"},
			wantExitCode: 0,
			wantContains: []string{"status", "connection", "Usage:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("./cub-scout-test", tt.args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			// Check exit code
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					t.Fatalf("Failed to run command: %v", err)
				}
			}

			if exitCode != tt.wantExitCode {
				t.Errorf("Exit code = %d, want %d\nStderr: %s", exitCode, tt.wantExitCode, stderr.String())
			}

			// Check output contains expected strings
			output := stdout.String() + stderr.String()
			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("Output missing %q\nGot: %s", want, output)
				}
			}

			// Check output doesn't contain unwanted strings
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(output, notWant) {
					t.Errorf("Output should not contain %q\nGot: %s", notWant, output)
				}
			}
		})
	}

	// Clean up test binary
	exec.Command("rm", "-f", "cub-scout-test").Run()
}

// TestSmoke_RootCommand verifies the root command behavior.
func TestSmoke_RootCommand(t *testing.T) {
	// Test that rootCmd exists and has expected structure
	if rootCmd == nil {
		t.Fatal("rootCmd is nil")
	}

	if rootCmd.Use != "cub-scout" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "cub-scout")
	}

	// Verify key subcommands exist
	expectedCmds := []string{"version", "completion", "map", "scan", "trace", "status"}
	for _, name := range expectedCmds {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Missing expected subcommand: %s", name)
		}
	}
}
