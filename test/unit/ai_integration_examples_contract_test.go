package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIIntegrationExamplesContract(t *testing.T) {
	required := []string{
		filepath.Join("..", "..", "examples", "ai-integration", "README.md"),
		filepath.Join("..", "..", "examples", "ai-integration", "run-fixture-session.sh"),
		filepath.Join("..", "..", "examples", "ai-integration", "testdata", "history_changesets.json"),
		filepath.Join("..", "..", "examples", "ai-integration", "testdata", "failed-session.transcript.txt"),
		filepath.Join("..", "..", "examples", "ai-integration", "claude-code", "README.md"),
		filepath.Join("..", "..", "examples", "ai-integration", "claude-code", "mcp.json"),
		filepath.Join("..", "..", "examples", "ai-integration", "cursor", "README.md"),
		filepath.Join("..", "..", "examples", "ai-integration", "cursor", "mcp.json"),
		filepath.Join("..", "..", "examples", "ai-integration", "copilot", "README.md"),
		filepath.Join("..", "..", "examples", "ai-integration", "copilot", "mcp.json"),
	}

	for _, p := range required {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing required ai-integration artifact %s: %v", p, err)
		}
	}

	examplesIndexPath := filepath.Join("..", "..", "examples", "README.md")
	examplesIndex, err := os.ReadFile(examplesIndexPath)
	if err != nil {
		t.Fatalf("read examples index: %v", err)
	}
	if !strings.Contains(string(examplesIndex), "ai-integration/") {
		t.Fatalf("examples index missing ai-integration link in %s", examplesIndexPath)
	}
}
