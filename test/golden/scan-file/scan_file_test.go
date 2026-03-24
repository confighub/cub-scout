// Package scanfile provides golden tests for the scan --file CLI command contract.
//
// These tests verify that the scan --file command behaves as documented in:
// docs/reference/cli-contract.md
//
// Test scenarios cover:
//   - Valid YAML input with no findings (clean configuration)
//   - Valid YAML input with findings (misconfigurations detected)
//   - Invalid input: file not found
//   - Invalid input: malformed YAML
//   - JSON output format for all scenarios
//
// IMPORTANT: These tests are fully offline - they do NOT require:
//   - A Kubernetes cluster
//   - Network access
//   - kubectl
//
// Reference: docs/reference/cli-contract.md
//
// Contract requirements for scan --file (per cli-contract.md):
//
//	Exit code: 0 = No issues found, 1 = Issues found or error
//	Output: ✓ No misconfigurations found (clean) or severity-prefixed findings
package scanfile

import (
	"encoding/json"
	"os"
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

// TestScanFile_NoFindings verifies scan --file output on clean YAML with no misconfigurations.
// Expected: ✓ No misconfigurations found + exit code 0.
// Reference: cli-contract.md scan section - "No issues found".
// This test is fully offline - no cluster required.
func TestScanFile_NoFindings(t *testing.T) {
	fixturePath := fixtureAbsPath(t, "clean-deployment.yaml")

	result := golden.RunCubScout(t, "scan", "--file", fixturePath)

	// Per cli-contract.md: exit code 0 for "No issues found"
	golden.AssertExitCode(t, 0, result)

	// Normalize output
	normalized := normalizeScanOutput(result.Stdout + result.Stderr)

	// Verify contains expected "no issues" indicator
	if !strings.Contains(normalized, "No misconfigurations found") &&
		!strings.Contains(normalized, "No issues found") {
		t.Errorf("expected 'No misconfigurations found' in output:\n%s", normalized)
	}

	// Golden file comparison (with path normalized)
	goldenNormalized := normalizeScanForGolden(normalized, fixturePath)
	assertGolden(t, "no-findings", goldenNormalized)
}

// TestScanFile_WithFindings verifies scan --file output when misconfigurations are detected.
// Expected: severity-prefixed findings + summary + exit code 0.
// Scan exits 0 for successful scans even with findings (use --fail-on for non-zero exit).
// This test is fully offline - no cluster required.
func TestScanFile_WithFindings(t *testing.T) {
	fixturePath := fixtureAbsPath(t, "misconfigured-deployment.yaml")

	result := golden.RunCubScout(t, "scan", "--file", fixturePath)

	// Scan exits 0: findings present but no --fail-on threshold
	golden.AssertExitCode(t, 0, result)

	// Normalize output
	normalized := normalizeScanOutput(result.Stdout + result.Stderr)

	// Verify contains findings (warning or critical)
	if !strings.Contains(strings.ToLower(normalized), "warning") &&
		!strings.Contains(strings.ToLower(normalized), "critical") {
		t.Errorf("expected findings (warning/critical) in output:\n%s", normalized)
	}

	// Verify contains summary line
	if !strings.Contains(normalized, "Summary:") {
		t.Errorf("expected 'Summary:' in output:\n%s", normalized)
	}

	// Golden file comparison (with path normalized)
	goldenNormalized := normalizeScanForGolden(normalized, fixturePath)
	assertGolden(t, "with-findings", goldenNormalized)
}

// TestScanFile_FileNotFound verifies scan --file behavior when file doesn't exist.
// Expected: error message + exit code 1.
// Reference: cli-contract.md error behavior section.
// This test is fully offline - no cluster required.
func TestScanFile_FileNotFound(t *testing.T) {
	result := golden.RunCubScout(t, "scan", "--file", "/nonexistent/path/to/file.yaml")

	// Per cli-contract.md: exit code 1 for errors
	golden.AssertExitCode(t, 1, result)

	// Normalize and check output contains error indication
	normalized := golden.Normalize(result.Stdout + result.Stderr)

	// Should contain error message about file not found
	lower := strings.ToLower(normalized)
	if !strings.Contains(lower, "no such file") &&
		!strings.Contains(lower, "not found") &&
		!strings.Contains(lower, "does not exist") &&
		!strings.Contains(lower, "failed") {
		t.Errorf("expected file not found error in output:\n%s", normalized)
	}

	assertGolden(t, "file-not-found", normalized)
}

// TestScanFile_MalformedYAML verifies scan --file behavior with invalid YAML.
// Expected: error message + exit code 1.
// Reference: cli-contract.md error behavior section.
// This test is fully offline - no cluster required.
func TestScanFile_MalformedYAML(t *testing.T) {
	fixturePath := fixtureAbsPath(t, "malformed.yaml")

	result := golden.RunCubScout(t, "scan", "--file", fixturePath)

	// Per cli-contract.md: exit code 1 for errors
	golden.AssertExitCode(t, 1, result)

	// Normalize and check output contains error indication
	normalized := golden.Normalize(result.Stdout + result.Stderr)

	// Should contain error about YAML parsing
	lower := strings.ToLower(normalized)
	if !strings.Contains(lower, "yaml") &&
		!strings.Contains(lower, "parse") &&
		!strings.Contains(lower, "invalid") &&
		!strings.Contains(lower, "failed") &&
		!strings.Contains(lower, "error") {
		t.Errorf("expected YAML parse error in output:\n%s", normalized)
	}

	// Normalize path in error message
	goldenNormalized := normalizeScanForGolden(normalized, fixturePath)
	assertGolden(t, "malformed-yaml", goldenNormalized)
}

// TestScanFile_NoFindings_JSON verifies scan --file --json output on clean YAML.
// Expected: JSON with empty findings array + exit code 0.
// Reference: cli-contract.md scan JSON output section.
// This test is fully offline - no cluster required.
func TestScanFile_NoFindings_JSON(t *testing.T) {
	fixturePath := fixtureAbsPath(t, "clean-deployment.yaml")

	result := golden.RunCubScout(t, "scan", "--file", fixturePath, "--json")

	// Per cli-contract.md: exit code 0 for "No issues found"
	golden.AssertExitCode(t, 0, result)

	// Verify output is valid JSON
	combined := result.Stdout + result.Stderr
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(combined), &output); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Verify required fields per contract
	if _, ok := output["static"]; !ok {
		t.Errorf("JSON output missing 'static' field:\n%s", combined)
	}

	// Normalize for golden comparison
	goldenNormalized := normalizeJSONForGolden(combined, fixturePath)
	assertGolden(t, "no-findings-json", goldenNormalized)
}

