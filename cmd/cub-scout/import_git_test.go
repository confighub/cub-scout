// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildImportFromGitPreview_BasicContract(t *testing.T) {
	// Create a temporary Git repo structure
	tmpDir := t.TempDir()

	// Create standard Flux/Kustomize repo structure
	paths := []string{
		filepath.Join(tmpDir, "apps", "base", "podinfo"),
		filepath.Join(tmpDir, "apps", "staging"),
		filepath.Join(tmpDir, "apps", "prod"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", p, err)
		}
	}

	// Create kustomization files
	kustomization := "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - ../base/podinfo\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "apps", "staging", "kustomization.yaml"), []byte(kustomization), 0o644); err != nil {
		t.Fatalf("failed to write staging kustomization: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps", "prod", "kustomization.yaml"), []byte(kustomization), 0o644); err != nil {
		t.Fatalf("failed to write prod kustomization: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps", "base", "podinfo", "kustomization.yaml"), []byte("kind: Kustomization\n"), 0o644); err != nil {
		t.Fatalf("failed to write base kustomization: %v", err)
	}

	// Test Git-only preview (no cluster workloads)
	proposal, gitApps, err := buildImportFromGitPreview(tmpDir, nil)
	if err != nil {
		t.Fatalf("buildImportFromGitPreview() error = %v", err)
	}

	if len(gitApps) == 0 {
		t.Fatal("expected at least one Git app")
	}

	// Check that podinfo app was found
	var foundPodinfo bool
	for _, app := range gitApps {
		if app.Name == "podinfo" {
			foundPodinfo = true
			if app.BasePath != "apps/base/podinfo" {
				t.Errorf("podinfo.BasePath = %q, want apps/base/podinfo", app.BasePath)
			}
			if len(app.Variants) < 2 {
				t.Errorf("expected at least 2 variants (staging, prod), got %d", len(app.Variants))
			}
		}
	}
	if !foundPodinfo {
		t.Error("expected to find podinfo app")
	}

	if proposal == nil {
		t.Fatal("proposal is nil")
	}
	if len(proposal.Units) == 0 {
		t.Error("proposal.Units should not be empty")
	}

	// Check that units are marked as git-only (no cluster workloads provided)
	for _, unit := range proposal.Units {
		if unit.Status != "git-only" && unit.Status != "" {
			// Note: status might be empty if there are no variants/workloads to compare
			t.Errorf("unit %q status = %q, expected git-only or empty", unit.Slug, unit.Status)
		}
	}
}

func TestBuildImportFromGitPreview_WithClusterWorkloads(t *testing.T) {
	// Create a temporary Git repo structure
	tmpDir := t.TempDir()

	// Create standard Flux/Kustomize repo structure
	paths := []string{
		filepath.Join(tmpDir, "apps", "base", "podinfo"),
		filepath.Join(tmpDir, "apps", "staging"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("failed to create directory %s: %v", p, err)
		}
	}

	kustomization := "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - ../base/podinfo\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "apps", "staging", "kustomization.yaml"), []byte(kustomization), 0o644); err != nil {
		t.Fatalf("failed to write staging kustomization: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps", "base", "podinfo", "kustomization.yaml"), []byte("kind: Kustomization\n"), 0o644); err != nil {
		t.Fatalf("failed to write base kustomization: %v", err)
	}

	// Mock cluster workloads - podinfo deployed
	clusterWorkloads := []WorkloadInfo{
		{
			Kind:      "Deployment",
			Name:      "podinfo",
			Namespace: "staging",
			Owner:     "Flux",
			Ready:     true,
			Replicas:  3,
			Labels: map[string]string{
				"app": "podinfo",
			},
		},
	}

	// Test Git + cluster comparison
	proposal, _, err := buildImportFromGitPreview(tmpDir, clusterWorkloads)
	if err != nil {
		t.Fatalf("buildImportFromGitPreview() error = %v", err)
	}

	if proposal == nil {
		t.Fatal("proposal is nil")
	}

	// Check for aligned status
	var foundAligned bool
	for _, unit := range proposal.Units {
		if unit.Status == "aligned" {
			foundAligned = true
		}
	}
	if !foundAligned {
		t.Error("expected at least one aligned unit when cluster workload matches Git app")
	}
}

func TestBuildImportFromGitPreview_EmptyRepo(t *testing.T) {
	// Create an empty temporary directory
	tmpDir := t.TempDir()

	// Test with empty repo - should return error
	_, _, err := buildImportFromGitPreview(tmpDir, nil)
	if err == nil {
		t.Error("expected error for empty Git repo, got nil")
	}
}

