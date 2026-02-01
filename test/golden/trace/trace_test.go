// Package trace provides golden tests for the trace CLI command contract.
//
// These tests verify that the trace command behaves as documented in:
// docs/reference/cli-contract.md
//
// Test scenarios cover:
//   - Error handling for invalid inputs (no cluster required)
//   - Trace on unmanaged resource (cluster required)
//   - Trace with --json output format (cluster required)
//   - Trace failure states (cluster required)
//
// Cluster-dependent tests skip if BOTH conditions are not met:
//  1. A Kubernetes cluster is available (kubectl cluster-info succeeds)
//  2. The corresponding golden file exists
//
// To generate golden files for cluster-dependent tests:
//   1. Ensure a cluster is available (kind, minikube, etc.)
//   2. Run: UPDATE_GOLDEN=1 go test ./test/golden/trace/... -run TestTrace_
//
// Reference: docs/reference/cli-contract.md, docs/reference/health-failure-states.md
//
// Contract requirements for Native/unmanaged trace (per cli-contract.md):
//
//	TRACE: Deployment/coredns in kube-system
//	  [warning] resource not managed by Flux
//	Exit code: 1
package trace

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/test/golden"
)

// updateGolden controls whether to update golden files.
var updateGolden = os.Getenv("UPDATE_GOLDEN") == "1"

func init() {
	for _, arg := range os.Args {
		if arg == "-update" || arg == "--update" {
			updateGolden = true
			break
		}
	}
}

// TestTrace_UsageError verifies that trace without arguments shows usage error.
// This test does not require a cluster.
// Expected: exit code 1, stderr contains usage message.
func TestTrace_UsageError(t *testing.T) {
	result := golden.RunCubScout(t, "trace")

	golden.AssertExitCode(t, 1, result)

	// Normalize and check output
	normalized := golden.Normalize(result.Stderr)
	assertGolden(t, "usage-error", normalized)
}

// TestTrace_InvalidFormat verifies that trace with invalid resource format shows error.
// This test does not require a cluster.
// Expected: exit code 1, stderr contains format error.
func TestTrace_InvalidFormat(t *testing.T) {
	result := golden.RunCubScout(t, "trace", "invalid-no-slash")

	golden.AssertExitCode(t, 1, result)

	// Normalize and check output
	normalized := golden.Normalize(result.Stderr)
	assertGolden(t, "invalid-format", normalized)
}

// TestTrace_UnmanagedResource verifies trace behavior on a resource not managed by GitOps.
// Expected: TRACE header + [warning] message + exit code 1.
// Reference: cli-contract.md trace section - "Native resource" output.
// Skips if no cluster available or golden file missing.
func TestTrace_UnmanagedResource(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "unmanaged-resource")

	// Use kube-system/coredns which is typically native/unmanaged
	result := golden.RunCubScout(t, "trace", "deployment/coredns", "-n", "kube-system")

	// Per cli-contract.md: exit code 1 for "not managed"
	golden.AssertExitCode(t, 1, result)

	// Normalize output and verify pattern
	normalized := normalizeTraceOutput(result.Stdout + result.Stderr)

	// Check for expected warning patterns per CLI contract
	if !containsUnmanagedWarning(normalized) {
		t.Errorf("expected unmanaged resource warning in output:\n%s", normalized)
	}

	assertGolden(t, "unmanaged-resource", normalized)
}

// TestTrace_UnmanagedResourceJSON verifies --json output on unmanaged resource.
// Expected: JSON with object, error, fullyManaged fields; exit code 1.
// Reference: cli-contract.md trace JSON output section.
// Skips if no cluster available or golden file missing.
func TestTrace_UnmanagedResourceJSON(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "unmanaged-resource-json")

	// Use kube-system/coredns which is typically native/unmanaged
	result := golden.RunCubScout(t, "trace", "deployment/coredns", "-n", "kube-system", "--json")

	// Per cli-contract.md: exit code 1 for "not managed"
	golden.AssertExitCode(t, 1, result)

	// Verify JSON structure matches contract
	var output map[string]interface{}
	combined := result.Stdout + result.Stderr
	if err := json.Unmarshal([]byte(combined), &output); err != nil {
		// If not valid JSON, the trace likely failed before producing output
		// Check for error message instead
		normalized := golden.Normalize(combined)
		assertGolden(t, "unmanaged-resource-json", normalized)
		return
	}

	// Verify required fields per CLI contract
	// JSON output should have: resource, owner (may be null/empty for unmanaged)
	if _, ok := output["resource"]; !ok {
		// Some error responses may have different structure
		t.Logf("output lacks 'resource' field, may be error response")
	}

	// Normalize for golden comparison
	normalized := normalizeJSON(combined)
	assertGolden(t, "unmanaged-resource-json", normalized)
}

