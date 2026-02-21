// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"runtime/pprof"
	"testing"
)

// TestTUIProfile_CPU generates a CPU profile for TUI rendering at scale.
// Gated by CUB_SCOUT_PROFILE=1 to avoid running in normal CI.
// Output: tui_cpu.prof in the test working directory.
func TestTUIProfile_CPU(t *testing.T) {
	if os.Getenv("CUB_SCOUT_PROFILE") != "1" {
		t.Skip("set CUB_SCOUT_PROFILE=1 to enable profiling")
	}

	f, err := os.Create("tui_cpu.prof")
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	defer f.Close()

	m := createScaleModel(120, 40, 1000)

	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatalf("start CPU profile: %v", err)
	}
	defer pprof.StopCPUProfile()

	// Render dashboard and workloads views repeatedly
	for i := 0; i < 500; i++ {
		m.view = viewDashboard
		m.panelMode = false
		_ = m.View()

		m.view = viewWorkloads
		m.panelMode = true
		m.panelView = viewWorkloads
		_ = m.View()
	}

	t.Logf("CPU profile written to tui_cpu.prof")
	t.Logf("Analyze with: go tool pprof tui_cpu.prof")
}

// TestTUIProfile_Heap generates a heap profile for TUI memory at scale.
// Gated by CUB_SCOUT_PROFILE=1 to avoid running in normal CI.
// Output: tui_heap.prof in the test working directory.
func TestTUIProfile_Heap(t *testing.T) {
	if os.Getenv("CUB_SCOUT_PROFILE") != "1" {
		t.Skip("set CUB_SCOUT_PROFILE=1 to enable profiling")
	}

	m := createScaleModel(120, 40, 1000)

	// Render all views to generate allocations
	m.view = viewDashboard
	m.panelMode = false
	_ = m.View()

	m.view = viewWorkloads
	m.panelMode = true
	m.panelView = viewWorkloads
	_ = m.View()

	m.panelView = viewPipelines
	_ = m.View()

	m.panelView = viewMaps
	_ = m.View()

	f, err := os.Create("tui_heap.prof")
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	defer f.Close()

	if err := pprof.WriteHeapProfile(f); err != nil {
		t.Fatalf("write heap profile: %v", err)
	}

	t.Logf("Heap profile written to tui_heap.prof")
	t.Logf("Analyze with: go tool pprof tui_heap.prof")
}