// TestScanFile_WithFindings_JSON verifies scan --file --json output with findings.
// Expected: JSON with findings array populated + exit code 0.
// Scan exits 0 for successful scans even with findings (use --fail-on for non-zero exit).
// This test is fully offline - no cluster required.
func TestScanFile_WithFindings_JSON(t *testing.T) {
	fixturePath := fixtureAbsPath(t, "misconfigured-deployment.yaml")

	result := golden.RunCubScout(t, "scan", "--file", fixturePath, "--json")

	// Scan exits 0: findings present but no --fail-on threshold
	golden.AssertExitCode(t, 0, result)

	// Verify output is valid JSON
	combined := result.Stdout + result.Stderr
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(combined), &output); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Verify static field has findings
	if static, ok := output["static"].(map[string]interface{}); ok {
		if findings, ok := static["findings"].([]interface{}); ok {
			if len(findings) == 0 {
				t.Errorf("expected findings in JSON output, got empty array:\n%s", combined)
			}
		}
	}

	// Normalize for golden comparison
	goldenNormalized := normalizeJSONForGolden(combined, fixturePath)
	assertGolden(t, "with-findings-json", goldenNormalized)
}

// fixtureAbsPath returns the absolute path to a fixture file in testdata/inputs/.
func fixtureAbsPath(t *testing.T, name string) string {
	t.Helper()
	// Get project root by finding go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod found)")
		}
		dir = parent
	}

	return filepath.Join(dir, "test", "golden", "scan-file", "testdata", "inputs", name)
}

