// Package mapstatus provides golden tests for the map status CLI command contract.
//
// These tests verify that the map status command behaves as documented in:
// docs/reference/cli-contract.md
// docs/reference/health-failure-states.md
//
// Test scenarios cover:
//   - Healthy cluster (all deployers and workloads ready)
//   - Problems detected (some deployers/workloads not ready)
//   - Empty cluster baseline (no deployers, no workloads)
//   - Cluster unreachable (error case)
//
// Each test sets up and tears down its own fixtures to ensure independence.
// Tests skip if no Kubernetes cluster is available.
//
// Reference: docs/reference/cli-contract.md, docs/reference/health-failure-states.md
//
// Contract requirements for map status (per cli-contract.md):
//
//	✓ healthy: X/Y deployers, A/B workloads
//	Exit code: 0 (all healthy) or 1 (some unhealthy)
package mapstatus

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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

// TestMapStatus_Healthy verifies map status output when all workloads are healthy.
// Expected: ✓ healthy: X/Y deployers, A/B workloads + exit code 0.
// Reference: cli-contract.md map status section.
// Fixture: Creates a namespace with a healthy nginx deployment.
func TestMapStatus_Healthy(t *testing.T) {
	requireCluster(t)

	// Setup: Create a healthy deployment
	ns := "cub-scout-test-healthy"
	cleanupNamespace(t, ns)
	createNamespace(t, ns)
	defer cleanupNamespace(t, ns)

	// Create a healthy deployment that will become ready
	createDeployment(t, ns, "nginx-healthy", "nginx:alpine")
	waitForDeploymentReady(t, ns, "nginx-healthy", 60*time.Second)

	// Run map status
	result := golden.RunCubScout(t, "map", "status")

	// Per cli-contract.md: exit code 0 for "All healthy"
	golden.AssertExitCode(t, 0, result)

	// Normalize and verify pattern
	normalized := normalizeMapStatusOutput(result.Stdout + result.Stderr)

	// Should contain healthy indicator
	if !strings.Contains(normalized, "healthy") {
		t.Errorf("expected healthy indicator in output:\n%s", normalized)
	}

	// Verify exit code and format match contract
	// The exact counts depend on cluster state, but format must match
	if !healthyOutputPattern.MatchString(normalized) {
		t.Errorf("output does not match expected healthy format:\n%s", normalized)
	}
}

// TestMapStatus_Problems verifies map status output when some workloads have issues.
// Expected: ✗ N problem(s): X/Y deployers, A/B workloads + exit code 1.
// Reference: cli-contract.md map status exit codes.
// Fixture: Creates a namespace with a failing deployment (image pull error).
func TestMapStatus_Problems(t *testing.T) {
	requireCluster(t)

	// Setup: Create a failing deployment
	ns := "cub-scout-test-problems"
	cleanupNamespace(t, ns)
	createNamespace(t, ns)
	defer cleanupNamespace(t, ns)

	// Create a deployment that will fail (non-existent image)
	createDeployment(t, ns, "failing-deploy", "does-not-exist:v999")

	// Wait a moment for the deployment to register as not ready
	time.Sleep(3 * time.Second)

	// Run map status
	result := golden.RunCubScout(t, "map", "status")

	// Per cli-contract.md: exit code 1 for "Some unhealthy or error"
	golden.AssertExitCode(t, 1, result)

	// Normalize and verify pattern
	normalized := normalizeMapStatusOutput(result.Stdout + result.Stderr)

	// Should contain problem indicator
	if !strings.Contains(normalized, "problem") {
		t.Errorf("expected problem indicator in output:\n%s", normalized)
	}

	// Verify exit code and format match contract
	if !problemsOutputPattern.MatchString(normalized) {
		t.Errorf("output does not match expected problems format:\n%s", normalized)
	}
}

// TestMapStatus_EmptyBaseline verifies map status output on a baseline empty state.
// Expected: ✓ healthy: 0/0 deployers, 0/0 workloads + exit code 0.
// Reference: cli-contract.md - 0/0 is considered healthy (nothing unhealthy).
// This test cleans up any test namespaces first and waits for deletion.
func TestMapStatus_EmptyBaseline(t *testing.T) {
	requireCluster(t)

	// Clean up any test namespaces that might exist and wait for full deletion
	cleanupNamespaceAndWait(t, "cub-scout-test-healthy")
	cleanupNamespaceAndWait(t, "cub-scout-test-problems")

	// Run map status on clean cluster
	result := golden.RunCubScout(t, "map", "status")

	// Empty cluster with 0/0 deployers and 0/0 workloads is healthy
	golden.AssertExitCode(t, 0, result)

	// Normalize output
	normalized := normalizeMapStatusOutput(result.Stdout + result.Stderr)

	// Should show healthy with 0/0 counts
	if !strings.Contains(normalized, "healthy") {
		t.Errorf("expected healthy indicator in output:\n%s", normalized)
	}
	if !strings.Contains(normalized, "0/0") {
		t.Errorf("expected 0/0 counts in output:\n%s", normalized)
	}

	// Verify format
	if !healthyOutputPattern.MatchString(normalized) {
		t.Errorf("output does not match expected healthy format:\n%s", normalized)
	}
}

