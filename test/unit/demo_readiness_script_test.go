package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifyConnectedDemoScript_Success(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "verify-connected-demo.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	mockBinDir := t.TempDir()
	workerJSON := filepath.Join(t.TempDir(), "workers.json")
	targetJSON := filepath.Join(t.TempDir(), "targets.json")
	unitJSON := filepath.Join(t.TempDir(), "units.json")
	importJSON := filepath.Join(t.TempDir(), "import.json")

	mustWrite(t, workerJSON, `[{"Condition":"Ready","Name":"demo-worker","Cluster":"kind-demo"}]`)
	mustWrite(t, targetJSON, `[
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-kubernetes","ProviderType":"Kubernetes","ToolchainType":"Kubernetes"}},
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-argocdrenderer","ProviderType":"Renderer","ToolchainType":"argocdrenderer"}}
	]`)
	mustWrite(t, unitJSON, `[
	  {"Unit":{"Slug":"demo-app-dry"}},
	  {"Unit":{"Slug":"demo-app-wet"}}
	]`)
	mustWrite(t, importJSON, `{"workloads":[{"name":"api","connected":true},{"name":"worker","connected":true}]}`)

	writeExecutable(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "worker" && "$2" == "list" ]]; then
  cat "$MOCK_WORKER_JSON"
  exit 0
fi
if [[ "$1" == "target" && "$2" == "list" ]]; then
  cat "$MOCK_TARGET_JSON"
  exit 0
fi
if [[ "$1" == "unit" && "$2" == "list" ]]; then
  cat "$MOCK_UNIT_JSON"
  exit 0
fi
echo "unexpected cub args: $*" >&2
exit 2
`)

	writeExecutable(t, filepath.Join(mockBinDir, "cub-scout"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "import" && "$2" == "--dry-run" && "$3" == "--json" ]]; then
  cat "$MOCK_IMPORT_JSON"
  exit 0
fi
echo "unexpected cub-scout args: $*" >&2
exit 2
`)

	cmd := exec.Command("bash", scriptPath, "--space", "demo-space", "--renderer", "argocdrenderer")
	cmd.Env = append(os.Environ(),
		"MOCK_WORKER_JSON="+workerJSON,
		"MOCK_TARGET_JSON="+targetJSON,
		"MOCK_UNIT_JSON="+unitJSON,
		"MOCK_IMPORT_JSON="+importJSON,
		"CUB_SCOUT_BIN="+filepath.Join(mockBinDir, "cub-scout"),
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "PASS") {
		t.Fatalf("expected PASS output, got:\n%s", string(out))
	}
	if !strings.Contains(string(out), "connected_workloads=2") {
		t.Fatalf("expected connected workload count in output, got:\n%s", string(out))
	}
	if !strings.Contains(string(out), "units_dry=1") || !strings.Contains(string(out), "units_wet=1") {
		t.Fatalf("expected dry/wet unit counts in output, got:\n%s", string(out))
	}
}

func TestVerifyConnectedDemoScript_FailsWhenNoConnectedWorkloads(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "verify-connected-demo.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	mockBinDir := t.TempDir()
	workerJSON := filepath.Join(t.TempDir(), "workers.json")
	targetJSON := filepath.Join(t.TempDir(), "targets.json")
	unitJSON := filepath.Join(t.TempDir(), "units.json")
	importJSON := filepath.Join(t.TempDir(), "import.json")

	mustWrite(t, workerJSON, `[{"Condition":"Ready","Name":"demo-worker","Cluster":"kind-demo"}]`)
	mustWrite(t, targetJSON, `[
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-kubernetes","ProviderType":"Kubernetes","ToolchainType":"Kubernetes"}},
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-argocdrenderer","ProviderType":"Renderer","ToolchainType":"argocdrenderer"}}
	]`)
	mustWrite(t, unitJSON, `[
	  {"Unit":{"Slug":"demo-app-dry"}},
	  {"Unit":{"Slug":"demo-app-wet"}}
	]`)
	mustWrite(t, importJSON, `{"workloads":[{"name":"api","connected":false}]}`)

	writeExecutable(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "worker" && "$2" == "list" ]]; then
  cat "$MOCK_WORKER_JSON"
  exit 0
fi
if [[ "$1" == "target" && "$2" == "list" ]]; then
  cat "$MOCK_TARGET_JSON"
  exit 0
fi
if [[ "$1" == "unit" && "$2" == "list" ]]; then
  cat "$MOCK_UNIT_JSON"
  exit 0
fi
echo "unexpected cub args: $*" >&2
exit 2
`)

	writeExecutable(t, filepath.Join(mockBinDir, "cub-scout"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "import" && "$2" == "--dry-run" && "$3" == "--json" ]]; then
  cat "$MOCK_IMPORT_JSON"
  exit 0
fi
echo "unexpected cub-scout args: $*" >&2
exit 2
`)

	cmd := exec.Command("bash", scriptPath, "--space", "demo-space", "--renderer", "argocdrenderer")
	cmd.Env = append(os.Environ(),
		"MOCK_WORKER_JSON="+workerJSON,
		"MOCK_TARGET_JSON="+targetJSON,
		"MOCK_UNIT_JSON="+unitJSON,
		"MOCK_IMPORT_JSON="+importJSON,
		"CUB_SCOUT_BIN="+filepath.Join(mockBinDir, "cub-scout"),
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure when no connected workloads, got success:\n%s", string(out))
	}
	if !strings.Contains(string(out), "connected workloads below threshold") {
		t.Fatalf("expected connected-workload failure message, got:\n%s", string(out))
	}
}

