package scan

import (
	"context"
	"os"
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
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", tmpDir)
	defer func() { os.Setenv("PATH", origPath) }()

	p := NewConfighubScanProvider(ProviderConfig{})
	if p.Available() {
		t.Error("Available() = true, want false when confighub-scan not on PATH")
	}
}

func TestConfighubScanProvider_FallbackScanFile(t *testing.T) {
	fixture := findTestFixture(t, "test/golden/scan-file/testdata/inputs/clean-deployment.yaml")

	p := NewConfighubScanProvider(ProviderConfig{})
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
