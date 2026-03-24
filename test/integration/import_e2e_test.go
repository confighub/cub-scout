// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

// Package integration provides E2E tests for the import command.
//
// These tests exercise the full import round-trip:
//   - Deploy fixtures to a kind cluster
//   - Run cub-scout import --dry-run --json to test discovery + proposal
//   - Run cub-scout import -y to create real Spaces/Units in ConfigHub
//   - Verify the created resources via cub CLI
//   - Clean up (deferred)
//
// Dry-run tests (TestImportDryRun*) need only a Kubernetes cluster.
// Full round-trip tests (TestImportFull*, TestImportIdempotent, TestImportCleanup)
// also require ConfigHub authentication (cub auth login).
//
// Run with:
//
//	go test -tags=integration ./test/integration/... -run TestImport -v -timeout 300s
package integration

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureDir is the path to the import E2E fixtures relative to this test file.
const fixtureDir = "../fixtures/import-e2e"

// createTestNamespace creates a uniquely named namespace and registers cleanup.
func createTestNamespace(t *testing.T) string {
	t.Helper()
	skipIfNoCluster(t)

	name := fmt.Sprintf("e2e-import-%d", time.Now().UnixMilli())

	cmd := exec.Command("kubectl", "create", "namespace", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create namespace %s: %s (%v)", name, string(output), err)
	}

	t.Cleanup(func() {
		cleanupCmd := exec.Command("kubectl", "delete", "namespace", name, "--ignore-not-found", "--wait=false")
		if out, err := cleanupCmd.CombinedOutput(); err != nil {
			t.Logf("Warning: cleanup namespace %s failed: %s (%v)", name, string(out), err)
		}
	})

	t.Logf("Created test namespace: %s", name)
	return name
}

// deployFixtures applies all YAML fixtures to the given namespace.
func deployFixtures(t *testing.T, namespace string) {
	t.Helper()

	// Apply each fixture file individually so namespace override works
	files := []string{
		fixtureDir + "/argo-deployments.yaml",
		fixtureDir + "/helm-statefulset.yaml",
		fixtureDir + "/native-configmap.yaml",
	}

	for _, f := range files {
		cmd := exec.Command("kubectl", "apply", "-f", f, "-n", namespace)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to apply fixture %s: %s (%v)", f, string(output), err)
		}
		t.Logf("Applied: %s", f)
	}

	// Wait briefly for resources to be created
	time.Sleep(2 * time.Second)
}

// waitForDeployments waits for all deployments in the namespace to be available.
func waitForDeployments(t *testing.T, namespace string) {
	t.Helper()

	cmd := exec.Command("kubectl", "wait", "--for=condition=available",
		"deployment", "--all", "-n", namespace, "--timeout=60s")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: not all deployments ready (may be OK): %s", string(output))
	}
}

// deleteTestSpace deletes a ConfigHub space and all its contents.
func deleteTestSpace(t *testing.T, slug string) {
	t.Helper()

	cmd := exec.Command("cub", "space", "delete", slug, "--recursive")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: cleanup space %s failed: %s (%v)", slug, string(output), err)
	} else {
		t.Logf("Deleted test space: %s", slug)
	}
}

// spaceExistsInList checks whether a space slug appears in `cub space list --json`.
func spaceExistsInList(t *testing.T, slug string) bool {
	t.Helper()

	cmd := exec.Command("cub", "space", "list", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: space list failed: %s (%v)", string(output), err)
		return false
	}

	var spaces []map[string]interface{}
	if err := json.Unmarshal(output, &spaces); err != nil {
		t.Logf("Warning: failed to parse space list: %v", err)
		return false
	}

	for _, s := range spaces {
		// cub space list --json nests Space data inside a "Space" object
		if s["Slug"] == slug {
			return true
		}
		if inner, ok := s["Space"].(map[string]interface{}); ok {
			if inner["Slug"] == slug {
				return true
			}
		}
	}
	return false
}

// =============================================================================
// JSON types matching the actual import --dry-run --json output.
//
// The actual output shape is:
//
//	{
//	  "namespaces": ["ns1"],
//	  "proposal": { "app": "...", "deployer": "...", "units": [...], ... },
//	  "workloads": [{ "kind": "...", "name": "...", "owner": "...", ... }]
//	}
// =============================================================================

// importDryRunResult matches the top-level JSON output of `import --dry-run --json`.
type importDryRunResult struct {
	Namespaces []string         `json:"namespaces"`
	Proposal   *importProposal  `json:"proposal"`
	Workloads  []importWorkload `json:"workloads"`
	Evidence   *importEvidence  `json:"evidence,omitempty"`
}

