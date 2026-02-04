// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
)

// TestBuildAttributionReport_OwnedViaOwnerRef tests ownership via ownerReference.
// This is the strongest signal (score=100).
func TestBuildAttributionReport_OwnedViaOwnerRef(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{
				ID:      "mr:ref:/Instance:crossplane-system/staging-db",
				Type:    NodeMR,
				Ref:     AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:xpostgresql-staging",
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
				Present: true,
			},
		},
		Edges: []AttributionEdge{
			{
				ID:       "edge:ownerref-xr-to-mr",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:xpostgresql-staging",
				To:       "mr:ref:/Instance:crossplane-system/staging-db",
				Evidence: EvidenceOwnerReference,
			},
		},
		Summary: AttributionSummary{
			NodesByType: map[AttributionNodeType]int{NodeMR: 1, NodeXR: 1},
			EdgesByType: map[AttributionEdgeType]int{EdgeOwns: 1},
		},
	}

	report, err := BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("BuildAttributionReport failed: %v", err)
	}

	if report.SchemaVersion != AttributionReportSchemaVersion {
		t.Errorf("SchemaVersion = %s, want %s", report.SchemaVersion, AttributionReportSchemaVersion)
	}

	if len(report.Items) != 1 {
		t.Fatalf("Items count = %d, want 1", len(report.Items))
	}

	item := report.Items[0]
	if item.Reason != ReasonOwnedViaOwnerRef {
		t.Errorf("Reason = %s, want %s", item.Reason, ReasonOwnedViaOwnerRef)
	}
	if item.BestOwner == nil {
		t.Fatal("BestOwner should not be nil")
	}
	if item.BestOwner.Score != ScoreOwnerReference {
		t.Errorf("BestOwner.Score = %d, want %d", item.BestOwner.Score, ScoreOwnerReference)
	}
	if item.BestOwner.Evidence != EvidenceOwnerReference {
		t.Errorf("BestOwner.Evidence = %s, want %s", item.BestOwner.Evidence, EvidenceOwnerReference)
	}

	// Summary check
	if report.Summary.OwnedCount != 1 {
		t.Errorf("Summary.OwnedCount = %d, want 1", report.Summary.OwnedCount)
	}
	if report.Summary.UnattributedCount != 0 {
		t.Errorf("Summary.UnattributedCount = %d, want 0", report.Summary.UnattributedCount)
	}
}

// TestBuildAttributionReport_OwnedViaLabel tests ownership via composite label.
// This is a medium signal (score=80).
func TestBuildAttributionReport_OwnedViaLabel(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{
				ID:      "mr:ref:/Instance:crossplane-system/staging-db",
				Type:    NodeMR,
				Ref:     AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:xpostgresql-staging",
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
				Present: false, // XR not found, only label
			},
		},
		Edges: []AttributionEdge{
			{
				ID:       "edge:label-xr-to-mr",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:xpostgresql-staging",
				To:       "mr:ref:/Instance:crossplane-system/staging-db",
				Evidence: EvidenceCompositeLabel,
			},
		},
		Summary: AttributionSummary{
			NodesByType: map[AttributionNodeType]int{NodeMR: 1, NodeXR: 1},
			EdgesByType: map[AttributionEdgeType]int{EdgeOwns: 1},
		},
	}

	report, err := BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("BuildAttributionReport failed: %v", err)
	}

	if len(report.Items) != 1 {
		t.Fatalf("Items count = %d, want 1", len(report.Items))
	}

	item := report.Items[0]
	if item.Reason != ReasonOwnedViaLabel {
		t.Errorf("Reason = %s, want %s", item.Reason, ReasonOwnedViaLabel)
	}
	if item.BestOwner == nil {
		t.Fatal("BestOwner should not be nil")
	}
	if item.BestOwner.Score != ScoreCompositeLabel {
		t.Errorf("BestOwner.Score = %d, want %d", item.BestOwner.Score, ScoreCompositeLabel)
	}
}

