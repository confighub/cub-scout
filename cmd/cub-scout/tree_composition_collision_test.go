// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompositionRootDisplayCollisions(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "composition-root-collisions", "inventory.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, raw)
	var firstJSON, firstHuman string
	for _, reverse := range []bool{false, true} {
		if reverse {
			for i, j := 0, len(snapshot.Objects)-1; i < j; i, j = i+1, j-1 {
				snapshot.Objects[i], snapshot.Objects[j] = snapshot.Objects[j], snapshot.Objects[i]
			}
		}
		trees := buildCompositionIndex(snapshot.Objects)
		if len(trees) != 6 {
			t.Fatalf("reverse=%v: merged distinct references: %d trees", reverse, len(trees))
		}
		for key, tree := range trees {
			if !strings.Contains(key, "::ref=") {
				t.Fatalf("collision retains unsuffixed key %q", key)
			}
			if len(tree.Managed) != 1 {
				t.Fatalf("mixed children: %+v", tree)
			}
			expected := "child-a"
			switch {
			case !tree.XR.Present && tree.XR.Ref.Group == "":
				expected = "child-unknown"
				if tree.XR.Ref.Version != "" || tree.XR.Ref.Namespace != "" {
					t.Fatal("unknown bucket invented parent metadata")
				}
			case !tree.XR.Present:
				expected = "child-partial"
			case tree.Platform == "kro" && tree.XR.Ref.Group == "a.kro.run":
				expected = "workload-a"
			case tree.Platform == "kro":
				expected = "workload-b"
			case tree.XR.Ref.Group == "b.crossplane.io":
				expected = "child-b"
			}
			if tree.Managed[0].Ref.Name != expected {
				t.Fatalf("parent %s present=%v got %s want %s", tree.XR.Ref.Group, tree.XR.Present, tree.Managed[0].Ref.Name, expected)
			}
		}

		for _, obj := range snapshot.Objects {
			owners := obj.GetOwnerReferences()
			if len(owners) == 0 {
				continue
			}
			var lineageText string
			if obj.GetKind() == "ConfigMap" {
				lineage, ok := agent.ResolveCrossplaneLineage(obj, snapshot.Objects)
				if !ok {
					t.Fatal("missing Crossplane lineage")
				}
				lineageText = renderCrossplaneLineageHuman(lineage)
			} else if obj.GetKind() == "Deployment" {
				lineage, ok := agent.ResolveKroLineage(obj, snapshot.Objects)
				if !ok {
					t.Fatal("missing kro lineage")
				}
				lineageText = renderKroLineageHuman(lineage)
			} else {
				continue
			}
			pane := LocalClusterModel{traceOutput: lineageText}
			if !strings.Contains(pane.renderTrace(), owners[0].APIVersion) || !strings.Contains(pane.renderTrace(), obj.GetName()) {
				t.Fatal("loaded trace pane lost owner API evidence")
			}
		}
		encoded, err := json.Marshal(trees)
		if err != nil {
			t.Fatal(err)
		}
		human := captureStdout(t, func() { printCompositionTreeHuman(trees) })
		for _, value := range []string{"a.crossplane.io/v1", "b.crossplane.io/v1", "a.kro.run/v1alpha1", "b.kro.run/v1alpha1", "partial lineage", "apiVersion=unknown"} {
			if !strings.Contains(human, value) {
				t.Fatalf("human root misses %q: %s", value, human)
			}
		}
		if !reverse {
			firstJSON, firstHuman = string(encoded), human
		} else if string(encoded) != firstJSON || human != firstHuman {
			t.Fatal("input order changes tree projection")
		}
	}
}

func TestCompositionUnambiguousLegacyKeys(t *testing.T) {
	for _, fixture := range []string{filepath.Join("crossplane", "chain.yaml"), filepath.Join("kro", "chain.yaml")} {
		trees := buildCompositionIndex(loadUnstructuredFromYAML(t, fixture))
		if len(trees) != 1 {
			t.Fatalf("unexpected groups for %s", fixture)
		}
		for key, tree := range trees {
			if key != tree.Platform+"::"+tree.XR.Ref.String() {
				t.Fatalf("legacy unambiguous key changed: %s", key)
			}
		}
	}
}

func TestCompositionFullReferenceChildOrder(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "composition-root-collisions", "inventory.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, raw)
	for _, api := range []string{"a.example.org/v1", "b.example.org/v2"} {
		child := snapshot.Objects[2].DeepCopy()
		child.SetAPIVersion(api)
		child.SetName("same-child")
		snapshot.Objects = append(snapshot.Objects, child)
	}
	var first string
	for _, reverse := range []bool{false, true} {
		if reverse {
			for i, j := 0, len(snapshot.Objects)-1; i < j; i, j = i+1, j-1 {
				snapshot.Objects[i], snapshot.Objects[j] = snapshot.Objects[j], snapshot.Objects[i]
			}
		}
		trees := buildCompositionIndex(snapshot.Objects)
		encoded, err := json.Marshal(trees)
		if err != nil {
			t.Fatal(err)
		}
		if !reverse {
			first = string(encoded)
		} else if first != string(encoded) {
			t.Fatal("same-display children reorder JSON")
		}
		text := captureStdout(t, func() { printCompositionTreeHuman(trees) })
		for _, api := range []string{"apiVersion=a.example.org/v1", "apiVersion=b.example.org/v2"} {
			if !strings.Contains(text, api) {
				t.Fatalf("unqualified child reference %s", api)
			}
		}
	}
}

func TestCompositionServedVersionsStaySeparate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "composition-root-collisions", "inventory.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, raw)
	snapshot.Objects[1].SetAPIVersion("a.crossplane.io/v2")
	owners := snapshot.Objects[3].GetOwnerReferences()
	owners[0].APIVersion = "a.crossplane.io/v2"
	snapshot.Objects[3].SetOwnerReferences(owners)
	snapshot.Objects[6].SetAPIVersion("a.kro.run/v1alpha2")
	owners = snapshot.Objects[8].GetOwnerReferences()
	owners[0].APIVersion = "a.kro.run/v1alpha2"
	snapshot.Objects[8].SetOwnerReferences(owners)
	trees := buildCompositionIndex(snapshot.Objects)
	if len(trees) != 6 {
		t.Fatalf("served versions merged: %d", len(trees))
	}
	for _, tree := range trees {
		if !tree.XR.Present {
			continue
		}
		expected := "child-a"
		if tree.Platform == "crossplane" && tree.XR.Ref.Version == "v2" {
			expected = "child-b"
		}
		if tree.Platform == "kro" {
			expected = "workload-a"
			if tree.XR.Ref.Version == "v1alpha2" {
				expected = "workload-b"
			}
		}
		if len(tree.Managed) != 1 || tree.Managed[0].Ref.Name != expected {
			t.Fatalf("version mixed children: %+v", tree)
		}
	}
}
