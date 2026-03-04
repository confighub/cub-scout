// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

// Integration tests for scan provider selection (v1.2 — #193).
//
// These tests verify that SelectProvider correctly auto-detects the cub-scan
// binary and that the scan CLI command uses it when available.
//
// Test categories:
//   - Provider selection via CLI env vars (no cluster required)
//   - scan --file with provider override (no cluster required)
//   - scan --file output parity between providers (no cluster required)
//
// Run with:
//
//	go test -tags=integration ./test/integration/... -run TestScanProvider -v
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestScanProvider_LegacyEnvOverride verifies that CUB_SCOUT_SCAN_PROVIDER=legacy
// forces the legacy provider even when cub-scan binary is on PATH.
// No cluster required — uses --file mode.
func TestScanProvider_LegacyEnvOverride(t *testing.T) {
	cubScout := getCubAgentPath()
	fixture := findFixture(t, "test/golden/scan-file/testdata/inputs/misconfigured-deployment.yaml")

	// Run with legacy provider forced
	cmd := exec.Command(cubScout, "scan", "--file", fixture, "--json")
	cmd.Env = append(os.Environ(),
		"CUB_SCOUT_SCAN_PROVIDER=legacy",
		"NO_COLOR=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code 1 is expected (findings found)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				t.Fatalf("unexpected exit code %d: %s", exitErr.ExitCode(), output)
			}
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	// Verify output is valid JSON with static findings
	result, err := parseJSONMap(output)
	if err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}

	if _, ok := result["static"]; !ok {
		t.Error("JSON output missing 'static' field")
	}
}

// TestScanProvider_FileScan_WithFakeCubScan verifies that when a cub-scan binary
// is available on PATH, the scan --file command uses it and produces valid output.
// No cluster required.
func TestScanProvider_FileScan_WithFakeCubScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary test not supported on Windows")
	}

	cubScout := getCubAgentPath()
	fixture := findFixture(t, "test/golden/scan-file/testdata/inputs/misconfigured-deployment.yaml")

	// Create a fake confighub-scan binary that returns known JSON
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "confighub-scan")
	writeFakeBinary(t, fakeBinary, fakeCubScanJSON)

	// Run scan --file with fake binary on PATH (prepended)
	cmd := exec.Command(cubScout, "scan", "--file", fixture, "--json")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"CUB_SCOUT_SCAN_PROVIDER=", // ensure no override
		"NO_COLOR=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				t.Fatalf("unexpected exit code %d: %s", exitErr.ExitCode(), output)
			}
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	// Verify output is valid JSON
	result, err := parseJSONMap(output)
	if err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}

	// Verify the static field exists with findings from our fake binary
	static, ok := result["static"].(map[string]interface{})
	if !ok {
		t.Fatal("JSON output missing 'static' object")
	}

	findings, ok := static["findings"].([]interface{})
	if !ok {
		t.Fatal("static missing 'findings' array")
	}

	// Our fake binary returns 2 findings
	if len(findings) != 2 {
		t.Errorf("findings count = %d, want 2 (from fake cub-scan binary)", len(findings))
	}

	// Verify finding IDs match what fake binary returns
	// JSON output uses snake_case: ccve_id (from StaticFinding.CCVEID json tag)
	if len(findings) >= 1 {
		f0, _ := findings[0].(map[string]interface{})
		if ccve, _ := f0["ccve_id"].(string); ccve != "CCVE-2025-0244" {
			t.Errorf("findings[0].ccve_id = %q, want CCVE-2025-0244", ccve)
		}
	}
}

