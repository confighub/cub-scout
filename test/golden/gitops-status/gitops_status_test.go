// Package gitops_status provides golden tests for the gitops status CLI command contract.
//
// These tests verify that the gitops status command behaves as documented.
//
// Test scenarios cover:
//   - JSON output format verification
//   - Human-readable output verification
//   - Healthy cluster output
//   - Failure state output
//
// Tests use the CUB_SCOUT_TEST_GITOPS_JSON environment variable hook
// to bypass cluster access and use fixture data instead.
//
// Reference: docs/reference/cli-contract.md
package gitops_status

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// TestGitOpsStatus_JSON verifies that --json output is valid JSON with required fields.
// Uses test hook to bypass cluster access.
func TestGitOpsStatus_JSON(t *testing.T) {
	// Use test hook to load fixture data
	inputPath := filepath.Join(projectRoot(t), "test", "golden", "gitops-status", "input.json")
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", inputPath)

	result := golden.RunCubScout(t, "gitops", "status", "--json")

	golden.AssertExitCode(t, 0, result)

	// Verify JSON structure
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result.Stdout), &output); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, result.Stdout)
	}

	// Verify required fields per contract
	requiredFields := []string{"backend", "transport", "deployers", "sources"}
	for _, field := range requiredFields {
		if _, ok := output[field]; !ok {
			t.Errorf("JSON output missing required field: %s", field)
		}
	}

	// Verify backend value
	if backend, ok := output["backend"].(string); !ok || backend != "flux" {
		t.Errorf("expected backend 'flux', got %v", output["backend"])
	}

	// Verify transport value
	if transport, ok := output["transport"].(string); !ok || transport != "oci" {
		t.Errorf("expected transport 'oci', got %v", output["transport"])
	}

	// Verify deployers array
	deployers, ok := output["deployers"].([]interface{})
	if !ok {
		t.Fatalf("deployers should be an array")
	}
	if len(deployers) != 2 {
		t.Errorf("expected 2 deployers, got %d", len(deployers))
	}

	// Verify ConfigHub target
	confighubTarget, ok := output["confighubTarget"].(map[string]interface{})
	if !ok {
		t.Fatal("confighubTarget should be present")
	}
	if space, ok := confighubTarget["space"].(string); !ok || space != "production" {
		t.Errorf("expected space 'production', got %v", confighubTarget["space"])
	}
}

// TestGitOpsStatus_ASCII verifies human-readable output contains expected elements.
// Uses test hook to bypass cluster access.
func TestGitOpsStatus_ASCII(t *testing.T) {
	// Use test hook to load fixture data
	inputPath := filepath.Join(projectRoot(t), "test", "golden", "gitops-status", "input.json")
	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", inputPath)

	result := golden.RunCubScout(t, "gitops", "status")

	golden.AssertExitCode(t, 0, result)

	output := result.Stdout

	// Verify key elements are present
	expectedStrings := []string{
		"GITOPS STATUS",
		"Backend:",
		"Transport:",
		"SOURCES",
		"DEPLOYERS",
		"OCIRepository",
		"Kustomization",
		"app-frontend",
		"app-backend",
		"AuthenticationFailed",
		"source",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("output should contain %q", expected)
		}
	}

	// Verify failure indicators
	if !strings.Contains(output, "failing") {
		t.Error("output should indicate failures")
	}

	// Verify next steps for failures
	if !strings.Contains(output, "NEXT STEPS") {
		t.Error("output should contain next steps for failures")
	}
}

// TestGitOpsStatus_Healthy verifies output when all deployers are healthy.
// Uses test hook with a healthy fixture.
func TestGitOpsStatus_Healthy(t *testing.T) {
	// Create healthy fixture
	healthyFixture := `{
		"backend": "flux",
		"transport": "oci",
		"deployers": [
			{
				"kind": "Kustomization",
				"name": "app",
				"namespace": "flux-system",
				"ready": true,
				"stage": "healthy"
			}
		],
		"sources": [
			{
				"kind": "OCIRepository",
				"name": "manifests",
				"namespace": "flux-system",
				"ready": true,
				"url": "oci://ghcr.io/acme/manifests",
				"artifactDigest": "sha256:abc123"
			}
		],
		"healthyCount": 1,
		"failedCount": 0
	}`

	tmpFile, err := os.CreateTemp("", "gitops-healthy-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(healthyFixture); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", tmpFile.Name())

	result := golden.RunCubScout(t, "gitops", "status")

	golden.AssertExitCode(t, 0, result)

	output := result.Stdout

	// Verify healthy indicator
	if !strings.Contains(output, "healthy") {
		t.Error("output should indicate healthy status")
	}

	// Should NOT contain next steps (only shown for failures)
	if strings.Contains(output, "NEXT STEPS") {
		t.Error("healthy cluster should not show next steps")
	}
}

// TestGitOpsStatus_NoBackend verifies output when no GitOps backend is detected.
func TestGitOpsStatus_NoBackend(t *testing.T) {
	// Create no-backend fixture
	noBackendFixture := `{
		"backend": "none",
		"transport": "unknown",
		"deployers": [],
		"sources": [],
		"healthyCount": 0,
		"failedCount": 0
	}`

	tmpFile, err := os.CreateTemp("", "gitops-none-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(noBackendFixture); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	t.Setenv("CUB_SCOUT_TEST_GITOPS_JSON", tmpFile.Name())

	result := golden.RunCubScout(t, "gitops", "status")

	golden.AssertExitCode(t, 0, result)

	output := result.Stdout

	// Verify no backend message
	if !strings.Contains(output, "No GitOps/controller backend detected") {
		t.Error("output should indicate no backend detected")
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
