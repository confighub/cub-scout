package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadme_ScanVariantsAndAdvancedScannerLinkDocumented(t *testing.T) {
	readmePath := filepath.Join("..", "..", "README.md")
	b, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	content := string(b)

	requiredMarkers := []string{
		"### Scan Variants",
		"`cub-scout scan --state`",
		"`cub-scout scan --kyverno`",
		"`cub-scout scan --lifecycle-hazards`",
		"`cub-scout scan --timing-bombs`",
		"`cub-scout scan --dangling`",
		"`cub-scout scan --file manifest.yaml`",
		"`cub-scout scan --json`",
		"`cub-scout scan --normalized-json`",
		"[confighub-scan](https://github.com/confighubai/confighub-scan)",
		"46 patterns",
		"3,513 risk patterns",
	}
	for _, marker := range requiredMarkers {
		if !strings.Contains(content, marker) {
			t.Fatalf("README missing required scan docs marker: %s", marker)
		}
	}
}