// TestBuildAttributionReport_Unattributed tests objects without any owner.
func TestBuildAttributionReport_Unattributed(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{
				ID:      "mr:ref:/Instance:crossplane-system/orphan-db",
				Type:    NodeMR,
				Ref:     AttributionRef{Kind: "Instance", Name: "orphan-db", Namespace: "crossplane-system"},
				Present: true,
			},
		},
		Edges:   []AttributionEdge{}, // No edges
		Summary: AttributionSummary{
			NodesByType:       map[AttributionNodeType]int{NodeMR: 1},
			UnattributedCount: 1,
		},
	}

	report, err := BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("BuildAttributionReport failed: %v", err)
	}

	if len(report.Items) != 1 {
		t.Fatalf("Items count = %d, want 1", len(report.Items))
	}

	item := report.Items[0]
	if item.Reason != ReasonUnattributed {
		t.Errorf("Reason = %s, want %s", item.Reason, ReasonUnattributed)
	}
	if item.BestOwner != nil {
		t.Errorf("BestOwner should be nil for unattributed, got %+v", item.BestOwner)
	}
	if len(item.OwnerCandidates) != 0 {
		t.Errorf("OwnerCandidates should be empty, got %d", len(item.OwnerCandidates))
	}

	// Summary check
	if report.Summary.UnattributedCount != 1 {
		t.Errorf("Summary.UnattributedCount = %d, want 1", report.Summary.UnattributedCount)
	}
}

// TestBuildAttributionReport_Ambiguous tests objects with multiple equally-scored owners.
func TestBuildAttributionReport_Ambiguous(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{
				ID:      "mr:ref:/Instance:crossplane-system/contested-db",
				Type:    NodeMR,
				Ref:     AttributionRef{Kind: "Instance", Name: "contested-db", Namespace: "crossplane-system"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:xpostgresql-alpha",
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-alpha"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:xpostgresql-beta",
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-beta"},
				Present: true,
			},
		},
		Edges: []AttributionEdge{
			{
				ID:       "edge:label-alpha-to-mr",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:xpostgresql-alpha",
				To:       "mr:ref:/Instance:crossplane-system/contested-db",
				Evidence: EvidenceCompositeLabel, // Same evidence type = same score
			},
			{
				ID:       "edge:label-beta-to-mr",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:xpostgresql-beta",
				To:       "mr:ref:/Instance:crossplane-system/contested-db",
				Evidence: EvidenceCompositeLabel, // Same evidence type = same score
			},
		},
		Summary: AttributionSummary{
			NodesByType:    map[AttributionNodeType]int{NodeMR: 1, NodeXR: 2},
			EdgesByType:    map[AttributionEdgeType]int{EdgeOwns: 2},
			AmbiguousCount: 1,
		},
	}

	report, err := BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("BuildAttributionReport failed: %v", err)
	}

	if len(report.Items) != 1 {
		t.Fatalf("Items count = %d, want 1", len(report.Items))
	}

	item := report.Items[0]
	if item.Reason != ReasonAmbiguous {
		t.Errorf("Reason = %s, want %s", item.Reason, ReasonAmbiguous)
	}
	if len(item.OwnerCandidates) != 2 {
		t.Errorf("OwnerCandidates count = %d, want 2", len(item.OwnerCandidates))
	}
	// Best owner should still be set (deterministic tie-break)
	if item.BestOwner == nil {
		t.Fatal("BestOwner should not be nil even for ambiguous")
	}

	// Summary check
	if report.Summary.AmbiguousCount != 1 {
		t.Errorf("Summary.AmbiguousCount = %d, want 1", report.Summary.AmbiguousCount)
	}
}

