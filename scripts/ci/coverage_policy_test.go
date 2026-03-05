package ci

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

func TestCoveragePolicy_TestingReadmeDocumentsCoverageGate(t *testing.T) {
	workflowPath := filepath.Join("..", "..", ".github", "workflows", "ci.yaml")
	workflow, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	m := regexp.MustCompile(`COVERAGE_MIN_PERCENT:\s*"([^"]+)"`).FindStringSubmatch(string(workflow))
	if len(m) < 2 {
		t.Fatalf("COVERAGE_MIN_PERCENT not found in workflow")
	}
	requiredThreshold := m[1]

	readmePath := filepath.Join("..", "..", "docs", "testing", "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read docs/testing/README.md: %v", err)
	}
	content := string(readme)

	if !strings.Contains(content, "COVERAGE_MIN_PERCENT") {
		t.Fatalf("docs/testing/README.md must document COVERAGE_MIN_PERCENT")
	}
	if !strings.Contains(content, requiredThreshold) {
		t.Fatalf("docs/testing/README.md missing current threshold value %s", requiredThreshold)
	}
}

func TestCoveragePolicy_TestingReadmeDocumentsProofArtifactCoverageFields(t *testing.T) {
	readmePath := filepath.Join("..", "..", "docs", "testing", "README.md")
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read docs/testing/README.md: %v", err)
	}
	content := string(readme)

	requiredMarkers := []string{
		"proof-matrix.json",
		"proof-summary.md",
		"coverage_total",
		"coverage_min",
	}
	for _, marker := range requiredMarkers {
		if !strings.Contains(content, marker) {
			t.Fatalf("docs/testing/README.md missing proof artifact marker %q", marker)
		}
	}
}
