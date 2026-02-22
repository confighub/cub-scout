// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"testing"
	"time"
)

// createScaleModel creates a LocalClusterModel with n generated workload entries.
// Extends createTestModel with scale data for benchmarking.
func createScaleModel(width, height, n int) LocalClusterModel {
	m := initialLocalModel()
	m.ready = true
	m.loading = false
	m.width = width
	m.height = height
	m.clusterName = "bench-cluster"
	m.contextName = "bench-context"

	owners := []string{"Flux", "ArgoCD", "Helm", "Terraform", "Crossplane", "ConfigHub", "Native"}
	kinds := []string{"Deployment", "StatefulSet", "DaemonSet", "Service", "ConfigMap"}
	statuses := []string{"Ready", "Progressing", "Degraded", "CrashLoopBackOff"}

	m.entries = make([]MapEntry, n)
	for i := 0; i < n; i++ {
		m.entries[i] = MapEntry{
			Kind:      kinds[i%len(kinds)],
			Name:      fmt.Sprintf("workload-%04d", i),
			Namespace: fmt.Sprintf("ns-%02d", i%20),
			Owner:     owners[i%len(owners)],
			Status:    statuses[i%len(statuses)],
		}
	}

	// Add GitOps resources (proportional)
	gitopsCount := n / 10
	if gitopsCount < 3 {
		gitopsCount = 3
	}
	m.gitops = make([]GitOpsResource, gitopsCount)
	for i := 0; i < gitopsCount; i++ {
		m.gitops[i] = GitOpsResource{
			Kind:           "Kustomization",
			Name:           fmt.Sprintf("ks-%04d", i),
			Namespace:      "flux-system",
			Status:         "Ready",
			Source:         "flux-system",
			InventoryCount: n / gitopsCount,
		}
	}

	// Generate namespace list
	nsSet := make(map[string]bool)
	for _, e := range m.entries {
		nsSet[e.Namespace] = true
	}
	m.namespaces = make([]string, 0, len(nsSet))
	for ns := range nsSet {
		m.namespaces = append(m.namespaces, ns)
	}

	return m
}

// --- TUI View Render Benchmarks ---

func BenchmarkTUIView_Dashboard_500(b *testing.B) {
	m := createScaleModel(120, 40, 500)
	m.view = viewDashboard
	m.panelMode = false
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkTUIView_Dashboard_1000(b *testing.B) {
	m := createScaleModel(120, 40, 1000)
	m.view = viewDashboard
	m.panelMode = false
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkTUIView_Workloads_500(b *testing.B) {
	m := createScaleModel(120, 40, 500)
	m.view = viewWorkloads
	m.panelMode = true
	m.panelView = viewWorkloads
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

func BenchmarkTUIView_Workloads_1000(b *testing.B) {
	m := createScaleModel(120, 40, 1000)
	m.view = viewWorkloads
	m.panelMode = true
	m.panelView = viewWorkloads
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}

// --- CI Gate Tests ---

func TestTUIRender_Dashboard_500_Within3s(t *testing.T) {
	m := createScaleModel(120, 40, 500)
	m.view = viewDashboard
	m.panelMode = false

	start := time.Now()
	for i := 0; i < 100; i++ {
		_ = m.View()
	}
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("100 dashboard renders with 500 entries took %v (want < 3s)", elapsed)
	}
	t.Logf("100 dashboard renders (500 entries): %v", elapsed)
}

func TestTUIRender_Workloads_500_Within3s(t *testing.T) {
	m := createScaleModel(120, 40, 500)
	m.view = viewWorkloads
	m.panelMode = true
	m.panelView = viewWorkloads

	start := time.Now()
	for i := 0; i < 100; i++ {
		_ = m.View()
	}
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("100 workload renders with 500 entries took %v (want < 3s)", elapsed)
	}
	t.Logf("100 workload renders (500 entries): %v", elapsed)
}

func TestTUIRender_AllViews_500_Within3s(t *testing.T) {
	views := []struct {
		name      string
		view      localView
		panelMode bool
		panelView localView
	}{
		{"dashboard", viewDashboard, false, 0},
		{"workloads", viewWorkloads, true, viewWorkloads},
		{"pipelines", viewPipelines, true, viewPipelines},
		{"maps", viewMaps, true, viewMaps},
	}

	for _, v := range views {
		t.Run(v.name, func(t *testing.T) {
			m := createScaleModel(120, 40, 500)
			m.view = v.view
			m.panelMode = v.panelMode
			m.panelView = v.panelView

			start := time.Now()
			for i := 0; i < 100; i++ {
				_ = m.View()
			}
			elapsed := time.Since(start)

			if elapsed > 3*time.Second {
				t.Errorf("100 %s renders with 500 entries took %v (want < 3s)", v.name, elapsed)
			}
			t.Logf("100 %s renders (500 entries): %v", v.name, elapsed)
		})
	}
}
