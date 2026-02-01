// Package mapdeployersjson provides golden tests for the map deployers --json CLI command contract.
//
// These tests verify that the map deployers --json command behaves as documented in:
// docs/reference/cli-contract.md
//
// Test scenarios cover:
//   - Empty cluster (no deployers in test namespace)
//   - Single deployer (one Deployment)
//   - Multiple deployers with deterministic ordering
//
// Each test:
//   - Creates test resources (Deployments)
//   - Runs: cub-scout map deployers --json
//   - Parses JSON (fails hard if invalid)
//   - Filters entries to test namespace
//   - Canonicalizes entries (sorts by kind, namespace, name)
//   - Compares against golden file
//
// Tests skip if no Kubernetes cluster is available.
// Golden files are generated with UPDATE_GOLDEN=1.
//
// Reference: docs/reference/cli-contract.md
//
// Contract requirements for map deployers --json:
//
//	[
//	  {
//	    "kind": "Deployment",
//	    "name": "my-app",
//	    "namespace": "test-ns",
//	    "status": "Ready",
//	    "ready": true,
//	    "revision": "gen-1"
//	  }
//	]
package mapdeployersjson

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/test/golden"
)

// updateGolden controls whether to update golden files.
var updateGolden = os.Getenv("UPDATE_GOLDEN") == "1"

// testNamespace is used to isolate test resources from other cluster resources.
const testNamespace = "cub-scout-golden-deployers"

func init() {
	for _, arg := range os.Args {
		if arg == "-update" || arg == "--update" {
			updateGolden = true
			break
		}
	}
}

// DeployerEntry represents a single entry in the map deployers --json output.
type DeployerEntry struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Ready     bool   `json:"ready"`
	Revision  string `json:"revision,omitempty"`
	Resources int    `json:"resources,omitempty"`
}

// TestMapDeployersJSON_EmptyBaseline verifies map deployers --json on cluster without deployers.
// Expected: empty JSON array [] + exit code 0.
func TestMapDeployersJSON_EmptyBaseline(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "empty-baseline")

	// Setup: create clean test namespace, ensure no Deployments exist
	setupTestNamespace(t)
	defer cleanupTestNamespace(t)

	// Run map deployers --json
	result := golden.RunCubScout(t, "map", "deployers", "--json")

	// Exit code 0 for success
	golden.AssertExitCode(t, 0, result)

	// Parse JSON - fail hard if invalid
	combined := result.Stdout + result.Stderr
	var entries []DeployerEntry
	if err := json.Unmarshal([]byte(combined), &entries); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Filter to test namespace only
	entries = filterByNamespace(entries, testNamespace)

	// Empty namespace should have empty array
	if len(entries) != 0 {
		t.Errorf("expected empty array for empty namespace, got %d entries", len(entries))
	}

	// Canonicalize and compare to golden
	canonical := canonicalizeAndMarshal(t, entries)
	assertGolden(t, "empty-baseline", canonical)
}

// TestMapDeployersJSON_SingleDeployer verifies map deployers --json with one deployer.
// Expected: JSON array with one entry + exit code 0.
func TestMapDeployersJSON_SingleDeployer(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "single-deployer")

	// Setup: create clean test namespace
	setupTestNamespace(t)
	defer cleanupTestNamespace(t)

	// Create a single Deployment
	createDeployment(t, testNamespace, "test-single")
	waitForDeployment(t, testNamespace, "test-single")

	// Run map deployers --json
	result := golden.RunCubScout(t, "map", "deployers", "--json")

	// Exit code 0 for success
	golden.AssertExitCode(t, 0, result)

	// Parse JSON - fail hard if invalid
	combined := result.Stdout + result.Stderr
	var entries []DeployerEntry
	if err := json.Unmarshal([]byte(combined), &entries); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Filter to test namespace only
	entries = filterByNamespace(entries, testNamespace)

	// Should have exactly one entry
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	// Verify required fields per contract
	if len(entries) > 0 {
		e := entries[0]
		if e.Kind != "Deployment" {
			t.Errorf("expected kind Deployment, got %q", e.Kind)
		}
		if e.Name != "test-single" {
			t.Errorf("expected name test-single, got %q", e.Name)
		}
	}

	// Canonicalize and compare to golden
	canonical := canonicalizeAndMarshal(t, entries)
	assertGolden(t, "single-deployer", canonical)
}

