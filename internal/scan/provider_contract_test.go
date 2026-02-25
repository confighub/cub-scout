package scan

import (
	"context"
	"testing"
)

// providerContractSuite runs shared contract assertions against any Provider.
// This ensures all implementations behave consistently for core operations.
func providerContractSuite(t *testing.T, name string, p Provider) {
	t.Helper()

	t.Run(name+"/Name_NonEmpty", func(t *testing.T) {
		if got := p.Name(); got == "" {
			t.Error("Name() returned empty string")
		}
	})

	t.Run(name+"/ScanFile_CleanYAML", func(t *testing.T) {
		fixture := findTestFixture(t, "test/golden/scan-file/testdata/inputs/clean-deployment.yaml")
		result, err := p.ScanFile(context.Background(), FileScanOpts{
			Filename: fixture,
		})
		if err != nil {
			t.Fatalf("ScanFile() error = %v", err)
		}
		if result.Static == nil {
			t.Fatal("Static result is nil for clean YAML")
		}
		if len(result.Static.Findings) != 0 {
			t.Errorf("Findings = %d, want 0 for clean YAML", len(result.Static.Findings))
		}
	})

	t.Run(name+"/ScanFile_MalformedYAML", func(t *testing.T) {
		// A nonexistent file exercises the error path without needing
		// a real malformed fixture.
		result, err := p.ScanFile(context.Background(), FileScanOpts{
			Filename: "/nonexistent/file.yaml",
		})
		// Either a Go error or an error in the result is acceptable.
		if err != nil {
			return // Go error — acceptable
		}
		if result == nil || result.Static == nil {
			t.Fatal("expected non-nil Static result for error case")
		}
		if result.Static.Error == "" {
			t.Error("Static.Error is empty, want error message for missing file")
		}
	})

	t.Run(name+"/ListPolicies_NoDB", func(t *testing.T) {
		// Providers constructed without a PolicyDBDir should return an error.
		_, err := p.ListPolicies()
		if err == nil {
			t.Error("ListPolicies() error = nil, want error when no policy DB")
		}
	})
}

func TestProviderContract_Legacy(t *testing.T) {
	p := NewLegacyProvider(ProviderConfig{PolicyDBDir: ""})
	providerContractSuite(t, "legacy", p)
}

func TestProviderContract_Confighub(t *testing.T) {
	p := NewConfighubScanProvider(ProviderConfig{PolicyDBDir: ""})
	providerContractSuite(t, "confighub", p)
}
