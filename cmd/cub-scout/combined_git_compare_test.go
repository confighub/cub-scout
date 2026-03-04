// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/gitops"
)

func TestBuildGitComparison_Deterministic(t *testing.T) {
	left := &gitops.RepoStructure{
		Apps: []gitops.AppDefinition{
			{
				Name: "api",
				Variants: []gitops.VariantDefinition{
					{Name: "prod", Path: "apps/prod"},
				},
			},
			{
				Name: "worker",
				Variants: []gitops.VariantDefinition{
					{Name: "prod", Path: "apps/prod"},
				},
			},
		},
	}
	right := &gitops.RepoStructure{
		Apps: []gitops.AppDefinition{
			{
				Name: "api",
				Variants: []gitops.VariantDefinition{
					{Name: "prod", Path: "apps/prod"},
				},
			},
			{
				Name: "billing",
				Variants: []gitops.VariantDefinition{
					{Name: "prod", Path: "apps/prod"},
				},
			},
		},
	}

	got := buildGitComparison(left, right)
	want := []GitCompareEntry{
		{
			App:       "api",
			Variant:   "prod",
			Status:    "aligned",
			LeftPath:  "apps/prod",
			RightPath: "apps/prod",
		},
		{
			App:       "billing",
			Variant:   "prod",
			Status:    "right-only",
			RightPath: "apps/prod",
		},
		{
			App:      "worker",
			Variant:  "prod",
			Status:   "left-only",
			LeftPath: "apps/prod",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildGitComparison() mismatch\nwant=%+v\ngot=%+v", want, got)
	}
}

func TestRunCombined_GitPathCompareJSON(t *testing.T) {
	leftRepo := createCompareRepoFixture(t, []string{"api", "worker"})
	rightRepo := createCompareRepoFixture(t, []string{"api", "billing"})

	restore := setCombinedFlagState(combinedFlagState{
		gitPath:        leftRepo,
		gitPathCompare: rightRepo,
		json:           true,
	})
	defer restore()

	output := captureStdout(t, func() {
		if err := runCombined(nil, nil); err != nil {
			t.Fatalf("runCombined() error = %v", err)
		}
	})

	var result struct {
		GitCompare []GitCompareEntry `json:"gitCompare"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("parse combined JSON: %v\n%s", err, output)
	}
	if len(result.GitCompare) == 0 {
		t.Fatal("gitCompare output is empty")
	}

	hasAlignedAPI := false
	hasLeftOnlyWorker := false
	hasRightOnlyBilling := false
	for _, entry := range result.GitCompare {
		if entry.App == "api" && entry.Status == "aligned" {
			hasAlignedAPI = true
		}
		if entry.App == "worker" && entry.Status == "left-only" {
			hasLeftOnlyWorker = true
		}
		if entry.App == "billing" && entry.Status == "right-only" {
			hasRightOnlyBilling = true
		}
	}
	if !hasAlignedAPI || !hasLeftOnlyWorker || !hasRightOnlyBilling {
		t.Fatalf("unexpected gitCompare entries: %+v", result.GitCompare)
	}
}

func TestRunCombined_GitCompareRequiresPrimary(t *testing.T) {
	rightRepo := createCompareRepoFixture(t, []string{"api"})

	restore := setCombinedFlagState(combinedFlagState{
		gitPathCompare: rightRepo,
		json:           true,
	})
	defer restore()

	err := runCombined(nil, nil)
	if err == nil {
		t.Fatal("expected error when compare repo is set without primary repo")
	}
	if !strings.Contains(err.Error(), "requires --git-url or --git-path") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func createCompareRepoFixture(t *testing.T, apps []string) string {
	t.Helper()
	repoDir := t.TempDir()

	for _, app := range apps {
		baseDir := filepath.Join(repoDir, "apps", "base", app)
		if err := os.MkdirAll(baseDir, 0o755); err != nil {
			t.Fatalf("mkdir base dir: %v", err)
		}
		baseKustomization := []byte("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources: []\n")
		if err := os.WriteFile(filepath.Join(baseDir, "kustomization.yaml"), baseKustomization, 0o644); err != nil {
			t.Fatalf("write base kustomization: %v", err)
		}
	}

	prodDir := filepath.Join(repoDir, "apps", "prod")
	if err := os.MkdirAll(prodDir, 0o755); err != nil {
		t.Fatalf("mkdir prod dir: %v", err)
	}

	var resources strings.Builder
	for _, app := range apps {
		resources.WriteString("- ../base/" + app + "\n")
	}
	prodKustomization := []byte("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n" + resources.String())
	if err := os.WriteFile(filepath.Join(prodDir, "kustomization.yaml"), prodKustomization, 0o644); err != nil {
		t.Fatalf("write prod kustomization: %v", err)
	}

	return repoDir
}
