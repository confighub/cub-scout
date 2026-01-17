// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewImportLogger(t *testing.T) {
	// Clean up any existing test logs
	os.RemoveAll(".confighub")
	defer os.RemoveAll(".confighub")

	logger, err := NewImportLogger("test")
	if err != nil {
		t.Fatalf("NewImportLogger failed: %v", err)
	}

	// Log some entries
	logger.Log("Test message %d", 1)
	logger.Section("TEST SECTION")
	logger.Log("Another message")

	// Close and get path
	logPath := logger.Close()

	if logPath == "" {
		t.Fatal("Expected log path, got empty string")
	}

	if !strings.HasPrefix(logPath, ".confighub/logs/test-") {
		t.Errorf("Unexpected log path: %s", logPath)
	}

	// Verify file exists and has content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	// Check header
	if !strings.Contains(contentStr, "ConfigHub Agent: test") {
		t.Error("Missing header in log")
	}

	// Check logged messages
	if !strings.Contains(contentStr, "Test message 1") {
		t.Error("Missing 'Test message 1' in log")
	}

	if !strings.Contains(contentStr, "--- TEST SECTION ---") {
		t.Error("Missing section header in log")
	}

	if !strings.Contains(contentStr, "Another message") {
		t.Error("Missing 'Another message' in log")
	}

	// Check footer
	if !strings.Contains(contentStr, "Completed:") {
		t.Error("Missing completion timestamp in log")
	}
}

func TestLogWorkloads(t *testing.T) {
	os.RemoveAll(".confighub")
	defer os.RemoveAll(".confighub")

	logger, err := NewImportLogger("workloads-test")
	if err != nil {
		t.Fatalf("NewImportLogger failed: %v", err)
	}

	workloads := []WorkloadInfo{
		{Namespace: "default", Name: "nginx", Kind: "Deployment", Owner: "Flux", Ready: true},
		{Namespace: "monitoring", Name: "prometheus", Kind: "StatefulSet", Owner: "Helm", Ready: false},
	}

	logger.LogWorkloads(workloads)
	logPath := logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log: %v", err)
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, "Found 2 workloads") {
		t.Error("Missing workload count")
	}

	if !strings.Contains(contentStr, "default/nginx") {
		t.Error("Missing nginx workload")
	}

	if !strings.Contains(contentStr, "owner=Flux") {
		t.Error("Missing Flux owner")
	}
}

func TestLogProposal(t *testing.T) {
	os.RemoveAll(".confighub")
	defer os.RemoveAll(".confighub")

	logger, err := NewImportLogger("proposal-test")
	if err != nil {
		t.Fatalf("NewImportLogger failed: %v", err)
	}

	proposal := &FullProposal{
		AppSpace: "my-team",
		Units: []UnitProposal{
			{
				Slug:      "nginx-prod",
				App:       "nginx",
				Variant:   "prod",
				Labels:    map[string]string{"app": "nginx", "variant": "prod"},
				Workloads: []string{"default/nginx"},
			},
		},
	}

	logger.LogProposal(proposal)
	logPath := logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log: %v", err)
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, "App Space: my-team") {
		t.Error("Missing App Space")
	}

	if !strings.Contains(contentStr, "nginx-prod") {
		t.Error("Missing unit slug")
	}

	if !strings.Contains(contentStr, "app=nginx") {
		t.Error("Missing app label")
	}
}

func TestLogResult(t *testing.T) {
	os.RemoveAll(".confighub")
	defer os.RemoveAll(".confighub")

	logger, err := NewImportLogger("result-test")
	if err != nil {
		t.Fatalf("NewImportLogger failed: %v", err)
	}

	logger.LogResult(5, 2, nil)
	logPath := logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log: %v", err)
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, "Created: 5") {
		t.Error("Missing created count")
	}

	if !strings.Contains(contentStr, "Failed: 2") {
		t.Error("Missing failed count")
	}
}

func TestLogDirectoryCreation(t *testing.T) {
	// Ensure directory doesn't exist
	os.RemoveAll(".confighub")
	defer os.RemoveAll(".confighub")

	// Logger should create directory
	logger, err := NewImportLogger("dir-test")
	if err != nil {
		t.Fatalf("NewImportLogger failed: %v", err)
	}
	logger.Close()

	// Verify directory was created
	info, err := os.Stat(".confighub/logs")
	if err != nil {
		t.Fatalf("Log directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("Expected .confighub/logs to be a directory")
	}
}

func TestNilLoggerSafety(t *testing.T) {
	// All methods should be safe to call on nil logger
	var logger *ImportLogger

	// These should not panic
	logger.Log("test")
	logger.Section("test")
	logger.LogWorkloads(nil)
	logger.LogProposal(nil)
	logger.LogResult(0, 0, nil)
	path := logger.Close()

	if path != "" {
		t.Errorf("Expected empty path from nil logger, got: %s", path)
	}
}

func TestLogFileNaming(t *testing.T) {
	os.RemoveAll(".confighub")
	defer os.RemoveAll(".confighub")

	// Create two loggers with different commands
	logger1, _ := NewImportLogger("import")
	logger2, _ := NewImportLogger("apply")

	path1 := logger1.Close()
	path2 := logger2.Close()

	// Verify naming pattern
	if !strings.Contains(filepath.Base(path1), "import-") {
		t.Errorf("Import log should contain 'import-': %s", path1)
	}

	if !strings.Contains(filepath.Base(path2), "apply-") {
		t.Errorf("Apply log should contain 'apply-': %s", path2)
	}
}
