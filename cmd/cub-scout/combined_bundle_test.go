// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestResolveCombinedWorkloadsSource_FromBundle(t *testing.T) {
	bundleDir := t.TempDir()
	writer := agent.NewBundleWriter("test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "payments",
			},
		},
		Session: &agent.DebugSessionData{
			StartedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
			WorkloadHealth: &agent.WorkloadHealthSnapshot{
				Kind:          "Deployment",
				Name:          "api",
				Namespace:     "payments",
				Replicas:      2,
				ReadyReplicas: 2,
			},
		},
	}
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	workloads, namespace, err := resolveCombinedWorkloadsSource("", bundleDir)
	if err != nil {
		t.Fatalf("resolveCombinedWorkloadsSource() error = %v", err)
	}
	if namespace != "payments" {
		t.Fatalf("namespace = %q, want payments", namespace)
	}
	if len(workloads) != 1 {
		t.Fatalf("len(workloads) = %d, want 1", len(workloads))
	}
	if workloads[0].Name != "api" {
		t.Fatalf("workload name = %q, want api", workloads[0].Name)
	}
}

func TestResolveCombinedWorkloadsSource_Conflict(t *testing.T) {
	_, _, err := resolveCombinedWorkloadsSource("default", "/tmp/test-bundle")
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
}

func TestResolveCombinedWorkloadsSource_Empty(t *testing.T) {
	workloads, namespace, err := resolveCombinedWorkloadsSource("", "")
	if err != nil {
		t.Fatalf("resolveCombinedWorkloadsSource() error = %v", err)
	}
	if namespace != "" {
		t.Fatalf("namespace = %q, want empty", namespace)
	}
	if len(workloads) != 0 {
		t.Fatalf("len(workloads) = %d, want 0", len(workloads))
	}
}

func TestRunCombined_GitPathWithBundleJSON(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "apps", "base", "api"), 0o755); err != nil {
		t.Fatalf("mkdir base app: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "apps", "prod"), 0o755); err != nil {
		t.Fatalf("mkdir prod: %v", err)
	}
	baseKustomization := []byte("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources: []\n")
	if err := os.WriteFile(filepath.Join(repoDir, "apps", "base", "api", "kustomization.yaml"), baseKustomization, 0o644); err != nil {
		t.Fatalf("write base kustomization: %v", err)
	}
	prodKustomization := []byte("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n- ../base/api\n")
	if err := os.WriteFile(filepath.Join(repoDir, "apps", "prod", "kustomization.yaml"), prodKustomization, 0o644); err != nil {
		t.Fatalf("write prod kustomization: %v", err)
	}

	bundleDir := t.TempDir()
	writer := agent.NewBundleWriter("test")
	bundle := &agent.DebugBundle{
		Metadata: agent.BundleMetadata{
			Target: agent.BundleTarget{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "payments",
			},
		},
		Session: &agent.DebugSessionData{
			WorkloadHealth: &agent.WorkloadHealthSnapshot{
				Kind:          "Deployment",
				Name:          "api",
				Namespace:     "payments",
				Replicas:      1,
				ReadyReplicas: 1,
			},
		},
	}
	if err := writer.Write(bundle, bundleDir); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	restore := setCombinedFlagState(combinedFlagState{
		gitPath: repoDir,
		bundle:  bundleDir,
		json:    true,
	})
	defer restore()

	output := captureStdout(t, func() {
		if err := runCombined(nil, nil); err != nil {
			t.Fatalf("runCombined() error = %v", err)
		}
	})

	var result CombinedResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("parse combined JSON: %v\n%s", err, output)
	}

	if result.Cluster == nil {
		t.Fatal("cluster result is nil")
	}
	if result.Cluster.Namespace != "payments" {
		t.Fatalf("cluster namespace = %q, want payments", result.Cluster.Namespace)
	}
	if len(result.Alignment) == 0 {
		t.Fatal("alignment is empty")
	}
	foundAligned := false
	for _, a := range result.Alignment {
		if a.App == "api" && a.Status == "aligned" {
			foundAligned = true
			break
		}
	}
	if !foundAligned {
		t.Fatalf("expected aligned entry for app=api, got %+v", result.Alignment)
	}
}

type combinedFlagState struct {
	gitURL         string
	gitPath        string
	gitURLCompare  string
	gitPathCompare string
	namespace      string
	bundle         string
	format         string
	json           bool
	suggest        bool
	apply          bool
	dryRun         bool
}

func setCombinedFlagState(next combinedFlagState) func() {
	prev := combinedFlagState{
		gitURL:         combinedGitURL,
		gitPath:        combinedGitPath,
		gitURLCompare:  combinedGitURLCompare,
		gitPathCompare: combinedGitPathCompare,
		namespace:      combinedNamespace,
		bundle:         combinedBundle,
		format:         combinedFormat,
		json:           combinedJSON,
		suggest:        combinedSuggest,
		apply:          combinedApply,
		dryRun:         combinedDryRun,
	}

	combinedGitURL = next.gitURL
	combinedGitPath = next.gitPath
	combinedGitURLCompare = next.gitURLCompare
	combinedGitPathCompare = next.gitPathCompare
	combinedNamespace = next.namespace
	combinedBundle = next.bundle
	combinedFormat = next.format
	combinedJSON = next.json
	combinedSuggest = next.suggest
	combinedApply = next.apply
	combinedDryRun = next.dryRun

	return func() {
		combinedGitURL = prev.gitURL
		combinedGitPath = prev.gitPath
		combinedGitURLCompare = prev.gitURLCompare
		combinedGitPathCompare = prev.gitPathCompare
		combinedNamespace = prev.namespace
		combinedBundle = prev.bundle
		combinedFormat = prev.format
		combinedJSON = prev.json
		combinedSuggest = prev.suggest
		combinedApply = prev.apply
		combinedDryRun = prev.dryRun
	}
}
