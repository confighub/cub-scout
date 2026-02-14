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

// BenchmarkAttributionGraphBuild_500Nodes measures graph construction at production scale.
// This ensures the attribution graph can handle 500+ resources within 2 seconds.
func BenchmarkAttributionGraphBuild_500Nodes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		builder := NewAttributionGraphBuilder()

		// Create 500 nodes with mixed ownership types and edges
		for j := 0; j < 500; j++ {
			nodeType := NodeK8sObject
			switch j % 5 {
			case 0:
				nodeType = NodeClaim
			case 1:
				nodeType = NodeXR
			case 2:
				nodeType = NodeMR
			case 3:
				nodeType = NodeClaim
			}

			builder.AddNode(AttributionNode{
				Type: nodeType,
				Ref: AttributionRef{
					Kind:      "Deployment",
					Name:      fmt.Sprintf("workload-%04d", j),
					Namespace: fmt.Sprintf("ns-%02d", j%20),
					UID:       fmt.Sprintf("uid-%04d", j),
				},
				Present: true,
			})

			// Add ownership edges (every 5th node owns the next 4)
			if j%5 != 0 && j >= 5 {
				parentIdx := (j / 5) * 5
				builder.AddEdge(AttributionEdge{
					Type:     EdgeOwns,
					From:     fmt.Sprintf("claim:uid:uid-%04d", parentIdx),
					To:       fmt.Sprintf("k8s_object:uid:uid-%04d", j),
					Evidence: EvidenceOwnerReference,
				})
			}
		}

		_ = builder.Build()
	}
}

// TestAttributionGraphBuild_500Nodes_Within2s is a test (not benchmark) that
// asserts 500-node graph construction completes within 2 seconds.
// This provides a CI-enforceable performance gate.
func TestAttributionGraphBuild_500Nodes_Within2s(t *testing.T) {
	start := time.Now()

	builder := NewAttributionGraphBuilder()
	for j := 0; j < 500; j++ {
		nodeType := NodeK8sObject
		if j%5 == 0 {
			nodeType = NodeClaim
		}
		builder.AddNode(AttributionNode{
			Type: nodeType,
			Ref: AttributionRef{
				Kind:      "Deployment",
				Name:      fmt.Sprintf("workload-%04d", j),
				Namespace: fmt.Sprintf("ns-%02d", j%20),
				UID:       fmt.Sprintf("uid-%04d", j),
			},
			Present: true,
		})
		if j%5 != 0 && j >= 5 {
			parentIdx := (j / 5) * 5
			builder.AddEdge(AttributionEdge{
				Type:     EdgeOwns,
				From:     fmt.Sprintf("claim:uid:uid-%04d", parentIdx),
				To:       fmt.Sprintf("k8s_object:uid:uid-%04d", j),
				Evidence: EvidenceOwnerReference,
			})
		}
	}
	graph := builder.Build()
	_ = RenderAttributionGraphASCII(graph)

	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Errorf("500-node graph build+render took %v (want < 2s)", elapsed)
	}
	t.Logf("500-node graph build+render: %v", elapsed)
}

// BenchmarkOwnershipDetection_500Resources measures ownership detection throughput.
func BenchmarkOwnershipDetection_500Resources(b *testing.B) {
	objs := makeMixedOwnershipObjects(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, obj := range objs {
			DetectOwnership(obj)
		}
	}
}

// makeMixedOwnershipObjects creates n objects with varied ownership labels.
func makeMixedOwnershipObjects(n int) []*unstructured.Unstructured {
	objs := make([]*unstructured.Unstructured, n)
	for i := 0; i < n; i++ {
		labels := map[string]interface{}{}
		annotations := map[string]interface{}{}

		switch i % 7 {
		case 0: // Flux
			labels["kustomize.toolkit.fluxcd.io/name"] = fmt.Sprintf("ks-%d", i)
			labels["kustomize.toolkit.fluxcd.io/namespace"] = "flux-system"
		case 1: // ArgoCD
			labels["argocd.argoproj.io/instance"] = fmt.Sprintf("app-%d", i)
		case 2: // Helm
			labels["app.kubernetes.io/managed-by"] = "Helm"
		case 3: // Terraform
			annotations["app.terraform.io/run-id"] = fmt.Sprintf("run-%d", i)
		case 4: // Crossplane
			labels["crossplane.io/claim-name"] = fmt.Sprintf("claim-%d", i)
		case 5: // ConfigHub
			labels["confighub.com/UnitSlug"] = fmt.Sprintf("unit-%d", i)
		case 6: // Native (no ownership)
		}

		objs[i] = &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":        fmt.Sprintf("deploy-%04d", i),
					"namespace":   fmt.Sprintf("ns-%02d", i%20),
					"labels":      labels,
					"annotations": annotations,
				},
			},
		}
	}
	return objs
}
