package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectAndCompareExample_ArtifactsExist(t *testing.T) {
	required := []string{
		filepath.Join("..", "..", "examples", "connect-and-compare", "README.md"),
		filepath.Join("..", "..", "examples", "connect-and-compare", "demo.sh"),
		filepath.Join("..", "..", "examples", "connect-and-compare", "record-demo.sh"),
		filepath.Join("..", "..", "examples", "connect-and-compare", "testdata", "doctor_input.json"),
		filepath.Join("..", "..", "examples", "connect-and-compare", "testdata", "history_changesets.json"),
		filepath.Join("..", "..", "examples", "connect-and-compare", "expected-output", "01-doctor.txt"),
		filepath.Join("..", "..", "examples", "connect-and-compare", "expected-output", "03-compare.json"),
		filepath.Join("..", "..", "examples", "connect-and-compare", "expected-output", "04-history.txt"),
	}

	for _, p := range required {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing required connect-and-compare artifact %s: %v", p, err)
		}
	}
}

func TestConnectAndCompareExample_IndexedInExamplesREADME(t *testing.T) {
	examplesIndexPath := filepath.Join("..", "..", "examples", "README.md")
	content, err := os.ReadFile(examplesIndexPath)
	if err != nil {
		t.Fatalf("read examples index: %v", err)
	}
	if !strings.Contains(string(content), "connect-and-compare/") {
		t.Fatalf("examples index missing connect-and-compare link in %s", examplesIndexPath)
	}
}

func TestConnectAndCompareDemoScript_VerifySnapshots(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "examples", "connect-and-compare", "demo.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("demo script missing: %v", err)
	}

	binPath := filepath.Join(t.TempDir(), "cub-scout")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/cub-scout")
	buildCmd.Dir = repoRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build cub-scout: %v\n%s", err, string(out))
	}

	outDir := filepath.Join(t.TempDir(), "run-output")
	runCmd := exec.Command("bash", scriptPath, "--output-dir", outDir, "--verify")
	runCmd.Dir = repoRoot
	runCmd.Env = append(os.Environ(), "CUB_SCOUT_BIN="+binPath)

	out, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("demo script failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "All snapshots match expected output") {
		t.Fatalf("expected snapshot verification success message, got:\n%s", string(out))
	}
}