func TestBuildImportFromGitPreview_InvalidPath(t *testing.T) {
	// Test with non-existent path
	_, _, err := buildImportFromGitPreview("/nonexistent/path/12345", nil)
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestImportEvidenceJSON_GitOnly(t *testing.T) {
	// Reset global flags
	oldGitPath := importGitPath
	oldFromBundle := importFromBundle
	oldNamespace := importNamespace
	defer func() {
		importGitPath = oldGitPath
		importFromBundle = oldFromBundle
		importNamespace = oldNamespace
	}()

	importGitPath = "/path/to/repo"
	importFromBundle = ""
	importNamespace = ""

	evidence := buildImportEvidenceJSON()
	if evidence.Source != "git" {
		t.Errorf("evidence.Source = %q, want git", evidence.Source)
	}
	if evidence.GitPath != "/path/to/repo" {
		t.Errorf("evidence.GitPath = %q, want /path/to/repo", evidence.GitPath)
	}
}

func TestImportEvidenceJSON_GitClusterComparison(t *testing.T) {
	oldGitPath := importGitPath
	oldFromBundle := importFromBundle
	oldNamespace := importNamespace
	defer func() {
		importGitPath = oldGitPath
		importFromBundle = oldFromBundle
		importNamespace = oldNamespace
	}()

	importGitPath = "/path/to/repo"
	importFromBundle = ""
	importNamespace = "production"

	evidence := buildImportEvidenceJSON()
	if evidence.Source != "comparison" {
		t.Errorf("evidence.Source = %q, want comparison", evidence.Source)
	}
	if evidence.ClusterSource != "cluster" {
		t.Errorf("evidence.ClusterSource = %q, want cluster", evidence.ClusterSource)
	}
	if evidence.GitPath != "/path/to/repo" {
		t.Errorf("evidence.GitPath = %q, want /path/to/repo", evidence.GitPath)
	}
}

func TestImportEvidenceJSON_GitBundleComparison(t *testing.T) {
	oldGitPath := importGitPath
	oldFromBundle := importFromBundle
	oldNamespace := importNamespace
	defer func() {
		importGitPath = oldGitPath
		importFromBundle = oldFromBundle
		importNamespace = oldNamespace
	}()

	importGitPath = "/path/to/repo"
	importFromBundle = "/path/to/bundle"
	importNamespace = ""

	evidence := buildImportEvidenceJSON()
	if evidence.Source != "comparison" {
		t.Errorf("evidence.Source = %q, want comparison", evidence.Source)
	}
	if evidence.ClusterSource != "bundle" {
		t.Errorf("evidence.ClusterSource = %q, want bundle", evidence.ClusterSource)
	}
	if evidence.BundlePath != "/path/to/bundle" {
		t.Errorf("evidence.BundlePath = %q, want /path/to/bundle", evidence.BundlePath)
	}
}

func TestBuildImportFromGitPreview_ApplicationSet(t *testing.T) {
	// Create a temporary Git repo with ApplicationSet structure
	tmpDir := t.TempDir()

	// Create ApplicationSet directory
	appSetDir := filepath.Join(tmpDir, "applicationsets")
	if err := os.MkdirAll(appSetDir, 0o755); err != nil {
		t.Fatalf("failed to create applicationsets dir: %v", err)
	}

	// Create ApplicationSet with git generator
	appSetContent := `apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: apps
spec:
  generators:
    - git:
        repoURL: https://github.com/org/repo
        directories:
          - path: apps/*
          - path: "!apps/excluded"
`
	if err := os.WriteFile(filepath.Join(appSetDir, "apps.yaml"), []byte(appSetContent), 0o644); err != nil {
		t.Fatalf("failed to write applicationset: %v", err)
	}

	// Create app directories that match the pattern
	for _, app := range []string{"frontend", "backend", "api"} {
		appDir := filepath.Join(tmpDir, "apps", app)
		if err := os.MkdirAll(appDir, 0o755); err != nil {
			t.Fatalf("failed to create app dir %s: %v", app, err)
		}
	}

	// Create excluded app (should not appear due to exclude pattern)
	excludedDir := filepath.Join(tmpDir, "apps", "excluded")
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		t.Fatalf("failed to create excluded dir: %v", err)
	}

	// Test Git-only preview
	proposal, gitApps, err := buildImportFromGitPreview(tmpDir, nil)
	if err != nil {
		t.Fatalf("buildImportFromGitPreview() error = %v", err)
	}

	// Should find 3 apps (frontend, backend, api) - not excluded
	if len(gitApps) != 3 {
		t.Errorf("expected 3 apps from ApplicationSet git generator, got %d: %v", len(gitApps), func() []string {
			names := make([]string, len(gitApps))
			for i, a := range gitApps {
				names[i] = a.Name
			}
			return names
		}())
	}

	// Verify specific apps were found
	foundApps := make(map[string]bool)
	for _, app := range gitApps {
		foundApps[app.Name] = true
	}

	for _, expected := range []string{"frontend", "backend", "api"} {
		if !foundApps[expected] {
			t.Errorf("expected to find app %q in Git apps", expected)
		}
	}

	// Excluded app should not be found
	if foundApps["excluded"] {
		t.Error("excluded app should not be found (exclude pattern should filter it out)")
	}

	if proposal == nil {
		t.Fatal("proposal is nil")
	}
}
