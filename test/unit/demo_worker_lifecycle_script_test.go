package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestDemoWorkerLifecycleScript_StartStop(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "demo-worker-lifecycle.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	mockBinDir := t.TempDir()
	callsLog := filepath.Join(t.TempDir(), "calls.log")
	pidFile := filepath.Join(t.TempDir(), "demo-worker.pid")
	logFile := filepath.Join(t.TempDir(), "demo-worker.log")

	writeExecutableLifecycle(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "$MOCK_CALLS_LOG"
if [[ "$1" == "worker" && "$2" == "run" ]]; then
  sleep 30
  exit 0
fi
exit 0
`)

	start := exec.Command("bash", scriptPath,
		"start",
		"--space", "demo-space",
		"--worker", "discovery-worker",
		"--target", "Kubernetes",
		"--pid-file", pidFile,
		"--log-file", logFile,
	)
	start.Env = append(os.Environ(),
		"MOCK_CALLS_LOG="+callsLog,
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := start.CombinedOutput()
	if err != nil {
		t.Fatalf("start failed: %v\n%s", err, string(out))
	}

	pidRaw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidRaw)))
	if err != nil || pid <= 0 {
		t.Fatalf("expected valid pid in pid file, got %q", strings.TrimSpace(string(pidRaw)))
	}

	stop := exec.Command("bash", scriptPath, "stop", "--pid-file", pidFile)
	stop.Env = append(os.Environ(), "PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err = stop.CombinedOutput()
	if err != nil {
		t.Fatalf("stop failed: %v\n%s", err, string(out))
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("expected pid file to be removed after stop, got err=%v", err)
	}

	data, err := os.ReadFile(callsLog)
	if err != nil {
		t.Fatalf("read calls log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "worker create --space demo-space discovery-worker") {
		t.Fatalf("expected worker create call, got:\n%s", log)
	}
	if !strings.Contains(log, "worker run --space demo-space -t Kubernetes discovery-worker") {
		t.Fatalf("expected worker run call, got:\n%s", log)
	}
}

func TestDemoWorkerLifecycleScript_CleanupKeepDoesNotStop(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "demo-worker-lifecycle.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	mockBinDir := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "demo-worker.pid")
	logFile := filepath.Join(t.TempDir(), "demo-worker.log")

	writeExecutableLifecycle(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "worker" && "$2" == "run" ]]; then
  sleep 30
  exit 0
fi
exit 0
`)

	start := exec.Command("bash", scriptPath,
		"start",
		"--space", "demo-space",
		"--worker", "discovery-worker",
		"--target", "Kubernetes",
		"--pid-file", pidFile,
		"--log-file", logFile,
	)
	start.Env = append(os.Environ(), "PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := start.CombinedOutput()
	if err != nil {
		t.Fatalf("start failed: %v\n%s", err, string(out))
	}

	cleanup := exec.Command("bash", scriptPath, "cleanup", "--pid-file", pidFile, "--keep")
	cleanup.Env = append(os.Environ(), "PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err = cleanup.CombinedOutput()
	if err != nil {
		t.Fatalf("cleanup failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "keeping worker process running") {
		t.Fatalf("expected keep message, got:\n%s", string(out))
	}
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatalf("expected pid file to remain when keep is set: %v", err)
	}

	stop := exec.Command("bash", scriptPath, "stop", "--pid-file", pidFile)
	stop.Env = append(os.Environ(), "PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := stop.CombinedOutput(); err != nil {
		t.Fatalf("stop failed: %v\n%s", err, string(out))
	}
}

func writeExecutableLifecycle(t *testing.T, path, content string) {
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
