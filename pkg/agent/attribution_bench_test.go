// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// makeCrossplaneObjects creates n Crossplane-managed objects for benchmarking.
func makeCrossplaneObjects(n int) []*unstructured.Unstructured {
	objs := make([]*unstructured.Unstructured, n)
	for i := 0; i < n; i++ {
		objs[i] = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "rds.aws.crossplane.io/v1alpha1",
				"kind":       "Instance",
				"metadata": map[string]interface{}{
					"name":      fmt.Sprintf("instance-%d", i),
					"namespace": "default",
					"labels": map[string]interface{}{
						"crossplane.io/composite": "xr-database",
					},
				},
			},
		}
	}
	return objs
}

// BenchmarkCrossplaneLineageNoIndex measures resolver without pre-built index (O(n²)).
// This benchmark demonstrates the cost of rebuilding the index per-call.
func BenchmarkCrossplaneLineageNoIndex(b *testing.B) {
	sizes := []int{10, 100, 500}
	for _, n := range sizes {
		objs := makeCrossplaneObjects(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				for _, obj := range objs {
					ResolveCrossplaneLineage(obj, objs)
				}
			}
		})
	}
}

// BenchmarkCrossplaneLineageWithIndex measures resolver with pre-built index (O(n)).
// This benchmark shows the improvement from reusing a single index.
func BenchmarkCrossplaneLineageWithIndex(b *testing.B) {
	sizes := []int{10, 100, 500}
	for _, n := range sizes {
		objs := makeCrossplaneObjects(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				idx := NewUnstructuredIndex(objs) // Build once per batch
				for _, obj := range objs {
					ResolveCrossplaneLineageWithIndex(obj, idx)
				}
			}
		})
	}
}