func TestVerifyConnectedDemoScript_SkipsConnectedGateWhenProposalSpaceDiffers(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "verify-connected-demo.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	mockBinDir := t.TempDir()
	workerJSON := filepath.Join(t.TempDir(), "workers.json")
	targetJSON := filepath.Join(t.TempDir(), "targets.json")
	unitJSON := filepath.Join(t.TempDir(), "units.json")
	importJSON := filepath.Join(t.TempDir(), "import.json")

	mustWrite(t, workerJSON, `[{"Condition":"Ready","Name":"demo-worker","Cluster":"kind-demo"}]`)
	mustWrite(t, targetJSON, `[
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-kubernetes","ProviderType":"Kubernetes","ToolchainType":"Kubernetes"}},
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-argocdrenderer","ProviderType":"Renderer","ToolchainType":"argocdrenderer"}}
	]`)
	mustWrite(t, unitJSON, `[
	  {"Unit":{"Slug":"demo-app-dry"}},
	  {"Unit":{"Slug":"demo-app-wet"}}
	]`)
	mustWrite(t, importJSON, `{
	  "proposal":{"app":"other-space"},
	  "workloads":[{"name":"api","connected":false}]
	}`)

	writeExecutable(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "worker" && "$2" == "list" ]]; then
  cat "$MOCK_WORKER_JSON"
  exit 0
fi
if [[ "$1" == "target" && "$2" == "list" ]]; then
  cat "$MOCK_TARGET_JSON"
  exit 0
fi
if [[ "$1" == "unit" && "$2" == "list" ]]; then
  cat "$MOCK_UNIT_JSON"
  exit 0
fi
echo "unexpected cub args: $*" >&2
exit 2
`)

	writeExecutable(t, filepath.Join(mockBinDir, "cub-scout"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "import" && "$2" == "--dry-run" && "$3" == "--json" ]]; then
  cat "$MOCK_IMPORT_JSON"
  exit 0
fi
echo "unexpected cub-scout args: $*" >&2
exit 2
`)

	cmd := exec.Command("bash", scriptPath, "--space", "demo-space", "--renderer", "argocdrenderer")
	cmd.Env = append(os.Environ(),
		"MOCK_WORKER_JSON="+workerJSON,
		"MOCK_TARGET_JSON="+targetJSON,
		"MOCK_UNIT_JSON="+unitJSON,
		"MOCK_IMPORT_JSON="+importJSON,
		"CUB_SCOUT_BIN="+filepath.Join(mockBinDir, "cub-scout"),
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "connected_gate=skipped") {
		t.Fatalf("expected connected gate skip note, got:\n%s", string(out))
	}
	if !strings.Contains(string(out), "proposal_app_name=other-space") {
		t.Fatalf("expected proposal app name note, got:\n%s", string(out))
	}
}

func TestVerifyConnectedDemoScript_SkipsConnectedGateWhenImportPreviewTimesOut(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "scripts", "verify-connected-demo.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	mockBinDir := t.TempDir()
	workerJSON := filepath.Join(t.TempDir(), "workers.json")
	targetJSON := filepath.Join(t.TempDir(), "targets.json")
	unitJSON := filepath.Join(t.TempDir(), "units.json")

	mustWrite(t, workerJSON, `[{"Condition":"Ready","Name":"demo-worker","Cluster":"kind-demo"}]`)
	mustWrite(t, targetJSON, `[
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-kubernetes","ProviderType":"Kubernetes","ToolchainType":"Kubernetes"}},
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-argocdrenderer","ProviderType":"Renderer","ToolchainType":"argocdrenderer"}}
	]`)
	mustWrite(t, unitJSON, `[
	  {"Unit":{"Slug":"demo-app-dry"}},
	  {"Unit":{"Slug":"demo-app-wet"}}
	]`)

	writeExecutable(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "worker" && "$2" == "list" ]]; then
  cat "$MOCK_WORKER_JSON"
  exit 0
fi
if [[ "$1" == "target" && "$2" == "list" ]]; then
  cat "$MOCK_TARGET_JSON"
  exit 0
fi
if [[ "$1" == "unit" && "$2" == "list" ]]; then
  cat "$MOCK_UNIT_JSON"
  exit 0
fi
echo "unexpected cub args: $*" >&2
exit 2
`)

	writeExecutable(t, filepath.Join(mockBinDir, "cub-scout"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "import" && "$2" == "--dry-run" && "$3" == "--json" ]]; then
  sleep 2
  echo '{"workloads":[{"name":"api","connected":true}]}'
  exit 0
fi
echo "unexpected cub-scout args: $*" >&2
exit 2
`)

	cmd := exec.Command("bash", scriptPath, "--space", "demo-space", "--renderer", "argocdrenderer")
	cmd.Env = append(os.Environ(),
		"MOCK_WORKER_JSON="+workerJSON,
		"MOCK_TARGET_JSON="+targetJSON,
		"MOCK_UNIT_JSON="+unitJSON,
		"CUB_SCOUT_BIN="+filepath.Join(mockBinDir, "cub-scout"),
		"VERIFY_IMPORT_TIMEOUT_SECONDS=1",
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected timeout path to pass, got %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), "connected_gate=skipped-timeout") {
		t.Fatalf("expected timeout gate skip note, got:\n%s", string(out))
	}
	if !strings.Contains(string(out), "connected_preview=timeout") {
		t.Fatalf("expected timeout preview note, got:\n%s", string(out))
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	mustWrite(t, path, content)
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
