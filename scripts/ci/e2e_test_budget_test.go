package ci

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// #800: the cmd/cub-scout e2e package ran at 96% of its timeout, so a PR that
// added tests failed with a panic inside an unrelated test. The budget check
// fails earlier and says why.
func TestE2ETestBudget(t *testing.T) {
	const pkg = "github.com/confighub/cub-scout/v2/cmd/cub-scout"
	for _, tc := range []struct {
		name    string
		log     string
		wantErr bool
		want    string
	}{
		{
			name: "within budget",
			log:  "--- PASS: TestQuery (19.29s)\nPASS\nok  \t" + pkg + "\t174.464s\n",
			want: "E2E time budget passed: " + pkg + " took 174.464s of a 300s budget (hard cap 600s).",
		},
		{
			name:    "over budget, under the cap",
			log:     "PASS\nok  \t" + pkg + "\t301.2s\n",
			wantErr: true,
			want:    "took 301.2s, over the 300s budget (hard cap 600s)",
		},
		{
			// The run recorded on #798.
			name:    "hard cap hit",
			log:     "panic: test timed out after 10m0s\n\trunning tests:\n\t\tTestStatefulSetReleaseImageReadBudgetsAndFailures (3s)\nFAIL\t" + pkg + "\t600.027s\n",
			wantErr: true,
			want:    "hit the 600s hard cap (budget 300s)",
		},
		{
			name:    "a failing run over budget is reported as over budget",
			log:     "--- FAIL: TestQuery (19.29s)\nFAIL\nFAIL\t" + pkg + "\t320.000s\n",
			wantErr: true,
			want:    "took 320.000s, over the 300s budget",
		},
		{
			name:    "cached result has no elapsed time",
			log:     "ok  \t" + pkg + "\t(cached)\n",
			wantErr: true,
			want:    "elapsed time is unknown, not within budget",
		},
		{
			name:    "another package's time is not this package's",
			log:     "ok  \t" + pkg + "/other\t1.000s\n",
			wantErr: true,
			want:    "elapsed time is unknown, not within budget",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "go-test.log")
			if err := os.WriteFile(log, []byte(tc.log), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", "./e2e-test-budget.sh", "--check-log", log)
			cmd.Env = append(os.Environ(), "E2E_TEST_TIMEOUT_SECONDS=600", "E2E_TEST_BUDGET_SECONDS=300")
			output, err := cmd.CombinedOutput()
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v\n%s", err, tc.wantErr, output)
			}
			if !strings.Contains(string(output), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, output)
			}
		})
	}
}

// The workflow must run the package through the budget script, not a bare
// go test with its own timeout.
func TestCIRunsE2EThroughTheBudgetScript(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/ci.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(workflow), "run: scripts/ci/e2e-test-budget.sh") {
		t.Error("ci.yaml does not run scripts/ci/e2e-test-budget.sh")
	}
	if strings.Contains(string(workflow), "go test -tags=e2e ./cmd/cub-scout/...") {
		t.Error("ci.yaml still runs the e2e package directly, outside the time budget")
	}
}
