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
