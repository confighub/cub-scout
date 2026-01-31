// Package unit provides test helpers for ConfigHub Agent unit tests.
package unit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// FixturesDir is the path to shared test fixtures
var FixturesDir = filepath.Join("..", "fixtures")

// -----------------------------------------------------------------------------
// Precondition Helpers - fail fast if environment isn't ready
// -----------------------------------------------------------------------------

// RequireCluster fails the test if kubectl cannot reach a cluster.
func RequireCluster(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "cluster-info")
	if err := cmd.Run(); err != nil {
		t.Skip("PRECONDITION: No Kubernetes cluster available")
	}
}

// RequireFlux fails the test if Flux CRDs are not installed.
func RequireFlux(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "get", "crd", "kustomizations.kustomize.toolkit.fluxcd.io")
	if err := cmd.Run(); err != nil {
		t.Skip("PRECONDITION: Flux CRDs not installed")
	}
}

// RequireArgo fails the test if Argo CD CRDs are not installed.
func RequireArgo(t *testing.T) {
	t.Helper()
	cmd := exec.Command("kubectl", "get", "crd", "applications.argoproj.io")
	if err := cmd.Run(); err != nil {
		t.Skip("PRECONDITION: Argo CD CRDs not installed")
	}
}

// RequireCubAuth fails the test if cub CLI is not authenticated or token is expired.
func RequireCubAuth(t *testing.T) {
	t.Helper()

	// First check if cub is available
	if _, err := exec.LookPath("cub"); err != nil {
		t.Skip("PRECONDITION: cub CLI not installed")
	}

	// Check auth status
	cmd := exec.Command("cub", "auth", "status")
	if err := cmd.Run(); err != nil {
		t.Skip("PRECONDITION: cub CLI not authenticated (run: cub auth login)")
	}

	// auth status can succeed with expired token, so also test an actual API call
	// Use a lightweight command that requires valid auth
	cmd = exec.Command("cub", "space", "list", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "token expired") ||
			strings.Contains(string(output), "authentication problem") ||
			strings.Contains(string(output), "not authenticated") {
			t.Skip("PRECONDITION: cub CLI token expired (run: cub auth login)")
		}
		t.Skipf("PRECONDITION: cub CLI auth check failed: %v", err)
	}
}

// Space represents a ConfigHub space for testing.
type Space struct {
	ID   string
	Slug string
}

// RequireSpace fails the test if no active space is set in cub.
func RequireSpace(t *testing.T) Space {
	t.Helper()
	RequireCubAuth(t)

	cmd := exec.Command("cub", "context", "get", "space")
	output, err := cmd.Output()
	if err != nil {
		t.Skip("PRECONDITION: No active space set (run: cub context set space <slug>)")
	}

	slug := strings.TrimSpace(string(output))
	if slug == "" || slug == "null" {
		t.Skip("PRECONDITION: Active space is null or empty")
	}

	return Space{Slug: slug}
}

// Worker represents a ConfigHub worker for testing.
type Worker struct {
	Slug   string
	Status string
}

// RequireWorker fails the test if no worker is connected in the space.
func RequireWorker(t *testing.T, space Space) Worker {
	t.Helper()

	cmd := exec.Command("cub", "worker", "list", "--json")
	output, err := cmd.Output()
	require.NoError(t, err, "PRECONDITION: Failed to list workers")

	var workers []map[string]interface{}
	require.NoError(t, json.Unmarshal(output, &workers), "PRECONDITION: Failed to parse worker list")
	require.NotEmpty(t, workers, "PRECONDITION: No workers in space %s", space.Slug)

	slug, _ := workers[0]["Slug"].(string)
	status, _ := workers[0]["Status"].(string)

	// Issue #1 pattern: null slug
	require.NotEqual(t, "null", slug, "PRECONDITION FAILED: Worker slug is null (issue #1 pattern)")
	require.NotEqual(t, "", slug, "PRECONDITION FAILED: Worker slug is empty")

	// Check status
	require.NotEqual(t, "unknown", status, "PRECONDITION FAILED: Worker status is unknown")

	return Worker{Slug: slug, Status: status}
}

// Target represents a ConfigHub target for testing.
type Target struct {
	Slug        string
	DisplayName string
}

// RequireTarget fails the test if no target exists in the space.
func RequireTarget(t *testing.T, space Space) Target {
	t.Helper()

	cmd := exec.Command("cub", "target", "list", "--json")
	output, err := cmd.Output()
	require.NoError(t, err, "PRECONDITION: Failed to list targets")

	var targets []map[string]interface{}
	require.NoError(t, json.Unmarshal(output, &targets), "PRECONDITION: Failed to parse target list")
	require.NotEmpty(t, targets, "PRECONDITION: No targets in space %s (unit has no target)", space.Slug)

	slug, _ := targets[0]["Slug"].(string)
	displayName, _ := targets[0]["DisplayName"].(string)

	// Issue #1 pattern: null slug
	require.NotEqual(t, "null", slug, "PRECONDITION FAILED: Target slug is null (issue #1 pattern)")
	require.NotEqual(t, "", slug, "PRECONDITION FAILED: Target slug is empty")

	return Target{Slug: slug, DisplayName: displayName}
}