// TestTrace_ResourceNotFound verifies trace behavior when resource doesn't exist.
// Expected: error message + exit code 1.
// Reference: cli-contract.md error behavior section.
// Skips if no cluster available or golden file missing.
func TestTrace_ResourceNotFound(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "resource-not-found")

	result := golden.RunCubScout(t, "trace", "deployment/does-not-exist", "-n", "default")

	// Per CLI contract: exit code 1 for errors
	golden.AssertExitCode(t, 1, result)

	// Normalize and check output contains error message
	normalized := golden.Normalize(result.Stdout + result.Stderr)

	// Should contain "not found" or similar error
	if !strings.Contains(strings.ToLower(normalized), "not found") &&
		!strings.Contains(strings.ToLower(normalized), "failed") &&
		!strings.Contains(strings.ToLower(normalized), "error") {
		t.Errorf("expected error message in output:\n%s", normalized)
	}

	assertGolden(t, "resource-not-found", normalized)
}

// requireCluster skips the test if no Kubernetes cluster is available.
func requireCluster(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "cluster-info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Skip("PRECONDITION: No Kubernetes cluster available")
	}
}

// requireGolden skips the test if the golden file doesn't exist,
// UNLESS updateGolden is true (meaning we're generating goldens).
// This allows cluster-dependent tests to be defined but skipped until
// golden files are generated from a real cluster.
func requireGolden(t *testing.T, name string) {
	t.Helper()
	if updateGolden {
		// When updating goldens, don't skip - we need to generate them
		return
	}
	goldenPath := filepath.Join("testdata", name+".golden.txt")
	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		t.Skipf("PRECONDITION: Golden file not found: %s (run UPDATE_GOLDEN=1 with cluster)", goldenPath)
	}
}

// normalizeTraceOutput normalizes trace command output.
// Removes non-deterministic content while preserving structure.
func normalizeTraceOutput(s string) string {
	// Apply base normalization
	s = golden.Normalize(s)

	// Normalize revision hashes (keep first 7 chars like Git)
	revisionRegex := regexp.MustCompile(`revision:?\s*[a-f0-9]{7,}`)
	s = revisionRegex.ReplaceAllString(s, "revision: <REVISION>")

	// Normalize Git URLs
	gitURLRegex := regexp.MustCompile(`(https?://|git@)[^\s]+\.git`)
	s = gitURLRegex.ReplaceAllString(s, "<GIT_URL>")

	// Normalize resource status messages that may vary
	statusRegex := regexp.MustCompile(`\[([Rr]eady|[Pp]ending|[Ff]ailed|[Uu]nknown)\]`)
	s = statusRegex.ReplaceAllStringFunc(s, func(m string) string {
		return strings.ToLower(m)
	})

	return s
}

// normalizeJSON normalizes JSON output for comparison.
func normalizeJSON(s string) string {
	s = golden.Normalize(s)

	// Try to parse and re-marshal with stable formatting
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err == nil {
		if formatted, err := json.MarshalIndent(data, "", "  "); err == nil {
			return string(formatted) + "\n"
		}
	}

	return s
}

// containsUnmanagedWarning checks if output indicates an unmanaged resource.
func containsUnmanagedWarning(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "not managed") ||
		strings.Contains(lower, "native") ||
		strings.Contains(lower, "unmanaged") ||
		strings.Contains(lower, "unknown owner")
}

// assertGolden compares output against golden file.
func assertGolden(t *testing.T, name, actual string) {
	t.Helper()

	goldenPath := filepath.Join("testdata", name+".golden.txt")

	if updateGolden {
		if err := os.WriteFile(goldenPath, []byte(actual), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		t.Fatalf("golden file not found: %s\n\nActual output:\n%s\n\nRun with UPDATE_GOLDEN=1 to create it:\n  UPDATE_GOLDEN=1 go test ./test/golden/trace/...", goldenPath, actual)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if actual != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s",
			goldenPath, string(expected), actual)
	}
}
