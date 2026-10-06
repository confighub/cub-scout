// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

func TestCrossplaneAmbiguitySharedTraceRendering(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "crossplane-system", "lineage-ambiguity.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, raw)
	lineage, ok := agent.ResolveCrossplaneLineage(snapshot.Objects[0], snapshot.Objects)
	if !ok || lineage.Composite.Present || lineage.Composite.Ref.Kind != "CompositeResource" {
		t.Fatalf("asserted ambiguous XR: %+v", lineage)
	}
	result := &agent.ReverseTraceResult{Object: lineage.Managed.Ref, Owner: "crossplane", K8sChain: []agent.ChainLink{{Kind: "ConfigMap", Name: "child"}}, Objects: snapshot.Objects}
	var output bytes.Buffer
	if err := renderReverseTraceHuman(&output, result, false); err != nil {
		t.Fatal(err)
	}
	model := LocalClusterModel{traceOutput: output.String()}
	for label, text := range map[string]string{"CLI": output.String(), "TUI": model.renderTrace(), "lineage": renderCrossplaneLineageHuman(lineage)} {
		for _, fact := range []string{"CompositeResource/parent", "partial lineage", "xr:ambiguous"} {
			if !strings.Contains(text, fact) {
				t.Errorf("%s missing %q", label, fact)
			}
		}
		if strings.Contains(text, "XOther/parent") || strings.Contains(text, "XApp/parent") {
			t.Errorf("%s guessed a parent type", label)
		}
	}
	composition := buildCompositionIndex(snapshot.Objects)
	for _, root := range composition {
		if root.XR.Present {
			t.Fatalf("composition asserted ambiguous root: %+v", root)
		}
	}
}
