// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runEvalReport(t *testing.T, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required to exercise evals/scripts/report.py")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(root, "evals", "scripts", "report.py")
	cmdArgs := append([]string{report}, args...)
	cmd := exec.Command("python3", cmdArgs...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func reportFixture(name string) string {
	return filepath.Join("test", "fixtures", "evals-report", name+".json")
}

func TestEvalReportShowsCompletenessAndPreservesPartialCosts(t *testing.T) {
	out, err := runEvalReport(t, reportFixture("partial"))
	if err != nil {
		t.Fatalf("report failed: %v\n%s", err, out)
	}
	for _, want := range []string{"Completeness: INCOMPLETE", "partial=true", "reason=interrupted", "live-case (with): 2 planned, 2 observed", "$1.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q:\n%s", want, out)
		}
	}
}

func TestEvalReportDetectsMissingPlannedArmRuns(t *testing.T) {
	out, err := runEvalReport(t, reportFixture("missing-arm"))
	if err != nil {
		t.Fatalf("report failed: %v\n%s", err, out)
	}
	for _, want := range []string{"Completeness: INCOMPLETE", "paired-case (without): expected 2, observed 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q:\n%s", want, out)
		}
	}
}

func TestEvalReportRequireComplete(t *testing.T) {
	for _, fixture := range []string{"partial", "missing-arm", "unknown", "empty"} {
		out, err := runEvalReport(t, "--require-complete", reportFixture(fixture))
		if err == nil {
			t.Errorf("--require-complete accepted %s fixture:\n%s", fixture, out)
		}
	}
	for _, fixture := range []string{"complete", "none-complete"} {
		out, err := runEvalReport(t, "--require-complete", reportFixture(fixture))
		if err != nil {
			t.Fatalf("--require-complete rejected %s fixture: %v\n%s", fixture, err, out)
		}
		if !strings.Contains(out, "Completeness: COMPLETE") {
			t.Errorf("complete report missing status:\n%s", out)
		}
	}
}

func TestEvalReportUnknownCompletenessIsVisible(t *testing.T) {
	for _, fixture := range []string{"unknown", "empty"} {
		out, err := runEvalReport(t, reportFixture(fixture))
		if err != nil {
			t.Fatalf("report failed: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Completeness: UNKNOWN") {
			t.Errorf("unknown completeness not reported:\n%s", out)
		}
	}
}

// evals/scripts/report.py turns plugin eval results into correctness, cost and
// speed per arm. This fixture pins the existing output arithmetic and tag/error
// reporting while completeness metadata is added.
func TestEvalReportCostPerCorrectAnswer(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	out, err := runEvalReport(t, "test/fixtures/evals-report/result.json", "--by-tag")
	if err != nil {
		t.Fatalf("report.py: %v\n%s", err, out)
	}
	for _, want := range []string{
		"| sample | with | 2 | 0.75 | $0.46 | $0.61 | 15.0 | 90 |",
		"|  | without | 1 | 0.00 | $0.40 | n/a | 30.0 | 300 |",
		"| tag: demo | with | 2 | 0.75 | $0.46 | $0.61 | 15.0 | 90 |",
		"- sample (without): timed out after 300s",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q\n%s", want, out)
		}
	}
}

func TestEvalReportSyntheticCostArithmetic(t *testing.T) {
	out, err := runEvalReport(t, reportFixture("complete"))
	if err != nil {
		t.Fatalf("report failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "| paired-case | with | 2 | 1.00 | $1.00 | $1.00 |") ||
		!strings.Contains(out, "|  | without | 2 | 0.50 | $0.75 | $1.50 |") {
		t.Errorf("existing cost-per-correct arithmetic changed:\n%s", out)
	}
}