// TestBuildAttributionReport_TieBreak tests deterministic ordering when scores tie.
// The winner should be determined alphabetically by ref.
func TestBuildAttributionReport_TieBreak(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{
				ID:      "mr:ref:/Instance:crossplane-system/test-db",
				Type:    NodeMR,
				Ref:     AttributionRef{Kind: "Instance", Name: "test-db", Namespace: "crossplane-system"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:zebra-xr", // Alphabetically later
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "zebra-xr"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:alpha-xr", // Alphabetically earlier
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "alpha-xr"},
				Present: true,
			},
		},
		Edges: []AttributionEdge{
			{
				ID:       "edge:label-zebra-to-mr",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:zebra-xr",
				To:       "mr:ref:/Instance:crossplane-system/test-db",
				Evidence: EvidenceCompositeLabel,
			},
			{
				ID:       "edge:label-alpha-to-mr",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:alpha-xr",
				To:       "mr:ref:/Instance:crossplane-system/test-db",
				Evidence: EvidenceCompositeLabel,
			},
		},
		Summary: AttributionSummary{
			NodesByType: map[AttributionNodeType]int{NodeMR: 1, NodeXR: 2},
			EdgesByType: map[AttributionEdgeType]int{EdgeOwns: 2},
		},
	}

	// Run multiple times to verify determinism
	for i := 0; i < 5; i++ {
		report, err := BuildAttributionReport(graph)
		if err != nil {
			t.Fatalf("BuildAttributionReport failed: %v", err)
		}

		item := report.Items[0]

		// "alpha-xr" should always win because it's alphabetically first
		if item.BestOwner == nil {
			t.Fatal("BestOwner should not be nil")
		}
		if item.BestOwner.Ref.Name != "alpha-xr" {
			t.Errorf("Iteration %d: BestOwner.Name = %s, want alpha-xr (deterministic tie-break)",
				i, item.BestOwner.Ref.Name)
		}

		// Candidates should also be in deterministic order
		if len(item.OwnerCandidates) != 2 {
			t.Fatalf("Iteration %d: OwnerCandidates count = %d, want 2", i, len(item.OwnerCandidates))
		}
		if item.OwnerCandidates[0].Ref.Name != "alpha-xr" {
			t.Errorf("Iteration %d: First candidate = %s, want alpha-xr", i, item.OwnerCandidates[0].Ref.Name)
		}
		if item.OwnerCandidates[1].Ref.Name != "zebra-xr" {
			t.Errorf("Iteration %d: Second candidate = %s, want zebra-xr", i, item.OwnerCandidates[1].Ref.Name)
		}
	}
}

// TestBuildAttributionReport_Nil tests nil graph input.
func TestBuildAttributionReport_Nil(t *testing.T) {
	report, err := BuildAttributionReport(nil)
	if err != nil {
		t.Errorf("Expected no error for nil graph, got %v", err)
	}
	if report != nil {
		t.Errorf("Expected nil report for nil graph, got %+v", report)
	}
}

// TestBuildAttributionReport_EmptyGraph tests empty graph input.
func TestBuildAttributionReport_EmptyGraph(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes:         []AttributionNode{},
		Edges:         []AttributionEdge{},
		Summary:       AttributionSummary{},
	}

	report, err := BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("BuildAttributionReport failed: %v", err)
	}

	if report == nil {
		t.Fatal("Report should not be nil for empty graph")
	}
	if len(report.Items) != 0 {
		t.Errorf("Items count = %d, want 0", len(report.Items))
	}
	if report.Summary.TotalItems != 0 {
		t.Errorf("Summary.TotalItems = %d, want 0", report.Summary.TotalItems)
	}
}

// TestScoreForEvidence tests the scoring function.
func TestScoreForEvidence(t *testing.T) {
	tests := []struct {
		evidence AttributionEvidence
		want     int
	}{
		{EvidenceOwnerReference, ScoreOwnerReference},
		{EvidenceCompositeLabel, ScoreCompositeLabel},
		{EvidenceKustomizeOverlay, ScoreKustomizeOverlay},
		{EvidenceClaimLabel, ScoreClaimLabel},
		{AttributionEvidence("unknown"), 10}, // Unknown gets low score
	}

	for _, tt := range tests {
		t.Run(string(tt.evidence), func(t *testing.T) {
			got := scoreForEvidence(tt.evidence)
			if got != tt.want {
				t.Errorf("scoreForEvidence(%s) = %d, want %d", tt.evidence, got, tt.want)
			}
		})
	}
}

