package patterns_list_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPatternsList(t *testing.T) {
	binary := findBinary(t)

	cmd := exec.Command(binary, "patterns", "list")
	cmd.Env = append(os.Environ(),
		"CUB_SCOUT_TEST_TIME=2026-01-01T00:00:00Z",
		"CUB_SCOUT_TEST_CLUSTER=test-cluster",
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Logf("stderr: %s", string(exitErr.Stderr))
		}
		t.Fatalf("command failed: %v", err)
	}

	// Load golden
	goldenPath := filepath.Join("testdata", "list.golden.txt")
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
