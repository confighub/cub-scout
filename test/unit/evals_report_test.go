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

func TestEvalReportBinaryVerifiedIgnoresToolUsedAcrossAblations(t *testing.T) {
	for _, fixture := range []string{"verified-none", "verified-paired"} {
		out, err := runEvalReport(t, reportFixture(fixture))
		if err != nil {
			t.Fatalf("report failed for %s: %v\n%s", fixture, err, out)
		}
		if !strings.Contains(out, "| same-answer | with | 1 | 1/1 | 0 | 0 | $1.00 | $1.00 |") {
			t.Errorf("answer verification changed under %s ablation:\n%s", fixture, out)
		}
	}
}

func TestEvalReportBinaryVerifiedRequiresUniqueBooleanAnswerGrades(t *testing.T) {
	out, err := runEvalReport(t, reportFixture("verified-unknown"))
	if err != nil {
		t.Fatalf("report failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "| unknown-correctness | with | 4 | 0/1 | 3 | 0 | $1.00 | UNKNOWN |") {
		t.Errorf("missing, duplicate, and nonboolean grades must be unknown; errored passing run must fail:\n%s", out)
	}
	if !strings.Contains(out, "| error-without-grader-metadata | with | 1 | 0/1 | 0 | 0 | $0.50 | n/a |") {
		t.Errorf("a run error must fail even when grader metadata is absent:\n%s", out)
	}
	for _, name := range []string{"nan-weight", "infinite-weight"} {
		if !strings.Contains(out, "| "+name+" | with | 1 | 0/0 | 1 | 0 | $0.50 | UNKNOWN |") {
			t.Errorf("nonfinite grader weight must make %s unknown:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "| unknown-correctness | with | 4 | 0.75 | $1.00 | $1.33 |") {
		t.Errorf("legacy harness score and cost diagnostic was not retained:\n%s", out)
	}
	if !strings.Contains(out, "| Case | Arm | Runs | Score | $/run | $/score | Turns | Seconds |") || strings.Contains(out, "$/correct") {
		t.Errorf("legacy harness metric must be labeled as score, not correctness:\n%s", out)
	}
}

func TestEvalReportVerifiedCostsRejectMalformedSpend(t *testing.T) {
	out, err := runEvalReport(t, reportFixture("verified-costs"))
	if err != nil {
		t.Fatalf("report should retain diagnostics for malformed spend: %v\n%s", err, out)
	}
	if !strings.Contains(out, "| invalid-spend | with | 7 | 7/7 | 0 | 7 | UNKNOWN | UNKNOWN |") {
		t.Errorf("unknown spend must suppress both binary cost metrics:\n%s", out)
	}
	if !strings.Contains(out, "| invalid-spend | with | 7 | 1.00 | UNKNOWN | n/a |") {
		t.Errorf("legacy diagnostics should render malformed aggregate spend without crashing or inventing a value:\n%s", out)
	}
	if !strings.Contains(out, "| optional-spend-absent | with | 1 | 1/1 | 0 | 0 | $1.00 | $1.00 |") {
		t.Errorf("absent optional judge/mock costs should retain historical zero defaults:\n%s", out)
	}
}
