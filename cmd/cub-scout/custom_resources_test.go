// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCompileCustomResourceConfigs(t *testing.T) {
	specs := []customResourceSpec{
		{Group: "example.io", Version: "v1", Resource: "widgets"},
		{Group: "", Version: "v1", Resource: "invalid-missing-group"},
		{Group: "example.io", Version: "", Resource: "invalid-missing-version"},
		{Group: "example.io", Version: "v1", Resource: ""},
		{Group: "  another.io  ", Version: "  v1beta1  ", Resource: "  pipelines  "},
	}

	got := compileCustomResourceConfigs(specs)
	if len(got) != 2 {
		t.Fatalf("compile count = %d, want 2", len(got))
	}

	if got[0].GVR.Group != "example.io" || got[0].GVR.Version != "v1" || got[0].GVR.Resource != "widgets" {
		t.Fatalf("first gvr = %#v", got[0].GVR)
	}
	if got[1].GVR.Group != "another.io" || got[1].GVR.Version != "v1beta1" || got[1].GVR.Resource != "pipelines" {
		t.Fatalf("second gvr = %#v", got[1].GVR)
	}
}

func TestBuildMapWatchResourceList_DeduplicatesAndAppends(t *testing.T) {
	oldLoader := mapWatchLoadCustomResourceConfigs
	mapWatchLoadCustomResourceConfigs = func() []customResourceConfig {
		return []customResourceConfig{
			{GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}}, // duplicate default
			{GVR: schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "widgets"}},
		}
	}
	defer func() { mapWatchLoadCustomResourceConfigs = oldLoader }()

	got := buildMapWatchResourceList()
	if len(got) < len(defaultMapWatchResources)+1 {
		t.Fatalf("resource list len = %d, want at least %d", len(got), len(defaultMapWatchResources)+1)
	}

	dupCount := 0
	customCount := 0
	for _, gvr := range got {
		if gvr.Group == "apps" && gvr.Version == "v1" && gvr.Resource == "deployments" {
			dupCount++
		}
		if gvr.Group == "example.io" && gvr.Version == "v1" && gvr.Resource == "widgets" {
			customCount++
		}
	}
	if dupCount != 1 {
		t.Fatalf("deployments count = %d, want 1", dupCount)
	}
	if customCount != 1 {
		t.Fatalf("custom widgets count = %d, want 1", customCount)
	}

	last := got[len(got)-1]
	if last.Group != "example.io" || last.Version != "v1" || last.Resource != "widgets" {
		t.Fatalf("last resource = %#v, want custom widgets", last)
	}
}

func TestLoadCustomResourceConfigs_InvalidYAMLWarns(t *testing.T) {
	t.Setenv(customResourceConfigEnvVar, filepath.Join(t.TempDir(), "resources.yaml"))
	resetCustomResourceConfigCacheForTest()

	if err := os.WriteFile(os.Getenv(customResourceConfigEnvVar), []byte("resources: [\n"), 0644); err != nil {
		t.Fatalf("write invalid yaml: %v", err)
	}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	first := loadCustomResourceConfigs()
	second := loadCustomResourceConfigs()

	_ = w.Close()
	os.Stderr = oldStderr
	out, _ := io.ReadAll(r)
	_ = r.Close()

	if len(first) != 0 || len(second) != 0 {
		t.Fatalf("expected no configs on invalid yaml, got first=%d second=%d", len(first), len(second))
	}
	if !strings.Contains(string(out), "custom resource config ignored") {
		t.Fatalf("expected warning message, got: %q", string(out))
	}
}

func TestLoadCustomResourceConfigs_EnvOverrideParses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resources.yaml")
	t.Setenv(customResourceConfigEnvVar, path)
	resetCustomResourceConfigCacheForTest()

	content := `
resources:
  - group: example.io
    version: v1
    resource: widgets
  - group: platform.example.io
    version: v1beta1
    resource: pipelines
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got := loadCustomResourceConfigs()
	if len(got) != 2 {
		t.Fatalf("config count = %d, want 2", len(got))
	}
	if got[0].GVR.Group != "example.io" || got[0].GVR.Resource != "widgets" {
		t.Fatalf("first config gvr = %#v", got[0].GVR)
	}
	if got[1].GVR.Group != "platform.example.io" || got[1].GVR.Version != "v1beta1" || got[1].GVR.Resource != "pipelines" {
		t.Fatalf("second config gvr = %#v", got[1].GVR)
	}
}

func TestCollectWatchResourceList_IncludesConfiguredCustomResources(t *testing.T) {
	oldLoader := mapWatchLoadCustomResourceConfigs
	mapWatchLoadCustomResourceConfigs = func() []customResourceConfig {
		return []customResourceConfig{
			{GVR: schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "widgets"}},
		}
	}
	defer func() { mapWatchLoadCustomResourceConfigs = oldLoader }()

	got := collectWatchResourceList()
	if len(got) == 0 {
		t.Fatal("collectWatchResourceList() returned empty list")
	}
	last := got[len(got)-1]
	if last.Group != "example.io" || last.Version != "v1" || last.Resource != "widgets" {
		t.Fatalf("last resource = %#v, want custom widgets", last)
	}
}

func TestMapResourceList_IncludesConfiguredCustomResources(t *testing.T) {
	oldLoader := mapWatchLoadCustomResourceConfigs
	mapWatchLoadCustomResourceConfigs = func() []customResourceConfig {
		return []customResourceConfig{
			{GVR: schema.GroupVersionResource{Group: "example.io", Version: "v1", Resource: "widgets"}},
		}
	}
	defer func() { mapWatchLoadCustomResourceConfigs = oldLoader }()

	got := collectMapResourceList()
	if len(got) == 0 {
		t.Fatal("collectMapResourceList() returned empty list")
	}
	last := got[len(got)-1]
	if last.Group != "example.io" || last.Version != "v1" || last.Resource != "widgets" {
		t.Fatalf("last resource = %#v, want custom widgets", last)
	}
}
