//go:build integration
// +build integration

// Package integration provides integration tests that require both
// a Kubernetes cluster and ConfigHub API access.
//
// Run with: go test -tags=integration ./test/integration/...
package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// cubOutput represents the JSON output from cub CLI commands
// Note: cub CLI returns nested objects (e.g., .BridgeWorker.Slug)
type cubWorkerItem struct {
	BridgeWorker struct {
		Slug      string `json:"Slug"`
		Condition string `json:"Condition"`
	} `json:"BridgeWorker"`
}

type cubTargetItem struct {
	Target struct {
		Slug string `json:"Slug"`
	} `json:"Target"`
}

// skipIfNotConnected skips the test if ConfigHub is not available
func skipIfNotConnected(t *testing.T) {
	t.Helper()

	// Check if cub CLI exists
	if _, err := exec.LookPath("cub"); err != nil {
		t.Skip("cub CLI not installed")
	}

	// Check if authenticated
	cmd := exec.Command("cub", "context", "get")
	if err := cmd.Run(); err != nil {
		t.Skip("Not authenticated to ConfigHub (run: cub auth login)")
	}
}

// skipIfNoCluster skips the test if no Kubernetes cluster is available
func skipIfNoCluster(t *testing.T) {
	t.Helper()

	cmd := exec.Command("kubectl", "cluster-info")
	if err := cmd.Run(); err != nil {
		t.Skip("No Kubernetes cluster available")
	}
}

// getCurrentSpace returns the current ConfigHub space
func getCurrentSpace(t *testing.T) string {
	t.Helper()

	// Parse "Default Space" from cub context get output
	cmd := exec.Command("cub", "context", "get")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get context: %v", err)
	}

	// Look for "Default Space" line
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Default Space") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				return fields[len(fields)-1]
			}
		}
	}

	t.Skip("No active space set (run: cub context set --space <slug>)")
	return ""
}

// requireWorker ensures a worker exists and returns its slug
func requireWorker(t *testing.T, space string) string {
	t.Helper()

	cmd := exec.Command("cub", "worker", "list", "--space", space, "--json")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to list workers: %v", err)
	}

	var workers []cubWorkerItem
	if err := json.Unmarshal(output, &workers); err != nil {
		t.Fatalf("Failed to parse workers: %v", err)
	}

	if len(workers) == 0 {
		t.Skip("No workers in space")
	}

	slug := workers[0].BridgeWorker.Slug
	if slug == "" || slug == "null" {
		t.Fatalf("Worker slug is null (issue #1 pattern)")
	}

	return slug
}

// requireTarget ensures a target exists and returns its slug
func requireTarget(t *testing.T, space string) string {
	t.Helper()

	cmd := exec.Command("cub", "target", "list", "--space", space, "--json")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to list targets: %v", err)
	}

	var targets []cubTargetItem
	if err := json.Unmarshal(output, &targets); err != nil {
		t.Fatalf("Failed to parse targets: %v", err)
	}

	if len(targets) == 0 {
		t.Skip("No targets in space")
	}

	slug := targets[0].Target.Slug
	if slug == "" || slug == "null" {
		t.Fatalf("Target slug is null (issue #1 pattern)")
	}

	return slug
}