// importProposal matches the "proposal" object in the JSON output.
type importProposal struct {
	App         string             `json:"app"`
	Deployer    string             `json:"deployer,omitempty"`
	Units       []importUnit       `json:"units"`
	ClusterOnly []importClusterApp `json:"clusterOnly,omitempty"`
}

// importUnit matches a unit in the proposal.
type importUnit struct {
	Slug      string            `json:"slug"`
	App       string            `json:"app"`
	Variant   string            `json:"variant"`
	Workloads []string          `json:"workloads"`
	Status    string            `json:"status,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// importClusterApp matches a cluster-only app in the proposal.
type importClusterApp struct {
	App       string   `json:"app"`
	Workloads []string `json:"workloads"`
	Owner     string   `json:"owner"`
}

// importWorkload matches a workload in the JSON output.
type importWorkload struct {
	Kind      string            `json:"kind"`
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Owner     string            `json:"owner"`
	Connected bool              `json:"connected"`
	Ready     bool              `json:"ready"`
	Replicas  int32             `json:"replicas"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type importEvidence struct {
	Source     string `json:"source"`
	BundlePath string `json:"bundlePath,omitempty"`
}

// =============================================================================
// Dry-Run Tests (cluster only, no ConfigHub needed)
// =============================================================================

// TestImportDryRunJSON verifies that import --dry-run --json produces valid output
// with correct workload discovery and ownership detection.
func TestImportDryRunJSON(t *testing.T) {
	skipIfNoCluster(t)

	ns := createTestNamespace(t)
	deployFixtures(t, ns)
	waitForDeployments(t, ns)

	// Run import --dry-run --json
	output := runCubAgent(t, "import", "-n", ns, "--dry-run", "--json")

	// Must parse as our expected structure
	var result importDryRunResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("import --dry-run --json output is not valid JSON: %v\nOutput (first 500 chars): %.500s", err, output)
	}

	// Must have namespaces
	if len(result.Namespaces) == 0 {
		t.Error("Expected at least one namespace in output")
	}

	// Must have workloads
	// We deployed 2 Deployments + 1 StatefulSet = 3 workloads
	// (ConfigMap is not a workload type — import only discovers Deployments/StatefulSets/DaemonSets)
	if len(result.Workloads) < 3 {
		t.Errorf("Expected at least 3 workloads, got %d", len(result.Workloads))
	}

	// Check ownership detection
	foundArgo := 0
	foundHelm := 0
	for _, w := range result.Workloads {
		switch w.Owner {
		case "ArgoCD":
			foundArgo++
		case "Helm":
			foundHelm++
		}
	}

	if foundArgo != 2 {
		t.Errorf("Expected 2 ArgoCD-owned workloads, found %d", foundArgo)
	}
	if foundHelm != 1 {
		t.Errorf("Expected 1 Helm-owned workload, found %d", foundHelm)
	}

	// Must have a proposal
	if result.Proposal == nil {
		t.Fatal("Expected 'proposal' to be present, got nil")
	}

	// Proposal must have app
	if result.Proposal.App == "" {
		t.Error("Expected proposal.app to be non-empty")
	}

	// Proposal must have units
	if len(result.Proposal.Units) == 0 {
		t.Error("Expected at least one unit in proposal")
	}

	t.Logf("Dry-run result: %d workloads, app=%s, %d units",
		len(result.Workloads), result.Proposal.App, len(result.Proposal.Units))
}

// TestImportDryRunSuggestionContract verifies the JSON contract shape of the
// import output, ensuring all required fields are present and valid.
func TestImportDryRunSuggestionContract(t *testing.T) {
	skipIfNoCluster(t)

	ns := createTestNamespace(t)
	deployFixtures(t, ns)
	waitForDeployments(t, ns)

	output := runCubAgent(t, "import", "-n", ns, "--dry-run", "--json")

	var result importDryRunResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to parse import JSON output: %v", err)
	}

	// Contract: namespaces contains the namespace we requested
	found := false
	for _, n := range result.Namespaces {
		if n == ns {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected namespaces to contain %s, got %v", ns, result.Namespaces)
	}

	// Contract: each workload has required fields
	validOwners := map[string]bool{
		"Flux": true, "ArgoCD": true, "Helm": true, "Native": true,
		"ConfigHub": true, "Terraform": true, "Crossplane": true,
	}
	for i, w := range result.Workloads {
		if w.Kind == "" {
			t.Errorf("Workload[%d]: kind is empty", i)
		}
		if w.Namespace == "" {
			t.Errorf("Workload[%d]: namespace is empty", i)
		}
		if w.Name == "" {
			t.Errorf("Workload[%d]: name is empty", i)
		}
		if w.Owner == "" {
			t.Errorf("Workload[%d] (%s/%s): owner is empty", i, w.Kind, w.Name)
		}
		if !validOwners[w.Owner] {
			t.Errorf("Workload[%d] (%s/%s): unexpected owner %q", i, w.Kind, w.Name, w.Owner)
		}
	}

	// Contract: proposal has non-empty app
	if result.Proposal == nil {
		t.Fatal("proposal is nil")
	}
	if result.Proposal.App == "" {
		t.Error("proposal.app is empty")
	}

	// Contract: each unit has slug and workloads
	for i, u := range result.Proposal.Units {
		if u.Slug == "" {
			t.Errorf("Unit[%d]: slug is empty", i)
		}
		if len(u.Workloads) == 0 {
			t.Errorf("Unit[%d] (%s): workloads array is empty", i, u.Slug)
		}
	}

	t.Logf("Contract check passed: %d workloads, %d units",
		len(result.Workloads), len(result.Proposal.Units))
}

