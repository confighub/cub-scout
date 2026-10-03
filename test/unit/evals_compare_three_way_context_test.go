// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareThreeWayContextEvalScaffoldIsSyntheticAndExact(t *testing.T) {
	checkCompareThreeWayContextScaffold(t, filepath.Join("..", "..", "evals", "compare-three-way-context"))
}

func checkCompareThreeWayContextScaffold(t *testing.T, caseDir string) {
	t.Helper()
	root, err := filepath.Abs(caseDir)
	if err != nil {
		t.Fatal(err)
	}
	for name, marker := range map[string]string{"case.yaml": "FIXTURE-OWNED-SCAFFOLD", "scaffold.sh": "FIXTURE-OWNED"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !strings.Contains(string(data), marker) {
			t.Fatalf("%s must declare fixture-owned evidence: %v", name, err)
		}
	}
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Provenance struct {
			Kind, ModelExecution                       string
			LiveClusterProof, RealConfigHubServerProof bool
		}
		SelectedObservation struct {
			Context             string
			LiveImages          []string
			Agreement, Followup string
			Omissions           []struct{ Phase, Reason string }
		}
		DeniedDiscovery struct {
			Context, Agreement string
			Resources          []json.RawMessage
			Omissions          []struct{ Phase, Reason string }
		}
		ViewSelection struct {
			SelectedUnit, Candidate struct{ UnitID, SpaceID, Slug string }
			CandidateMembership     string
		}
		AmbientContext string
	}
	if err := json.Unmarshal(fixture, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Provenance.Kind != "synthetic-example" || evidence.Provenance.ModelExecution != "not-run" || evidence.Provenance.LiveClusterProof || evidence.Provenance.RealConfigHubServerProof {
		t.Fatal("synthetic provenance boundary changed")
	}
	selected := evidence.SelectedObservation
	if selected.Context != "alpha-context" || len(selected.LiveImages) != 1 || selected.LiveImages[0] != "alpha:v1" || selected.Agreement != "partial" || !strings.Contains(selected.Followup, "--kube-context alpha-context") || len(selected.Omissions) != 2 || selected.Omissions[0].Reason != "forbidden" {
		t.Fatal("selected partial comparison evidence changed")
	}
	denied := evidence.DeniedDiscovery
	if denied.Context != "alpha-context" || denied.Agreement != "partial" || len(denied.Resources) != 0 || len(denied.Omissions) != 3 {
		t.Fatal("denied discovery is not complete empty inventory")
	}
	view := evidence.ViewSelection
	if view.SelectedUnit.Slug != view.Candidate.Slug || view.SelectedUnit.SpaceID == view.Candidate.SpaceID || view.SelectedUnit.UnitID == view.Candidate.UnitID || view.CandidateMembership != "excluded" {
		t.Fatal("cross-space slug collision fixture changed")
	}
	outputDir := t.TempDir()
	command := exec.Command("bash", filepath.Join(root, "scaffold.sh"))
	command.Dir = outputDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("scaffold: %v: %s", err, output)
	}
	copied, err := os.ReadFile(filepath.Join(outputDir, "cluster", "evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fixture, copied) {
		t.Fatal("scaffold must copy exact identical evidence")
	}
	for _, name := range []string{"README.md", "graders/verified-answer.md"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ToLower(string(data)), "not run") {
			t.Fatalf("%s must retain unrun execution status", name)
		}
	}
}
