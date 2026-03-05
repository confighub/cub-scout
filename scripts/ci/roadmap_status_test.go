package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoadmapStatus_FleetAndCoverageWorkstreamsMarkedComplete(t *testing.T) {
	roadmapPath := filepath.Join("..", "..", "docs", "roadmap.md")
	b, err := os.ReadFile(roadmapPath)
	if err != nil {
		t.Fatalf("read roadmap: %v", err)
	}
	content := string(b)

	requiredCheckedItems := []string{
		"- [x] Workstream F: Fleet query ergonomics and provenance readability",
		"- [x] Workstream F: Impact analysis ergonomics and multi-cluster context clarity",
		"- [x] Workstream F: Testing gate contract (coverage matrix, CI-enforced gate, per-run proof artifact)",
		"- [x] CI-enforced coverage metrics",
	}
	for _, item := range requiredCheckedItems {
		if !strings.Contains(content, item) {
			t.Fatalf("roadmap missing completed item marker: %s", item)
		}
	}
}

