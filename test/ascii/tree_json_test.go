// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package ascii_test provides JSON golden tests for cub-scout CLI output.
//
// NOTE: These goldens lock the v0.14 JSON schema output.
// Changes here should be reviewed as API changes.
//
// To update goldens: go test ./test/ascii/... -update
package ascii_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/internal/mapsvc"
	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

// TestTreeOwnership_JSON tests the v0.14 JSON schema output for tree ownership.
func TestTreeOwnership_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)

	// Load test entries from JSON fixture
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "tree", "testdata", "ownership.json")
	data, err := os.ReadFile(fixtureAbs)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var entries []mapsvc.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	// Generate JSON output using the same function as the CLI
	opts := mapsvc.OwnershipTreeOpts{}
	output := mapsvc.BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	// Marshal with indent for readable golden
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}

	got := string(jsonBytes) + "\n"

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tree", "ownership.json")
	golden.AssertGolden(t, got, goldenPath)
}

// TestTreeOwnership_JSON_FilterOwner tests JSON output with owner filter.
func TestTreeOwnership_JSON_FilterOwner(t *testing.T) {
	repoRoot := runner.RepoRoot(t)

	// Load test entries from JSON fixture
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "tree", "testdata", "ownership.json")
	data, err := os.ReadFile(fixtureAbs)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var entries []mapsvc.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	// Generate JSON output with Flux filter
	opts := mapsvc.OwnershipTreeOpts{FilterOwner: "Flux"}
	output := mapsvc.BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	// Marshal with indent for readable golden
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}

	got := string(jsonBytes) + "\n"

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "tree", "ownership_filtered.json")
	golden.AssertGolden(t, got, goldenPath)
}

// TestTreeOwnership_JSON_MaxDepth tests JSON output with depth limit.
func TestTreeOwnership_JSON_MaxDepth(t *testing.T) {
	repoRoot := runner.RepoRoot(t)

	// Load test entries from JSON fixture
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "tree", "testdata", "ownership.json")
	data, err := os.ReadFile(fixtureAbs)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var entries []mapsvc.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	// Generate JSON output with depth limit
	opts := mapsvc.OwnershipTreeOpts{MaxDepth: 1}
	output := mapsvc.BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	// Verify JSON is lossless (contains ALL items)
	for _, group := range output.Groups {
		if len(group.Items) != group.DisplayMeta.TotalItems {
			t.Errorf("owner=%s: items count (%d) != totalItems (%d) - JSON should be lossless",
				group.Owner, len(group.Items), group.DisplayMeta.TotalItems)
		}
		// DisplayMeta.MaxItems should reflect the depth limit
		if group.DisplayMeta.TotalItems > 1 && group.DisplayMeta.MaxItems != 1 {
			t.Errorf("owner=%s: displayMeta.maxItems should be 1 (depth limit), got %d",
				group.Owner, group.DisplayMeta.MaxItems)
		}
	}
}

// TestTreeOwnership_JSON_Deterministic verifies that JSON output is deterministic.
func TestTreeOwnership_JSON_Deterministic(t *testing.T) {
	repoRoot := runner.RepoRoot(t)

	// Load test entries from JSON fixture
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "tree", "testdata", "ownership.json")
	data, err := os.ReadFile(fixtureAbs)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var entries []mapsvc.Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	opts := mapsvc.OwnershipTreeOpts{}

	// Generate JSON output twice
	output1 := mapsvc.BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)
	output2 := mapsvc.BuildOwnershipTreeJSON(entries, opts, "test-cluster", nil)

	json1, _ := json.MarshalIndent(output1, "", "  ")
	json2, _ := json.MarshalIndent(output2, "", "  ")

	if string(json1) != string(json2) {
		t.Error("JSON output is not deterministic - same input produced different output")
	}
}
