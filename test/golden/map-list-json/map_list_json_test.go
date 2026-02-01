// Package maplistjson provides golden tests for the map list --json CLI command contract.
//
// These tests verify that the map list --json command behaves as documented in:
// docs/reference/cli-contract.md
//
// Test scenarios cover:
//   - Empty cluster baseline (no resources in test namespace)
//   - Single entry (one deployment)
//   - Multiple entries with mixed owners (Native + Helm)
//
// Each test:
//   - Creates a dedicated namespace
//   - Applies fixtures
//   - Runs: cub-scout map list --json -n <namespace>
//   - Parses JSON (fails hard if invalid)
//   - Canonicalizes entries (sorts by namespace, kind, name)
//   - Compares against golden file
//
// Tests skip if no Kubernetes cluster is available.
// Golden files are generated with UPDATE_GOLDEN=1.
//
// Reference: docs/reference/cli-contract.md
//
// Contract requirements for map list --json (per cli-contract.md):
//
//	[
//	  {
//	    "id": "default/default//Deployment/nginx",
//	    "clusterName": "default",
//	    "namespace": "default",
//	    "kind": "Deployment",
//	    "name": "nginx",
//	    "apiVersion": "apps/v1",
//	    "owner": "Flux",
//	    "status": "Ready",
//	    "createdAt": "...",
//	    "updatedAt": "..."
//	  }
//	]
package maplistjson

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

// MapListEntry represents a single entry in the map list --json output.
// Fields match the contract in cli-contract.md.
type MapListEntry struct {
	ID          string `json:"id"`
	ClusterName string `json:"clusterName"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	APIVersion  string `json:"apiVersion"`
	Owner       string `json:"owner"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// TestMapListJSON_EmptyBaseline verifies map list --json output on empty namespace.
// Expected: empty JSON array [] + exit code 0.
// Reference: cli-contract.md map list section.
func TestMapListJSON_EmptyBaseline(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "empty-baseline")

	ns := "cub-scout-golden-maplist-empty"
	cleanupNamespaceAndWait(t, ns)
	createNamespace(t, ns)
	defer cleanupNamespace(t, ns)

	// Run map list --json on empty namespace, filtered to Deployments
	result := golden.RunCubScout(t, "map", "list", "--json", "--namespace", ns, "--kind", "Deployment")

	// Per cli-contract.md: exit code 0 for success
	golden.AssertExitCode(t, 0, result)

	// Parse JSON - fail hard if invalid
	combined := result.Stdout + result.Stderr
	var entries []MapListEntry
	if err := json.Unmarshal([]byte(combined), &entries); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Empty namespace should have empty array
	if len(entries) != 0 {
		t.Errorf("expected empty array for empty namespace, got %d entries", len(entries))
	}

	// Canonicalize and compare to golden
	canonical := canonicalizeAndMarshal(t, entries)
	assertGolden(t, "empty-baseline", canonical)
}

// TestMapListJSON_SingleEntry verifies map list --json output with one resource.
// Expected: JSON array with one entry + exit code 0.
// Reference: cli-contract.md map list section.
func TestMapListJSON_SingleEntry(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "single-entry")

	ns := "cub-scout-golden-maplist-single"
	cleanupNamespaceAndWait(t, ns)
	createNamespace(t, ns)
	defer cleanupNamespace(t, ns)

	// Create a single deployment (Native owner - no labels)
	createDeployment(t, ns, "test-app", "nginx:alpine", nil)
	waitForDeploymentReady(t, ns, "test-app", 60*time.Second)

	// Run map list --json, filtered to Deployments
	result := golden.RunCubScout(t, "map", "list", "--json", "--namespace", ns, "--kind", "Deployment")

	// Per cli-contract.md: exit code 0 for success
	golden.AssertExitCode(t, 0, result)

	// Parse JSON - fail hard if invalid
	combined := result.Stdout + result.Stderr
	var entries []MapListEntry
	if err := json.Unmarshal([]byte(combined), &entries); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Should have exactly one Deployment entry
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	// Verify required fields per contract
	if len(entries) > 0 {
		e := entries[0]
		if e.Namespace != ns {
			t.Errorf("expected namespace %q, got %q", ns, e.Namespace)
		}
		if e.Kind != "Deployment" {
			t.Errorf("expected kind Deployment, got %q", e.Kind)
		}
		if e.Name != "test-app" {
			t.Errorf("expected name test-app, got %q", e.Name)
		}
		// Native owner (no ownership labels)
		if e.Owner != "Native" {
			t.Errorf("expected owner Native, got %q", e.Owner)
		}
	}

	// Canonicalize and compare to golden
	canonical := canonicalizeAndMarshal(t, entries)
	assertGolden(t, "single-entry", canonical)
}

