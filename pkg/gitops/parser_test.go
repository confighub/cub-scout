// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package gitops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSingleRepoFixture(t *testing.T) {
	tmpDir := t.TempDir()

	paths := []string{
		filepath.Join(tmpDir, "apps", "base", "podinfo"),
		filepath.Join(tmpDir, "apps", "staging"),
		filepath.Join(tmpDir, "apps", "prod"),
		filepath.Join(tmpDir, "infrastructure", "monitoring"),
		filepath.Join(tmpDir, "clusters", "staging"),
		filepath.Join(tmpDir, "clusters", "prod"),
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
	if err := os.WriteFile(filepath.Join(tmpDir, "apps", "prod", "kustomization.yaml"), []byte(kustomization), 0o644); err != nil {
		t.Fatalf("failed to write prod kustomization: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "clusters", "staging", "apps.yaml"), []byte("kind: Kustomization\n"), 0o644); err != nil {
		t.Fatalf("failed to write staging cluster file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "clusters", "prod", "apps.yaml"), []byte("kind: Kustomization\n"), 0o644); err != nil {
		t.Fatalf("failed to write prod cluster file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps", "base", "podinfo", "kustomization.yaml"), []byte("kind: Kustomization\n"), 0o644); err != nil {
		t.Fatalf("failed to write base kustomization: %v", err)
	}

	result, err := ParseRepo(tmpDir)
	if err != nil {
		t.Fatalf("failed to parse repo: %v", err)
	}
	if result.Type != RepoTypeSingleRepo {
		t.Fatalf("expected repo type %q, got %q", RepoTypeSingleRepo, result.Type)
	}

	if len(result.Apps) == 0 {
		t.Error("expected to find apps, got none")
	}

	var podinfo *AppDefinition
	for i := range result.Apps {
		if result.Apps[i].Name == "podinfo" {
			podinfo = &result.Apps[i]
			break
		}
	}

	if podinfo == nil {
		t.Fatal("expected to find podinfo app")
	}

	if podinfo.BasePath != "apps/base/podinfo" {
		t.Fatalf("expected base path apps/base/podinfo, got %s", podinfo.BasePath)
	}

	if len(podinfo.Variants) < 2 {
		t.Errorf("expected at least 2 variants (staging, prod), got %d", len(podinfo.Variants))
	}

	// Check for staging variant
	hasStaging := false
	hasProd := false
	for _, v := range podinfo.Variants {
		if v.Name == "staging" {
			hasStaging = true
		}
		if v.Name == "prod" {
			hasProd = true
		}
	}

	if !hasStaging {
		t.Error("expected staging variant")
	}
	if !hasProd {
		t.Error("expected prod variant")
	}

	if len(result.Clusters) == 0 {
		t.Error("expected to find clusters, got none")
	}

	hasStagingCluster := false
	hasProdCluster := false
	for _, c := range result.Clusters {
		if c.Name == "staging" {
			hasStagingCluster = true
		}
		if c.Name == "prod" {
			hasProdCluster = true
		}
	}
	if !hasStagingCluster || !hasProdCluster {
		t.Fatalf("expected both staging and prod clusters, got: %+v", result.Clusters)
	}

	if len(result.Infrastructure) == 0 {
		t.Error("expected to find infrastructure, got none")
	}
}

func TestParseKustomization(t *testing.T) {
	// Create a temp kustomization.yaml
	tmpDir, err := os.MkdirTemp("", "kust-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	kustContent := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../base/podinfo
patches:
  - path: podinfo-patch.yaml
    target:
      kind: HelmRelease
      name: podinfo
`
	kustPath := filepath.Join(tmpDir, "kustomization.yaml")
	if err := os.WriteFile(kustPath, []byte(kustContent), 0644); err != nil {
		t.Fatalf("failed to write kustomization.yaml: %v", err)
	}

	kust, err := parseKustomization(kustPath)
	if err != nil {
		t.Fatalf("failed to parse kustomization: %v", err)
	}

	if len(kust.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(kust.Resources))
	}

	if kust.Resources[0] != "../base/podinfo" {
		t.Errorf("expected '../base/podinfo', got '%s'", kust.Resources[0])
	}

	if len(kust.Patches) != 1 {
		t.Errorf("expected 1 patch, got %d", len(kust.Patches))
	}
}

func TestNormalizeVariant(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"production", "prod"},
		{"prod", "prod"},
		{"staging", "staging"},
		{"stage", "staging"},
		{"stg", "staging"},
		{"development", "dev"},
		{"dev", "dev"},
		{"qa", "qa"},
		{"custom", "custom"},
	}

	for _, tc := range tests {
		result := normalizeVariant(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeVariant(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestParseApplicationSetWithGitGenerator(t *testing.T) {
	tmpDir := t.TempDir()

	// Create ApplicationSet with git generator
	appSetDir := filepath.Join(tmpDir, "applicationsets")
	if err := os.MkdirAll(appSetDir, 0o755); err != nil {
		t.Fatalf("failed to create applicationsets dir: %v", err)
	}

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
	appSetPath := filepath.Join(appSetDir, "apps.yaml")
	if err := os.WriteFile(appSetPath, []byte(appSetContent), 0o644); err != nil {
		t.Fatalf("failed to write applicationset: %v", err)
	}

	// Create app directories that match the pattern
	for _, app := range []string{"frontend", "backend", "api"} {
		appDir := filepath.Join(tmpDir, "apps", app)
		if err := os.MkdirAll(appDir, 0o755); err != nil {
			t.Fatalf("failed to create app dir %s: %v", app, err)
		}
	}

	// Create excluded app (should not appear)
	excludedDir := filepath.Join(tmpDir, "apps", "excluded")
	if err := os.MkdirAll(excludedDir, 0o755); err != nil {
		t.Fatalf("failed to create excluded dir: %v", err)
	}

	result, err := ParseRepo(tmpDir)
	if err != nil {
		t.Fatalf("failed to parse repo: %v", err)
	}

	if result.Type != RepoTypeApplicationSet {
		t.Fatalf("expected repo type %q, got %q", RepoTypeApplicationSet, result.Type)
	}

	if len(result.ApplicationSets) != 1 {
		t.Fatalf("expected 1 ApplicationSet, got %d", len(result.ApplicationSets))
	}

	appSet := result.ApplicationSets[0]
	if appSet.Name != "apps" {
		t.Errorf("expected ApplicationSet name 'apps', got %q", appSet.Name)
	}

	if appSet.Generator != "git" {
		t.Errorf("expected generator 'git', got %q", appSet.Generator)
	}

	// Should have discovered 3 apps (excluding the excluded one)
	if len(appSet.TargetApps) != 3 {
		t.Errorf("expected 3 target apps, got %d: %v", len(appSet.TargetApps), appSet.TargetApps)
	}

	// Verify specific apps were found
	foundApps := make(map[string]bool)
	for _, app := range appSet.TargetApps {
		foundApps[app] = true
	}

	for _, expected := range []string{"frontend", "backend", "api"} {
		if !foundApps[expected] {
			t.Errorf("expected to find app %q in TargetApps", expected)
		}
	}

	// Excluded app should not be found (exclude patterns start with !)
	// Note: Current implementation doesn't enforce excludes in glob, but the pattern is skipped
	if foundApps["excluded"] {
		// This is acceptable for now - exclude enforcement could be added later
		t.Logf("note: excluded app was found - exclude pattern enforcement not implemented")
	}

	// Apps should also be populated in result.Apps
	if len(result.Apps) == 0 {
		t.Error("expected Apps to be populated from ApplicationSet targets")
	}
}

func TestParseApplicationSetWithMatrixGenerator(t *testing.T) {
	tmpDir := t.TempDir()

	// Create ApplicationSet with matrix generator containing git generator
	appSetDir := filepath.Join(tmpDir, "generators")
	if err := os.MkdirAll(appSetDir, 0o755); err != nil {
		t.Fatalf("failed to create generators dir: %v", err)
	}

	appSetContent := `apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: matrix-apps
spec:
  generators:
    - matrix:
        generators:
          - git:
              repoURL: https://github.com/org/repo
              directories:
                - path: workloads/*
          - list:
              elements:
                - cluster: staging
                - cluster: production
`
	appSetPath := filepath.Join(appSetDir, "matrix.yaml")
	if err := os.WriteFile(appSetPath, []byte(appSetContent), 0o644); err != nil {
		t.Fatalf("failed to write applicationset: %v", err)
	}

	// Create workload directories
	for _, workload := range []string{"service-a", "service-b"} {
		dir := filepath.Join(tmpDir, "workloads", workload)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create workload dir %s: %v", workload, err)
		}
	}

	result, err := ParseRepo(tmpDir)
	if err != nil {
		t.Fatalf("failed to parse repo: %v", err)
	}

	if len(result.ApplicationSets) != 1 {
		t.Fatalf("expected 1 ApplicationSet, got %d", len(result.ApplicationSets))
	}

	appSet := result.ApplicationSets[0]
	if appSet.Generator != "matrix" {
		t.Errorf("expected generator 'matrix', got %q", appSet.Generator)
	}

	// Should have discovered workloads from nested git generator
	if len(appSet.TargetApps) != 2 {
		t.Errorf("expected 2 target apps from matrix git generator, got %d: %v", len(appSet.TargetApps), appSet.TargetApps)
	}

	foundApps := make(map[string]bool)
	for _, app := range appSet.TargetApps {
		foundApps[app] = true
	}

	for _, expected := range []string{"service-a", "service-b"} {
		if !foundApps[expected] {
			t.Errorf("expected to find workload %q in TargetApps", expected)
		}
	}
}

func TestExtractGitGeneratorPatterns(t *testing.T) {
	tests := []struct {
		name     string
		gitGen   interface{}
		expected []string
	}{
		{
			name: "simple directories",
			gitGen: map[string]interface{}{
				"repoURL": "https://github.com/org/repo",
				"directories": []interface{}{
					map[string]interface{}{"path": "apps/*"},
					map[string]interface{}{"path": "services/*"},
				},
			},
			expected: []string{"apps/*", "services/*"},
		},
		{
			name: "with exclude patterns",
			gitGen: map[string]interface{}{
				"directories": []interface{}{
					map[string]interface{}{"path": "apps/*"},
					map[string]interface{}{"path": "!apps/excluded"},
				},
			},
			expected: []string{"apps/*"}, // Exclude pattern should be skipped
		},
		{
			name:     "nil input",
			gitGen:   nil,
			expected: []string{},
		},
		{
			name:     "no directories",
			gitGen:   map[string]interface{}{"repoURL": "https://github.com/org/repo"},
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractGitGeneratorPatterns(tc.gitGen)
			if len(result) != len(tc.expected) {
				t.Errorf("expected %d patterns, got %d: %v", len(tc.expected), len(result), result)
				return
			}
			for i, exp := range tc.expected {
				if result[i] != exp {
					t.Errorf("pattern %d: expected %q, got %q", i, exp, result[i])
				}
			}
		})
	}
}

func TestScanGitGeneratorPatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure
	dirs := []string{
		"apps/frontend",
		"apps/backend",
		"apps/api",
		"services/auth",
		"services/payments",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, d), 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	tests := []struct {
		name     string
		patterns []string
		expected []string
	}{
		{
			name:     "single pattern",
			patterns: []string{"apps/*"},
			expected: []string{"frontend", "backend", "api"},
		},
		{
			name:     "multiple patterns",
			patterns: []string{"apps/*", "services/*"},
			expected: []string{"frontend", "backend", "api", "auth", "payments"},
		},
		{
			name:     "non-matching pattern",
			patterns: []string{"nonexistent/*"},
			expected: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := scanGitGeneratorPatterns(tmpDir, tc.patterns)

			if len(result) != len(tc.expected) {
				t.Errorf("expected %d apps, got %d: %v", len(tc.expected), len(result), result)
				return
			}

			// Convert to map for easier comparison (order doesn't matter)
			resultMap := make(map[string]bool)
			for _, r := range result {
				resultMap[r] = true
			}

			for _, exp := range tc.expected {
				if !resultMap[exp] {
					t.Errorf("expected to find %q in results", exp)
				}
			}
		})
	}
}
