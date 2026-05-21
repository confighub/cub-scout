// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func writeBackResolveTestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestBackResolveFieldGitSource_NoSourcePath(t *testing.T) {
	// Defaults: compareSourcePath is empty → nil return regardless of inputs.
	prev := compareSourcePath
	compareSourcePath = ""
	defer func() { compareSourcePath = prev }()

	got := backResolveFieldGitSource(compareSideSummary{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "prod",
		GitSource: &agent.GitSourceAnchor{RepoURL: "https://github.com/o/r"},
	}, "replicas")
	if got != nil {
		t.Errorf("expected nil when --source-path unset; got %+v", got)
	}
}

func TestBackResolveFieldGitSource_HappyPath(t *testing.T) {
	root := t.TempDir()
	writeBackResolveTestFile(t, root, "apps/prod/api/deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 4
`)

	prev := compareSourcePath
	compareSourcePath = root
	defer func() { compareSourcePath = prev }()

	live := compareSideSummary{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "prod",
		GitSource: &agent.GitSourceAnchor{
			RepoURL:  "https://github.com/o/r",
			Revision: "abc123",
			Path:     "apps/prod/api",
		},
	}

	got := backResolveFieldGitSource(live, "replicas")
	if got == nil {
		t.Fatal("expected non-nil anchor")
	}
	if got.File != "deployment.yaml" {
		t.Errorf("File = %q (expected file relative to GitSource.Path)", got.File)
	}
	if got.Line != 7 {
		t.Errorf("Line = %d, want 7", got.Line)
	}
	// Resource-level anchor preserved.
	if got.RepoURL != "https://github.com/o/r" || got.Revision != "abc123" || got.Path != "apps/prod/api" {
		t.Errorf("clone lost resource-level fields: %+v", got)
	}
}

func TestBackResolveFieldGitSource_UnmappedFieldFallsThrough(t *testing.T) {
	root := t.TempDir()
	writeBackResolveTestFile(t, root, "deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 3
`)

	prev := compareSourcePath
	compareSourcePath = root
	defer func() { compareSourcePath = prev }()

	// "images" has no canonical-path mapping → expected nil.
	got := backResolveFieldGitSource(compareSideSummary{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "prod",
		GitSource: &agent.GitSourceAnchor{},
	}, "images")
	if got != nil {
		t.Errorf("expected nil for unmapped field; got %+v", got)
	}
}

func TestBackResolveFieldGitSource_FieldAbsentFallsThrough(t *testing.T) {
	root := t.TempDir()
	writeBackResolveTestFile(t, root, "deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec: {}
`)
	prev := compareSourcePath
	compareSourcePath = root
	defer func() { compareSourcePath = prev }()

	// .spec.replicas isn't present → BackResolveGitSource returns false → nil.
	got := backResolveFieldGitSource(compareSideSummary{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "prod",
		GitSource: &agent.GitSourceAnchor{},
	}, "replicas")
	if got != nil {
		t.Errorf("expected nil when field absent from source; got %+v", got)
	}
}