// TestMapListJSON_MultipleEntriesMixedOwners verifies map list --json with mixed ownership.
// Expected: JSON array with entries having different owner types + exit code 0.
// Reference: cli-contract.md map list section, ownership-precedence.md.
//
// This test creates:
//   - One deployment with Helm labels (owner: Helm)
//   - One deployment without labels (owner: Native)
func TestMapListJSON_MultipleEntriesMixedOwners(t *testing.T) {
	requireCluster(t)
	requireGolden(t, "multiple-entries-mixed-owners")

	ns := "cub-scout-golden-maplist-mixed"
	cleanupNamespaceAndWait(t, ns)
	createNamespace(t, ns)
	defer cleanupNamespace(t, ns)

	// Create deployment with Helm labels (Helm owner)
	helmLabels := map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   "my-release",
	}
	createDeployment(t, ns, "helm-app", "nginx:alpine", helmLabels)

	// Create deployment without labels (Native owner)
	createDeployment(t, ns, "native-app", "nginx:alpine", nil)

	// Wait for both to be ready
	waitForDeploymentReady(t, ns, "helm-app", 60*time.Second)
	waitForDeploymentReady(t, ns, "native-app", 60*time.Second)

	// Run map list --json, filtered to Deployments
	result := golden.RunCubScout(t, "map", "list", "--json", "--namespace", ns, "--kind", "Deployment")

	// Per cli-contract.md: exit code 0 for success
	golden.AssertExitCode(t, 0, result)

	// Parse JSON - fail hard if invalid
	combined := result.Stdout + result.Stderr
	var entries []MapListEntry
	if err := json.Unmarshal([]byte(combined), &entries); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Should have exactly 2 Deployment entries
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify we have mixed owners
	owners := make(map[string]bool)
	for _, e := range entries {
		owners[e.Owner] = true
	}

	// We expect both Helm and Native owners
	if !owners["Helm"] {
		t.Errorf("expected at least one Helm-owned resource, got owners: %v", owners)
	}
	if !owners["Native"] {
		t.Errorf("expected at least one Native-owned resource, got owners: %v", owners)
	}

	// Canonicalize and compare to golden
	canonical := canonicalizeAndMarshal(t, entries)
	assertGolden(t, "multiple-entries-mixed-owners", canonical)
}

// canonicalizeAndMarshal sorts entries and produces deterministic JSON output.
// Sort order: namespace, kind, name (ascending).
// Timestamps are normalized to <TIMESTAMP>.
func canonicalizeAndMarshal(t *testing.T, entries []MapListEntry) string {
	t.Helper()

	// Sort entries by namespace, kind, name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Namespace != entries[j].Namespace {
			return entries[i].Namespace < entries[j].Namespace
		}
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Name < entries[j].Name
	})

	// Normalize timestamps and IDs for golden comparison
	for i := range entries {
		entries[i].CreatedAt = "<TIMESTAMP>"
		entries[i].UpdatedAt = "<TIMESTAMP>"
		// Normalize ID to remove cluster-specific parts
		entries[i].ID = normalizeID(entries[i].ID, entries[i].Namespace, entries[i].Kind, entries[i].Name)
	}

	// Marshal with stable formatting
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal entries: %v", err)
	}

	return string(data) + "\n"
}

// normalizeID produces a deterministic ID format.
// Input format: "clusterName/namespace/subnamespace/Kind/name"
// Output: "<CLUSTER>/namespace//Kind/name"
func normalizeID(id, namespace, kind, name string) string {
	// Construct normalized ID with placeholder for cluster
	return "<CLUSTER>/" + namespace + "//" + kind + "/" + name
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
	cmd.Run()
}

// cleanupNamespaceAndWait deletes a namespace and waits for full deletion.
func cleanupNamespaceAndWait(t *testing.T, name string) {
	t.Helper()
	cmd := exec.Command("kubectl", "delete", "namespace", name, "--ignore-not-found", "--wait=true", "--timeout=120s")
	cmd.Run()

	// Double-check namespace is gone
	for i := 0; i < 30; i++ {
		checkCmd := exec.Command("kubectl", "get", "namespace", name)
		if err := checkCmd.Run(); err != nil {
			return
		}
		time.Sleep(1 * time.Second)
	}
}

// createDeployment creates a deployment with optional labels.
func createDeployment(t *testing.T, namespace, name, image string, labels map[string]string) {
	t.Helper()

	// Build kubectl create deployment command
	args := []string{"create", "deployment", name, "--image=" + image, "-n", namespace}
	cmd := exec.Command("kubectl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to create deployment %s/%s: %v\n%s", namespace, name, err, output)
	}

	// Apply labels if provided
	if len(labels) > 0 {
		labelArgs := []string{"label", "deployment", name, "-n", namespace}
		for k, v := range labels {
			labelArgs = append(labelArgs, k+"="+v)
		}
		cmd := exec.Command("kubectl", labelArgs...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to label deployment %s/%s: %v\n%s", namespace, name, err, output)
		}
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
		t.Fatalf("golden file not found: %s\n\nActual output:\n%s\n\nRun with UPDATE_GOLDEN=1 to create it:\n  UPDATE_GOLDEN=1 go test ./test/golden/map-list-json/...", goldenPath, actual)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if actual != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s",
			goldenPath, string(expected), actual)
	}
}
