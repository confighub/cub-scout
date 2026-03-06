package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSeedConnectedDemoHistoryScript_RequiresAllowSynthetic(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "seed-connected-demo-history.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	cmd := exec.Command("bash", scriptPath, "--space", "demo-space", "--apply")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure without --allow-synthetic, got success:\n%s", string(out))
	}
	if !strings.Contains(string(out), "--allow-synthetic") {
		t.Fatalf("expected allow-synthetic guard message, got:\n%s", string(out))
	}
}

func TestSeedConnectedDemoHistoryScript_DryRunDoesNotCallCub(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "seed-connected-demo-history.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	mockBinDir := t.TempDir()
	callsLog := filepath.Join(t.TempDir(), "calls.log")

	writeExecutableSeed(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "$MOCK_CALLS_LOG"
exit 0
`)

	cmd := exec.Command("bash", scriptPath, "--space", "demo-space", "--allow-synthetic")
	cmd.Env = append(os.Environ(),
		"MOCK_CALLS_LOG="+callsLog,
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "DRY RUN") {
		t.Fatalf("expected dry-run output, got:\n%s", string(out))
	}
	if _, err := os.Stat(callsLog); err == nil {
		data, _ := os.ReadFile(callsLog)
		if strings.TrimSpace(string(data)) != "" {
			t.Fatalf("expected no cub calls in dry-run, got:\n%s", string(data))
		}
	}
}

func TestSeedConnectedDemoHistoryScript_ApplyUsesSyntheticLabels(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "seed-connected-demo-history.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	mockBinDir := t.TempDir()
	callsLog := filepath.Join(t.TempDir(), "calls.log")

	writeExecutableSeed(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "$MOCK_CALLS_LOG"
if [[ "$1" == "changeset" && "$2" == "create" ]]; then
  echo '{"status":"ok"}'
  exit 0
fi
echo "unexpected cub invocation: $*" >&2
exit 2
`)

	cmd := exec.Command("bash", scriptPath,
		"--space", "demo-space",
		"--allow-synthetic",
		"--apply",
		"--allow-ci",
	)
	cmd.Env = append(os.Environ(),
		"MOCK_CALLS_LOG="+callsLog,
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "Created") {
		t.Fatalf("expected creation output, got:\n%s", string(out))
	}

	data, err := os.ReadFile(callsLog)
	if err != nil {
		t.Fatalf("read calls log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "--label demo=true") {
		t.Fatalf("expected demo label in cub calls, got:\n%s", log)
	}
	if !strings.Contains(log, "--label synthetic=true") {
		t.Fatalf("expected synthetic label in cub calls, got:\n%s", log)
	}
	if !strings.Contains(log, "--label source=cub-scout-demo-seed") {
		t.Fatalf("expected source label in cub calls, got:\n%s", log)
	}
	if !strings.Contains(log, "--space demo-space") {
		t.Fatalf("expected space argument in cub calls, got:\n%s", log)
	}
}

func writeExecutableSeed(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}
