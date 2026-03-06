package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAskModeContractScript_BlocksHighRiskWithoutConfirm(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	scriptRel := filepath.Join("scripts", "ask-mode-contract.sh")
	if _, err := os.Stat(filepath.Join(repoRoot, scriptRel)); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	cmd := exec.Command("bash", scriptRel,
		"--mode", "connected",
		"--command", "./cub-scout import -n payments",
	)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "Verdict: BLOCKED_CONFIRMATION") {
		t.Fatalf("missing blocked verdict:\n%s", text)
	}
	if !strings.Contains(text, "Risk: high") {
		t.Fatalf("missing high risk marker:\n%s", text)
	}
	if !strings.Contains(text, "DryRunCommand: ./cub-scout import -n payments --dry-run") {
		t.Fatalf("missing dry-run preference:\n%s", text)
	}
	if !strings.Contains(text, "RequiresConfirm: true") {
		t.Fatalf("missing confirm requirement:\n%s", text)
	}
	if !strings.Contains(text, "AllowedToExecute: false") {
		t.Fatalf("missing execution block marker:\n%s", text)
	}
}

func TestAskModeContractScript_AllowsLowRiskStandalone(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	scriptRel := filepath.Join("scripts", "ask-mode-contract.sh")
	if _, err := os.Stat(filepath.Join(repoRoot, scriptRel)); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	cmd := exec.Command("bash", scriptRel,
		"--mode", "standalone",
		"--command", "./cub-scout map list",
	)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}
	text := string(out)
	if !strings.Contains(text, "Verdict: READY") {
		t.Fatalf("missing ready verdict:\n%s", text)
	}
	if !strings.Contains(text, "Risk: low") {
		t.Fatalf("missing low risk marker:\n%s", text)
	}
	if !strings.Contains(text, "RequiresConfirm: false") {
		t.Fatalf("missing no-confirm marker:\n%s", text)
	}
	if !strings.Contains(text, "AllowedToExecute: true") {
		t.Fatalf("missing execution allow marker:\n%s", text)
	}
}

func TestAskModeContractScript_ExecuteBlocksWithoutConfirm(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	scriptRel := filepath.Join("scripts", "ask-mode-contract.sh")
	if _, err := os.Stat(filepath.Join(repoRoot, scriptRel)); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	cmd := exec.Command("bash", scriptRel,
		"--mode", "connected",
		"--command", "./cub-scout import -n payments",
		"--execute",
	)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected blocked execution failure, got success:\n%s", string(out))
	}
	if !strings.Contains(string(out), "Execution blocked: explicit confirmation required") {
		t.Fatalf("missing blocked execution message:\n%s", string(out))
	}
}

func TestAskModeContractScript_FixturesExist(t *testing.T) {
	required := []string{
		filepath.Join("..", "..", "test", "fixtures", "ai", "ask-mode", "failure.txt"),
		filepath.Join("..", "..", "test", "fixtures", "ai", "ask-mode", "success.txt"),
	}
	for _, p := range required {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing ask-mode fixture %s: %v", p, err)
		}
	}
}