// TestMapStatus_ClusterUnreachable verifies map status behavior when cluster is unreachable.
// Expected: error message + exit code 1.
// Reference: cli-contract.md error behavior section.
// This test intentionally uses an invalid kubeconfig.
func TestMapStatus_ClusterUnreachable(t *testing.T) {
	// Create a temporary invalid kubeconfig
	tmpDir := t.TempDir()
	invalidKubeconfig := filepath.Join(tmpDir, "invalid-kubeconfig")
	if err := os.WriteFile(invalidKubeconfig, []byte("invalid"), 0600); err != nil {
		t.Fatalf("failed to create invalid kubeconfig: %v", err)
	}

	// Run with invalid kubeconfig
	result := runCubScoutWithEnv(t, map[string]string{
		"KUBECONFIG": invalidKubeconfig,
	}, "map", "status")

	// Per CLI contract: exit code 1 for errors
	golden.AssertExitCode(t, 1, result)

	// Output should contain error indication
	combined := result.Stdout + result.Stderr
	if !strings.Contains(strings.ToLower(combined), "error") &&
		!strings.Contains(strings.ToLower(combined), "failed") &&
		!strings.Contains(strings.ToLower(combined), "invalid") {
		t.Errorf("expected error message in output:\n%s", combined)
	}
}

// Output format patterns per CLI contract
var (
	// healthyOutputPattern matches: ✓ healthy: X/Y deployers, A/B workloads
	healthyOutputPattern = regexp.MustCompile(`✓ healthy: \d+/\d+ deployers, \d+/\d+ workloads`)

	// problemsOutputPattern matches: ✗ N problem(s): X/Y deployers, A/B workloads
	problemsOutputPattern = regexp.MustCompile(`✗ \d+ problem\(s\): \d+/\d+ deployers, \d+/\d+ workloads`)
)

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

// createNamespace creates a namespace for testing.
func createNamespace(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command("kubectl", "create", "namespace", name)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create namespace %s: %v\n%s", name, err, output)
	}
}

// cleanupNamespace deletes a namespace if it exists (non-blocking).
func cleanupNamespace(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command("kubectl", "delete", "namespace", name, "--ignore-not-found", "--wait=false")
	cmd.Run() // Ignore errors - namespace may not exist
}

// cleanupNamespaceAndWait deletes a namespace and waits for full deletion.
func cleanupNamespaceAndWait(t *testing.T, name string) {
	t.Helper()
	// Delete the namespace
	cmd := exec.Command("kubectl", "delete", "namespace", name, "--ignore-not-found", "--wait=true", "--timeout=120s")
	cmd.Run() // Ignore errors - namespace may not exist

	// Double-check namespace is gone
	for i := 0; i < 30; i++ {
		checkCmd := exec.Command("kubectl", "get", "namespace", name)
		if err := checkCmd.Run(); err != nil {
			// Namespace doesn't exist, we're done
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// createDeployment creates a simple deployment in the given namespace.
func createDeployment(t *testing.T, namespace, name, image string) {
	t.Helper()
	cmd := exec.Command("kubectl", "create", "deployment", name,
		"--image="+image,
		"-n", namespace)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create deployment %s/%s: %v\n%s", namespace, name, err, output)
	}
}

// waitForDeploymentReady waits for a deployment to become ready.
func waitForDeploymentReady(t *testing.T, namespace, name string, timeout time.Duration) {
	t.Helper()
	cmd := exec.Command("kubectl", "wait", "--for=condition=available",
		"deployment/"+name, "-n", namespace,
		"--timeout="+timeout.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("deployment %s/%s did not become ready: %v\n%s", namespace, name, err, output)
	}
}

// normalizeMapStatusOutput normalizes map status command output.
// Removes non-deterministic content while preserving structure.
func normalizeMapStatusOutput(s string) string {
	// Apply base normalization (ANSI codes, timestamps, etc.)
	s = golden.Normalize(s)

	return strings.TrimSpace(s) + "\n"
}

// runCubScoutWithEnv runs cub-scout with custom environment variables.
func runCubScoutWithEnv(t *testing.T, env map[string]string, args ...string) golden.Result {
	t.Helper()

	binary := os.Getenv("CUB_SCOUT_BINARY")
	if binary == "" {
		binary = filepath.Join(projectRoot(t), "cub-scout")
	}

	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Fatalf("cub-scout binary not found at %s - run 'go build ./cmd/cub-scout' first", binary)
	}

	cmd := exec.Command(binary, args...)

	// Set base environment
	cmd.Env = append(os.Environ(),
		"NO_COLOR=1",
		"TERM=dumb",
		"CI=true",
	)

	// Add custom environment
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	return golden.Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
}

// projectRoot finds the project root by looking for go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

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
