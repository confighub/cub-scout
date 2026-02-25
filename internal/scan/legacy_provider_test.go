package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyProvider_Name(t *testing.T) {
	p := NewLegacyProvider(ProviderConfig{})
	if got := p.Name(); got != "legacy" {
		t.Errorf("Name() = %q, want legacy", got)
	}
}

func TestLegacyProvider_Available(t *testing.T) {
	p := NewLegacyProvider(ProviderConfig{})
	if !p.Available() {
		t.Error("Available() = false, want true (legacy provider is always available)")
	}
}

func TestLegacyProvider_ScanFile_CleanDeployment(t *testing.T) {
	fixture := findTestFixture(t, "test/golden/scan-file/testdata/inputs/clean-deployment.yaml")

	p := NewLegacyProvider(ProviderConfig{})
	result, err := p.ScanFile(context.Background(), FileScanOpts{
		Filename: fixture,
	})
	if err != nil {
		t.Fatalf("ScanFile() error = %v", err)
	}
	if result.Static == nil {
		t.Fatal("Static result is nil")
	}
	if len(result.Static.Findings) != 0 {
		t.Errorf("Findings = %d, want 0 for clean deployment", len(result.Static.Findings))
	}
}

func TestLegacyProvider_ScanFile_WithFindings(t *testing.T) {
	fixture := findTestFixture(t, "test/golden/scan-file/testdata/inputs/misconfigured-deployment.yaml")

	p := NewLegacyProvider(ProviderConfig{})
	result, err := p.ScanFile(context.Background(), FileScanOpts{
		Filename: fixture,
	})
	if err != nil {
		t.Fatalf("ScanFile() error = %v", err)
	}
	if result.Static == nil {
		t.Fatal("Static result is nil")
	}
	if len(result.Static.Findings) == 0 {
		t.Error("Findings = 0, want >0 for misconfigured deployment")
	}
}

func TestLegacyProvider_ScanFile_FileNotFound(t *testing.T) {
	p := NewLegacyProvider(ProviderConfig{})
	result, err := p.ScanFile(context.Background(), FileScanOpts{
		Filename: "/nonexistent/file.yaml",
	})
	// StaticScanner.ScanFile returns errors in the result's Error field
	// (not as a Go error) per the CLI contract: exit 1 with error message.
	if err != nil {
		// If it does return a Go error, that's also acceptable
		return
	}
	if result == nil || result.Static == nil {
		t.Fatal("expected non-nil Static result")
	}
	if result.Static.Error == "" {
		t.Error("Static.Error is empty, want error message for missing file")
	}
}

func TestLegacyProvider_ListPolicies_NoDB(t *testing.T) {
	p := NewLegacyProvider(ProviderConfig{PolicyDBDir: ""})
	_, err := p.ListPolicies()
	if err == nil {
		t.Error("ListPolicies() error = nil, want error when no policy DB")
	}
}

// findTestFixture locates a test fixture relative to the repo root.
// Walks up from the current directory looking for go.mod to find root.
func findTestFixture(t *testing.T, relPath string) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for {
		// Check if this looks like the repo root (has go.mod)
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			candidate := filepath.Join(dir, relPath)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			t.Skipf("fixture %s not found under repo root %s", relPath, dir)
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skipf("fixture %s not found (could not locate repo root)", relPath)
			return ""
		}
		dir = parent
	}
}