// TestMapDeployersJSON_MultipleDeployers verifies map deployers --json with multiple deployers.
// Expected: JSON array with multiple entries in deterministic order + exit code 0.
func TestMapDeployersJSON_MultipleDeployers(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "multiple-deployers")

	// Setup: create clean test namespace
	setupTestNamespace(t)
	defer cleanupTestNamespace(t)

	// Create multiple Deployments (created out of alphabetical order to test sorting)
	createDeployment(t, testNamespace, "app-charlie")
	createDeployment(t, testNamespace, "app-alpha")
	createDeployment(t, testNamespace, "app-bravo")
	waitForDeployment(t, testNamespace, "app-charlie")
	waitForDeployment(t, testNamespace, "app-alpha")
	waitForDeployment(t, testNamespace, "app-bravo")

	// Run map deployers --json
	result := golden.RunCubScout(t, "map", "deployers", "--json")

	// Exit code 0 for success
	golden.AssertExitCode(t, 0, result)

	// Parse JSON - fail hard if invalid
	combined := result.Stdout + result.Stderr
	var entries []DeployerEntry
	if err := json.Unmarshal([]byte(combined), &entries); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Filter to test namespace only
	entries = filterByNamespace(entries, testNamespace)

	// Should have 3 entries
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Canonicalize and compare to golden
	canonical := canonicalizeAndMarshal(t, entries)
	assertGolden(t, "multiple-deployers", canonical)
}

// filterByNamespace returns only entries matching the given namespace.
func filterByNamespace(entries []DeployerEntry, namespace string) []DeployerEntry {
	var filtered []DeployerEntry
	for _, e := range entries {
		if e.Namespace == namespace {
			filtered = append(filtered, e)
		}
	}
	// Ensure non-nil slice for JSON marshaling
	if filtered == nil {
		filtered = []DeployerEntry{}
	}
	return filtered
}

// canonicalizeAndMarshal sorts entries and produces deterministic JSON output.
// Sort order: kind, namespace, name (ascending).
// Revisions are normalized for golden comparison.
func canonicalizeAndMarshal(t *testing.T, entries []DeployerEntry) string {
	t.Helper()

	// Sort entries by kind, namespace, name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		if entries[i].Namespace != entries[j].Namespace {
			return entries[i].Namespace < entries[j].Namespace
		}
		return entries[i].Name < entries[j].Name
	})

	// Normalize revisions for golden comparison (generation numbers may vary)
	for i := range entries {
		if entries[i].Revision != "" && entries[i].Revision != "-" {
			entries[i].Revision = "-"
		}
	}

	// Marshal with stable formatting
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal entries: %v", err)
	}

	return string(data) + "\n"
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
func requireGolden(t *testing.T, name string) {
	t.Helper()
	if updateGolden {
		return
	}
	goldenPath := filepath.Join("testdata", name+".golden.json")
	if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
		t.Skipf("PRECONDITION: Golden file not found: %s (run UPDATE_GOLDEN=1 with cluster)", goldenPath)
	}
}

// setupTestNamespace creates a clean test namespace.
func setupTestNamespace(t *testing.T) {
	t.Helper()
	// Delete if exists, then recreate
	cmd := exec.Command("kubectl", "delete", "namespace", testNamespace, "--ignore-not-found", "--wait=true", "--timeout=60s")
	cmd.Run()
	cmd = exec.Command("kubectl", "create", "namespace", testNamespace)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create test namespace: %v\n%s", err, output)
	}
}

// cleanupTestNamespace deletes the test namespace.
func cleanupTestNamespace(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "delete", "namespace", testNamespace, "--ignore-not-found", "--wait=false")
	cmd.Run()
}

// createDeployment creates a minimal Deployment for testing.
func createDeployment(t *testing.T, namespace, name string) {
	t.Helper()
	yaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: ` + name + `
  namespace: ` + namespace + `
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ` + name + `
  template:
    metadata:
      labels:
        app: ` + name + `
    spec:
      containers:
      - name: nginx
        image: nginx:1.27-alpine
        resources:
          limits:
            memory: "64Mi"
            cpu: "100m"`

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yaml)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create Deployment %s/%s: %v\n%s", namespace, name, err, output)
	}
}

// waitForDeployment waits for a Deployment to be available.
func waitForDeployment(t *testing.T, namespace, name string) {
	t.Helper()
	// Wait up to 60s for the deployment to be available
	cmd := exec.Command("kubectl", "rollout", "status", "deployment/"+name, "-n", namespace, "--timeout=60s")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Not fatal - deployment may not be fully ready, but should still appear
		t.Logf("warning: deployment %s/%s not fully ready: %v\n%s", namespace, name, err, output)
	}
	// Small additional delay to ensure visibility
	time.Sleep(500 * time.Millisecond)
}

// assertGolden compares output against golden file.
func assertGolden(t *testing.T, name, actual string) {
	t.Helper()

	goldenPath := filepath.Join("testdata", name+".golden.json")

	if updateGolden {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(actual), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		t.Fatalf("golden file not found: %s\n\nActual output:\n%s\n\nRun with UPDATE_GOLDEN=1 to create it:\n  UPDATE_GOLDEN=1 go test ./test/golden/map-deployers-json/...", goldenPath, actual)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if actual != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s",
			goldenPath, string(expected), actual)
	}
}
