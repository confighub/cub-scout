package ci

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCheckCoverage_PassesWhenAtOrAboveThreshold(t *testing.T) {
	cmd := exec.Command("bash", "./check-coverage.sh", "unused.cover")
	cmd.Env = append(os.Environ(),
		"COVERAGE_TOTAL_OVERRIDE=25.5",
		"COVERAGE_MIN_PERCENT=20.0",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check-coverage expected success, got error: %v\n%s", err, string(output))
	}
	if !strings.Contains(string(output), "Coverage gate passed") {
		t.Fatalf("expected pass message, got:\n%s", string(output))
	}
}

func TestCheckCoverage_FailsWhenBelowThreshold(t *testing.T) {
	cmd := exec.Command("bash", "./check-coverage.sh", "unused.cover")
	cmd.Env = append(os.Environ(),
		"COVERAGE_TOTAL_OVERRIDE=12.0",
		"COVERAGE_MIN_PERCENT=20.0",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("check-coverage expected failure, got success:\n%s", string(output))
	}
	if !strings.Contains(string(output), "Coverage gate failed") {
		t.Fatalf("expected failure message, got:\n%s", string(output))
	}
}

func TestCheckCoverage_FailsOnMissingInputWithoutOverride(t *testing.T) {
	cmd := exec.Command("bash", "./check-coverage.sh", "missing.cover")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing-input failure, got success:\n%s", string(output))
	}
	if !strings.Contains(string(output), "coverage profile not found") {
		t.Fatalf("expected missing-input message, got:\n%s", string(output))
	}
}

