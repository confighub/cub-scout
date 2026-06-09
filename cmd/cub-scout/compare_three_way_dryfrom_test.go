// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"testing"
)

func threeWayDeploymentDryMap(name, ns string, replicas int64) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": name, "namespace": ns},
		"spec":       map[string]interface{}{"replicas": replicas},
	}
}

func withFakeDryFromLoader(t *testing.T, summaries []*compareSideSummary) {
	t.Helper()
	prev := loadCompareDryFromPathFn
	loadCompareDryFromPathFn = func(string) ([]*compareSideSummary, error) { return summaries, nil }
	t.Cleanup(func() { loadCompareDryFromPathFn = prev })
}

// withStandaloneThreeWayLive fakes LIVE (replicas) and forces standalone mode.
func withStandaloneThreeWayLive(t *testing.T, replicas int64) {
	t.Helper()
	prevLive := loadCompareLiveSnapshotFn
	prevConn := compareConnectedFn
	loadCompareLiveSnapshotFn = func(_ context.Context, _, name, ns string) (compareSideSummary, error) {
		r := replicas
		return compareSideSummary{Source: "cluster", APIVersion: "apps/v1", Kind: "Deployment", Name: name, Namespace: ns, Replicas: &r}, nil
	}
	compareConnectedFn = func() bool { return false }
	t.Cleanup(func() {
		loadCompareLiveSnapshotFn = prevLive
		compareConnectedFn = prevConn
	})
}

func resetThreeWayFlags(t *testing.T) {
	t.Helper()
	prev := struct{ scope, view, format, failOn, dryFrom, ns string }{
		compareThreeWayScopeRaw, compareThreeWayView, compareThreeWayFormat, compareThreeWayFailOn, compareThreeWayDryFrom, combinedNamespace,
	}
	compareThreeWayScopeRaw, compareThreeWayView, compareThreeWayFormat, compareThreeWayFailOn, compareThreeWayDryFrom, combinedNamespace = "", "", "ascii", "", "", ""
	compareThreeWayJSON = false
	t.Cleanup(func() {
		compareThreeWayScopeRaw, compareThreeWayView, compareThreeWayFormat, compareThreeWayFailOn, compareThreeWayDryFrom, combinedNamespace = prev.scope, prev.view, prev.format, prev.failOn, prev.dryFrom, prev.ns
		compareThreeWayJSON = false
	})
}

func runStandaloneThreeWay(t *testing.T) threeWayReport {
	t.Helper()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"compare", "three-way", "--scope", "deploy/web", "-n", "prod", "--dry-from", "/tmp/ignored-by-fake", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	var report threeWayReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("unmarshal report: %v\n%s", err, out)
	}
	if len(report.Resources) != 1 {
		t.Fatalf("resources=%d want 1\n%s", len(report.Resources), out)
	}
	return report
}

// Standalone DRY (from a rendered file) vs LIVE: replicas differ -> drift.
func TestThreeWay_DryFromFile_DriftDetected(t *testing.T) {
	resetThreeWayFlags(t)
	withFakeDryFromLoader(t, []*compareSideSummary{summarizeCompareManifestObject("dry", threeWayDeploymentDryMap("web", "prod", 3))})
	withStandaloneThreeWayLive(t, 5)

	report := runStandaloneThreeWay(t)
	r := report.Resources[0]
	if r.Result.Connected {
		t.Fatal("standalone --dry-from must be Connected=false")
	}
	if r.Result.Dry == nil {
		t.Fatal("DRY must be sourced from --dry-from")
	}
	if r.Pattern == PatternDisconnected {
		t.Fatal("standalone-with-DRY must NOT be classified disconnected")
	}
	if report.Summary.MismatchedResources != 1 {
		t.Fatalf("mismatchedResources=%d want 1", report.Summary.MismatchedResources)
	}
	found := false
	for _, m := range r.Result.Mismatches {
		if m.Field == "replicas" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a replicas mismatch (DRY 3 vs LIVE 5); got %+v", r.Result.Mismatches)
	}
}

// Standalone DRY matches LIVE -> agreed, no mismatches.
func TestThreeWay_DryFromFile_Agreed(t *testing.T) {
	resetThreeWayFlags(t)
	withFakeDryFromLoader(t, []*compareSideSummary{summarizeCompareManifestObject("dry", threeWayDeploymentDryMap("web", "prod", 3))})
	withStandaloneThreeWayLive(t, 3)

	report := runStandaloneThreeWay(t)
	if report.Summary.MismatchedResources != 0 {
		t.Fatalf("mismatchedResources=%d want 0", report.Summary.MismatchedResources)
	}
	if report.Resources[0].Pattern != PatternAgreed {
		t.Fatalf("pattern=%v want Agreed", report.Resources[0].Pattern)
	}
}
