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

// fixedTime is a constant time used for deterministic testing.
var fixedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// TestAttributionGraph_Determinism verifies that building an attribution graph
// from the same input always produces byte-identical JSON output.
// This is a core semantic contract: deterministic output everywhere.
func TestAttributionGraph_Determinism(t *testing.T) {
	// Build a graph from fixed input
	graph := buildTestGraph()

	// Marshal to JSON multiple times
	first, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal graph: %v", err)
	}

	for i := 0; i < 10; i++ {
		next, err := json.MarshalIndent(graph, "", "  ")
		if err != nil {
			t.Fatalf("iteration %d: failed to marshal graph: %v", i, err)
		}
		if string(next) != string(first) {
			t.Fatalf("iteration %d: non-deterministic output\n--- first ---\n%s\n--- next ---\n%s",
				i, string(first), string(next))
		}
	}
}

// TestAttributionGraph_GoldenHash verifies the JSON output matches a recorded hash.
// This catches any semantic drift in graph construction.
func TestAttributionGraph_GoldenHash(t *testing.T) {
	graph := buildTestGraph()

	jsonBytes, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal graph: %v", err)
	}

	hash := sha256.Sum256(jsonBytes)
	got := hex.EncodeToString(hash[:])

	goldenPath := filepath.Join(testDir(t), "testdata", "attribution-graph.golden.sha256")
	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("missing golden hash %q: %v\n\nRun: GENERATE_SNAPSHOTS=1 go test ./test/contract/... -run TestGenerateSnapshots -v", goldenPath, err)
	}
	want := string(wantBytes)

	if got != want {
		t.Fatalf("GOLDEN HASH MISMATCH for attribution graph\n\nExpected: %s\nGot:      %s\n\n"+
			"If you intentionally changed attribution semantics, bump the schema version.\n"+
			"Otherwise, fix the regression.",
			want, got)
	}
}

// TestAttributionReport_Determinism verifies report construction is deterministic.
// Note: GeneratedAt timestamp is set to a fixed value since it's expected to vary.
func TestAttributionReport_Determinism(t *testing.T) {
	graph := buildTestGraph()
	report, err := agent.BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("failed to build report: %v", err)
	}
	// Use fixed timestamp for deterministic comparison
	report.GeneratedAt = fixedTime

	first, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	for i := 0; i < 10; i++ {
		report2, _ := agent.BuildAttributionReport(graph)
		report2.GeneratedAt = fixedTime
		next, err := json.MarshalIndent(report2, "", "  ")
		if err != nil {
			t.Fatalf("iteration %d: failed to marshal report: %v", i, err)
		}
		if string(next) != string(first) {
			t.Fatalf("iteration %d: non-deterministic report output", i)
		}
	}
}

// TestAttributionReport_GoldenHash verifies report output matches a recorded hash.
func TestAttributionReport_GoldenHash(t *testing.T) {
	graph := buildTestGraph()
	report, err := agent.BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("failed to build report: %v", err)
	}

	// Use fixed timestamp for deterministic comparison
	report.GeneratedAt = fixedTime

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	hash := sha256.Sum256(jsonBytes)
	got := hex.EncodeToString(hash[:])

	goldenPath := filepath.Join(testDir(t), "testdata", "attribution-report.golden.sha256")
	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("missing golden hash %q: %v\n\nRun: GENERATE_SNAPSHOTS=1 go test ./test/contract/... -run TestGenerateSnapshots -v", goldenPath, err)
	}
	want := string(wantBytes)

	if got != want {
		t.Fatalf("GOLDEN HASH MISMATCH for attribution report\n\nExpected: %s\nGot:      %s\n\n"+
			"If you intentionally changed attribution semantics, bump the schema version.\n"+
			"Otherwise, fix the regression.",
			want, got)
	}
}

// buildTestGraph creates a deterministic test graph for contract testing.
// This graph is LOCKED - do not change it without bumping schema versions.
func buildTestGraph() *agent.AttributionGraph {
	builder := agent.NewAttributionGraphBuilder()

	// Add nodes in a specific order (builder sorts them)
	builder.AddNode(agent.AttributionNode{
		Type:    agent.NodeClaim,
		Ref:     agent.AttributionRef{Kind: "PostgreSQLInstance", Name: "db", Namespace: "prod", UID: "uid-claim-001"},
		Present: true,
	})
	builder.AddNode(agent.AttributionNode{
		Type:    agent.NodeXR,
		Ref:     agent.AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-abc", UID: "uid-xr-001"},
		Present: true,
	})
	builder.AddNode(agent.AttributionNode{
		Type:    agent.NodeMR,
		Ref:     agent.AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system", UID: "uid-mr-001"},
		Present: true,
	})
	builder.AddNode(agent.AttributionNode{
		Type:    agent.NodeMR,
		Ref:     agent.AttributionRef{Kind: "SecurityGroup", Name: "sg-db", UID: "uid-mr-002"},
		Present: false,
	})

	// Add edges
	builder.AddEdge(agent.AttributionEdge{
		Type:     agent.EdgeOwns,
		From:     "claim:uid:uid-claim-001",
		To:       "xr:uid:uid-xr-001",
		Evidence: agent.EvidenceOwnerReference,
	})
	builder.AddEdge(agent.AttributionEdge{
		Type:     agent.EdgeOwns,
		From:     "xr:uid:uid-xr-001",
		To:       "mr:uid:uid-mr-001",
		Evidence: agent.EvidenceCompositeLabel,
	})
	builder.AddEdge(agent.AttributionEdge{
		Type:     agent.EdgeOwns,
		From:     "xr:uid:uid-xr-001",
		To:       "mr:uid:uid-mr-002",
		Evidence: agent.EvidenceCompositeLabel,
	})

	return builder.Build()
}
