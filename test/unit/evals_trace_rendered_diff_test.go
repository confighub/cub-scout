// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTraceRenderedDiffContractScaffoldIsExactAndSynthetic(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "trace-rendered-diff-contract")
	checkTraceRenderedDiffScaffold(t, root)
	prompt, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"synthetic teaching fixture", "not a", "does not predict reconciliation"} {
		if !strings.Contains(string(prompt), marker) {
			t.Errorf("prompt must preserve fixture limit %q", marker)
		}
	}
	if strings.Contains(string(prompt), "claude plugin eval") || strings.Contains(string(prompt), "model run") {
		t.Fatal("the prepared interpretation case must not imply it was model-run")
	}
	grader, err := os.ReadFile(filepath.Join(root, "graders", "answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindStringSubmatch(string(grader))
	if len(match) != 2 {
		t.Fatal("grader must declare one portable single-quoted regex")
	}
	pattern, err := regexp.Compile(match[1])
	if err != nil {
		t.Fatalf("compile rendered-diff grader: %v", err)
	}
	answer := `{"status":"changed","comparison":"authored-fields-only","coverage":"one-selected-object","resource":{"apiVersion":"apps/v1","kind":"Deployment","namespace":"shop","name":"checkout","uid":"synthetic-uid-alpha","resourceVersion":"17"},"live_read":{"observed_at":"2026-10-01T12:13:14Z","scope_discovery_reads":0,"bounded_reader_discovery_reads":1,"object_reads":1},"difference":{"field":"spec.replicas","desired":"2","live":"1"},"limits":"local rendered input is not controller desired state; one object only; no reconciliation prediction"}`
	if !pattern.MatchString(answer) || pattern.MatchString(strings.Replace(answer, `"one-selected-object"`, `"complete-object-set"`, 1)) || pattern.MatchString(strings.Replace(answer, `no reconciliation prediction`, `will reconcile to desired`, 1)) {
		t.Fatal("grader must accept the fixture interpretation and reject completeness or prediction overclaims")
	}
}

func checkTraceRenderedDiffScaffold(t *testing.T, root string) {
	t.Helper()
	caseData, err := os.ReadFile(filepath.Join(root, "case.yaml"))
	if err != nil || !strings.Contains(string(caseData), "FIXTURE-OWNED-SCAFFOLD") || !strings.Contains(string(caseData), "scaffold_script: scaffold.sh") {
		t.Fatalf("case must declare its fixture-owned scaffold: %v", err)
	}
	wantHashes := map[string]string{
		"desired.yaml":    "99f510d46a2df709dc33cc17a6498ba25fd69fc33389926aeaf0657536b636e6",
		"comparison.json": "f4b03252184ca163b229d902fbceae0100a54b22742f8dd3967114d158e9bdc1",
	}
	for name, want := range wantHashes {
		data, err := os.ReadFile(filepath.Join(root, "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != want {
			t.Errorf("fixture %s SHA-256 = %s, want %s", name, got, want)
		}
	}

	scaffoldPath, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		workspace := t.TempDir()
		cmd := exec.Command("bash", scaffoldPath)
		cmd.Dir = workspace
		cmd.Env = offlineKubeconfigEnvironment()
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("rendered-diff scaffold: %v: %s", err, output)
		}
		entries, err := os.ReadDir(filepath.Join(workspace, "evidence"))
		if err != nil || len(entries) != len(wantHashes) {
			t.Fatalf("staged evidence inventory: entries=%d err=%v", len(entries), err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				t.Fatalf("unexpected directory in staged evidence: %s", entry.Name())
			}
			if _, ok := wantHashes[entry.Name()]; !ok {
				t.Fatalf("unexpected staged evidence file %q", entry.Name())
			}
			want, err := os.ReadFile(filepath.Join(root, "fixtures", entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(workspace, "evidence", entry.Name()))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("staged %s differs from fixture: %v", entry.Name(), err)
			}
		}
	}
}
