// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRecordedExplainContractFixtureAndScaffoldArePinned(t *testing.T) {
	caseRoot := filepath.Join("..", "..", "evals", "recorded-explain-contract")
	fixture, err := os.ReadFile(filepath.Join(caseRoot, "fixtures", "deployments.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join("..", "..", "evals", "fixtures", "cluster", "deployments.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(fixture) != string(source) {
		t.Fatal("recorded explain input must remain byte-identical to the complete source List")
	}
	digest := sha256.Sum256(fixture)
	if got := hex.EncodeToString(digest[:]); got != "305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8" {
		t.Fatalf("fixture SHA-256 = %s", got)
	}

	workspace := t.TempDir()
	scaffoldPath, err := filepath.Abs(filepath.Join(caseRoot, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scaffoldPath)
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("recorded scaffold failed: %v\n%s", err, output)
	}
	written, err := os.ReadFile(filepath.Join(workspace, "cluster", "deployments.yaml"))
	if err != nil || string(written) != string(fixture) {
		t.Fatalf("scaffold output differs from immutable fixture: err=%v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(workspace, "cluster", "recording.json"))
	if err != nil || !strings.Contains(string(manifest), hex.EncodeToString(digest[:])) || !strings.Contains(string(manifest), `"objectCount": 14`) {
		t.Fatalf("scaffold recording manifest is missing source facts: err=%v\n%s", err, manifest)
	}

	caseData, err := os.ReadFile(filepath.Join(caseRoot, "case.yaml"))
	if err != nil || !strings.Contains(string(caseData), "FIXTURE-OWNED-SCAFFOLD") {
		t.Fatalf("case must declare fixture-owned scaffold: %v", err)
	}
	grader, err := os.ReadFile(filepath.Join(caseRoot, "graders", "recorded-schema.md"))
	if err != nil {
		t.Fatal(err)
	}
	patternLine := ""
	for _, line := range strings.Split(string(grader), "\n") {
		if strings.HasPrefix(line, "pattern: '") {
			patternLine = strings.TrimSuffix(strings.TrimPrefix(line, "pattern: '"), "'")
			break
		}
	}
	pattern, err := regexp.Compile(patternLine)
	if err != nil {
		t.Fatalf("recorded grader regex does not compile: %v", err)
	}
	answer := `{"owner":"Flux","health":"Current","healthMeasurement":{"status":"measured","scope":"object-local-readiness"},"mutationCause":"manual-edit","mutationManager":"kubectl-set","fieldPath":".spec.template.spec.containers[name=\"checkout\"].image","recordedInputSha256":"305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8","resourceRead":null}`
	if !pattern.MatchString(answer) || pattern.MatchString(strings.Replace(answer, `"resourceRead":null`, `"resourceRead":{}`, 1)) || pattern.MatchString(strings.Replace(answer, `"owner":"Flux"`, `"owner":"Unknown"`, 1)) {
		t.Fatal("recorded grader must accept the exact evidence contract and reject wrong owner or live-read claims")
	}
}
