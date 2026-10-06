// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

func TestKroOwnerScopeSharedTraceRendering(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "kro-composition", "owner-reference-scope.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := loadRecordedMapTestSnapshot(t, raw)
	lineage, ok := agent.ResolveKroLineage(snapshot.Objects[0], snapshot.Objects)
	if !ok || lineage.Instance.Present || lineage.Instance.Ref.Namespace != "" {
		t.Fatalf("asserted wrong parent: %+v", lineage)
	}
	encoded, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	var decoded agent.KroLineage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Instance.Present || decoded.Instance.Ref.Namespace != "" {
		t.Fatal("JSON lost partial scope")
	}
	result := &agent.ReverseTraceResult{Object: lineage.Managed.Ref, Owner: "kro", Objects: snapshot.Objects, K8sChain: []agent.ChainLink{{Kind: "Deployment", Name: "child", Namespace: "prod"}}}
	var output bytes.Buffer
	if err := renderReverseTraceHuman(&output, result, false); err != nil {
		t.Fatal(err)
	}
	pane := LocalClusterModel{traceOutput: output.String()}
	for label, text := range map[string]string{"CLI": output.String(), "TUI": pane.renderTrace()} {
		for _, fact := range []string{"WebApp/checkout", "partial lineage", "instance:owner_uid_not_observed"} {
			if !strings.Contains(text, fact) {
				t.Fatalf("%s missing %q: %s", label, fact, text)
			}
		}
		if strings.Contains(text, "WebApp/checkout in dev") || strings.Contains(text, "WebApp/checkout in prod") {
			t.Fatalf("%s guessed parent namespace", label)
		}
	}
	trees := buildCompositionIndex(snapshot.Objects)
	for _, tree := range trees {
		for _, managed := range tree.Managed {
			if managed.Ref.Name == "child" && tree.XR.Present {
				t.Fatalf("composition promoted partial child: %+v", tree)
			}
		}
	}
}
