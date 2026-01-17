// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package gitops

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseFluxExample(t *testing.T) {
	// Clone the example repo to a temp directory
	tmpDir, err := os.MkdirTemp("", "flux-example-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Clone the repo
	cmd := exec.Command("git", "clone", "--depth=1",
		"https://github.com/fluxcd/flux2-kustomize-helm-example.git",
		tmpDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to clone repo: %v\n%s", err, output)
	}

	// Parse the repo
	result, err := ParseRepo(tmpDir)
	if err != nil {
		t.Fatalf("failed to parse repo: %v", err)
	}

	// Verify apps were found
	if len(result.Apps) == 0 {
		t.Error("expected to find apps, got none")
	}

	// Find podinfo
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

	t.Logf("Found podinfo app:")
	t.Logf("  Base: %s", podinfo.BasePath)
	t.Logf("  Variants: %d", len(podinfo.Variants))
	for _, v := range podinfo.Variants {
		t.Logf("    - %s (%s)", v.Name, v.Path)
	}

	// Verify variants
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

	// Verify clusters were found
	if len(result.Clusters) == 0 {
		t.Error("expected to find clusters, got none")
	}

	t.Logf("\nClusters found: %d", len(result.Clusters))
	for _, c := range result.Clusters {
		t.Logf("  - %s (%s)", c.Name, c.Path)
	}

	// Verify infrastructure was found
	if len(result.Infrastructure) == 0 {
		t.Error("expected to find infrastructure, got none")
	}

	t.Logf("\nInfrastructure found: %d", len(result.Infrastructure))
	for _, i := range result.Infrastructure {
		t.Logf("  - %s (%s)", i.Name, i.Path)
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