// TestImportFromBundleDryRunJSONConnectedContract verifies bundle-driven import
// JSON contract in connected environments (skip when not authenticated).
func TestImportFromBundleDryRunJSONConnectedContract(t *testing.T) {
	skipIfNotConnected(t)

	bundlePath := filepath.Join("..", "..", "examples", "workflows", "fleet-demo", "bundles", "dev")
	output := runCubAgent(t, "import", "--from-bundle", bundlePath, "--dry-run", "--json")

	var result importDryRunResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Failed to parse import bundle JSON output: %v", err)
	}

	if result.Evidence == nil {
		t.Fatal("Expected evidence block in JSON output")
	}
	if result.Evidence.Source != "bundle" {
		t.Fatalf("evidence.source = %q, want bundle", result.Evidence.Source)
	}
	if result.Evidence.BundlePath != bundlePath {
		t.Fatalf("evidence.bundlePath = %q, want %q", result.Evidence.BundlePath, bundlePath)
	}
	if result.Proposal == nil || result.Proposal.App == "" {
		t.Fatal("Expected non-empty proposal.app for bundle import dry-run")
	}
	if len(result.Workloads) == 0 {
		t.Fatal("Expected at least one workload from bundle import dry-run")
	}
}

// =============================================================================
// Full Round-Trip Tests (cluster + ConfigHub)
// =============================================================================

// TestImportFullRoundTrip exercises the complete import lifecycle:
// discover → propose → create space + units → verify in ConfigHub → cleanup.
func TestImportFullRoundTrip(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNotConnected(t)

	ns := createTestNamespace(t)
	deployFixtures(t, ns)
	waitForDeployments(t, ns)

	// Step 1: Dry-run to capture what will be created
	dryRunOutput := runCubAgent(t, "import", "-n", ns, "--dry-run", "--json")

	var dryResult importDryRunResult
	if err := json.Unmarshal([]byte(dryRunOutput), &dryResult); err != nil {
		t.Fatalf("Failed to parse dry-run output: %v", err)
	}
	if dryResult.Proposal == nil {
		t.Fatal("Dry-run returned nil proposal")
	}

	spaceName := dryResult.Proposal.App
	if spaceName == "" {
		t.Fatal("Dry-run returned empty app — cannot proceed with import")
	}
	t.Logf("Will create space: %s with %d units", spaceName, len(dryResult.Proposal.Units))

	// Register cleanup (runs even if import fails)
	t.Cleanup(func() {
		deleteTestSpace(t, spaceName)
	})

	// Step 2: Actual import (non-interactive)
	importOutput := runCubAgentAllowFailures(t, "import", "-n", ns, "-y", "--no-log")
	t.Logf("Import output:\n%s", importOutput)

	// Import should mention creating units
	if !strings.Contains(importOutput, "Creating unit") && !strings.Contains(importOutput, "created") {
		t.Errorf("Import output doesn't mention creating units")
	}

	// Step 3: Verify space exists in ConfigHub
	if !spaceExistsInList(t, spaceName) {
		t.Fatalf("Space %s was not found in ConfigHub after import", spaceName)
	}
	t.Logf("Verified: space %s exists", spaceName)

	// Step 4: Verify units exist
	cmd := exec.Command("cub", "unit", "list", "--space", spaceName, "--json")
	unitOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to list units in space %s: %s (%v)", spaceName, string(unitOutput), err)
	}

	var units []map[string]interface{}
	if err := json.Unmarshal(unitOutput, &units); err != nil {
		t.Fatalf("Failed to parse unit list JSON: %v", err)
	}

	// Should have created at least one unit
	if len(units) == 0 {
		t.Error("No units found in space after import")
	}

	// Verify no null slugs (issue #1 regression)
	// cub unit list --json nests unit data: [{Unit: {Slug: "..."}, ...}, ...]
	for _, u := range units {
		slug, _ := u["Slug"].(string)
		if slug == "" {
			// Check nested structure
			if inner, ok := u["Unit"].(map[string]interface{}); ok {
				slug, _ = inner["Slug"].(string)
			}
		}
		if slug == "" || slug == "null" {
			t.Errorf("Unit has null or empty slug (issue #1 pattern): %v", u)
		}
	}

	t.Logf("Round-trip complete: space=%s, units=%d", spaceName, len(units))
}

