package unit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFluxImportDemo_AIFirstBundleExists(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "AI_START_HERE.md"),
		filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "prompts.md"),
		filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "contracts.md"),
		filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "setup.sh"),
		filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "verify.sh"),
		filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "cleanup.sh"),
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
		if strings.HasSuffix(path, ".sh") && runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			t.Fatalf("expected %s to be executable", path)
		}
	}
}

func TestFluxImportSetupScript_ExplainJSONContract(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "setup.sh")

	cmd := exec.Command("bash", scriptPath, "--explain-json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup.sh --explain-json failed: %v\n%s", err, string(out))
	}

	var result struct {
		Example             string   `json:"example"`
		Entrypoint          string   `json:"entrypoint"`
		Mutates             bool     `json:"mutates"`
		ClusterName         string   `json:"clusterName"`
		ConfigHubSpace      string   `json:"configHubSpace"`
		KeepsClusterRunning bool     `json:"keepsClusterRunning"`
		ExecutionCommand    string   `json:"executionCommand"`
		EvidenceSurfaces    []string `json:"evidenceSurfaces"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("setup.sh --explain-json output is not valid JSON: %v\n%s", err, string(out))
	}

	if result.Example != "flux-import-confighub-demo" {
		t.Fatalf("example = %q, want flux-import-confighub-demo", result.Example)
	}
	if result.Entrypoint != "./setup.sh" {
		t.Fatalf("entrypoint = %q, want ./setup.sh", result.Entrypoint)
	}
	if result.Mutates {
		t.Fatal("mutates = true, want false for explain-json")
	}
	if result.ClusterName != "flux-import-demo" {
		t.Fatalf("clusterName = %q, want flux-import-demo", result.ClusterName)
	}
	if result.ConfigHubSpace != "flux-import-demo" {
		t.Fatalf("configHubSpace = %q, want flux-import-demo", result.ConfigHubSpace)
	}
	if !result.KeepsClusterRunning {
		t.Fatal("keepsClusterRunning = false, want true")
	}
	if !strings.Contains(result.ExecutionCommand, "./demo.sh --keep") {
		t.Fatalf("executionCommand = %q, want delegated demo.sh --keep path", result.ExecutionCommand)
	}
	if len(result.EvidenceSurfaces) != 3 {
		t.Fatalf("evidenceSurfaces length = %d, want 3", len(result.EvidenceSurfaces))
	}
}

func TestFluxImportVerifyScript_SucceedsWithMockedEvidenceSurfaces(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "verify.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("verify script missing: %v", err)
	}

	tmpDir := t.TempDir()
	mockBinDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(mockBinDir, 0o755); err != nil {
		t.Fatalf("mkdir mock bin: %v", err)
	}

	workerDir := filepath.Join(tmpDir, "cub-scout-demo-workers")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}
	pidFile := filepath.Join(workerDir, "flux-import-demo-discovery-worker.pid")
	if err := os.WriteFile(pidFile, []byte("12345\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	workerJSON := filepath.Join(tmpDir, "workers.json")
	targetJSON := filepath.Join(tmpDir, "targets.json")
	unitJSON := filepath.Join(tmpDir, "units.json")
	importJSON := filepath.Join(tmpDir, "import.json")

	mustWriteFluxDemo(t, workerJSON, `[{"Condition":"Ready","Name":"demo-worker","Cluster":"kind-flux-import-demo"}]`)
	mustWriteFluxDemo(t, targetJSON, `[
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-kubernetes","ProviderType":"Kubernetes","ToolchainType":"Kubernetes"}},
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-fluxrenderer","ProviderType":"Renderer","ToolchainType":"fluxrenderer"}}
	]`)
	mustWriteFluxDemo(t, unitJSON, `[
	  {"Unit":{"Slug":"demo-app-dry"}},
	  {"Unit":{"Slug":"demo-app-wet"}}
	]`)
	mustWriteFluxDemo(t, importJSON, `{"workloads":[{"name":"podinfo","connected":true},{"name":"frontend","connected":true}]}`)

	writeExecutableFluxDemo(t, filepath.Join(mockBinDir, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *"get nodes"*)
    exit 0
    ;;
  *"get namespace flux-system"*)
    exit 0
    ;;
  *"get gitrepositories,kustomizations,helmreleases -A"*)
    echo "NAMESPACE NAME"
    exit 0
    ;;
  *"get gitrepository/podinfo -n flux-system"*)
    exit 0
    ;;
  *"get gitrepository/platform-config -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/podinfo -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/infrastructure -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/apps -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/payment-api -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/frontend -n flux-system"*)
    exit 0
    ;;
  *"get helmrelease/cert-manager -n cert-manager"*)
    exit 0
    ;;
  *"get helmrelease/monitoring -n monitoring"*)
    exit 0
    ;;
esac
echo "unexpected kubectl args: $*" >&2
exit 2
`)

	writeExecutableFluxDemo(t, filepath.Join(mockBinDir, "flux"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *"get all -A"* ]]; then
  echo "NAME READY MESSAGE"
  exit 0
