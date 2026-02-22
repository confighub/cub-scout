// Package bundlesummarize provides golden tests for the bundle summarize CLI command.
//
// These tests verify that the bundle summarize command produces deterministic
// output in JSON and Slack formats, matching the evidence-export-v1 spec.
//
// Tests are fully offline — no cluster required. They use a static fixture
// bundle with known metadata, drift findings, and events.
//
// Reference: docs/reference/evidence-export-v1.md, docs/reference/cli-contract.md
//
// To update golden files:
//
//	UPDATE_GOLDEN=1 go test ./test/golden/bundle-summarize/...
package bundlesummarize

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

// fixtureBundlePath returns the absolute path to the fixture bundle.
func fixtureBundlePath(t *testing.T) string {
	t.Helper()
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

	return filepath.Join(dir, "test", "golden", "bundle-summarize", "testdata", "fixture-bundle")
}

// TestBundleSummarize_JSON verifies --format json output against golden file.
// Validates the BundleSummary JSON shape per evidence-export-v1.md.
// This test is fully offline — no cluster required.
func TestBundleSummarize_JSON(t *testing.T) {
	bundlePath := fixtureBundlePath(t)

	result := golden.RunCubScout(t, "bundle", "summarize", bundlePath, "--format", "json")

	golden.AssertExitCode(t, 0, result)

	combined := result.Stdout + result.Stderr

	// Verify output is valid JSON
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(combined), &output); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Verify required fields per evidence-export-v1.md
	requiredTopLevel := []string{"formatVersion", "target", "capturedAt", "changes", "evidence"}
	for _, field := range requiredTopLevel {
		if _, ok := output[field]; !ok {
			t.Errorf("Required field %q missing from JSON output", field)
		}
	}

	// Verify formatVersion is "v1"
	if fv, ok := output["formatVersion"].(string); !ok || fv != "v1" {
		t.Errorf("formatVersion = %v, want \"v1\"", output["formatVersion"])
	}

	// Normalize for golden comparison
	normalized := normalizeJSON(combined, bundlePath)
	assertGolden(t, "summarize-json", normalized)
}

// TestBundleSummarize_Slack verifies --format slack output against golden file.
// Validates the Slack Block Kit JSON shape per evidence-export-v1.md.
// This test is fully offline — no cluster required.
func TestBundleSummarize_Slack(t *testing.T) {
	bundlePath := fixtureBundlePath(t)

	result := golden.RunCubScout(t, "bundle", "summarize", bundlePath, "--format", "slack")

	golden.AssertExitCode(t, 0, result)

	combined := result.Stdout + result.Stderr

	// Verify output is valid JSON (Slack Block Kit is JSON)
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(combined), &output); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, combined)
	}

	// Verify Slack Block Kit structure has "blocks" array
	if _, ok := output["blocks"]; !ok {
		t.Errorf("Slack output missing 'blocks' array")
	}

	// Normalize for golden comparison
	normalized := normalizeJSON(combined, bundlePath)
	assertGolden(t, "summarize-slack", normalized)
}

// TestBundleSummarize_ASCII verifies --format ascii output against golden file.
// This test is fully offline — no cluster required.
func TestBundleSummarize_ASCII(t *testing.T) {
	bundlePath := fixtureBundlePath(t)

	result := golden.RunCubScout(t, "bundle", "summarize", bundlePath, "--format", "ascii")

	golden.AssertExitCode(t, 0, result)

	// Normalize output
	normalized := normalizeASCII(result.Stdout+result.Stderr, bundlePath)
	assertGolden(t, "summarize-ascii", normalized)
}

// TestBundleSummarize_InvalidBundle verifies error handling for invalid bundle path.
// Expected: exit code 1, error message.
// This test is fully offline — no cluster required.
func TestBundleSummarize_InvalidBundle(t *testing.T) {
	result := golden.RunCubScout(t, "bundle", "summarize", "/nonexistent/bundle/path")

	golden.AssertExitCode(t, 1, result)

	combined := result.Stdout + result.Stderr
	lower := strings.ToLower(combined)
	if !strings.Contains(lower, "error") &&
		!strings.Contains(lower, "failed") &&
		!strings.Contains(lower, "not found") &&
		!strings.Contains(lower, "no such file") {
		t.Errorf("expected error message in output:\n%s", combined)
	}
}

// normalizeJSON normalizes JSON output for golden file comparison.
// Replaces bundle paths with a placeholder, then re-formats for stable output.
func normalizeJSON(s string, bundlePath string) string {
	// Replace bundle path with placeholder
	s = strings.ReplaceAll(s, bundlePath, "<BUNDLE_PATH>")

	// Apply base normalization
	s = golden.Normalize(s)

	// Replace any remaining absolute paths from normalization
	pathRegex := regexp.MustCompile(`\$HOME[^"]*?/testdata/fixture-bundle`)
	s = pathRegex.ReplaceAllString(s, "<BUNDLE_PATH>")

	// Parse and re-marshal for stable formatting
	var data interface{}
	if err := json.Unmarshal([]byte(s), &data); err == nil {
		data = normalizeBundlePaths(data)
		if formatted, err := json.MarshalIndent(data, "", "  "); err == nil {
			return string(formatted) + "\n"
		}
	}

	return s
}

// normalizeASCII normalizes ASCII output for golden file comparison.
func normalizeASCII(s string, bundlePath string) string {
	// Replace bundle path with placeholder
	s = strings.ReplaceAll(s, bundlePath, "<BUNDLE_PATH>")

	// Apply base normalization
	s = golden.Normalize(s)

	// Replace any remaining absolute paths from normalization
	pathRegex := regexp.MustCompile(`\$HOME[^\s]*?/testdata/fixture-bundle`)
	s = pathRegex.ReplaceAllString(s, "<BUNDLE_PATH>")

	return strings.TrimSpace(s) + "\n"
}

// normalizeBundlePaths recursively replaces bundle paths in parsed JSON data.
func normalizeBundlePaths(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			if k == "bundlePath" {
				result[k] = "<BUNDLE_PATH>"
			} else {
				result[k] = normalizeBundlePaths(val)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = normalizeBundlePaths(val)
		}
		return result
	case string:
		if strings.Contains(v, "testdata/fixture-bundle") {
			return "<BUNDLE_PATH>"
		}
		return v
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
		t.Fatalf("golden file not found: %s\n\nActual output:\n%s\n\nRun with UPDATE_GOLDEN=1 to create it:\n  UPDATE_GOLDEN=1 go test ./test/golden/bundle-summarize/...", goldenPath, actual)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if actual != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s",
			goldenPath, string(expected), actual)
	}
}
