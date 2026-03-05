package ci

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestCoveragePolicy_CIThresholdIs25Percent(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "ci.yaml")
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	content := string(b)
	re := regexp.MustCompile(`COVERAGE_MIN_PERCENT:\s*"([^"]+)"`)
	m := re.FindStringSubmatch(content)
	if len(m) < 2 {
		t.Fatalf("COVERAGE_MIN_PERCENT not found in workflow")
	}
	if m[1] != "25.0" {
		t.Fatalf("expected COVERAGE_MIN_PERCENT=25.0, got %s", m[1])
	}
}

func TestCoveragePolicy_ProofArtifactCoverageWiring(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "ci.yaml")
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	content := string(b)

	if !regexp.MustCompile(`(?m)^\s*outputs:\s*$`).MatchString(content) {
		t.Fatalf("unit job outputs block missing")
	}
	if !regexp.MustCompile(`coverage_total:\s*\$\{\{\s*steps\.coverage_metric\.outputs\.coverage_total\s*\}\}`).MatchString(content) {
		t.Fatalf("unit coverage_total output mapping missing")
	}
	if !regexp.MustCompile(`coverage_min:\s*\$\{\{\s*steps\.coverage_metric\.outputs\.coverage_min\s*\}\}`).MatchString(content) {
		t.Fatalf("unit coverage_min output mapping missing")
	}
	if !regexp.MustCompile(`PROOF_COVERAGE_TOTAL:\s*\$\{\{\s*needs\.unit\.outputs\.coverage_total\s*\}\}`).MatchString(content) {
		t.Fatalf("proof-artifact coverage total env wiring missing")
	}
	if !regexp.MustCompile(`PROOF_COVERAGE_MIN:\s*\$\{\{\s*needs\.unit\.outputs\.coverage_min\s*\}\}`).MatchString(content) {
		t.Fatalf("proof-artifact coverage min env wiring missing")
	}
}
