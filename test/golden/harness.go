// Package golden provides a CLI-level golden test harness for cub-scout.
//
// Golden tests verify that CLI output (stdout, stderr, exit code) matches
// expected files. This harness:
//
//   - Executes the compiled cub-scout binary
//   - Captures stdout, stderr, and exit code
//   - Normalizes output (removes ANSI codes, timestamps, temp paths)
//   - Compares against golden files in testdata/
//   - Supports -update flag to regenerate golden files
//
// Reference: docs/reference/cli-contract.md
package golden

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// updateGolden is a flag to update golden files instead of comparing.
// Also supports UPDATE_GOLDEN=1 environment variable for CI-friendly usage.
var updateGolden = flag.Bool("update", false, "update golden files")

func init() {
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		// Defer setting until after flag.Parse — but since we use a pointer,
		// we can set the underlying value directly.
		v := true
		updateGolden = &v
	}
}

// Result captures the output of a CLI command execution.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// RunCubScout executes the cub-scout binary with the given arguments.
// It returns the captured stdout, stderr, and exit code.
// The binary path can be overridden via CUB_SCOUT_BINARY env var.
func RunCubScout(t *testing.T, args ...string) Result {
	t.Helper()

	binary := os.Getenv("CUB_SCOUT_BINARY")
	if binary == "" {
		// Default to the binary at the project root
		binary = filepath.Join(projectRoot(t), "cub-scout")
	}

	// Ensure binary exists
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Fatalf("cub-scout binary not found at %s - run 'go build ./cmd/cub-scout' first", binary)
	}

	cmd := exec.Command(binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set environment to disable colors and interactive features
	cmd.Env = append(os.Environ(),
		"NO_COLOR=1",
		"TERM=dumb",
		"CI=true",
	)

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// RunCubScoutWithEnv executes the cub-scout binary with extra environment variables.
func RunCubScoutWithEnv(t *testing.T, env map[string]string, args ...string) Result {
	t.Helper()

	binary := os.Getenv("CUB_SCOUT_BINARY")
	if binary == "" {
		binary = filepath.Join(projectRoot(t), "cub-scout")
	}

	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Fatalf("cub-scout binary not found at %s - run 'go build ./cmd/cub-scout' first", binary)
	}

	cmd := exec.Command(binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Env = append(os.Environ(),
		"NO_COLOR=1",
		"TERM=dumb",
		"CI=true",
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// ProjectRoot returns the project root directory (exported for use by golden test packages).
func ProjectRoot(t *testing.T) string {
	t.Helper()
	return projectRoot(t)
}

// Normalize removes non-deterministic content from CLI output:
//   - ANSI escape codes (colors)
//   - Timestamps (ISO 8601, RFC 3339, etc.)
//   - Temporary paths (/tmp/*, /var/folders/*)
//   - UUIDs
//   - Git commit SHAs beyond first 7 chars
//   - Elapsed time values
func Normalize(s string) string {
	// Strip ANSI escape codes
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	s = ansiRegex.ReplaceAllString(s, "")

	// Strip carriage returns (Windows compatibility)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "")

	// Normalize timestamps: 2025-01-15T10:30:00Z or 2025-01-15T10:30:00.123456Z -> <TIMESTAMP>
	timestampRegex := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	s = timestampRegex.ReplaceAllString(s, "<TIMESTAMP>")

	// Normalize dates: 2025-01-15 10:30:00 -> <DATETIME>
	datetimeRegex := regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}(:\d{2})?`)
	s = datetimeRegex.ReplaceAllString(s, "<DATETIME>")

	// Normalize elapsed times: 5m 30s, 2h 15m, 3d 4h -> <ELAPSED>
	elapsedRegex := regexp.MustCompile(`\d+[dhms]\s*\d*[dhms]?`)
	s = elapsedRegex.ReplaceAllString(s, "<ELAPSED>")

	// Normalize temp paths
	tempPathRegex := regexp.MustCompile(`(/tmp|/var/folders)/[^\s"']+`)
	s = tempPathRegex.ReplaceAllString(s, "<TEMP_PATH>")

	// Normalize UUIDs
	uuidRegex := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	s = uuidRegex.ReplaceAllString(s, "<UUID>")

	// Normalize long git SHAs (keep first 7 chars)
	shaRegex := regexp.MustCompile(`\b[0-9a-f]{40}\b`)
	s = shaRegex.ReplaceAllStringFunc(s, func(sha string) string {
		return sha[:7] + "..."
	})

	// Normalize home directory paths
	if home, err := os.UserHomeDir(); err == nil {
		s = strings.ReplaceAll(s, home, "$HOME")
	}

	return s
}

// AssertGolden compares the actual output against a golden file.
// If -update flag is set, it updates the golden file instead.
// The golden file path is relative to the test's testdata directory.
func AssertGolden(t *testing.T, goldenPath string, actual string) {
	t.Helper()

	// Build full path: test/golden/<subdir>/testdata/<filename>
	testDir := filepath.Dir(goldenPath)
	testFile := filepath.Base(goldenPath)
	fullPath := filepath.Join(projectRoot(t), "test", "golden", testDir, "testdata", testFile)

	if *updateGolden {
		// Ensure directory exists
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create golden directory: %v", err)
		}

		// Write the golden file
		if err := os.WriteFile(fullPath, []byte(actual), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("updated golden file: %s", fullPath)
		return
	}

	// Read expected golden content
	expected, err := os.ReadFile(fullPath)
	if os.IsNotExist(err) {
		t.Fatalf("golden file not found: %s\n\nRun with -update flag to create it:\n  go test ./test/golden/... -update", fullPath)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	// Compare
	if actual != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s\n--- DIFF ---\n%s",
			fullPath,
			string(expected),
			actual,
			diff(string(expected), actual))
	}
}

// AssertExitCode verifies the command exited with the expected code.
func AssertExitCode(t *testing.T, expected int, result Result) {
	t.Helper()
	if result.ExitCode != expected {
		t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s",
			result.ExitCode, expected, result.Stdout, result.Stderr)
	}
}

// projectRoot finds the project root by looking for go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()

	// Start from current working directory
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Walk up until we find go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

// diff produces a simple line-by-line diff between two strings.
func diff(expected, actual string) string {
	expectedLines := strings.Split(expected, "\n")
	actualLines := strings.Split(actual, "\n")

	var buf strings.Builder
	maxLines := len(expectedLines)
	if len(actualLines) > maxLines {
		maxLines = len(actualLines)
	}

	for i := 0; i < maxLines; i++ {
		var exp, act string
		if i < len(expectedLines) {
			exp = expectedLines[i]
		}
		if i < len(actualLines) {
			act = actualLines[i]
		}

		if exp != act {
			buf.WriteString("  exp: ")
			buf.WriteString(exp)
			buf.WriteString("\n")
			buf.WriteString("  got: ")
			buf.WriteString(act)
			buf.WriteString("\n")
		}
	}

	return buf.String()
}

// LoadFixture loads a YAML fixture file from the testdata directory.
func LoadFixture(t *testing.T, subdir, name string) string {
	t.Helper()
	path := filepath.Join(projectRoot(t), "test", "golden", subdir, "testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", path, err)
	}
	return string(data)
}
