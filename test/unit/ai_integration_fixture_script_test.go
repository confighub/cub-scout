package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIIntegrationFixtureSessionScript(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	scriptRel := filepath.Join("examples", "ai-integration", "run-fixture-session.sh")
	if _, err := os.Stat(filepath.Join(repoRoot, scriptRel)); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "cub-scout")
	outputDir := filepath.Join(tmpDir, "output")

	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/cub-scout")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build cub-scout: %v\n%s", err, string(out))
	}

	run := exec.Command("bash", scriptRel, "--output-dir", outputDir)
	run.Dir = repoRoot
	run.Env = append(os.Environ(), "CUB_SCOUT_BINARY="+binaryPath)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture script failed: %v\n%s", err, string(out))
	}

	requiredOutputs := []struct {
		path    string
		needle  string
		section string
	}{
		{path: filepath.Join(outputDir, "01-debug-deployment.txt"), needle: "OutOfSync", section: "trace"},
		{path: filepath.Join(outputDir, "02-change-history.txt"), needle: "Change History", section: "history"},
		{path: filepath.Join(outputDir, "03-scan-safety.json"), needle: "CCVE-2025-0244", section: "scan"},
		{path: filepath.Join(outputDir, "04-unmanaged-resources.txt"), needle: "debug-nginx", section: "orphans"},
	}

	for _, check := range requiredOutputs {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s output %s: %v", check.section, check.path, err)
		}
		if !strings.Contains(string(data), check.needle) {
			t.Fatalf("%s output missing %q in %s", check.section, check.needle, check.path)
		}
	}
}
