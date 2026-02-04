// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

// fixedTimeGen must match fixedTime in attribution_determinism_test.go
var fixedTimeGen = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// TestGenerateSnapshots is a helper test that generates snapshot files.
// Run with: GENERATE_SNAPSHOTS=1 go test ./test/contract/... -run TestGenerateSnapshots -v
// This is NOT run in normal test mode - it's a manual tool.
func TestGenerateSnapshots(t *testing.T) {
	if os.Getenv("GENERATE_SNAPSHOTS") != "1" {
		t.Skip("Set GENERATE_SNAPSHOTS=1 to run this test")
	}

	dir := testDir(t)
	testdataDir := filepath.Join(dir, "testdata")
	schemaDir := filepath.Join(testdataDir, "schema")
	if err := os.MkdirAll(schemaDir, 0755); err != nil {
		t.Fatalf("failed to create schema dir: %v", err)
	}

	// Generate schema snapshots
	snapshots := []struct {
		name string
		typ  any
		file string
	}{
		{"attribution-graph.v1", agent.AttributionGraph{}, "attribution-graph.v1.snapshot.txt"},
		{"attribution-report.v1", agent.AttributionReport{}, "attribution-report.v1.snapshot.txt"},
	}

	for _, s := range snapshots {
		content := snapshotStruct(s.typ)
		path := filepath.Join(schemaDir, s.file)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write snapshot %s: %v", s.file, err)
		}
		t.Logf("wrote %s", path)
	}

	// Generate golden hashes for attribution graph
	graph := buildTestGraph()
	graphJSON, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal graph: %v", err)
	}
	graphHash := sha256.Sum256(graphJSON)
	graphHashPath := filepath.Join(testdataDir, "attribution-graph.golden.sha256")
	if err := os.WriteFile(graphHashPath, []byte(hex.EncodeToString(graphHash[:])), 0644); err != nil {
		t.Fatalf("failed to write graph hash: %v", err)
	}
	t.Logf("wrote %s", graphHashPath)

	// Generate golden hashes for attribution report
	report, err := agent.BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("failed to build report: %v", err)
	}
	report.GeneratedAt = fixedTimeGen
	reportJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}
	reportHash := sha256.Sum256(reportJSON)
	reportHashPath := filepath.Join(testdataDir, "attribution-report.golden.sha256")
	if err := os.WriteFile(reportHashPath, []byte(hex.EncodeToString(reportHash[:])), 0644); err != nil {
		t.Fatalf("failed to write report hash: %v", err)
	}
	t.Logf("wrote %s", reportHashPath)
}