fi
echo "unexpected flux args: $*" >&2
exit 2
`)

	writeExecutableFluxDemo(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "auth" && "$2" == "get-token" ]]; then
  echo "token"
  exit 0
fi
if [[ "$1" == "worker" && "$2" == "list" && "$*" == *"--json"* ]]; then
  cat "$MOCK_WORKER_JSON"
  exit 0
fi
if [[ "$1" == "target" && "$2" == "list" ]]; then
  if [[ "$*" == *"--json"* ]]; then
    cat "$MOCK_TARGET_JSON"
  else
    echo "demo-kubernetes Kubernetes"
    echo "demo-fluxrenderer fluxrenderer"
  fi
  exit 0
fi
if [[ "$1" == "unit" && "$2" == "list" ]]; then
  if [[ "$*" == *"--json"* ]]; then
    cat "$MOCK_UNIT_JSON"
  else
    echo "podinfo"
    echo "podinfo-rendered"
  fi
  exit 0
fi
echo "unexpected cub args: $*" >&2
exit 2
`)

	writeExecutableFluxDemo(t, filepath.Join(mockBinDir, "cub-scout"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "import" && "$2" == "--dry-run" && "$3" == "--json" ]]; then
  cat "$MOCK_IMPORT_JSON"
  exit 0
fi
if [[ "$1" == "gitops" && "$2" == "status" ]]; then
  echo "GITOPS STATUS"
  exit 0
fi
if [[ "$1" == "tree" && "$2" == "ownership" ]]; then
  echo "Ownership Hierarchy"
  exit 0
fi
if [[ "$1" == "scan" && "$2" == "--state" && "$3" == "--json" ]]; then
  cat "$MOCK_SCAN_JSON"
  exit 0
