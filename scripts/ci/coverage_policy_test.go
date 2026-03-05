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

