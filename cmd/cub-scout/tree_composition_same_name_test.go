// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

func TestCompositionSameNameDistinctChildren(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "composition-same-name", "inventory.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, raw)
	for _, reverse := range []bool{false, true} {
		if reverse {
			for i, j := 0, len(snapshot.Objects)-1; i < j; i, j = i+1, j-1 {
				snapshot.Objects[i], snapshot.Objects[j] = snapshot.Objects[j], snapshot.Objects[i]
			}
		}
		trees := buildCompositionIndex(snapshot.Objects)
		if len(trees) != 2 {
			t.Fatalf("reverse=%v: got %d roots: %+v", reverse, len(trees), trees)
		}
		for _, tree := range trees {
			childKind := "ConfigMap"
			if tree.Platform == "kro" {
				childKind = "Deployment"
			}
			if !tree.XR.Present || len(tree.Managed) != 1 || tree.Managed[0].Ref.Kind != childKind || tree.Managed[0].Ref.Name != tree.XR.Ref.Name || !tree.Managed[0].Present {
				t.Fatalf("reverse=%v: lost distinct child: %+v", reverse, tree)
			}
			if tree.Managed[0].Ref == tree.XR.Ref {
				t.Fatal("self-reference included")
			}
		}
		for _, childIndex := range []int{1, 4} {
			// Find the child after either input ordering; exercise the existing trace pane.
			childName, childKind := "shared", "ConfigMap"
			if childIndex == 4 {
				childName, childKind = "checkout", "Deployment"
			}
			var text string
			for _, obj := range snapshot.Objects {
				if obj.GetName() != childName || obj.GetKind() != childKind {
					continue
				}
				if childKind == "ConfigMap" {
					lineage, ok := agent.ResolveCrossplaneLineage(obj, snapshot.Objects)
					if !ok {
						t.Fatal("missing Crossplane lineage")
					}
					text = renderCrossplaneLineageHuman(lineage)
				} else {
					lineage, ok := agent.ResolveKroLineage(obj, snapshot.Objects)
					if !ok {
						t.Fatal("missing kro lineage")
					}
					text = renderKroLineageHuman(lineage)
				}
			}
			pane := LocalClusterModel{traceOutput: text}
			if !strings.Contains(pane.renderTrace(), childKind+"/"+childName) {
				t.Fatal("loaded trace pane lost child")
			}
		}
		encoded, err := json.Marshal(trees)
		if err != nil {
			t.Fatal(err)
		}
		var roundtrip map[string]*CrossplaneCompositionTree
		if err := json.Unmarshal(encoded, &roundtrip); err != nil {
			t.Fatal(err)
		}
		for key, tree := range trees {
			if len(roundtrip[key].Managed) != 1 || roundtrip[key].Managed[0].Ref != tree.Managed[0].Ref {
				t.Fatal("JSON lost child")
			}
		}
		human := captureStdout(t, func() { printCompositionTreeHuman(trees) })
		for _, child := range []string{"ConfigMap/shared in team-a", "Deployment/checkout in app"} {
			if !strings.Contains(human, child) {
				t.Fatalf("human renderer missing %s: %s", child, human)
			}
		}
	}
}