// TestBuildAttributionReport_ScorePriority tests that owner_reference beats label.
func TestBuildAttributionReport_ScorePriority(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{
				ID:      "mr:ref:/Instance:crossplane-system/test-db",
				Type:    NodeMR,
				Ref:     AttributionRef{Kind: "Instance", Name: "test-db", Namespace: "crossplane-system"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:weak-owner", // via label only
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "weak-owner"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:strong-owner", // via ownerRef
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "strong-owner"},
				Present: true,
			},
		},
		Edges: []AttributionEdge{
			{
				ID:       "edge:label-weak",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:weak-owner",
				To:       "mr:ref:/Instance:crossplane-system/test-db",
				Evidence: EvidenceCompositeLabel, // score=80
			},
			{
				ID:       "edge:ownerref-strong",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:strong-owner",
				To:       "mr:ref:/Instance:crossplane-system/test-db",
				Evidence: EvidenceOwnerReference, // score=100
			},
		},
		Summary: AttributionSummary{
			NodesByType: map[AttributionNodeType]int{NodeMR: 1, NodeXR: 2},
			EdgesByType: map[AttributionEdgeType]int{EdgeOwns: 2},
		},
	}

	report, err := BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("BuildAttributionReport failed: %v", err)
	}

	item := report.Items[0]

	// strong-owner should win due to higher score
	if item.BestOwner == nil {
		t.Fatal("BestOwner should not be nil")
	}
	if item.BestOwner.Ref.Name != "strong-owner" {
		t.Errorf("BestOwner.Name = %s, want strong-owner", item.BestOwner.Ref.Name)
	}
	if item.BestOwner.Score != ScoreOwnerReference {
		t.Errorf("BestOwner.Score = %d, want %d", item.BestOwner.Score, ScoreOwnerReference)
	}

	// Reason should be owned_via_owner_ref
	if item.Reason != ReasonOwnedViaOwnerRef {
		t.Errorf("Reason = %s, want %s", item.Reason, ReasonOwnedViaOwnerRef)
	}

	// Should not be ambiguous since scores differ
	if report.Summary.AmbiguousCount != 0 {
		t.Errorf("Summary.AmbiguousCount = %d, want 0", report.Summary.AmbiguousCount)
	}
}

// TestBuildAttributionReport_OwnedViaKustomize tests ownership via kustomize overlay.
func TestBuildAttributionReport_OwnedViaKustomize(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{
				ID:      "k8s_object:ref:/Deployment:production/api-server",
				Type:    NodeK8sObject,
				Ref:     AttributionRef{Kind: "Deployment", Name: "api-server", Namespace: "production"},
				Present: true,
			},
			{
				ID:      "kustomize_overlay:ref:kustomize/overlays/prod",
				Type:    NodeKustomizeOverlay,
				Ref:     AttributionRef{Kind: "kustomize", Name: "overlays/prod"},
				Present: true,
			},
		},
		Edges: []AttributionEdge{
			{
				ID:       "edge:kustomize-overlay",
				Type:     EdgeOwns,
				From:     "kustomize_overlay:ref:kustomize/overlays/prod",
				To:       "k8s_object:ref:/Deployment:production/api-server",
				Evidence: EvidenceKustomizeOverlay,
			},
		},
		Summary: AttributionSummary{
			NodesByType: map[AttributionNodeType]int{NodeK8sObject: 1, NodeKustomizeOverlay: 1},
			EdgesByType: map[AttributionEdgeType]int{EdgeOwns: 1},
		},
	}

	report, err := BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("BuildAttributionReport failed: %v", err)
	}

	// Report should process k8s_object nodes as targets
	if len(report.Items) != 1 {
		t.Fatalf("Items count = %d, want 1", len(report.Items))
	}

	item := report.Items[0]

	// Should be owned via kustomize
	if item.Reason != ReasonOwnedViaKustomize {
		t.Errorf("Reason = %s, want %s", item.Reason, ReasonOwnedViaKustomize)
	}
	if item.BestOwner == nil {
		t.Fatal("BestOwner should not be nil")
	}
	if item.BestOwner.Score != ScoreKustomizeOverlay {
		t.Errorf("BestOwner.Score = %d, want %d", item.BestOwner.Score, ScoreKustomizeOverlay)
	}
	if item.BestOwner.Evidence != EvidenceKustomizeOverlay {
		t.Errorf("BestOwner.Evidence = %s, want %s", item.BestOwner.Evidence, EvidenceKustomizeOverlay)
	}
}

