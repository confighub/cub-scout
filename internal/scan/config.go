package scan

import (
	"os"
	"path/filepath"
	"strings"
)

// ProviderConfig controls provider-level behavior.
type ProviderConfig struct {
	// PolicyDBDir is the Kyverno policy database directory.
	// Resolution order: explicit value > CUB_SCOUT_KYVERNO_DB env > legacy probing.
	PolicyDBDir string

	// CCVEDir is the CCVE database directory for static scanning.
	// When empty, derived from PolicyDBDir (its parent if PolicyDBDir ends in /kyverno).
	CCVEDir string
}

// ResolveConfig returns a fully-resolved ProviderConfig.
// It applies the resolution chain: explicit > env > legacy probe.
func ResolveConfig(explicit ProviderConfig) ProviderConfig {
	resolved := explicit

	if resolved.PolicyDBDir == "" {
		resolved.PolicyDBDir = os.Getenv("CUB_SCOUT_KYVERNO_DB")
	}

	if resolved.PolicyDBDir == "" {
		if kyvernoAsset := ResolveBundleAsset(bundleAssetKyvernoCCVE); kyvernoAsset != "" {
			resolved.PolicyDBDir = kyvernoAsset
		}
	}

	if resolved.PolicyDBDir == "" {
		resolved.PolicyDBDir = probeLegacyPolicyDBDir()
	}

	if resolved.CCVEDir == "" && resolved.PolicyDBDir != "" {
		if strings.HasSuffix(resolved.PolicyDBDir, "/kyverno") {
			resolved.CCVEDir = filepath.Dir(resolved.PolicyDBDir)
		} else {
			resolved.CCVEDir = resolved.PolicyDBDir
		}
	}

	return resolved
}

// SelectProvider returns the best available scan provider.
//
// Selection logic:
//  1. CUB_SCOUT_SCAN_PROVIDER=legacy forces LegacyProvider
//  2. If cub-scan binary is available, returns ConfighubScanProvider
//  3. Otherwise returns LegacyProvider
//
// The ConfighubScanProvider falls back internally to LegacyProvider for
// cluster scanning (cub-scan is a static file scanner).
func SelectProvider(cfg ProviderConfig) Provider {
	if os.Getenv("CUB_SCOUT_SCAN_PROVIDER") == "legacy" {
		return NewLegacyProvider(cfg)
	}

	confighub := NewConfighubScanProvider(cfg)
	if confighub.Available() {
		return confighub
	}

	return NewLegacyProvider(cfg)
}

// probeLegacyPolicyDBDir probes hardcoded filesystem locations for the
// Kyverno policy database. This is the legacy discovery mechanism preserved
// as a fallback for backward compatibility.
func probeLegacyPolicyDBDir() string {
	// Try relative to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "..", "cve", "ccve", "kyverno")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	// Try relative to current directory
	cwd, err := os.Getwd()
	if err == nil {
		dir := filepath.Join(cwd, "cve", "ccve", "kyverno")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	// Try common system locations
	locations := []string{
		"/usr/local/share/confighub/cve/ccve/kyverno",
		"/usr/share/confighub/cve/ccve/kyverno",
	}
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}
