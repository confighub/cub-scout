package scan

import (
	"context"
	"os/exec"
)

// ConfighubScanProvider delegates scanning to the confighub-scan binary when
// available, falling back to LegacyProvider for all operations when it is not.
// This is the future provider for ConfigHub-managed scan patterns.
type ConfighubScanProvider struct {
	fallback *LegacyProvider
}

// NewConfighubScanProvider creates a ConfighubScanProvider that probes for the
// confighub-scan binary. If unavailable, all methods silently delegate to the
// LegacyProvider.
func NewConfighubScanProvider(cfg ProviderConfig) *ConfighubScanProvider {
	return &ConfighubScanProvider{
		fallback: NewLegacyProvider(cfg),
	}
}

func (p *ConfighubScanProvider) Name() string { return "confighub-scan" }

// Available reports whether the confighub-scan binary is on PATH.
func (p *ConfighubScanProvider) Available() bool {
	_, err := exec.LookPath("confighub-scan")
	return err == nil
}

// ScanCluster delegates to confighub-scan when available, otherwise falls back.
func (p *ConfighubScanProvider) ScanCluster(ctx context.Context, opts ClusterScanOpts) (*CombinedResult, error) {
	// TODO: invoke confighub-scan binary for cluster scanning
	return p.fallback.ScanCluster(ctx, opts)
}

// ScanFile delegates to confighub-scan when available, otherwise falls back.
func (p *ConfighubScanProvider) ScanFile(ctx context.Context, opts FileScanOpts) (*CombinedResult, error) {
	// TODO: invoke confighub-scan binary for file scanning
	return p.fallback.ScanFile(ctx, opts)
}

// ListPolicies delegates to confighub-scan when available, otherwise falls back.
func (p *ConfighubScanProvider) ListPolicies() ([]PolicyEntry, error) {
	// TODO: invoke confighub-scan binary for policy listing
	return p.fallback.ListPolicies()
}
