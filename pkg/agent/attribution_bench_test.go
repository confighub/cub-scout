// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"
)

// BenchmarkAttributionGraphBuild measures graph construction time.
func BenchmarkAttributionGraphBuild(b *testing.B) {
	for i := 0; i < b.N; i++ {
		builder := NewAttributionGraphBuilder()

		// Simulate a moderate Crossplane hierarchy
		builder.AddNode(AttributionNode{
			Type:    NodeClaim,
			Ref:     AttributionRef{Kind: "PostgreSQLInstance", Name: "db", Namespace: "prod", UID: "uid-claim"},
			Present: true,
		})
		builder.AddNode(AttributionNode{
			Type:    NodeXR,
			Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-abc", UID: "uid-xr"},
			Present: true,
		})
		for j := 0; j < 5; j++ {
			builder.AddNode(AttributionNode{
				Type:    NodeMR,
				Ref:     AttributionRef{Kind: "Instance", Name: "mr-" + string(rune('a'+j)), UID: "uid-mr-" + string(rune('a'+j))},
				Present: true,
			})
			builder.AddEdge(AttributionEdge{
				Type:     EdgeOwns,
				From:     "xr:uid:uid-xr",
				To:       "mr:uid:uid-mr-" + string(rune('a'+j)),
				Evidence: EvidenceOwnerReference,
			})
		}
		builder.AddEdge(AttributionEdge{
			Type:     EdgeOwns,
			From:     "claim:uid:uid-claim",
			To:       "xr:uid:uid-xr",
			Evidence: EvidenceOwnerReference,
		})

		_ = builder.Build()
	}
}

// BenchmarkAttributionGraphRender measures ASCII render time.
func BenchmarkAttributionGraphRender(b *testing.B) {
	// Build a graph once
	builder := NewAttributionGraphBuilder()
	builder.AddNode(AttributionNode{
		Type:    NodeClaim,
		Ref:     AttributionRef{Kind: "PostgreSQLInstance", Name: "db", Namespace: "prod", UID: "uid-claim"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeXR,
		Ref:     AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-abc", UID: "uid-xr"},
		Present: true,
	})
	builder.AddNode(AttributionNode{
		Type:    NodeMR,
		Ref:     AttributionRef{Kind: "Instance", Name: "staging-db", UID: "uid-mr"},
		Present: true,
	})
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "claim:uid:uid-claim",
		To:       "xr:uid:uid-xr",
		Evidence: EvidenceOwnerReference,
	})
	builder.AddEdge(AttributionEdge{
		Type:     EdgeOwns,
		From:     "xr:uid:uid-xr",
		To:       "mr:uid:uid-mr",
		Evidence: EvidenceOwnerReference,
	})
	graph := builder.Build()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderAttributionGraphASCII(graph)
	}
}

// BenchmarkAttributionReportRender measures report ASCII render time.
func BenchmarkAttributionReportRender(b *testing.B) {
	report := &AttributionReport{
		SchemaVersion: AttributionReportSchemaVersion,
		GeneratedAt:   time.Now(),
		Summary: AttributionReportSummary{
			TotalItems:        10,
			OwnedCount:        7,
			UnattributedCount: 2,
			AmbiguousCount:    1,
		},
		Items: []AttributionReportItem{
			{
				Ref:    AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
				Reason: ReasonOwnedViaOwnerRef,
				BestOwner: &OwnerCandidate{
					Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
					NodeType: NodeXR,
					Score:    ScoreOwnerReference,
					Evidence: EvidenceOwnerReference,
				},
				OwnerCandidates: []OwnerCandidate{
					{
						Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
						NodeType: NodeXR,
						Score:    ScoreOwnerReference,
						Evidence: EvidenceOwnerReference,
					},
				},
			},
			{
				Ref:             AttributionRef{Kind: "ConfigMap", Name: "config", Namespace: "prod"},
				Reason:          ReasonUnattributed,
				OwnerCandidates: []OwnerCandidate{},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = RenderAttributionReportASCII(report)
	}
}