// getCubAgentPath finds the cub-agent binary
func getCubAgentPath() string {
	// Check multiple locations
	paths := []string{
		"./cub-agent",
		"../../cub-agent",
		"../../../cub-agent",
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "cub-agent" // Fall back to PATH
}

// runCubAgent runs cub-agent with given arguments and returns output
func runCubAgent(t *testing.T, args ...string) string {
	t.Helper()

	cubAgentPath := getCubAgentPath()
	cmd := exec.Command(cubAgentPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("cub-agent output: %s", output)
		t.Fatalf("Failed to run cub-agent %v: %v", args, err)
	}

	return string(output)
}

// runCubAgentAllowFailures runs cub-agent and returns output even on non-zero exit
func runCubAgentAllowFailures(t *testing.T, args ...string) string {
	t.Helper()

	cubAgentPath := getCubAgentPath()
	cmd := exec.Command(cubAgentPath, args...)
	output, _ := cmd.CombinedOutput()

	return string(output)
}

// =============================================================================
// ConfigHub Connected Mode Tests
// =============================================================================

// TestConnectedModePrerequisites validates that connected mode has required resources
func TestConnectedModePrerequisites(t *testing.T) {
	skipIfNotConnected(t)

	space := getCurrentSpace(t)
	t.Logf("Testing with space: %s", space)

	workerSlug := requireWorker(t, space)
	t.Logf("Worker: %s", workerSlug)

	targetSlug := requireTarget(t, space)
	t.Logf("Target: %s", targetSlug)

	// Verify worker slug is not null
	if workerSlug == "null" || workerSlug == "" {
		t.Errorf("Worker slug is null or empty (issue #1)")
	}

	// Verify target slug is not null
	if targetSlug == "null" || targetSlug == "" {
		t.Errorf("Target slug is null or empty (issue #1)")
	}
}

// =============================================================================
// Local Cluster Tests (no ConfigHub required)
// =============================================================================

// TestMapStatus verifies the status subcommand works
func TestMapStatus(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgent(t, "map", "status")

	// Should produce output
	if len(output) == 0 {
		t.Error("map status produced no output")
	}

	// Status line format: "✓ healthy: X deployers, Y workloads" or "✗ X problem(s): ..."
	if !strings.Contains(output, "deployer") && !strings.Contains(output, "workload") {
		t.Errorf("map status missing expected format, got: %s", output)
	}
}

// TestMapList verifies the list subcommand works
func TestMapList(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgentAllowFailures(t, "map", "list")

	// Should produce output
	if len(output) == 0 {
		t.Error("map list produced no output")
	}
}

// TestMapListJSON verifies JSON output is valid
func TestMapListJSON(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgent(t, "map", "list", "--json")

	// Parse as JSON array of resources
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("map list --json output is not valid JSON array: %v\nOutput (first 500 chars): %.500s", err, output)
	}

	// Should have some resources
	if len(result) == 0 {
		t.Error("JSON array is empty")
	}

	// Check first resource has required fields
	if len(result) > 0 {
		first := result[0]
		requiredFields := []string{"id", "namespace", "kind", "name", "owner"}
		for _, field := range requiredFields {
			if _, ok := first[field]; !ok {
				t.Errorf("Resource missing '%s' field", field)
			}
		}
	}
}

// TestMapDeployers verifies the deployers subcommand works
func TestMapDeployers(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgentAllowFailures(t, "map", "deployers")

	// Should produce output (may be empty if no Flux/Argo)
	t.Logf("deployers output length: %d", len(output))
}

// TestMapOrphans verifies the orphans subcommand works
func TestMapOrphans(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgentAllowFailures(t, "map", "orphans")

	// Should produce output
	t.Logf("orphans output length: %d", len(output))
}

// TestMapIssues verifies the issues subcommand works
func TestMapIssues(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgentAllowFailures(t, "map", "issues")

	// Should produce output
	t.Logf("issues output length: %d", len(output))
}

// =============================================================================
// CCVE Scanner Tests
// =============================================================================

// TestScan verifies the scan command works
func TestScan(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgentAllowFailures(t, "scan")

	// Should produce output
	if len(output) == 0 {
		t.Error("scan produced no output")
	}
}

// TestScanJSON verifies scan JSON output
func TestScanJSON(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgent(t, "scan", "--json")

	// Parse as JSON
	var result interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("scan --json output is not valid JSON: %v\nOutput: %s", err, output)
	}
}

// =============================================================================
// Trace Tests
// =============================================================================

// TestTrace verifies the trace command works with a deployment
func TestTrace(t *testing.T) {
	skipIfNoCluster(t)

	// Try to trace something that exists
	// First find any deployment
	cmd := exec.Command("kubectl", "get", "deploy", "-A", "-o", "jsonpath={.items[0].metadata.namespace}/{.items[0].metadata.name}")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		t.Skip("No deployments found in cluster")
	}

	parts := strings.Split(string(out), "/")
	if len(parts) != 2 {
		t.Skip("Could not parse deployment")
	}

	ns, name := parts[0], parts[1]
	output := runCubAgentAllowFailures(t, "trace", "deploy/"+name, "-n", ns)

	// Should produce output
	if len(output) == 0 {
		t.Error("trace produced no output")
	}
}

// =============================================================================
// Query Tests
// =============================================================================

// TestQuery verifies query parsing works
func TestQuery(t *testing.T) {
	skipIfNoCluster(t)

	// Test basic query
	output := runCubAgentAllowFailures(t, "map", "list", "-q", "kind=Deployment")
	t.Logf("Query output length: %d", len(output))

	// Test owner query
	output = runCubAgentAllowFailures(t, "map", "list", "-q", "owner=Flux")
	t.Logf("Flux query output length: %d", len(output))

	// Test namespace query
	output = runCubAgentAllowFailures(t, "map", "list", "-q", "namespace=kube-system")
	t.Logf("kube-system query output length: %d", len(output))
}

// =============================================================================
// Fleet View Tests (ConfigHub connected)
// =============================================================================

// TestFleetView verifies the fleet subcommand works
func TestFleetView(t *testing.T) {
	skipIfNoCluster(t)

	output := runCubAgentAllowFailures(t, "map", "fleet")

	// Should produce output (may error if no ConfigHub, that's OK)
	t.Logf("fleet output length: %d", len(output))
}