// TestImportIdempotent verifies that running import twice on the same namespace
// does not fail and does not create duplicate resources.
func TestImportIdempotent(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNotConnected(t)

	ns := createTestNamespace(t)
	deployFixtures(t, ns)
	waitForDeployments(t, ns)

	// Dry-run to get the expected space name
	dryRunOutput := runCubAgent(t, "import", "-n", ns, "--dry-run", "--json")
	var dryResult importDryRunResult
	if err := json.Unmarshal([]byte(dryRunOutput), &dryResult); err != nil {
		t.Fatalf("Failed to parse dry-run output: %v", err)
	}
	if dryResult.Proposal == nil {
		t.Fatal("Dry-run returned nil proposal")
	}
	spaceName := dryResult.Proposal.App

	t.Cleanup(func() {
		deleteTestSpace(t, spaceName)
	})

	// First import
	output1 := runCubAgentAllowFailures(t, "import", "-n", ns, "-y", "--no-log")
	t.Logf("First import output:\n%s", output1)

	// Count units after first import
	cmd := exec.Command("cub", "unit", "list", "--space", spaceName, "--json")
	unitOutput1, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to list units after first import: %s (%v)", string(unitOutput1), err)
	}
	var units1 []map[string]interface{}
	if err := json.Unmarshal(unitOutput1, &units1); err != nil {
		t.Fatalf("Failed to parse unit list: %v", err)
	}
	count1 := len(units1)

	// Second import (should not fail)
	output2 := runCubAgentAllowFailures(t, "import", "-n", ns, "-y", "--no-log")
	t.Logf("Second import output:\n%s", output2)

	// The second import should mention "(exists)" for the space
	if !strings.Contains(output2, "exists") && !strings.Contains(output2, "Created") {
		t.Logf("Note: second import output doesn't show space status")
	}

	// Count units after second import — should be the same
	cmd = exec.Command("cub", "unit", "list", "--space", spaceName, "--json")
	unitOutput2, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to list units after second import: %s (%v)", string(unitOutput2), err)
	}
	var units2 []map[string]interface{}
	if err := json.Unmarshal(unitOutput2, &units2); err != nil {
		t.Fatalf("Failed to parse unit list: %v", err)
	}
	count2 := len(units2)

	// Unit count should not have doubled
	if count2 > count1 {
		t.Errorf("Second import created additional units: first=%d, second=%d", count1, count2)
	}

	t.Logf("Idempotency check: first=%d units, second=%d units", count1, count2)
}

// TestImportCleanup verifies that deleting a space removes all resources cleanly.
func TestImportCleanup(t *testing.T) {
	skipIfNoCluster(t)
	skipIfNotConnected(t)

	ns := createTestNamespace(t)
	deployFixtures(t, ns)
	waitForDeployments(t, ns)

	// Import to create resources
	dryRunOutput := runCubAgent(t, "import", "-n", ns, "--dry-run", "--json")
	var dryResult importDryRunResult
	if err := json.Unmarshal([]byte(dryRunOutput), &dryResult); err != nil {
		t.Fatalf("Failed to parse dry-run output: %v", err)
	}
	if dryResult.Proposal == nil {
		t.Fatal("Dry-run returned nil proposal")
	}
	spaceName := dryResult.Proposal.App

	// Import (no deferred cleanup — we're testing manual cleanup)
	importOutput := runCubAgentAllowFailures(t, "import", "-n", ns, "-y", "--no-log")
	t.Logf("Import output:\n%s", importOutput)

	// Verify space exists
	if !spaceExistsInList(t, spaceName) {
		t.Fatalf("Space %s not found after import — nothing to test cleanup on", spaceName)
	}

	// Delete the space
	cmd := exec.Command("cub", "space", "delete", spaceName, "--recursive")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to delete space %s: %s (%v)", spaceName, string(output), err)
	}
	t.Logf("Deleted space: %s", spaceName)

	// Verify space is gone
	if spaceExistsInList(t, spaceName) {
		t.Errorf("Space %s still exists after deletion", spaceName)
	}

	t.Log("Cleanup verification passed: space deleted successfully")
}
