// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func loadUnstructuredFromYAML(t *testing.T, relPath string) []*unstructured.Unstructured {
	t.Helper()
	p := filepath.Join("testdata", relPath)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read fixture %s: %v", p, err)
	}

	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(b), 4096)
	var objs []*unstructured.Unstructured
	for {
		m := map[string]interface{}{}
		err := dec.Decode(&m)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode fixture %s: %v", p, err)
		}
		if len(m) == 0 {
			continue
		}
		objs = append(objs, &unstructured.Unstructured{Object: m})
	}
	return objs
}

func TestBuildCompositionIndex_GroupsByXR(t *testing.T) {
	objs := loadUnstructuredFromYAML(t, filepath.Join("crossplane", "chain.yaml"))
	idx := buildCompositionIndex(objs)

	if len(idx) != 1 {
		t.Fatalf("expected 1 XR group, got %d", len(idx))
	}

	var tree *CrossplaneCompositionTree
	for _, v := range idx {
		tree = v
		break
	}

	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.Platform != "crossplane" {
		t.Fatalf("expected platform crossplane, got %q", tree.Platform)
	}

	if tree.XR.Ref.Name != "example-xr" {
		t.Fatalf("expected XR name example-xr, got %q", tree.XR.Ref.Name)
	}

	if tree.Claim == nil || tree.Claim.Ref.Name != "example-claim" {
		t.Fatalf("expected claim example-claim, got %#v", tree.Claim)
	}

	if len(tree.Managed) != 1 || tree.Managed[0].Ref.Name != "example-instance" {
		t.Fatalf("expected 1 managed instance, got %#v", tree.Managed)
	}
}

func TestBuildCompositionIndex_GroupsKroByInstance(t *testing.T) {
	objs := loadUnstructuredFromYAML(t, filepath.Join("kro", "chain.yaml"))
	idx := buildCompositionIndex(objs)

	if len(idx) != 1 {
		t.Fatalf("expected 1 kro group, got %d", len(idx))
	}

	var tree *CrossplaneCompositionTree
	for _, v := range idx {
		tree = v
		break
	}

	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.Platform != "kro" {
		t.Fatalf("expected platform kro, got %q", tree.Platform)
	}
	if tree.XR.Ref.Name != "checkout" {
		t.Fatalf("expected kro instance checkout, got %q", tree.XR.Ref.Name)
	}
	if tree.Claim == nil || tree.Claim.Ref.Name != "webapp-stack" {
		t.Fatalf("expected definition webapp-stack, got %#v", tree.Claim)
	}
	if len(tree.Managed) != 1 || tree.Managed[0].Ref.Name != "checkout-api" {
		t.Fatalf("expected 1 managed deployment, got %#v", tree.Managed)
	}
}
