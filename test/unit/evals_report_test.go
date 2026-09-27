// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// evals/scripts/report.py turns `claude plugin eval --json` results into
// correctness, cost and speed per arm (#603). The fixture's arithmetic:
// with = 2 runs, score 1.5, cost 0.62+0.30 = 0.92 → $0.46/run, $0.61/correct;
// without = 1 errored run scoring 0 → cost per correct is n/a.
func TestEvalReportCostPerCorrectAnswer(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := filepath.Join("..", "..")
	cmd := exec.Command("python3", "evals/scripts/report.py", "test/fixtures/evals-report/result.json", "--by-tag")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("report.py: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		"| sample | with | 2 | 0.75 | $0.46 | $0.61 | 15.0 | 90 |",
		"|  | without | 1 | 0.00 | $0.40 | n/a | 30.0 | 300 |",
		"| tag: demo | with | 2 | 0.75 | $0.46 | $0.61 | 15.0 | 90 |",
		"- sample (without): timed out after 300s",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report output missing %q\n%s", want, got)
		}
	}
}