// TestScanProvider_NormalizedJSON_WithFakeCubScan verifies that cub-scan output
// maps correctly through the --normalized-json pipeline.
// No cluster required.
func TestScanProvider_NormalizedJSON_WithFakeCubScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary test not supported on Windows")
	}

	cubScout := getCubAgentPath()
	fixture := findFixture(t, "test/golden/scan-file/testdata/inputs/misconfigured-deployment.yaml")

	// Create fake confighub-scan binary
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "confighub-scan")
	writeFakeBinary(t, fakeBinary, fakeCubScanJSON)

	// Run with --normalized-json
	cmd := exec.Command(cubScout, "scan", "--file", fixture, "--normalized-json")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"CUB_SCOUT_SCAN_PROVIDER=",
		"NO_COLOR=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				t.Fatalf("unexpected exit code %d: %s", exitErr.ExitCode(), output)
			}
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	// Verify normalized JSON structure
	result, err := parseJSONMap(output)
	if err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}

	// Verify schema_version
	if sv, _ := result["schema_version"].(string); sv != "scan.normalized.v1" {
		t.Errorf("schema_version = %q, want scan.normalized.v1", sv)
	}

	// Verify findings mapped correctly
	findings, ok := result["findings"].([]interface{})
	if !ok || len(findings) != 2 {
		t.Fatalf("expected 2 normalized findings, got %v", result["findings"])
	}

	// Verify first finding has all required normalized fields
	f0, _ := findings[0].(map[string]interface{})
	requiredFields := []string{"id", "title", "category", "track", "severity", "resource", "message"}
	for _, field := range requiredFields {
		if _, ok := f0[field]; !ok {
			t.Errorf("normalized finding[0] missing required field %q", field)
		}
	}

	// Verify track = "static" for file scan
	if track, _ := f0["track"].(string); track != "static" {
		t.Errorf("finding[0].track = %q, want static", track)
	}
}

// TestScanProvider_FallbackToLegacy_WhenBinaryFails verifies that when the
// cub-scan binary is present but exits with error and no output, the scan
// command falls back to the legacy provider and still produces results.
// No cluster required.
func TestScanProvider_FallbackToLegacy_WhenBinaryFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary test not supported on Windows")
	}

	cubScout := getCubAgentPath()
	fixture := findFixture(t, "test/golden/scan-file/testdata/inputs/misconfigured-deployment.yaml")

	// Create a fake confighub-scan binary that fails (exits 1, no output)
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "confighub-scan")
	writeFakeBinary(t, fakeBinary, "") // empty output → triggers fallback

	// Run scan --file
	cmd := exec.Command(cubScout, "scan", "--file", fixture, "--json")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"CUB_SCOUT_SCAN_PROVIDER=",
		"NO_COLOR=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				t.Fatalf("unexpected exit code %d: %s", exitErr.ExitCode(), output)
			}
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	// Should still produce valid JSON (from legacy fallback)
	result, err := parseJSONMap(output)
	if err != nil {
		t.Fatalf("fallback output is not valid JSON: %v\n%s", err, output)
	}

	if _, ok := result["static"]; !ok {
		t.Error("fallback JSON output missing 'static' field")
	}
}

// TestScanProvider_ClusterScan_WithFakeCubScan verifies cluster scan path:
// legacy runtime signals are preserved while static findings are populated from cub-scan.
func TestScanProvider_ClusterScan_WithFakeCubScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary test not supported on Windows")
	}
	skipIfNoCluster(t)

	cubScout := getCubAgentPath()
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "confighub-scan")
	writeFakeBinary(t, fakeBinary, fakeCubScanClusterJSON)

	cmd := exec.Command(cubScout, "scan", "--json")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"CUB_SCOUT_SCAN_PROVIDER=",
		"CUB_SCOUT_SCAN_RAW_DIR="+tmpDir,
		"NO_COLOR=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				t.Fatalf("unexpected exit code %d: %s", exitErr.ExitCode(), output)
			}
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	result, err := parseJSONMap(output)
	if err != nil {
		t.Fatalf("cluster scan output is not valid JSON: %v\n%s", err, output)
	}

	if _, ok := result["state"]; !ok {
		t.Error("cluster scan JSON missing 'state' field (legacy runtime output should remain)")
	}
	if !hasStaticFindingID(result, "CCVE-CLUSTER-FAKE-0001") {
		t.Fatalf("cluster scan static findings missing expected fake cub-scan finding id")
	}
}

