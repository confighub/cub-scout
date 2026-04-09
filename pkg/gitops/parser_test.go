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
	// TargetApps now contains paths as unique identifiers
	if len(appSet.TargetApps) != 3 {
		t.Errorf("expected 3 target apps, got %d: %v", len(appSet.TargetApps), appSet.TargetApps)
	}

	// Verify specific app paths were found
	foundPaths := make(map[string]bool)
	for _, appPath := range appSet.TargetApps {
		foundPaths[appPath] = true
	}

	expectedPaths := []string{
		filepath.Join("apps", "frontend"),
		filepath.Join("apps", "backend"),
		filepath.Join("apps", "api"),
	}
	for _, expected := range expectedPaths {
		if !foundPaths[expected] {
			t.Errorf("expected to find path %q in TargetApps", expected)
		}
	}

	// Excluded app path should not be found
	excludedPath := filepath.Join("apps", "excluded")
	if foundPaths[excludedPath] {
		t.Error("excluded app path should not be found")
	}

	// Verify TargetAppNames maps path -> name
	if appSet.TargetAppNames == nil {
		t.Error("expected TargetAppNames to be populated")
	} else {
		for _, path := range expectedPaths {
			expectedName := filepath.Base(path)
			name, ok := appSet.TargetAppNames[path]
			if !ok {
				t.Errorf("expected TargetAppNames to contain path %q", path)
			} else if name != expectedName {
				t.Errorf("TargetAppNames[%q] = %q, want %q", path, name, expectedName)
			}
		}
	}

	// Apps should also be populated in result.Apps with correct BasePath
	if len(result.Apps) != 3 {
		t.Errorf("expected 3 Apps, got %d", len(result.Apps))
	} else {
		for _, app := range result.Apps {
			expectedPath := filepath.Join("apps", app.Name)
			if app.BasePath != expectedPath {
				t.Errorf("App %q BasePath = %q, want %q", app.Name, app.BasePath, expectedPath)
			}
		}
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
	// TargetApps now contains paths as unique identifiers
	if len(appSet.TargetApps) != 2 {
		t.Errorf("expected 2 target apps from matrix git generator, got %d: %v", len(appSet.TargetApps), appSet.TargetApps)
	}

	foundPaths := make(map[string]bool)
	for _, path := range appSet.TargetApps {
		foundPaths[path] = true
	}

	expectedPaths := []string{
		filepath.Join("workloads", "service-a"),
		filepath.Join("workloads", "service-b"),
	}
	for _, expected := range expectedPaths {
		if !foundPaths[expected] {
			t.Errorf("expected to find workload path %q in TargetApps", expected)
		}
	}
}

