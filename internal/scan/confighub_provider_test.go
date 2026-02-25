package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfighubScanProvider_Name(t *testing.T) {
	p := NewConfighubScanProvider(ProviderConfig{})
	if got := p.Name(); got != "confighub-scan" {
		t.Errorf("Name() = %q, want confighub-scan", got)
	}
}

func TestConfighubScanProvider_Available_NoBinary(t *testing.T) {
	// Use empty temp dir as PATH to ensure binary not found
	tmpDir := t.TempDir()
	t.Setenv("PATH", tmpDir)

	p := NewConfighubScanProvider(ProviderConfig{})
	// Reset cached binary path so Available() re-probes.
	p.binaryPath = ""
	if p.Available() {
		t.Error("Available() = true, want false when confighub-scan not on PATH")
	}
}

func TestConfighubScanProvider_Available_WithFakeBinary(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "confighub-scan")
	writeFakeCubScan(t, fakeBinary, `{"findings":[]}`)

	t.Setenv("PATH", tmpDir)

	p := NewConfighubScanProvider(ProviderConfig{})
	p.binaryPath = "" // reset cache
	if !p.Available() {
		t.Error("Available() = false, want true when confighub-scan is on PATH")
	}
}

func TestConfighubScanProvider_FallbackScanFile(t *testing.T) {
	fixture := findTestFixture(t, "test/golden/scan-file/testdata/inputs/clean-deployment.yaml")

	// Ensure cub-scan is NOT on PATH so fallback is exercised.
	tmpDir := t.TempDir()
	t.Setenv("PATH", tmpDir)

	p := NewConfighubScanProvider(ProviderConfig{})
	p.binaryPath = "" // reset cache
	result, err := p.ScanFile(context.Background(), FileScanOpts{
		Filename: fixture,
	})
	if err != nil {
		t.Fatalf("ScanFile() error = %v", err)
	}
	if result.Static == nil {
		t.Fatal("Static result is nil (fallback should produce result)")
	}
}

func TestConfighubScanProvider_FallbackListPolicies_NoDB(t *testing.T) {
	p := NewConfighubScanProvider(ProviderConfig{PolicyDBDir: ""})
	_, err := p.ListPolicies()
	if err == nil {
		t.Error("ListPolicies() error = nil, want error when no policy DB (fallback behavior)")
	}
}

func TestConfighubScanProvider_ScanFile_WithCubScan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary test not supported on Windows")
	}

	fixture := findTestFixture(t, "test/golden/scan-file/testdata/inputs/misconfigured-deployment.yaml")

	// Create a fake cub-scan binary that outputs known JSON.
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "confighub-scan")
	writeFakeCubScan(t, fakeBinary, fakeCubScanOutput)

	t.Setenv("PATH", tmpDir)

	p := NewConfighubScanProvider(ProviderConfig{})
	p.binaryPath = "" // reset cache

	result, err := p.ScanFile(context.Background(), FileScanOpts{
		Filename: fixture,
	})
	if err != nil {
		t.Fatalf("ScanFile() error = %v", err)
	}
	if result.Static == nil {
		t.Fatal("Static result is nil")
	}
	if len(result.Static.Findings) != 2 {
		t.Fatalf("Findings count = %d, want 2", len(result.Static.Findings))
	}

	// Verify first finding mapping
	f := result.Static.Findings[0]
	if f.CCVEID != "CCVE-2025-0244" {
		t.Errorf("Findings[0].CCVEID = %q, want CCVE-2025-0244", f.CCVEID)
	}
	if f.Name != "Probe timeout exceeds period" {
		t.Errorf("Findings[0].Name = %q, want 'Probe timeout exceeds period'", f.Name)
	}
	if f.Kind != "Deployment" {
		t.Errorf("Findings[0].Kind = %q, want Deployment", f.Kind)
	}
	if f.ResourceName != "misconfigured-app" {
		t.Errorf("Findings[0].ResourceName = %q, want misconfigured-app", f.ResourceName)
	}
	if f.Severity != "warning" {
		t.Errorf("Findings[0].Severity = %q, want warning", f.Severity)
	}
	if f.Remediation != "Ensure probe timeoutSeconds <= periodSeconds" {
		t.Errorf("Findings[0].Remediation = %q, want 'Ensure probe timeoutSeconds <= periodSeconds'", f.Remediation)
	}

	// Verify second finding
	f2 := result.Static.Findings[1]
	if f2.CCVEID != "CCVE-2025-0248" {
		t.Errorf("Findings[1].CCVEID = %q, want CCVE-2025-0248", f2.CCVEID)
	}
	if f2.Remediation != "kubectl set resources deployment/misconfigured-app --limits=cpu=500m,memory=256Mi" {
		t.Errorf("Findings[1].Remediation = %q, want kubectl command", f2.Remediation)
	}
}

