// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"runtime"
	"testing"
)

// measureTUIMemory creates a scale model and renders it, returning heap alloc in bytes.
func measureTUIMemory(t *testing.T, n int) uint64 {
	t.Helper()

	// Force GC before measuring
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	m := createScaleModel(120, 40, n)
	// Render all views to allocate display buffers
	_ = m.View()
	m.view = viewWorkloads
	m.panelMode = true
	m.panelView = viewWorkloads
	_ = m.View()
	m.panelView = viewPipelines
	_ = m.View()
	m.panelView = viewMaps
	_ = m.View()

	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("TUI memory for %d resources: %d MB (heap alloc delta)", n, allocated/(1024*1024))
	return allocated
}

func TestTUIMemory_500Resources_Under200MB(t *testing.T) {
	allocated := measureTUIMemory(t, 500)
	limit := uint64(200 * 1024 * 1024) // 200 MB
	if allocated > limit {
		t.Errorf("TUI memory for 500 resources: %d MB (want < 200 MB)", allocated/(1024*1024))
	}
}

func TestTUIMemory_1000Resources_Under500MB(t *testing.T) {
	allocated := measureTUIMemory(t, 1000)
	limit := uint64(500 * 1024 * 1024) // 500 MB
	if allocated > limit {
		t.Errorf("TUI memory for 1000 resources: %d MB (want < 500 MB)", allocated/(1024*1024))
	}
}