// TestBuildAttributionReport_CrossplaneOwnerRefBeatsKustomizeOverlay tests priority ordering.
func TestBuildAttributionReport_CrossplaneOwnerRefBeatsKustomizeOverlay(t *testing.T) {
	graph := &AttributionGraph{
		SchemaVersion: AttributionGraphSchemaVersion,
		Nodes: []AttributionNode{
			{
				ID:      "mr:ref:/Instance:crossplane-system/staging-db",
				Type:    NodeMR,
				Ref:     AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
				Present: true,
			},
			{
				ID:      "xr:ref:/XPostgreSQLInstance:xpostgresql-staging",
				Type:    NodeXR,
				Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
				Present: true,
			},
			{
				ID:      "kustomize_overlay:ref:kustomize/overlays/prod",
				Type:    NodeKustomizeOverlay,
				Ref:     AttributionRef{Kind: "kustomize", Name: "overlays/prod"},
				Present: true,
			},
		},
		Edges: []AttributionEdge{
			{
				ID:       "edge:ownerref-xr-to-mr",
				Type:     EdgeOwns,
				From:     "xr:ref:/XPostgreSQLInstance:xpostgresql-staging",
				To:       "mr:ref:/Instance:crossplane-system/staging-db",
				Evidence: EvidenceOwnerReference, // score=100
			},
			{
				ID:       "edge:kustomize-overlay",
				Type:     EdgeOwns,
				From:     "kustomize_overlay:ref:kustomize/overlays/prod",
				To:       "mr:ref:/Instance:crossplane-system/staging-db",
				Evidence: EvidenceKustomizeOverlay, // score=75
			},
		},
		Summary: AttributionSummary{
			NodesByType: map[AttributionNodeType]int{NodeMR: 1, NodeXR: 1, NodeKustomizeOverlay: 1},
			EdgesByType: map[AttributionEdgeType]int{EdgeOwns: 2},
		},
	}

	report, err := BuildAttributionReport(graph)
	if err != nil {
		t.Fatalf("BuildAttributionReport failed: %v", err)
	}

	item := report.Items[0]

	// Crossplane ownerRef should win (score 100 > 75)
	if item.BestOwner == nil {
		t.Fatal("BestOwner should not be nil")
	}
	if item.BestOwner.Score != ScoreOwnerReference {
		t.Errorf("BestOwner.Score = %d, want %d (owner_reference should beat kustomize)", item.BestOwner.Score, ScoreOwnerReference)
	}
	if item.Reason != ReasonOwnedViaOwnerRef {
		t.Errorf("Reason = %s, want %s", item.Reason, ReasonOwnedViaOwnerRef)
	}

	// Kustomize should still be in candidates
	if len(item.OwnerCandidates) != 2 {
		t.Fatalf("OwnerCandidates count = %d, want 2", len(item.OwnerCandidates))
	}
}

// TestScoreForEvidence_Kustomize tests kustomize scoring.
func TestScoreForEvidence_Kustomize(t *testing.T) {
	if scoreForEvidence(EvidenceKustomizeOverlay) != ScoreKustomizeOverlay {
		t.Errorf("scoreForEvidence(kustomize_overlay) = %d, want %d",
			scoreForEvidence(EvidenceKustomizeOverlay), ScoreKustomizeOverlay)
	}
}

// TestReasonForEvidence tests reason code mapping.
func TestReasonForEvidence(t *testing.T) {
	tests := []struct {
		evidence AttributionEvidence
		want     AttributionReason
	}{
		{EvidenceOwnerReference, ReasonOwnedViaOwnerRef},
		{EvidenceKustomizeOverlay, ReasonOwnedViaKustomize},
		{EvidenceCompositeLabel, ReasonOwnedViaLabel},
		{EvidenceClaimLabel, ReasonOwnedViaLabel},
	}

	for _, tt := range tests {
		t.Run(string(tt.evidence), func(t *testing.T) {
			got := reasonForEvidence(tt.evidence)
			if got != tt.want {
				t.Errorf("reasonForEvidence(%s) = %s, want %s", tt.evidence, got, tt.want)
			}
		})
	}
}