func TestConfighubScanProvider_ScanFile_BinaryFails_FallsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary test not supported on Windows")
	}

	fixture := findTestFixture(t, "test/golden/scan-file/testdata/inputs/clean-deployment.yaml")

	// Create a fake cub-scan binary that exits with error and no output.
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "confighub-scan")
	writeFakeCubScan(t, fakeBinary, "") // empty output triggers fallback

	t.Setenv("PATH", tmpDir)

	p := NewConfighubScanProvider(ProviderConfig{})
	p.binaryPath = "" // reset cache

	result, err := p.ScanFile(context.Background(), FileScanOpts{
		Filename: fixture,
	})
	if err != nil {
		t.Fatalf("ScanFile() error = %v, want fallback to succeed", err)
	}
	if result.Static == nil {
		t.Fatal("Static result is nil (fallback should produce result)")
	}
}

func TestConfighubScanProvider_ScanFile_CubScanNameAlternative(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binary test not supported on Windows")
	}

	fixture := findTestFixture(t, "test/golden/scan-file/testdata/inputs/misconfigured-deployment.yaml")

	// Create a fake binary named "cub-scan" (alternative name).
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "cub-scan")
	writeFakeCubScan(t, fakeBinary, fakeCubScanOutput)

	t.Setenv("PATH", tmpDir)

	p := NewConfighubScanProvider(ProviderConfig{})
	p.binaryPath = "" // reset cache

	if !p.Available() {
		t.Fatal("Available() = false, want true for cub-scan alternative name")
	}

	result, err := p.ScanFile(context.Background(), FileScanOpts{
		Filename: fixture,
	})
	if err != nil {
		t.Fatalf("ScanFile() error = %v", err)
	}
	if result.Static == nil {
		t.Fatal("Static result is nil")
	}
	if len(result.Static.Findings) != 2 {
		t.Errorf("Findings count = %d, want 2", len(result.Static.Findings))
	}
}

