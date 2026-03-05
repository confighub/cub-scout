package ci

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmitProofArtifact_WritesExpectedFiles(t *testing.T) {
	outDir := t.TempDir()
	cmd := exec.Command("bash", "./emit-proof-artifact.sh", outDir)
	cmd.Env = append(os.Environ(),
		"PROOF_TIMESTAMP=2026-01-01T00:00:00Z",
		"GITHUB_RUN_ID=12345",
		"GITHUB_WORKFLOW=CI",
		"GITHUB_EVENT_NAME=pull_request",
		"GITHUB_REF_NAME=main",
		"GITHUB_SHA=deadbeef",
		"PROOF_UNIT=success",
		"PROOF_INTEGRATION=success",
		"PROOF_GITOPS=success",
		"PROOF_DEMOS=skipped",
		"PROOF_CONNECTED=skipped",
		"PROOF_FULL=skipped",
		"PROOF_COVERAGE_TOTAL=27.7",
		"PROOF_COVERAGE_MIN=25.0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-proof-artifact failed: %v\n%s", err, string(output))
	}

	jsonPath := filepath.Join(outDir, "proof-matrix.json")
	mdPath := filepath.Join(outDir, "proof-summary.md")

	jsonBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read json artifact: %v", err)
	}
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read markdown artifact: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		t.Fatalf("parse json artifact: %v", err)
	}
	if got := doc["run_id"]; got != "12345" {
		t.Fatalf("run_id mismatch: got=%v want=12345", got)
	}
	if got := doc["workflow"]; got != "CI" {
		t.Fatalf("workflow mismatch: got=%v want=CI", got)
	}
	if got := doc["event"]; got != "pull_request" {
		t.Fatalf("event mismatch: got=%v want=pull_request", got)
	}
	if got := doc["generated_at"]; got != "2026-01-01T00:00:00Z" {
		t.Fatalf("generated_at mismatch: got=%v want=2026-01-01T00:00:00Z", got)
	}
	if got := doc["coverage_total"]; got != "27.7" {
		t.Fatalf("coverage_total mismatch: got=%v want=27.7", got)
	}
	if got := doc["coverage_min"]; got != "25.0" {
		t.Fatalf("coverage_min mismatch: got=%v want=25.0", got)
	}

	md := string(mdBytes)
	if !strings.Contains(md, "| unit | success |") {
		t.Fatalf("markdown missing unit row: %s", md)
	}
	if !strings.Contains(md, "| gitops | success |") {
		t.Fatalf("markdown missing gitops row: %s", md)
	}
	if !strings.Contains(md, "Coverage total: 27.7% (required >= 25.0%)") {
		t.Fatalf("markdown missing coverage summary: %s", md)
	}
}

func TestEmitProofArtifact_DefaultsToSkipped(t *testing.T) {
	outDir := t.TempDir()
	cmd := exec.Command("bash", "./emit-proof-artifact.sh", outDir)
	cmd.Env = append(os.Environ(),
		"PROOF_TIMESTAMP=2026-01-01T00:00:00Z",
		"GITHUB_RUN_ID=999",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("emit-proof-artifact failed: %v\n%s", err, string(output))
	}

	mdPath := filepath.Join(outDir, "proof-summary.md")
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read markdown artifact: %v", err)
	}
	md := string(mdBytes)
	if !strings.Contains(md, "| unit | skipped |") {
		t.Fatalf("expected default skipped unit row: %s", md)
	}
	if !strings.Contains(md, "| integration | skipped |") {
		t.Fatalf("expected default skipped integration row: %s", md)
	}
}
