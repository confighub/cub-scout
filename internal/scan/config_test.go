package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfig_ExplicitTakesPrecedence(t *testing.T) {
	t.Setenv("CUB_SCOUT_KYVERNO_DB", "/env/path")

	got := ResolveConfig(ProviderConfig{PolicyDBDir: "/explicit/path"})
	if got.PolicyDBDir != "/explicit/path" {
		t.Errorf("PolicyDBDir = %q, want /explicit/path (explicit should win over env)", got.PolicyDBDir)
	}
}

func TestResolveConfig_EnvFallback(t *testing.T) {
	t.Setenv("CUB_SCOUT_KYVERNO_DB", "/env/kyverno")

	got := ResolveConfig(ProviderConfig{})
	if got.PolicyDBDir != "/env/kyverno" {
		t.Errorf("PolicyDBDir = %q, want /env/kyverno", got.PolicyDBDir)
	}
}

func TestResolveConfig_LegacyProbe(t *testing.T) {
	t.Setenv("CUB_SCOUT_KYVERNO_DB", "")

	// Create a temp dir with the expected structure
	tmp := t.TempDir()
	kyvernoDir := filepath.Join(tmp, "cve", "ccve", "kyverno")
	if err := os.MkdirAll(kyvernoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Probe uses cwd as one of the search paths, so chdir
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(tmp)

	got := ResolveConfig(ProviderConfig{})
	if got.PolicyDBDir == "" {
		t.Error("PolicyDBDir is empty, expected legacy probe to find cwd/cve/ccve/kyverno")
	}
}

func TestResolveConfig_CCVEDirDerivedFromKyvernoSuffix(t *testing.T) {
	got := ResolveConfig(ProviderConfig{PolicyDBDir: "/data/cve/ccve/kyverno"})
	if got.CCVEDir != "/data/cve/ccve" {
		t.Errorf("CCVEDir = %q, want /data/cve/ccve", got.CCVEDir)
	}
}

func TestResolveConfig_CCVEDirPreservedWhenNoKyvernoSuffix(t *testing.T) {
	got := ResolveConfig(ProviderConfig{PolicyDBDir: "/data/policies"})
	if got.CCVEDir != "/data/policies" {
		t.Errorf("CCVEDir = %q, want /data/policies (same as PolicyDBDir without /kyverno suffix)", got.CCVEDir)
	}
}

func TestResolveConfig_ExplicitCCVEDirPreserved(t *testing.T) {
	got := ResolveConfig(ProviderConfig{
		PolicyDBDir: "/data/kyverno",
		CCVEDir:     "/explicit/ccve",
	})
	if got.CCVEDir != "/explicit/ccve" {
		t.Errorf("CCVEDir = %q, want /explicit/ccve (explicit should be preserved)", got.CCVEDir)
	}
}

func TestResolveConfig_EmptyWhenNothingFound(t *testing.T) {
	t.Setenv("CUB_SCOUT_KYVERNO_DB", "")

	// Use a temp dir with no kyverno structure as cwd
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(tmp)

	got := ResolveConfig(ProviderConfig{})
	// PolicyDBDir may be empty if none of the probe paths exist
	// (depends on whether exe-relative or system paths exist on this machine)
	// Just verify no panic and CCVEDir is consistent
	if got.PolicyDBDir == "" && got.CCVEDir != "" {
		t.Error("CCVEDir should be empty when PolicyDBDir is empty")
	}
}