// TestScanProvider_ClusterScan_FallbackToLegacy_WhenBinaryFails verifies that
// a failing cub-scan binary does not poison cluster scan output.
func TestScanProvider_ClusterScan_FallbackToLegacy_WhenBinaryFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary test not supported on Windows")
	}
	skipIfNoCluster(t)

	cubScout := getCubAgentPath()
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "confighub-scan")
	writeFakeBinary(t, fakeBinary, "") // empty output -> provider fallback

	cmd := exec.Command(cubScout, "scan", "--json")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"CUB_SCOUT_SCAN_PROVIDER=",
		"NO_COLOR=1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 1 {
				t.Fatalf("unexpected exit code %d: %s", exitErr.ExitCode(), output)
			}
		} else {
			t.Fatalf("failed to run cub-scout: %v", err)
		}
	}

	result, err := parseJSONMap(output)
	if err != nil {
		t.Fatalf("cluster scan fallback output is not valid JSON: %v\n%s", err, output)
	}

	if _, ok := result["state"]; !ok {
		t.Error("cluster scan fallback JSON missing 'state' field")
	}
	if hasStaticFindingID(result, "CCVE-CLUSTER-FAKE-0001") {
		t.Fatalf("cluster scan fallback unexpectedly included fake cub-scan finding id")
	}
}

// --- helpers ---

func hasStaticFindingID(result map[string]interface{}, wantID string) bool {
	static, ok := result["static"].(map[string]interface{})
	if !ok {
		return false
	}
	findings, ok := static["findings"].([]interface{})
	if !ok {
		return false
	}
	for _, raw := range findings {
		f, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if id, _ := f["ccve_id"].(string); id == wantID {
			return true
		}
	}
	return false
}

// parseJSONMap tolerates warning/log lines around JSON output from CLI commands.
func parseJSONMap(output []byte) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err == nil {
		return result, nil
	}

	start := bytes.IndexByte(output, '{')
	end := bytes.LastIndexByte(output, '}')
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("no JSON object found in output")
	}

	if err := json.Unmarshal(output[start:end+1], &result); err != nil {
		return nil, fmt.Errorf("parse JSON payload: %w", err)
	}
	return result, nil
}

// findFixture returns the absolute path to a fixture file relative to project root.
func findFixture(t *testing.T, relPath string) string {
	t.Helper()

	// Walk up from cwd to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root")
		}
		dir = parent
	}

	abs := filepath.Join(dir, relPath)
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("fixture not found: %s", abs)
	}
	return abs
}

// writeFakeBinary creates a shell script that mimics the confighub-scan binary.
// If jsonOutput is empty, the script exits with error code 1 and no stdout.
func writeFakeBinary(t *testing.T, path, jsonOutput string) {
	t.Helper()

	var script string
	if jsonOutput == "" {
		script = "#!/bin/sh\nexit 1\n"
	} else {
		script = "#!/bin/sh\nprintf '%s' '" + jsonOutput + "'\nexit 1\n"
	}

	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}
}

// fakeCubScanJSON is a canned JSON response from a fake cub-scan binary.
const fakeCubScanJSON = `{"findings":[{"id":"CCVE-2025-0244","name":"Probe timeout exceeds period","category":"SILENT","track":"static","severity":"warning","confidence":"high","tool":"cub-scan","resource":{"kind":"Deployment","name":"misconfigured-app","namespace":"default"},"message":"livenessProbe timeout (15s) > period (10s)","remedy_type":"config","remedy_safety":"safe","remediation":{"steps":["Ensure probe timeoutSeconds <= periodSeconds"],"commands":[]}},{"id":"CCVE-2025-0248","name":"Missing resource limits","category":"CONFIG","track":"static","severity":"warning","confidence":"high","tool":"cub-scan","resource":{"kind":"Deployment","name":"misconfigured-app","namespace":"default"},"message":"Container app has no resource limits defined","remedy_type":"config","remedy_safety":"safe","remediation":{"steps":["Add resources.limits.cpu and resources.limits.memory"],"commands":["kubectl set resources deployment/misconfigured-app --limits=cpu=500m,memory=256Mi"]}}],"scanned_at":"2026-02-25T12:00:00Z","file":"test.yaml","summary":{"total":2,"critical":0,"warning":2,"info":0}}`

const fakeCubScanClusterJSON = `{"findings":[{"id":"CCVE-CLUSTER-FAKE-0001","name":"Cluster fake finding","category":"CONFIG","track":"static","severity":"warning","confidence":"high","tool":"cub-scan","resource":{"kind":"Deployment","name":"cluster-fake","namespace":"default"},"message":"fake cluster finding for integration test","remedy_type":"config","remedy_safety":"safe","remediation":{"steps":["none"],"commands":[]}}],"scanned_at":"2026-03-04T12:00:00Z","file":"cluster-export.yaml","summary":{"total":1,"critical":0,"warning":1,"info":0}}`