func TestMapCubScanResult(t *testing.T) {
	cs := &cubScanResult{
		Findings: []cubScanFinding{
			{
				ID:       "CCVE-2025-0100",
				Name:     "Test finding",
				Category: "TEST",
				Track:    "static",
				Severity: "critical",
				Resource: cubScanResourceRef{Kind: "Deployment", Name: "test-app", Namespace: "production"},
				Message:  "Something is wrong",
				Remediation: cubScanRemediation{
					Steps:    []string{"Fix it"},
					Commands: []string{"kubectl fix it"},
				},
			},
			{
				ID:       "CCVE-2025-0101",
				Name:     "Steps-only finding",
				Category: "CONFIG",
				Track:    "static",
				Severity: "warning",
				Resource: cubScanResourceRef{Kind: "Pod", Name: "my-pod"},
				Message:  "Consider this",
				Remediation: cubScanRemediation{
					Steps: []string{"Do step 1", "Do step 2"},
				},
			},
			{
				ID:       "CCVE-2025-0102",
				Name:     "No remediation",
				Category: "INFO",
				Track:    "static",
				Severity: "info",
				Resource: cubScanResourceRef{Kind: "Service", Name: "my-svc", Namespace: "default"},
				Message:  "Just FYI",
			},
		},
	}

	result := mapCubScanResult(cs, "/tmp/test.yaml")

	if result.File != "/tmp/test.yaml" {
		t.Errorf("File = %q, want /tmp/test.yaml", result.File)
	}
	if len(result.Findings) != 3 {
		t.Fatalf("Findings count = %d, want 3", len(result.Findings))
	}

	// First: commands take precedence over steps
	if result.Findings[0].Remediation != "kubectl fix it" {
		t.Errorf("Findings[0].Remediation = %q, want 'kubectl fix it'", result.Findings[0].Remediation)
	}
	if result.Findings[0].Namespace != "production" {
		t.Errorf("Findings[0].Namespace = %q, want 'production'", result.Findings[0].Namespace)
	}

	// Second: falls back to first step
	if result.Findings[1].Remediation != "Do step 1" {
		t.Errorf("Findings[1].Remediation = %q, want 'Do step 1'", result.Findings[1].Remediation)
	}

	// Third: no remediation
	if result.Findings[2].Remediation != "" {
		t.Errorf("Findings[2].Remediation = %q, want empty", result.Findings[2].Remediation)
	}
}

func TestResolveCatalogPath_FromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	catalogPath := filepath.Join(tmpDir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CUB_SCAN_CATALOG", catalogPath)
	got := resolveCatalogPath()
	if got != catalogPath {
		t.Errorf("resolveCatalogPath() = %q, want %q", got, catalogPath)
	}
}

func TestResolveCatalogPath_EnvMissing(t *testing.T) {
	t.Setenv("CUB_SCAN_CATALOG", "/nonexistent/catalog.json")
	// Should not return the invalid path
	got := resolveCatalogPath()
	if got == "/nonexistent/catalog.json" {
		t.Error("resolveCatalogPath() returned nonexistent path from env")
	}
}

// --- test helpers ---

// writeFakeCubScan creates a shell script that mimics the cub-scan binary.
// If jsonOutput is empty, the script exits with error code 1 and no stdout.
func writeFakeCubScan(t *testing.T, path, jsonOutput string) {
	t.Helper()

	var script string
	if jsonOutput == "" {
		script = "#!/bin/sh\nexit 1\n"
	} else {
		// Use printf to avoid newline issues with echo on different platforms.
		script = "#!/bin/sh\nprintf '%s' '" + jsonOutput + "'\nexit 1\n"
	}

	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write fake cub-scan: %v", err)
	}
}

// fakeCubScanOutput is a canned JSON response from a fake cub-scan binary.
const fakeCubScanOutput = `{"findings":[{"id":"CCVE-2025-0244","name":"Probe timeout exceeds period","category":"SILENT","track":"static","severity":"warning","confidence":"high","tool":"cub-scan","resource":{"kind":"Deployment","name":"misconfigured-app","namespace":"default"},"message":"livenessProbe timeout (15s) > period (10s)","remedy_type":"config","remedy_safety":"safe","remediation":{"steps":["Ensure probe timeoutSeconds <= periodSeconds"],"commands":[]}},{"id":"CCVE-2025-0248","name":"Missing resource limits","category":"CONFIG","track":"static","severity":"warning","confidence":"high","tool":"cub-scan","resource":{"kind":"Deployment","name":"misconfigured-app","namespace":"default"},"message":"Container app has no resource limits defined","remedy_type":"config","remedy_safety":"safe","remediation":{"steps":["Add resources.limits.cpu and resources.limits.memory"],"commands":["kubectl set resources deployment/misconfigured-app --limits=cpu=500m,memory=256Mi"]}}],"scanned_at":"2026-02-25T12:00:00Z","file":"test.yaml","summary":{"total":2,"critical":0,"warning":2,"info":0}}`