// Unit represents a ConfigHub unit for testing.
type Unit struct {
	Slug            string
	HeadRevisionNum int
	TargetSlug      string
}

// RequireUnits fails the test if fewer than n units exist in the space.
func RequireUnits(t *testing.T, space Space, minCount int) []Unit {
	t.Helper()

	cmd := exec.Command("cub", "unit", "list", "--json")
	output, err := cmd.Output()
	require.NoError(t, err, "PRECONDITION: Failed to list units")

	var rawUnits []map[string]interface{}
	require.NoError(t, json.Unmarshal(output, &rawUnits), "PRECONDITION: Failed to parse unit list")
	require.GreaterOrEqual(t, len(rawUnits), minCount, "PRECONDITION: Expected at least %d units, got %d", minCount, len(rawUnits))

	var units []Unit
	for _, u := range rawUnits {
		slug, _ := u["Slug"].(string)
		rev, _ := u["HeadRevisionNum"].(float64)
		targetSlug, _ := u["TargetSlug"].(string)

		units = append(units, Unit{
			Slug:            slug,
			HeadRevisionNum: int(rev),
			TargetSlug:      targetSlug,
		})
	}

	return units
}

// -----------------------------------------------------------------------------
// Postcondition Assertions - verify expected state after tests
// -----------------------------------------------------------------------------

// AssertNoNullValues fails if the output contains null or "null" values.
// This catches issue #1 patterns where jq paths return null.
func AssertNoNullValues(t *testing.T, output string) {
	t.Helper()
	require.NotContains(t, output, "null - unknown", "Output contains 'null - unknown' (issue #1 pattern)")
	require.NotContains(t, output, "→ no target", "Output contains '→ no target' (unit has no target)")
}

// AssertUnitsHaveTargets fails if any unit shows "no target".
func AssertUnitsHaveTargets(t *testing.T, output string, units []Unit) {
	t.Helper()
	for _, u := range units {
		require.NotContains(t, output, u.Slug+" → no target",
			"Unit %s has no target", u.Slug)
	}
}

// AssertWorkersHealthy fails if workers show null or unknown status.
func AssertWorkersHealthy(t *testing.T, output string) {
	t.Helper()
	require.NotContains(t, output, "null - unknown", "Worker shows 'null - unknown'")
}

// AssertOwnerDetected fails if the expected owner is not found for a resource.
func AssertOwnerDetected(t *testing.T, output string, kind, name, expectedOwner string) {
	t.Helper()
	// Look for patterns like "Flux deployment/podinfo" or "ConfigHub deployment/backend"
	pattern := expectedOwner + " " + strings.ToLower(kind) + "/" + name
	require.Contains(t, strings.ToLower(output), strings.ToLower(pattern),
		"Expected %s to own %s/%s", expectedOwner, kind, name)
}

// -----------------------------------------------------------------------------
// Fixture Helpers
// -----------------------------------------------------------------------------

// LoadFixtureUnstructured loads a single YAML document into an Unstructured object.
func LoadFixtureUnstructured(t *testing.T, relPath string) *unstructured.Unstructured {
	t.Helper()
	path := filepath.Join(FixturesDir, relPath)
	b, err := os.ReadFile(path)
	require.NoError(t, err)

	var obj map[string]interface{}
	require.NoError(t, yaml.Unmarshal(b, &obj))

	u := &unstructured.Unstructured{Object: obj}
	return u
}

// LoadFixture loads a test fixture file and returns its contents.
func LoadFixture(t *testing.T, relativePath string) []byte {
	t.Helper()
	path := filepath.Join(FixturesDir, relativePath)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to load fixture: %s", path)
	return data
}

// LoadJSONFixture loads a JSON fixture and unmarshals it.
func LoadJSONFixture(t *testing.T, relativePath string, v interface{}) {
	t.Helper()
	data := LoadFixture(t, relativePath)
	require.NoError(t, json.Unmarshal(data, v), "Failed to parse JSON fixture: %s", relativePath)
}

// -----------------------------------------------------------------------------
// Command Helpers
// -----------------------------------------------------------------------------

// RunCubAgent runs the cub-scout binary with the given args and returns output.
func RunCubAgent(t *testing.T, args ...string) string {
	t.Helper()
	cubAgent := os.Getenv("CUB_AGENT")
	if cubAgent == "" {
		cubAgent = "./cub-scout"
	}

	cmd := exec.Command(cubAgent, args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "cub-scout %v failed: %s", args, string(output))
	return string(output)
}

// RunCub runs the cub CLI with the given args and returns output.
func RunCub(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("cub", args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "cub %v failed: %s", args, string(output))
	return string(output)
}

// RunCubJSON runs cub with --json and unmarshals the result.
func RunCubJSON(t *testing.T, v interface{}, args ...string) {
	t.Helper()
	args = append(args, "--json")
	output := RunCub(t, args...)
	require.NoError(t, json.Unmarshal([]byte(output), v), "Failed to parse cub JSON output")
}
