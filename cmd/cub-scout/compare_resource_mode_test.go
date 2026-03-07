package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildCompareResourceResult_Standalone(t *testing.T) {
	restoreFlags := setCombinedFlagState(combinedFlagState{
		namespace: "prod",
		format:    "json",
	})
	defer restoreFlags()

	restoreLive := loadCompareLiveSnapshotFn
	loadCompareLiveSnapshotFn = func(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
		replicas := int64(3)
		return compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
			Replicas:   &replicas,
			Images:     []string{"ghcr.io/acme/api:v1"},
		}, nil
	}
	defer func() { loadCompareLiveSnapshotFn = restoreLive }()

	restoreConnected := compareConnectedFn
	compareConnectedFn = func() bool { return false }
	defer func() { compareConnectedFn = restoreConnected }()

	result, err := buildCompareResourceResult(context.Background(), "deploy/api", "prod")
	if err != nil {
		t.Fatalf("buildCompareResourceResult: %v", err)
	}
	if result.Resource != "Deployment/api" {
		t.Fatalf("result.Resource = %q, want Deployment/api", result.Resource)
	}
	if result.Mode != "live-only" {
		t.Fatalf("result.Mode = %q, want live-only", result.Mode)
	}
	if result.Connected {
		t.Fatal("expected result.Connected=false")
	}
	if result.Live.Replicas == nil || *result.Live.Replicas != 3 {
		t.Fatalf("result.Live.Replicas = %#v, want 3", result.Live.Replicas)
	}
	if !strings.Contains(strings.Join(result.Notes, "\n"), "Connect to ConfigHub") {
		t.Fatalf("expected upsell note in %v", result.Notes)
	}
}

func TestBuildCompareResourceResult_Connected(t *testing.T) {
	restoreLive := loadCompareLiveSnapshotFn
	loadCompareLiveSnapshotFn = func(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
		return compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
		}, nil
	}
	defer func() { loadCompareLiveSnapshotFn = restoreLive }()

	restoreConnected := compareConnectedFn
	compareConnectedFn = func() bool { return true }
	defer func() { compareConnectedFn = restoreConnected }()

	result, err := buildCompareResourceResult(context.Background(), "deployment/api", "prod")
	if err != nil {
		t.Fatalf("buildCompareResourceResult: %v", err)
	}
	if !result.Connected {
		t.Fatal("expected result.Connected=true")
	}
	if !strings.Contains(strings.Join(result.Notes, "\n"), "Connected mode detected") {
		t.Fatalf("expected connected-mode note in %v", result.Notes)
	}
}

func TestRunCombined_ResourceCompareRejectsGitFlags(t *testing.T) {
	restoreFlags := setCombinedFlagState(combinedFlagState{
		gitPath:   "/tmp/repo",
		namespace: "prod",
	})
	defer restoreFlags()

	err := runCombined(&cobra.Command{}, []string{"deploy/api"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot be combined with --git-*") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCombined_ResourceCompareJSONOutput(t *testing.T) {
	restoreFlags := setCombinedFlagState(combinedFlagState{
		namespace: "prod",
		format:    "json",
	})
	defer restoreFlags()

	restoreLive := loadCompareLiveSnapshotFn
	loadCompareLiveSnapshotFn = func(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
		return compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
		}, nil
	}
	defer func() { loadCompareLiveSnapshotFn = restoreLive }()

	restoreConnected := compareConnectedFn
	compareConnectedFn = func() bool { return false }
	defer func() { compareConnectedFn = restoreConnected }()

	out := captureStdout(t, func() {
		if err := runCombined(&cobra.Command{}, []string{"deploy/api"}); err != nil {
			t.Fatalf("runCombined: %v", err)
		}
	})

	var payload compareResourceResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse compare json: %v\n%s", err, out)
	}
	if payload.Live.Name != "api" {
		t.Fatalf("payload.Live.Name = %q, want api", payload.Live.Name)
	}
	if payload.Live.Namespace != "prod" {
		t.Fatalf("payload.Live.Namespace = %q, want prod", payload.Live.Namespace)
	}
}
