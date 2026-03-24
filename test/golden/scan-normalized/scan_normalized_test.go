// Package scannormalized provides golden tests for the --normalized-json scan output.
//
// These tests verify that the scan.normalized.v1 schema is produced correctly
// via both scan paths:
//   - --file (static file scan) + --normalized-json
//   - CUB_SCOUT_TEST_SCAN_JSON (cluster scan hook) + --normalized-json
//
// IMPORTANT: These tests are fully offline - they do NOT require:
//   - A Kubernetes cluster
//   - Network access
//   - kubectl
package scannormalized

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/test/golden"
)

var updateGolden = os.Getenv("UPDATE_GOLDEN") == "1"

func init() {
	for _, arg := range os.Args {
		if arg == "-update" || arg == "--update" {
			updateGolden = true
			break
		}
	}
}

// TestNormalizedJSON_FileScan verifies --file --normalized-json produces valid
// scan.normalized.v1 output with correct track mapping for static findings.
func TestNormalizedJSON_FileScan(t *testing.T) {
	root := golden.ProjectRoot(t)
	fixturePath := filepath.Join(root, "test", "golden", "scan-file", "testdata", "inputs", "misconfigured-deployment.yaml")

	result := golden.RunCubScout(t, "scan", "--file", fixturePath, "--normalized-json")

	// Exit code 0: scan succeeded (findings present but no --fail-on threshold)
	golden.AssertExitCode(t, 0, result)

	// Verify output is valid JSON with required schema fields
	combined := result.Stdout + result.Stderr
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(combined), &output); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Verify schema_version field
	if sv, ok := output["schema_version"].(string); !ok || sv != "scan.normalized.v1" {
		t.Errorf("schema_version = %v, want scan.normalized.v1", output["schema_version"])
	}

	// Verify findings array exists and has entries
	if findings, ok := output["findings"].([]interface{}); !ok || len(findings) == 0 {
		t.Errorf("expected non-empty findings array, got %v", output["findings"])
	}

	// Verify each finding has required fields and track = "static"
	if findings, ok := output["findings"].([]interface{}); ok {
		for i, f := range findings {
			finding, ok := f.(map[string]interface{})
			if !ok {
				t.Errorf("finding[%d] is not an object", i)
				continue
			}
			for _, field := range []string{"id", "title", "category", "track", "severity"} {
				if _, ok := finding[field]; !ok {
					t.Errorf("finding[%d] missing required field %q", i, field)
				}
			}
			if track, ok := finding["track"].(string); ok && track != "static" {
				t.Errorf("finding[%d] track = %q, want static for file scan", i, track)
			}
		}
	}

	// Normalize for golden comparison
	goldenNormalized := normalizeNormalizedJSON(combined, fixturePath)
	assertGolden(t, "file-scan-normalized", goldenNormalized)
}

// TestNormalizedJSON_ClusterScanHook verifies CUB_SCOUT_TEST_SCAN_JSON + --normalized-json
// produces normalized output for cluster scan results with multiple tracks.
func TestNormalizedJSON_ClusterScanHook(t *testing.T) {
	root := golden.ProjectRoot(t)
	fixturePath := filepath.Join(root, "test", "golden", "scan-normalized", "testdata", "inputs", "mixed-findings.json")

	result := golden.RunCubScoutWithEnv(t,
		map[string]string{"CUB_SCOUT_TEST_SCAN_JSON": fixturePath},
		"scan", "--normalized-json",
	)

	// Exit code 0 (test hook path doesn't exit 1 for findings)
	golden.AssertExitCode(t, 0, result)

	// Verify output is valid JSON
	combined := result.Stdout + result.Stderr
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(combined), &output); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Verify schema_version
	if sv, ok := output["schema_version"].(string); !ok || sv != "scan.normalized.v1" {
		t.Errorf("schema_version = %v, want scan.normalized.v1", output["schema_version"])
	}

	// Verify findings from multiple tracks exist
	tracks := map[string]bool{}
	if findings, ok := output["findings"].([]interface{}); ok {
		for _, f := range findings {
			if finding, ok := f.(map[string]interface{}); ok {
				if track, ok := finding["track"].(string); ok {
					tracks[track] = true
				}
			}
		}
	}

	// The mixed fixture has kyverno, state, and timing-bomb tracks
	for _, expected := range []string{"kyverno", "state", "timing-bomb"} {
		if !tracks[expected] {
			t.Errorf("missing expected track %q in normalized output, got tracks: %v", expected, tracks)
		}
	}

	// Normalize for golden comparison
	goldenNormalized := normalizeNormalizedJSON(combined, fixturePath)
	assertGolden(t, "cluster-scan-normalized", goldenNormalized)
}

// normalizeNormalizedJSON normalizes the JSON output for golden comparison.
func normalizeNormalizedJSON(s string, fixturePath string) string {
	// Replace fixture path
	s = strings.ReplaceAll(s, fixturePath, "<FILE>")
	s = strings.ReplaceAll(s, strings.ToLower(fixturePath), "<FILE>")

	// Parse and re-marshal with stable formatting + timestamp normalization
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err == nil {
		data = normalizeJSONData(data)
		if formatted, err := json.MarshalIndent(data, "", "  "); err == nil {
			return string(formatted) + "\n"
		}
	}

	return s
}

// normalizeJSONData recursively normalizes JSON data for golden comparison.
func normalizeJSONData(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			switch k {
			case "scanned_at", "scannedAt":
				result[k] = "<TIMESTAMP>"
			case "file":
				result[k] = "<FILE>"
			default:
				result[k] = normalizeJSONData(val)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = normalizeJSONData(val)
		}
		return result
	default:
		return data
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
		t.Fatalf("golden file not found: %s\n\nActual output:\n%s\n\nRun with UPDATE_GOLDEN=1 to create it:\n  UPDATE_GOLDEN=1 go test ./test/golden/scan-normalized/...", goldenPath, actual)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if actual != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s",
			goldenPath, string(expected), actual)
	}
}
