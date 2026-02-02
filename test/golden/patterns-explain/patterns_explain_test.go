package patterns_explain_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPatternsExplain_OwnershipChain(t *testing.T) {
	binary := findBinary(t)

	cmd := exec.Command(binary, "patterns", "explain", "k8s.ownership_chain_complete", "--empty")
	cmd.Env = append(os.Environ(),
		"CUB_SCOUT_TEST_TIME=2026-01-01T00:00:00Z",
		"CUB_SCOUT_TEST_CLUSTER=test-cluster",
	)

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed: %v", err)
		}
	}

	// Load golden
	goldenPath := filepath.Join("testdata", "explain-ownership-chain.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	// Compare
	got := strings.TrimSpace(string(output))
	want := strings.TrimSpace(string(golden))

	if got != want {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", got, want)
	}

	// Skip status should be exit code 0
	if exitCode != 0 {
		t.Errorf("expected exit code 0 (skip), got %d", exitCode)
	}
}

func TestPatternsExplain_GitOpsPresence(t *testing.T) {
	binary := findBinary(t)

	cmd := exec.Command(binary, "patterns", "explain", "gitops.controller_presence", "--empty")
	cmd.Env = append(os.Environ(),
		"CUB_SCOUT_TEST_TIME=2026-01-01T00:00:00Z",
		"CUB_SCOUT_TEST_CLUSTER=test-cluster",
	)

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed: %v", err)
		}
	}

	// Load golden
	goldenPath := filepath.Join("testdata", "explain-gitops-presence.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	// Compare
	got := strings.TrimSpace(string(output))
	want := strings.TrimSpace(string(golden))

	if got != want {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", got, want)
	}

	// Fail status should be exit code 4
	if exitCode != 4 {
		t.Errorf("expected exit code 4 (fail), got %d", exitCode)
	}
}

func TestPatternsExplain_UnknownPattern(t *testing.T) {
	binary := findBinary(t)

	cmd := exec.Command(binary, "patterns", "explain", "nonexistent.pattern")
	cmd.Env = append(os.Environ(),
		"CUB_SCOUT_TEST_TIME=2026-01-01T00:00:00Z",
		"CUB_SCOUT_TEST_CLUSTER=test-cluster",
	)

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed: %v", err)
		}
	}

	// Load golden
	goldenPath := filepath.Join("testdata", "explain-unknown.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	// Compare
	got := strings.TrimSpace(string(output))
	want := strings.TrimSpace(string(golden))

	if got != want {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", got, want)
	}

	// Unknown pattern should be exit code 3
	if exitCode != 3 {
		t.Errorf("expected exit code 3 (unknown), got %d", exitCode)
	}
}

func TestPatternsExplain_MissingArgument(t *testing.T) {
	binary := findBinary(t)

	cmd := exec.Command(binary, "patterns", "explain")
	cmd.Env = append(os.Environ(),
		"CUB_SCOUT_TEST_TIME=2026-01-01T00:00:00Z",
		"CUB_SCOUT_TEST_CLUSTER=test-cluster",
	)

	output, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed: %v", err)
		}
	}

	// Load golden
	goldenPath := filepath.Join("testdata", "explain-missing-arg.golden.txt")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	// Compare
	got := strings.TrimSpace(string(output))
	want := strings.TrimSpace(string(golden))

	if got != want {
		t.Errorf("output mismatch\ngot:\n%s\n\nwant:\n%s", got, want)
	}

	// Missing argument should be exit code 2
	if exitCode != 2 {
		t.Errorf("expected exit code 2 (usage), got %d", exitCode)
	}
}

func findBinary(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}

	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
	binary := filepath.Join(repoRoot, "cub-scout")

	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Fatalf("cub-scout binary not found at %s - run 'go build ./cmd/cub-scout' first", binary)
	}

	return binary
}