fi
echo "unexpected cub-scout args: $*" >&2
exit 2
`)

	scanJSON := filepath.Join(tmpDir, "scan.json")
	mustWriteFluxDemo(t, scanJSON, `{
	  "state": {
	    "findings": [
	      {
	        "ccveId": "CCVE-2025-0169",
	        "namespace": "flux-system",
	        "kind": "Kustomization",
	        "name": "apps"
	      }
	    ],
	    "runtimeFindings": [],
	    "summary": {
	      "applicationStuck": 0,
	      "runtimeFailures": 0,
	      "total": 1
	    }
	  }
	}`)

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"TMPDIR="+tmpDir,
		"MOCK_WORKER_JSON="+workerJSON,
		"MOCK_TARGET_JSON="+targetJSON,
		"MOCK_UNIT_JSON="+unitJSON,
		"MOCK_IMPORT_JSON="+importJSON,
		"MOCK_SCAN_JSON="+scanJSON,
		"CUB_SCOUT_BIN="+filepath.Join(mockBinDir, "cub-scout"),
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify.sh failed: %v\n%s", err, string(out))
	}

	output := string(out)
	for _, needle := range []string{
		"Checking cluster connectivity",
		"Checking Flux overview",
		"Checking ConfigHub evidence",
		"PASS connected demo readiness",
		"Checking cub-scout gitops status",
		"Checking cub-scout tree ownership",
		"Checking cub-scout scan evidence",
		"scan_contract=findings-present",
		"scan_sample=CCVE-2025-0169 flux-system/Kustomization/apps",
		"Verification completed.",
		"three evidence surfaces",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("verify output missing %q:\n%s", needle, output)
		}
	}
}

func TestFluxImportVerifyScript_ReportsNoFindingsContract(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "flux-import-confighub-demo", "verify.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("verify script missing: %v", err)
	}

	tmpDir := t.TempDir()
	mockBinDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(mockBinDir, 0o755); err != nil {
		t.Fatalf("mkdir mock bin: %v", err)
	}

	writeExecutableFluxDemo(t, filepath.Join(mockBinDir, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *"get nodes"*)
    exit 0
    ;;
  *"get namespace flux-system"*)
    exit 0
    ;;
  *"get gitrepositories,kustomizations,helmreleases -A"*)
    echo "NAMESPACE NAME"
    exit 0
    ;;
  *"get gitrepository/podinfo -n flux-system"*)
    exit 0
    ;;
  *"get gitrepository/platform-config -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/podinfo -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/infrastructure -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/apps -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/payment-api -n flux-system"*)
    exit 0
    ;;
  *"get kustomization/frontend -n flux-system"*)
    exit 0
    ;;
  *"get helmrelease/cert-manager -n cert-manager"*)
    exit 0
    ;;
  *"get helmrelease/monitoring -n monitoring"*)
    exit 0
    ;;
esac
echo "unexpected kubectl args: $*" >&2
exit 2
`)

	writeExecutableFluxDemo(t, filepath.Join(mockBinDir, "flux"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *"get all -A"* ]]; then
  echo "NAME READY MESSAGE"
  exit 0
fi
echo "unexpected flux args: $*" >&2
exit 2
`)

	writeExecutableFluxDemo(t, filepath.Join(mockBinDir, "cub-scout"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "gitops" && "$2" == "status" ]]; then
  echo "GITOPS STATUS"
  exit 0
fi
if [[ "$1" == "tree" && "$2" == "ownership" ]]; then
  echo "Ownership Hierarchy"
  exit 0
fi
if [[ "$1" == "scan" && "$2" == "--state" && "$3" == "--json" ]]; then
  cat "$MOCK_SCAN_JSON"
  exit 0
fi
echo "unexpected cub-scout args: $*" >&2
exit 2
`)

	scanJSON := filepath.Join(tmpDir, "scan-empty.json")
	mustWriteFluxDemo(t, scanJSON, `{
	  "state": {
	    "findings": [],
	    "runtimeFindings": [],
	    "summary": {
	      "applicationStuck": 0,
	      "runtimeFailures": 0,
	      "total": 0
	    }
	  }
	}`)

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"TMPDIR="+tmpDir,
		"MOCK_SCAN_JSON="+scanJSON,
		"CUB_SCOUT_BIN="+filepath.Join(mockBinDir, "cub-scout"),
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify.sh failed unexpectedly:\n%s", string(out))
	}

	output := string(out)
	for _, needle := range []string{
		"Checking cub-scout scan evidence",
		"scan_contract=no-findings-observed",
		"Verification completed.",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("verify output missing %q:\n%s", needle, output)
		}
	}
}

func writeExecutableFluxDemo(t *testing.T, path, content string) {
	t.Helper()
	mustWriteFluxDemo(t, path, content)
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func mustWriteFluxDemo(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