// normalizeScanOutput normalizes scan command output.
// Removes non-deterministic content while preserving structure.
func normalizeScanOutput(s string) string {
	// Apply base normalization (ANSI codes, timestamps, etc.)
	s = golden.Normalize(s)

	return strings.TrimSpace(s) + "\n"
}

// normalizeScanForGolden normalizes output for golden file comparison.
// Replaces the fixture file path with a placeholder.
func normalizeScanForGolden(s string, fixturePath string) string {
	// Replace fixture path BEFORE base normalization (which replaces $HOME)
	// to ensure consistent placeholder regardless of home dir case
	s = strings.ReplaceAll(s, fixturePath, "<FILE>")

	// Also try lowercase variant for case-insensitive filesystems
	s = strings.ReplaceAll(s, strings.ToLower(fixturePath), "<FILE>")

	s = normalizeScanOutput(s)

	// If generic temp-path normalization happened first, keep scan-file contract
	// stable by forcing the file field placeholder.
	s = strings.ReplaceAll(s, "File: <TEMP_PATH>", "File: <FILE>")
	s = strings.ReplaceAll(s, "File: /private<TEMP_PATH>", "File: <FILE>")

	// After normalization, the path might have $HOME prefix - replace that too
	// Pattern: $HOME/anything/testdata/inputs/filename -> <FILE>
	filePathRegex := regexp.MustCompile(`\$HOME[^\s]+/testdata/inputs/[^\s]+`)
	s = filePathRegex.ReplaceAllString(s, "<FILE>")

	return s
}

// normalizeJSONForGolden normalizes JSON output for golden file comparison.
func normalizeJSONForGolden(s string, fixturePath string) string {
	// Replace fixture path BEFORE base normalization (which replaces $HOME)
	s = strings.ReplaceAll(s, fixturePath, "<FILE>")
	s = strings.ReplaceAll(s, strings.ToLower(fixturePath), "<FILE>")

	s = golden.Normalize(s)

	// After normalization, the path might have $HOME prefix - replace that too
	filePathRegex := regexp.MustCompile(`\$HOME[^"]+/testdata/inputs/[^"]+`)
	s = filePathRegex.ReplaceAllString(s, "<FILE>")

	// Replace timestamps in JSON
	timestampRegex := regexp.MustCompile(`"scannedAt"\s*:\s*"[^"]+"`)
	s = timestampRegex.ReplaceAllString(s, `"scannedAt": "<TIMESTAMP>"`)

	// Try to parse and re-marshal with stable formatting
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err == nil {
		// Recursively normalize the data
		data = normalizeJSONData(data, fixturePath)
		if formatted, err := json.MarshalIndent(data, "", "  "); err == nil {
			return string(formatted) + "\n"
		}
	}

	return s
}

// normalizeJSONData recursively normalizes JSON data.
func normalizeJSONData(data interface{}, fixturePath string) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			if k == "scannedAt" {
				result[k] = "<TIMESTAMP>"
			} else if k == "file" {
				// Always normalize file path to <FILE>
				result[k] = "<FILE>"
			} else {
				result[k] = normalizeJSONData(val, fixturePath)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = normalizeJSONData(val, fixturePath)
		}
		return result
	case string:
		return strings.ReplaceAll(v, fixturePath, "<FILE>")
	default:
		return data
	}
}

// assertGolden compares output against golden file.
func assertGolden(t *testing.T, name, actual string) {
	t.Helper()

	goldenPath := filepath.Join("testdata", name+".golden.txt")

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
		t.Fatalf("golden file not found: %s\n\nActual output:\n%s\n\nRun with UPDATE_GOLDEN=1 to create it:\n  UPDATE_GOLDEN=1 go test ./test/golden/scan-file/...", goldenPath, actual)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if actual != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s",
			goldenPath, string(expected), actual)
	}
}