func TestParseApplicationSetWithDuplicateBasenames(t *testing.T) {
	tmpDir := t.TempDir()

	// Create ApplicationSet with multiple patterns that could match same-named directories
	appSetDir := filepath.Join(tmpDir, "applicationsets")
	if err := os.MkdirAll(appSetDir, 0o755); err != nil {
		t.Fatalf("failed to create applicationsets dir: %v", err)
	}

	appSetContent := `apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: multi-team
spec:
  generators:
    - git:
        repoURL: https://github.com/org/repo
        directories:
          - path: apps/team-a/*
          - path: services/team-b/*
`
	appSetPath := filepath.Join(appSetDir, "multi-team.yaml")
	if err := os.WriteFile(appSetPath, []byte(appSetContent), 0o644); err != nil {
		t.Fatalf("failed to write applicationset: %v", err)
	}

	// Create directories with duplicate basenames in different paths
	// Both team-a and team-b have an "api" app
	dirs := []string{
		filepath.Join(tmpDir, "apps", "team-a", "api"),
		filepath.Join(tmpDir, "apps", "team-a", "web"),
		filepath.Join(tmpDir, "services", "team-b", "api"),
		filepath.Join(tmpDir, "services", "team-b", "worker"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
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

	// Should have discovered 4 apps (2 from each pattern)
	// Even though both have "api", they should be treated as separate apps
	if len(appSet.TargetApps) != 4 {
		t.Errorf("expected 4 target apps, got %d: %v", len(appSet.TargetApps), appSet.TargetApps)
	}

	// All 4 paths should be in TargetApps
	expectedPaths := []string{
		filepath.Join("apps", "team-a", "api"),
		filepath.Join("apps", "team-a", "web"),
		filepath.Join("services", "team-b", "api"),
		filepath.Join("services", "team-b", "worker"),
	}

	foundPaths := make(map[string]bool)
	for _, path := range appSet.TargetApps {
		foundPaths[path] = true
	}

	for _, expected := range expectedPaths {
		if !foundPaths[expected] {
			t.Errorf("expected TargetApps to contain path %q", expected)
		}
	}

	// All 4 apps should be in result.Apps with correct paths
	if len(result.Apps) != 4 {
		t.Errorf("expected 4 Apps, got %d", len(result.Apps))
	}

	// Check that both "api" apps are present with different paths
	apiCount := 0
	apiPaths := make(map[string]bool)
	for _, app := range result.Apps {
		if app.Name == "api" {
			apiCount++
			apiPaths[app.BasePath] = true
		}
	}

	if apiCount != 2 {
		t.Errorf("expected 2 apps named 'api', got %d", apiCount)
	}

	if !apiPaths[filepath.Join("apps", "team-a", "api")] {
		t.Error("expected to find api app at apps/team-a/api")
	}
	if !apiPaths[filepath.Join("services", "team-b", "api")] {
		t.Error("expected to find api app at services/team-b/api")
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

func TestParseApplicationSetWithNestedPaths(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a deeply nested ApplicationSet structure like examples/apptique-examples/argo-applicationset
	appSetDir := filepath.Join(tmpDir, "examples", "apptique-examples", "argo-applicationset")
	if err := os.MkdirAll(appSetDir, 0o755); err != nil {
		t.Fatalf("failed to create nested appset dir: %v", err)
	}

	// Create ApplicationSet that targets envs/*
	appSetContent := `apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: apptique
spec:
  generators:
    - git:
        repoURL: https://github.com/org/repo
        directories:
          - path: examples/apptique-examples/argo-applicationset/envs/*
`
	appSetPath := filepath.Join(appSetDir, "applicationset.yaml")
	if err := os.WriteFile(appSetPath, []byte(appSetContent), 0o644); err != nil {
		t.Fatalf("failed to write applicationset: %v", err)
	}

	// Create env directories that match the pattern
	envsDir := filepath.Join(appSetDir, "envs")
	for _, env := range []string{"dev", "prod"} {
		envDir := filepath.Join(envsDir, env)
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			t.Fatalf("failed to create env dir %s: %v", env, err)
		}
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

	// Should have discovered 2 apps (dev, prod)
	// TargetApps now contains paths as unique identifiers
	if len(appSet.TargetApps) != 2 {
		t.Errorf("expected 2 target apps, got %d: %v", len(appSet.TargetApps), appSet.TargetApps)
	}

	// Expected paths for each app
	expectedPaths := map[string]string{
		"dev":  filepath.Join("examples", "apptique-examples", "argo-applicationset", "envs", "dev"),
		"prod": filepath.Join("examples", "apptique-examples", "argo-applicationset", "envs", "prod"),
	}

	// Verify TargetApps contains the full paths
	foundPaths := make(map[string]bool)
	for _, path := range appSet.TargetApps {
		foundPaths[path] = true
	}
	for name, expectedPath := range expectedPaths {
		if !foundPaths[expectedPath] {
			t.Errorf("expected TargetApps to contain path for %q: %q", name, expectedPath)
		}
	}

	// Verify TargetAppNames maps path -> name
	for name, path := range expectedPaths {
		actualName, ok := appSet.TargetAppNames[path]
		if !ok {
			t.Errorf("expected TargetAppNames to contain path %q", path)
		} else if actualName != name {
			t.Errorf("TargetAppNames[%q] = %q, want %q", path, actualName, name)
		}
	}

	// Verify Apps have correct BasePath
	if len(result.Apps) != 2 {
		t.Errorf("expected 2 Apps, got %d", len(result.Apps))
	}

	for _, app := range result.Apps {
		expectedPath, ok := expectedPaths[app.Name]
		if !ok {
			t.Errorf("unexpected app %q", app.Name)
			continue
		}
		if app.BasePath != expectedPath {
			t.Errorf("App %q BasePath = %q, want %q", app.Name, app.BasePath, expectedPath)
		}
	}
}
